# go-link-monitor

A small Go library (plus an example daemon) that tracks the live count of
**operationally-up physical NIC ports** on Linux via rtnetlink, persists a
crash-safe *baseline* of the expected count, and alerts when ports go missing.

- **`pkg/linkmonitor`** — the library: a pure port-state machine, a netlink
  decode/snapshot edge, a filename-encoded baseline store, and a
  dependency-injected event loop (`Monitor`). Depends only on
  `github.com/vishvananda/netlink`.
- **`pkg/prommetrics`** — an optional Prometheus adapter for `linkmonitor.Status`.
- **`cmd/go-link-monitor`** — a small daemon wiring the library to real
  netlink, `/metrics`, and a `SIGUSR1` re-baseline signal.

## Why "up" is stricter than IFF_UP

A NIC can be administratively up (`IFF_UP`) with no cable plugged in. A port is
counted as up only when it is admin-up **and** has carrier — `OperUp`, or
`OperUnknown` with the raw `IFF_RUNNING` flag set (some drivers never report an
operstate). See `IsOperUp` in `pkg/linkmonitor/state.go`.

## Install / build

Requires Go 1.24+ to build with the local toolchain; the Nix build pins Go 1.27.1.

```sh
go build -o bin/go-link-monitor ./cmd/go-link-monitor
# or
make build
```

## Running the daemon

The rtnetlink subscription needs `CAP_NET_ADMIN` (typically root):

```sh
sudo ./bin/go-link-monitor -baseline-dir /var/lib/go-link-monitor
curl -s localhost:9101/metrics | grep network_monitor
```

Flags (all have the defaults shown):

| Flag              | Default                            | Meaning                                                        |
|-------------------|------------------------------------|---------------------------------------------------------------|
| `-metrics-addr`   | `:9101`                            | address for the Prometheus `/metrics` endpoint                |
| `-baseline-dir`   | `/etc/network-monitor/target-ports`| directory holding the filename-encoded baseline              |
| `-settle`         | `30s`                              | count must be stable this long before the first baseline     |
| `-debounce`       | `200ms`                            | quiet period after a change before publishing                |
| `-resync`         | `1h`                               | interval between safety-net full netlink dumps               |
| `-physical-only`  | `true`                             | count only physical ports (exclude lo/veth/bridge/vlan/…)    |

Send `SIGUSR1` to re-record the baseline to the current up-count:

```sh
kill -SIGUSR1 "$(pidof go-link-monitor)"
```

### Exported metrics

| Metric                            | Type    | Meaning                                          |
|-----------------------------------|---------|--------------------------------------------------|
| `network_monitor_up_ports`        | gauge   | current operationally-up ports                   |
| `network_monitor_target_ports`    | gauge   | baseline recorded at first stable inventory      |
| `network_monitor_port_deficit`    | gauge   | `target - up`; `>0` means ports are missing      |
| `network_monitor_resyncs_total`   | counter | full netlink dumps, labelled by `reason`         |

## Baseline store design

The baseline is a single integer stored as a **filename** in a directory (a
file literally named `2`). This makes updates crash-safe with no serialization:

- **Load** is one `ReadDir`; a missing directory means "no baseline yet".
- The **first** write creates the file and `fsync`s the *directory* (the datum
  is the dirent itself).
- **Updates** are a single atomic `rename(2)` — there is never a window with
  zero or two files.
- Crash debris (multiple numeric files from an interrupted first create) is
  resolved by taking the max; non-canonical names (`02`, `+2`, ` 2`) are
  ignored so they can't alias a real value.

See `pkg/linkmonitor/baseline.go`.

## Using the library

```go
m := &linkmonitor.Monitor{
    Store:          linkmonitor.BaselineStore{Dir: "/var/lib/go-link-monitor"},
    ListLinks:      netlink.LinkList,
    Subscribe:      subscribe,      // wraps netlink.LinkSubscribeWithOptions
    Report:         report,         // func(linkmonitor.Status)
    PhysicalOnly:   true,
    SettlePeriod:   30 * time.Second,
    Debounce:       200 * time.Millisecond,
    ResyncInterval: time.Hour,
}
err := m.Run() // blocks; restart on error
```

Every dependency is injected, so the loop is fully testable with fakes (see
`pkg/linkmonitor/monitor_test.go`). `cmd/go-link-monitor/main.go` shows the real
wiring.

**Concurrency:** `PortState` has no mutex and is safe only because `Monitor.Run`
mutates it from a single goroutine. Do not touch a `PortState` concurrently with
`Run`.

## Development

```sh
make test    # go test ./pkg/... ./cmd/...
make race    # go test -race ...
make cover   # coverage summary
make bench   # benchmarks with -benchmem
make fuzz FUZZ=FuzzPortStateApply FUZZTIME=30s
```

### Benchmark analysis (low-hanging fruit)

Representative numbers (`make bench`, Ryzen Threadripper PRO 3945WX):

```
BenchmarkPortStateApplySteady   41.6 ns/op    0 B/op   0 allocs/op
BenchmarkPortStateApplyChurn    76.9 ns/op    0 B/op   0 allocs/op
BenchmarkUpCount                 1.7 ns/op    0 B/op   0 allocs/op
BenchmarkReplace                4973 ns/op    0 B/op   0 allocs/op
BenchmarkSnapshotLinks         18460 ns/op  4952 B/op   4 allocs/op
BenchmarkParseBaselineNames      884 ns/op   168 B/op   6 allocs/op
BenchmarkDecodeLink             16.0 ns/op    0 B/op   0 allocs/op
```

- **`UpCount` — optimized.** It runs on every publish and was previously an
  O(n) recount over the port map. It now returns an incrementally-maintained
  counter (O(1), 0 allocs), kept in sync by `Apply`/`Replace`. A test asserts
  the cached count always equals a naive recount, and `FuzzPortStateApply`
  checks the invariant under arbitrary event sequences.
- **`SnapshotLinks` (4 allocs)** — allocates a fresh map, but only on the
  hourly resync / overrun recovery path, so it is not worth complicating the
  single-goroutine ownership model. Left as-is.
- **`ParseBaselineNames` (6 allocs)** — runs once at startup over a tiny
  directory. Left as-is for clarity.
- **`Apply` / `decodeLink`** — already 0 allocs/op; benchmarked to guard
  against regressions.

## Nix

The repo is a Nix flake with a modular `./nix/` tree (the small `flake.nix`
just wires inputs and re-exports the aggregator in `nix/default.nix`). Targets
`x86_64-linux` and `aarch64-linux`.

```sh
nix develop                          # dev shell (go, linters, gopls, delve, skopeo, …)
nix build .#go-link-monitor          # reusable static binary derivation (default variant)
nix build .#go-link-monitor-stripped # hard-stripped variant (also -debug)
nix build .#oci-go-link-monitor      # minimal OCI image (scratch + CA certs)
./result | docker load               # load the streamed image
nix flake check                      # all static analysis + unit + race tests
nix run .#bench                      # benchmarks
nix run .#govulncheck                # govulncheck (needs network, so not a check)
```

**Layout** (mirrors the xtcp2 style):

| Path | Purpose |
|------|---------|
| `nix/versions.nix` | single source of truth: Go toolchain, linter versions, build variants/tags, `goVendorHash` |
| `nix/lib/mkGoBinary.nix` | reusable `buildGoModule` derivation (static, `netgo osusergo`, `-trimpath`, ldflags version stamp) |
| `nix/lib/goModules.nix` | vendored source reused by every hermetic check |
| `nix/lib/mkOciImage.nix` | `dockerTools.streamLayeredImage` minimal image builder |
| `nix/checks/` | `nix flake check` surface (see below) |
| `nix/tests/` | `test-go-unit`, `test-go-race`, and the `bench` app |
| `nix/devshell.nix`, `nix/packages.nix` | `nix develop` shell |
| `nix/overlays.nix` | `overlays.default` exposing `go-link-monitor` + `-oci` |

**Static analysis run by `nix flake check`** (each hermetic, against the
vendored tree — no network): `go vet`, `gofmt`, **golangci-lint** (config in
[.golangci.yml](.golangci.yml): staticcheck, errcheck, govet, ineffassign,
unused, revive, gocritic, misspell, unconvert, errorlint, bodyclose, prealloc),
**gosec**, plus Nix hygiene (`nixfmt`, `deadnix`, `statix`), a `cli-help-smoke`
of the built binary, and the unit + race test runners.

The binary is version-stamped via `-ldflags -X main.{version,commit,date}`;
`version` comes from the repo-root [`VERSION`](VERSION) file. Refresh the module
hash after changing dependencies:

```sh
nix build .#go-link-monitor 2>&1 | grep 'got:.*sha256-' | head -1
# paste the value into nix/versions.nix (goVendorHash)
```

## License

MIT — see [LICENSE](LICENSE).
