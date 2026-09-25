// Package linkmonitor tracks the live count of operationally-up physical
// network interface (NIC) ports on Linux via rtnetlink, and persists a
// "baseline" target count so it can alert when ports go missing.
//
// # Layers
//
// The package is split into a pure core and an I/O edge:
//
//   - PortState is a pure state machine (map of interface index -> up), with
//     no I/O, no netlink types and no time. It caches its up-count so
//     UpCount is O(1).
//   - The decode/snapshot helpers (IsOperUp, IsPhysical, SnapshotLinks and
//     the internal decodeLink) interpret netlink link objects into the
//     primitive LinkEvent / up-map values the core consumes.
//   - BaselineStore persists a single integer as a crash-safe, filename-
//     encoded datum (a file literally named after the count), updated via an
//     atomic rename(2) and a directory fsync.
//   - Monitor owns the event loop. Every external dependency (netlink list,
//     netlink subscribe, the outward report, the resync hook, the rebaseline
//     signal) is injected as a struct field, so it is fully testable with
//     fakes and millisecond timers, and imports no Prometheus code.
//
// # Concurrency
//
// PortState is NOT safe for concurrent use and deliberately has no mutex.
// Monitor.Run mutates it from a single goroutine; the only other goroutine
// in play is the one inside the injected Subscribe (netlink), which
// communicates solely through channels. Callers using PortState directly
// must uphold the single-goroutine invariant.
//
// A running Monitor requires CAP_NET_ADMIN (typically root) for the
// underlying rtnetlink subscription; the pure pieces do not.
package linkmonitor
