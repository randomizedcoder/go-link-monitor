package linkmonitor

import (
	"syscall"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestPortStateApply(t *testing.T) {
	// A single PortState is folded through the whole sequence; each row is
	// applied in order. wantChanged/wantCount are the expected results after
	// that step. Covers positive (new/flip), negative (duplicate/absent
	// delete), boundary (count back to 0) and corner (delete of down port)
	// cases, and asserts the cached count matches a naive recount every step.
	steps := []struct {
		description string
		ev          LinkEvent
		wantChanged bool
		wantCount   int
	}{
		{"eth0 comes up: new up key", LinkEvent{Index: 2, Up: true}, true, 1},
		{"duplicate up is a no-op", LinkEvent{Index: 2, Up: true}, false, 1},
		{"eth1 appears down: new down key, no count change", LinkEvent{Index: 3, Up: false}, true, 1},
		{"duplicate down is a no-op", LinkEvent{Index: 3, Up: false}, false, 1},
		{"eth1 comes up: down->up", LinkEvent{Index: 3, Up: true}, true, 2},
		{"eth0 loses carrier: up->down", LinkEvent{Index: 2, Up: false}, true, 1},
		{"eth1 removed: delete of up port drops count", LinkEvent{Index: 3, Deleted: true}, true, 0},
		{"eth0 removed: delete of down port keeps count at 0", LinkEvent{Index: 2, Deleted: true}, true, 0},
		{"double delete is a no-op", LinkEvent{Index: 2, Deleted: true}, false, 0},
		{"delete of never-seen index is a no-op", LinkEvent{Index: 99, Deleted: true}, false, 0},
	}

	s := NewPortState()
	for _, st := range steps {
		t.Run(st.description, func(t *testing.T) {
			if got := s.Apply(st.ev); got != st.wantChanged {
				t.Errorf("Apply(%+v) changed = %v, want %v", st.ev, got, st.wantChanged)
			}
			if got := s.UpCount(); got != st.wantCount {
				t.Errorf("UpCount = %d, want %d", got, st.wantCount)
			}
			if got, want := s.UpCount(), naiveUpCount(s); got != want {
				t.Errorf("cached count %d != naive recount %d (invariant broken)", got, want)
			}
		})
	}
}

func TestPortStateReplace(t *testing.T) {
	cases := []struct {
		description string
		initial     []LinkEvent // applied before Replace
		replaceWith map[int32]bool
		wantChanged bool
		wantCount   int
	}{
		{
			description: "same count reports no change",
			initial:     []LinkEvent{{Index: 2, Up: true}, {Index: 3, Up: true}},
			replaceWith: map[int32]bool{2: true, 3: true},
			wantChanged: false,
			wantCount:   2,
		},
		{
			description: "fewer up ports reports change",
			initial:     []LinkEvent{{Index: 2, Up: true}, {Index: 3, Up: true}},
			replaceWith: map[int32]bool{2: true, 3: false},
			wantChanged: true,
			wantCount:   1,
		},
		{
			description: "more up ports reports change",
			initial:     []LinkEvent{{Index: 2, Up: true}},
			replaceWith: map[int32]bool{2: true, 3: true, 4: true},
			wantChanged: true,
			wantCount:   3,
		},
		{
			description: "same count, different membership reports no change",
			initial:     []LinkEvent{{Index: 2, Up: true}},
			replaceWith: map[int32]bool{5: true},
			wantChanged: false,
			wantCount:   1,
		},
		{
			description: "replace with empty map drops to zero",
			initial:     []LinkEvent{{Index: 2, Up: true}},
			replaceWith: map[int32]bool{},
			wantChanged: true,
			wantCount:   0,
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			s := NewPortState()
			for _, ev := range c.initial {
				s.Apply(ev)
			}
			if got := s.Replace(c.replaceWith); got != c.wantChanged {
				t.Errorf("Replace changed = %v, want %v", got, c.wantChanged)
			}
			if got := s.UpCount(); got != c.wantCount {
				t.Errorf("UpCount = %d, want %d", got, c.wantCount)
			}
			if got, want := s.UpCount(), naiveUpCount(s); got != want {
				t.Errorf("cached count %d != naive recount %d after Replace", got, want)
			}
		})
	}
}

func TestIsOperUp(t *testing.T) {
	cases := []struct {
		description string
		a           netlink.LinkAttrs
		want        bool
	}{
		{"admin up + oper up", attrs(2, "eth0", true, netlink.OperUp), true},
		{"admin up, oper down (no carrier)", attrs(2, "eth0", true, netlink.OperDown), false},
		{"admin down, oper up", attrs(2, "eth0", false, netlink.OperUp), false},
		{"admin down, oper down", attrs(2, "eth0", false, netlink.OperDown), false},
		{"oper unknown, IFF_RUNNING clear", operAttrs(true, netlink.OperUnknown, 0), false},
		{"oper unknown, IFF_RUNNING set", operAttrs(true, netlink.OperUnknown, 0x40), true},
		{"oper unknown but admin down", operAttrs(false, netlink.OperUnknown, 0x40), false},
		{"oper lowerlayerdown", attrs(2, "eth0", true, netlink.OperLowerLayerDown), false},
		{"oper dormant", attrs(2, "eth0", true, netlink.OperDormant), false},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			if got := IsOperUp(&c.a); got != c.want {
				t.Errorf("IsOperUp = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIsPhysical(t *testing.T) {
	cases := []struct {
		description string
		link        netlink.Link
		want        bool
	}{
		{"physical device", &netlink.GenericLink{LinkAttrs: attrs(2, "eth0", true, netlink.OperUp), LinkType: "device"}, true},
		{"loopback excluded", &netlink.GenericLink{LinkAttrs: attrs(1, "lo", true, netlink.OperUnknown), LinkType: "device"}, false},
		{"veth excluded", &netlink.Veth{LinkAttrs: attrs(9, "veth0", true, netlink.OperUp)}, false},
		{"bridge excluded", &netlink.Bridge{LinkAttrs: attrs(10, "br0", true, netlink.OperUp)}, false},
		{"vlan excluded", &netlink.Vlan{LinkAttrs: attrs(11, "eth0.5", true, netlink.OperUp)}, false},
		{"bond excluded", &netlink.Bond{LinkAttrs: attrs(12, "bond0", true, netlink.OperUp)}, false},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			if got := IsPhysical(c.link); got != c.want {
				t.Errorf("IsPhysical = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDecodeLink(t *testing.T) {
	phys := &netlink.GenericLink{LinkAttrs: attrs(2, "eth0", true, netlink.OperUp), LinkType: "device"}
	physDown := &netlink.GenericLink{LinkAttrs: attrs(2, "eth0", true, netlink.OperDown), LinkType: "device"}
	veth := &netlink.Veth{LinkAttrs: attrs(9, "veth0", true, netlink.OperUp)}

	cases := []struct {
		description  string
		msgType      uint16
		link         netlink.Link
		physicalOnly bool
		wantOK       bool
		wantEvent    LinkEvent
	}{
		{"physical newlink up", syscall.RTM_NEWLINK, phys, true, true, LinkEvent{Index: 2, Up: true}},
		{"physical newlink down", syscall.RTM_NEWLINK, physDown, true, true, LinkEvent{Index: 2, Up: false}},
		{"physical dellink", syscall.RTM_DELLINK, phys, true, true, LinkEvent{Index: 2, Deleted: true}},
		{"veth filtered when physicalOnly", syscall.RTM_NEWLINK, veth, true, false, LinkEvent{}},
		{"veth passes when physicalOnly off", syscall.RTM_NEWLINK, veth, false, true, LinkEvent{Index: 9, Up: true}},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			ev, ok := decodeLink(c.msgType, c.link, c.physicalOnly)
			if ok != c.wantOK {
				t.Fatalf("decodeLink ok = %v, want %v", ok, c.wantOK)
			}
			if ok && ev != c.wantEvent {
				t.Errorf("decodeLink event = %+v, want %+v", ev, c.wantEvent)
			}
		})
	}
}

func TestSnapshotLinks(t *testing.T) {
	cases := []struct {
		description  string
		links        []netlink.Link
		physicalOnly bool
		want         map[int32]bool
	}{
		{
			description: "physicalOnly filters lo and veth, captures down state",
			links: []netlink.Link{
				&netlink.GenericLink{LinkAttrs: attrs(2, "eth0", true, netlink.OperUp), LinkType: "device"},
				&netlink.GenericLink{LinkAttrs: attrs(3, "eth1", true, netlink.OperDown), LinkType: "device"},
				&netlink.GenericLink{LinkAttrs: attrs(1, "lo", true, netlink.OperUnknown), LinkType: "device"},
				&netlink.Veth{LinkAttrs: attrs(9, "veth0", true, netlink.OperUp)},
			},
			physicalOnly: true,
			want:         map[int32]bool{2: true, 3: false},
		},
		{
			description:  "physicalOnly off keeps everything",
			links:        []netlink.Link{&netlink.Veth{LinkAttrs: attrs(9, "veth0", true, netlink.OperUp)}},
			physicalOnly: false,
			want:         map[int32]bool{9: true},
		},
		{
			description:  "empty input yields empty map",
			links:        nil,
			physicalOnly: true,
			want:         map[int32]bool{},
		},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			got := SnapshotLinks(c.links, c.physicalOnly)
			if len(got) != len(c.want) {
				t.Fatalf("snapshot = %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("snapshot[%d] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}
