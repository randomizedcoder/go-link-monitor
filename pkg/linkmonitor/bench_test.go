package linkmonitor

import (
	"strconv"
	"syscall"
	"testing"

	"github.com/vishvananda/netlink"
)

// benchN is the port-table size used by the state benchmarks; large enough
// that an O(n) UpCount would show up against the O(1) cached version.
const benchN = 256

func BenchmarkPortStateApplySteady(b *testing.B) {
	// Steady state: flip one existing port up/down repeatedly. Should be
	// allocation-free.
	s := NewPortState()
	for i := range benchN {
		s.Apply(LinkEvent{Index: int32(i), Up: true})
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		s.Apply(LinkEvent{Index: 7, Up: i%2 == 0})
		i++
	}
}

func BenchmarkPortStateApplyChurn(b *testing.B) {
	// Churn: add then delete a port each iteration (map insert + delete).
	s := NewPortState()
	b.ReportAllocs()
	for b.Loop() {
		s.Apply(LinkEvent{Index: 100000, Up: true})
		s.Apply(LinkEvent{Index: 100000, Deleted: true})
	}
}

func BenchmarkUpCount(b *testing.B) {
	// With the cached counter this is O(1); it was O(n) before the
	// optimization. Runs on every publish.
	s := NewPortState()
	for i := range benchN {
		s.Apply(LinkEvent{Index: int32(i), Up: i%2 == 0})
	}
	b.ReportAllocs()
	var sink int
	for b.Loop() {
		sink += s.UpCount()
	}
	_ = sink
}

func BenchmarkReplace(b *testing.B) {
	m := make(map[int32]bool, benchN)
	for i := range benchN {
		m[int32(i)] = i%2 == 0
	}
	s := NewPortState()
	b.ReportAllocs()
	for b.Loop() {
		s.Replace(m)
	}
}

func BenchmarkSnapshotLinks(b *testing.B) {
	links := make([]netlink.Link, 0, benchN)
	for i := range benchN {
		links = append(links, physLink(i, "eth"+strconv.Itoa(i), i%2 == 0))
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = SnapshotLinks(links, true)
	}
}

func BenchmarkParseBaselineNames(b *testing.B) {
	names := []string{".keep", "README", "2", "3", "notes.txt", "02", "+2"}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = ParseBaselineNames(names)
	}
}

func BenchmarkDecodeLink(b *testing.B) {
	l := physLink(2, "eth0", true)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = decodeLink(syscall.RTM_NEWLINK, l, true)
	}
}
