package linkmonitor

import (
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

// LinkEvent is a netlink link update reduced to exactly what the state
// machine needs. Decoding happens at the edge (decodeLink); everything
// past that point is pure and testable with literals.
type LinkEvent struct {
	Index   int32
	Deleted bool
	Up      bool
}

// IsOperUp reports whether a link is administratively up AND has carrier.
// IFF_UP alone is not enough — an unplugged NIC can still be admin-up.
func IsOperUp(a *netlink.LinkAttrs) bool {
	if a.Flags&net.FlagUp == 0 {
		return false
	}
	switch a.OperState {
	case netlink.OperUp:
		return true
	case netlink.OperUnknown:
		// Some drivers never set operstate; fall back to IFF_RUNNING.
		return a.RawFlags&uint32(0x40 /* IFF_RUNNING */) != 0
	default:
		return false
	}
}

// IsPhysical reports whether a link is a real NIC port (as opposed to
// lo, veth, bridge, vlan, bond, tunnel...).
func IsPhysical(l netlink.Link) bool {
	return l.Type() == "device" && l.Attrs().Name != "lo"
}

// decodeLink translates one netlink message into a LinkEvent.
// ok=false means the message should be ignored (filtered out).
// Taking primitives (msgType + Link) rather than netlink.LinkUpdate keeps
// this testable without constructing raw netlink message headers.
func decodeLink(msgType uint16, l netlink.Link, physicalOnly bool) (LinkEvent, bool) {
	if physicalOnly && !IsPhysical(l) {
		return LinkEvent{}, false
	}
	e := LinkEvent{Index: int32(l.Attrs().Index)}
	if msgType == syscall.RTM_DELLINK {
		e.Deleted = true
		return e, true
	}
	e.Up = IsOperUp(l.Attrs())
	return e, true
}

// SnapshotLinks reduces a full RTM_GETLINK dump to an up-map, applying the
// same filter and up-definition as the event path so the two can't drift.
func SnapshotLinks(links []netlink.Link, physicalOnly bool) map[int32]bool {
	m := make(map[int32]bool, len(links))
	for _, l := range links {
		if physicalOnly && !IsPhysical(l) {
			continue
		}
		m[int32(l.Attrs().Index)] = IsOperUp(l.Attrs())
	}
	return m
}

// PortState tracks per-interface operational state. Pure: no I/O, no
// netlink types, no time.
//
// PortState is NOT safe for concurrent use. Monitor.Run mutates it from a
// single goroutine; callers must uphold that invariant if they use it
// directly.
type PortState struct {
	up map[int32]bool
	// count caches the number of up ports so UpCount is O(1). It is kept
	// in sync incrementally by Apply and recomputed once by Replace. The
	// invariant count == (number of true values in up) always holds.
	count int
}

// NewPortState returns an empty PortState ready for use.
func NewPortState() *PortState {
	return &PortState{up: make(map[int32]bool)}
}

// Apply folds one event into the state, reporting whether anything changed.
func (s *PortState) Apply(e LinkEvent) (changed bool) {
	if e.Deleted {
		prev, ok := s.up[e.Index]
		if !ok {
			return false
		}
		delete(s.up, e.Index)
		if prev {
			s.count--
		}
		return true
	}
	prev, ok := s.up[e.Index]
	if ok && prev == e.Up {
		return false
	}
	s.up[e.Index] = e.Up
	switch {
	case !ok && e.Up: // new key, up
		s.count++
	case ok && !prev && e.Up: // down -> up
		s.count++
	case ok && prev && !e.Up: // up -> down
		s.count--
	}
	return true
}

// Replace swaps in a fresh snapshot (resync path), reporting whether the
// up-count changed as a result.
func (s *PortState) Replace(m map[int32]bool) (changed bool) {
	before := s.count
	n := 0
	for _, u := range m {
		if u {
			n++
		}
	}
	s.up = m
	s.count = n
	return n != before
}

// UpCount returns the number of up ports. It is O(1): the count is
// maintained incrementally by Apply and Replace.
func (s *PortState) UpCount() int {
	return s.count
}
