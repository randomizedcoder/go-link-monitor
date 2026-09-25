package linkmonitor

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vishvananda/netlink"
)

// Status is what the monitor reports outward (to Prometheus, logs, ...).
type Status struct {
	Up        int
	Target    int
	HasTarget bool
}

// Deficit is target-up when a target exists; >0 means missing ports.
func (s Status) Deficit() int {
	if !s.HasTarget {
		return 0
	}
	return s.Target - s.Up
}

// Monitor owns the event loop. All I/O and policy knobs are injected so
// tests can substitute fakes and millisecond durations.
type Monitor struct {
	Store     BaselineStore
	ListLinks func() ([]netlink.Link, error)
	Subscribe func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error
	Report    func(Status)
	OnResync  func(reason string)

	Rebaseline <-chan os.Signal // e.g. SIGUSR1; may be nil

	PhysicalOnly   bool
	SettlePeriod   time.Duration // count must be stable this long before first baseline
	Debounce       time.Duration
	ResyncInterval time.Duration

	Log *log.Logger

	// internal
	state     *PortState
	target    int
	hasTarget bool
}

func (m *Monitor) status() Status {
	return Status{Up: m.state.UpCount(), Target: m.target, HasTarget: m.hasTarget}
}

func (m *Monitor) publish() {
	st := m.status()
	if m.Report != nil {
		m.Report(st)
	}
	if st.Deficit() > 0 {
		m.Log.Printf("ALERT: %d ports up, baseline is %d", st.Up, st.Target)
	}
}

// commitBaseline persists the current up-count as the target.
func (m *Monitor) commitBaseline() error {
	n := m.state.UpCount()
	if err := m.Store.Save(n, m.target, m.hasTarget); err != nil {
		return fmt.Errorf("saving baseline: %w", err)
	}
	m.target, m.hasTarget = n, true
	m.Log.Printf("baseline recorded: %d ports", n)
	return nil
}

// resyncNow rebuilds state from a full dump (one netlink round trip).
// Used for ENOBUFS recovery and the periodic safety-net inventory.
func (m *Monitor) resyncNow(reason string) error {
	if m.OnResync != nil {
		m.OnResync(reason)
	}
	links, err := m.ListLinks()
	if err != nil {
		return fmt.Errorf("resync (%s): %w", reason, err)
	}
	m.state.Replace(SnapshotLinks(links, m.PhysicalOnly))
	return nil
}

// Run blocks, processing events until Subscribe's channel closes or a
// non-recoverable error occurs. The caller restarts it.
func (m *Monitor) Run() error {
	if m.Log == nil {
		m.Log = log.Default()
	}
	m.state = NewPortState()

	var err error
	if m.target, m.hasTarget, err = m.Store.Load(); err != nil {
		return fmt.Errorf("loading baseline: %w", err)
	}
	if m.hasTarget {
		m.Log.Printf("loaded baseline: %d target ports", m.target)
	} else {
		m.Log.Printf("no baseline; will record one after count is stable for %v", m.SettlePeriod)
	}

	updates := make(chan netlink.LinkUpdate, 256)
	done := make(chan struct{})
	defer close(done)
	overrun := make(chan struct{}, 1)
	if err := m.Subscribe(updates, done, func(err error) {
		m.Log.Printf("netlink error (will resync): %v", err)
		select {
		case overrun <- struct{}{}:
		default:
		}
	}); err != nil {
		return err
	}

	resync := time.NewTicker(m.ResyncInterval)
	defer resync.Stop()

	var quiet <-chan time.Time  // debounce before publishing
	var settle <-chan time.Time // stability clock before first baseline
	if !m.hasTarget {
		settle = time.After(m.SettlePeriod)
	}

	for {
		select {
		case u, ok := <-updates:
			if !ok {
				return errors.New("netlink channel closed")
			}
			ev, ok := decodeLink(u.Header.Type, u.Link, m.PhysicalOnly)
			if !ok || !m.state.Apply(ev) {
				continue
			}
			quiet = time.After(m.Debounce)
			if !m.hasTarget {
				settle = time.After(m.SettlePeriod) // restart stability clock
			}

		case <-quiet:
			quiet = nil
			m.publish()

		case <-settle:
			settle = nil
			if err := m.commitBaseline(); err != nil {
				m.Log.Print(err)
				settle = time.After(m.SettlePeriod) // retry later
				continue
			}
			m.publish()

		case <-m.Rebaseline:
			m.Log.Printf("re-baseline requested; committing current count")
			if err := m.commitBaseline(); err != nil {
				m.Log.Print(err)
				continue
			}
			m.publish()

		case <-resync.C:
			if err := m.resyncNow("periodic"); err != nil {
				return err
			}
			m.publish()

		case <-overrun:
			if err := m.resyncNow("overrun"); err != nil {
				return err
			}
			m.publish()
		}
	}
}
