package linkmonitor

import (
	"net"

	"github.com/vishvananda/netlink"
)

// attrs builds a netlink.LinkAttrs fixture. up toggles the admin IFF_UP
// flag; oper sets the operational state.
func attrs(idx int, name string, up bool, oper netlink.LinkOperState) netlink.LinkAttrs {
	a := netlink.NewLinkAttrs()
	a.Index = idx
	a.Name = name
	if up {
		a.Flags |= net.FlagUp
	}
	a.OperState = oper
	return a
}

// operAttrs is attrs plus an explicit RawFlags value, for exercising the
// IFF_RUNNING fallback in IsOperUp.
func operAttrs(up bool, oper netlink.LinkOperState, rawFlags uint32) netlink.LinkAttrs {
	a := attrs(2, "eth0", up, oper)
	a.RawFlags = rawFlags
	return a
}

// physLink builds a physical ("device") link that is admin-up, with oper
// state derived from up.
func physLink(idx int, name string, up bool) *netlink.GenericLink {
	oper := netlink.LinkOperState(netlink.OperDown)
	if up {
		oper = netlink.OperUp
	}
	return &netlink.GenericLink{LinkAttrs: attrs(idx, name, true, oper), LinkType: "device"}
}

// mkUpdate wraps a link in a netlink.LinkUpdate with the given message type
// (e.g. syscall.RTM_NEWLINK / RTM_DELLINK).
func mkUpdate(msgType uint16, l netlink.Link) netlink.LinkUpdate {
	var u netlink.LinkUpdate
	u.Header.Type = msgType
	u.Link = l
	return u
}

// naiveUpCount recomputes the up-count directly from the map, bypassing the
// cached counter. Used to assert the cache invariant.
func naiveUpCount(s *PortState) int {
	n := 0
	for _, u := range s.up {
		if u {
			n++
		}
	}
	return n
}
