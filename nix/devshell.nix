# `nix develop` shell with the Go toolchain, linters, and helper functions.
{
  pkgs,
  versions,
}:
let
  packages = import ./packages.nix { inherit pkgs versions; };
in
pkgs.mkShell {
  name = "go-link-monitor-dev";
  packages = packages.allDevPackages;

  shellHook = ''
    export CGO_ENABLED=0

    glm-help() {
      cat <<'EOF'
    go-link-monitor dev shell
      lint         golangci-lint run (.golangci.yml)
      lint-fix     golangci-lint run --fix
      vulncheck    govulncheck ./...
      sec          gosec ./...
      bench        go test -bench=. -benchmem ./pkg/linkmonitor/
      cover        go test -cover ./pkg/...

    nix helpers
      nix build .#go-link-monitor              build the daemon
      nix build .#oci-go-link-monitor          build the OCI image (streamed)
      ./result | docker load                   load the streamed image
      nix flake check                          run all static analysis + tests
      nix run .#bench                          run benchmarks
      nix run .#govulncheck                    run govulncheck (needs network)
    EOF
    }

    lint()      { golangci-lint run --config .golangci.yml ./...; }
    lint-fix()  { golangci-lint run --config .golangci.yml --fix ./...; }
    vulncheck() { govulncheck ./...; }
    sec()       { gosec ./...; }
    bench()     { go test -run=NONE -bench=. -benchmem ./pkg/linkmonitor/; }
    cover()     { go test -cover ./pkg/...; }

    glm-help
  '';
}
