package linkmonitor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vishvananda/netlink"
)

// waitFor drains a status channel until pred holds or a deadline passes.
func waitFor(t *testing.T, statuses <-chan Status, desc string, pred func(Status) bool) Status {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case st := <-statuses:
			if pred(st) {
				return st
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", desc)
		}
	}
}

// TestMonitorBaselineAndDeficit runs the real event loop against fakes: a
// Subscribe that replays a synthetic ListExisting dump, millisecond timers,
// and a Report hook that captures statuses. Exercises baseline commit after
// settle, deficit reporting after a link-down, and that the persisted
// baseline is untouched by the down event.
func TestMonitorBaselineAndDeficit(t *testing.T) {
	events := make(chan netlink.LinkUpdate, 16)
	statuses := make(chan Status, 64)

	m := &Monitor{
		Store:     BaselineStore{Dir: filepath.Join(t.TempDir(), "target-ports")},
		ListLinks: func() ([]netlink.Link, error) { return nil, nil },
		Subscribe: func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error {
			go func() {
				ch <- mkUpdate(syscall.RTM_NEWLINK, physLink(2, "eth0", true))
				ch <- mkUpdate(syscall.RTM_NEWLINK, physLink(3, "eth1", true))
				for {
					select {
					case u := <-events:
						ch <- u
					case <-done:
						return
					}
				}
			}()
			return nil
		},
		Report:         func(st Status) { statuses <- st },
		PhysicalOnly:   true,
		SettlePeriod:   50 * time.Millisecond,
		Debounce:       5 * time.Millisecond,
		ResyncInterval: time.Hour,
	}

	go m.Run() //nolint:errcheck // loop runs until the test process exits

	waitFor(t, statuses, "baseline=2", func(st Status) bool {
		return st.HasTarget && st.Target == 2 && st.Up == 2
	})

	events <- mkUpdate(syscall.RTM_NEWLINK, physLink(3, "eth1", false))
	waitFor(t, statuses, "deficit=1", func(st Status) bool {
		return st.Up == 1 && st.Deficit() == 1
	})

	if n, found, err := m.Store.Load(); err != nil || !found || n != 2 {
		t.Fatalf("persisted baseline = (%d,%v,%v), want (2,true,nil)", n, found, err)
	}
}

// TestMonitorLoadsExistingBaseline verifies that a baseline already on disk
// is loaded at startup (no settle needed) and drives deficit reporting.
func TestMonitorLoadsExistingBaseline(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "target-ports")
	store := BaselineStore{Dir: dir}
	if err := store.Save(2, 0, false); err != nil {
		t.Fatal(err)
	}

	statuses := make(chan Status, 64)
	m := &Monitor{
		Store:     store,
		ListLinks: func() ([]netlink.Link, error) { return nil, nil },
		Subscribe: func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error {
			go func() {
				ch <- mkUpdate(syscall.RTM_NEWLINK, physLink(2, "eth0", true)) // only one of two up
				<-done
			}()
			return nil
		},
		Report:         func(st Status) { statuses <- st },
		PhysicalOnly:   true,
		SettlePeriod:   time.Hour, // must not be needed; baseline is preloaded
		Debounce:       5 * time.Millisecond,
		ResyncInterval: time.Hour,
	}
	go m.Run() //nolint:errcheck

	st := waitFor(t, statuses, "loaded target with deficit", func(st Status) bool {
		return st.HasTarget && st.Up == 1
	})
	if st.Target != 2 || st.Deficit() != 1 {
		t.Fatalf("loaded status = %+v, want Target 2 Deficit 1", st)
	}
}

// TestMonitorRebaselineSignal verifies the SIGUSR1 path commits the current
// count as the new baseline on demand.
func TestMonitorRebaselineSignal(t *testing.T) {
	rebaseline := make(chan os.Signal, 1)
	statuses := make(chan Status, 64)

	m := &Monitor{
		Store:     BaselineStore{Dir: filepath.Join(t.TempDir(), "target-ports")},
		ListLinks: func() ([]netlink.Link, error) { return nil, nil },
		Subscribe: func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error {
			go func() {
				ch <- mkUpdate(syscall.RTM_NEWLINK, physLink(2, "eth0", true))
				ch <- mkUpdate(syscall.RTM_NEWLINK, physLink(3, "eth1", true))
				<-done
			}()
			return nil
		},
		Report:         func(st Status) { statuses <- st },
		Rebaseline:     rebaseline,
		PhysicalOnly:   true,
		SettlePeriod:   time.Hour, // auto-baseline must not fire
		Debounce:       5 * time.Millisecond,
		ResyncInterval: time.Hour,
	}
	go m.Run() //nolint:errcheck

	// Wait until both ports are reflected (no target yet).
	waitFor(t, statuses, "two up, no target", func(st Status) bool {
		return !st.HasTarget && st.Up == 2
	})

	rebaseline <- syscall.SIGUSR1
	st := waitFor(t, statuses, "baseline committed via signal", func(st Status) bool {
		return st.HasTarget && st.Target == 2
	})
	if st.Deficit() != 0 {
		t.Fatalf("after rebaseline deficit = %d, want 0", st.Deficit())
	}
	if n, found, err := m.Store.Load(); err != nil || !found || n != 2 {
		t.Fatalf("persisted baseline = (%d,%v,%v), want (2,true,nil)", n, found, err)
	}
}

// TestMonitorOverrunResync forces the netlink error callback (ENOBUFS-style
// overrun) and verifies a full resync happens off ListLinks, tagged
// "overrun". This also exercises the netlink-goroutine -> loop-goroutine
// hand-off that `go test -race` should find clean.
func TestMonitorOverrunResync(t *testing.T) {
	onErrCh := make(chan func(error), 1)
	reasons := make(chan string, 8)
	statuses := make(chan Status, 64)

	m := &Monitor{
		Store: BaselineStore{Dir: filepath.Join(t.TempDir(), "target-ports")},
		ListLinks: func() ([]netlink.Link, error) {
			return []netlink.Link{physLink(2, "eth0", true), physLink(3, "eth1", true)}, nil
		},
		Subscribe: func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error {
			onErrCh <- onErr
			go func() { <-done }()
			return nil
		},
		Report:         func(st Status) { statuses <- st },
		OnResync:       func(reason string) { reasons <- reason },
		PhysicalOnly:   true,
		SettlePeriod:   time.Hour,
		Debounce:       5 * time.Millisecond,
		ResyncInterval: time.Hour,
	}
	go m.Run() //nolint:errcheck

	onErr := <-onErrCh
	onErr(errors.New("no buffer space available"))

	if got := <-reasons; got != "overrun" {
		t.Fatalf("resync reason = %q, want overrun", got)
	}
	waitFor(t, statuses, "resync brought up=2", func(st Status) bool { return st.Up == 2 })
}

// TestMonitorPeriodicResync verifies the periodic safety-net inventory fires
// off the ticker and rebuilds state from ListLinks, tagged "periodic".
func TestMonitorPeriodicResync(t *testing.T) {
	reasons := make(chan string, 8)
	statuses := make(chan Status, 64)

	m := &Monitor{
		Store: BaselineStore{Dir: filepath.Join(t.TempDir(), "target-ports")},
		ListLinks: func() ([]netlink.Link, error) {
			return []netlink.Link{physLink(2, "eth0", true)}, nil
		},
		Subscribe: func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error {
			go func() { <-done }()
			return nil
		},
		Report:         func(st Status) { statuses <- st },
		OnResync:       func(reason string) { reasons <- reason },
		PhysicalOnly:   true,
		SettlePeriod:   time.Hour,
		Debounce:       5 * time.Millisecond,
		ResyncInterval: 20 * time.Millisecond,
	}
	go m.Run() //nolint:errcheck

	if got := <-reasons; got != "periodic" {
		t.Fatalf("resync reason = %q, want periodic", got)
	}
	waitFor(t, statuses, "periodic resync up=1", func(st Status) bool { return st.Up == 1 })
}

// TestMonitorCommitBaselineRetry drives the commitBaseline failure/retry
// branch: the baseline directory's parent starts unwritable so the first
// settle fails, then becomes writable so a later settle succeeds.
func TestMonitorCommitBaselineRetry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permissions")
	}
	parent := t.TempDir()
	ro := filepath.Join(parent, "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })

	statuses := make(chan Status, 64)
	m := &Monitor{
		Store:     BaselineStore{Dir: filepath.Join(ro, "target-ports")},
		ListLinks: func() ([]netlink.Link, error) { return nil, nil },
		Subscribe: func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error {
			go func() {
				ch <- mkUpdate(syscall.RTM_NEWLINK, physLink(2, "eth0", true))
				<-done
			}()
			return nil
		},
		Report:         func(st Status) { statuses <- st },
		PhysicalOnly:   true,
		SettlePeriod:   30 * time.Millisecond,
		Debounce:       5 * time.Millisecond,
		ResyncInterval: time.Hour,
	}
	go m.Run() //nolint:errcheck

	// Give the first settle a chance to fire and fail, then unblock writes.
	time.Sleep(60 * time.Millisecond)
	if err := os.Chmod(ro, 0o700); err != nil {
		t.Fatal(err)
	}

	st := waitFor(t, statuses, "baseline committed after retry", func(st Status) bool {
		return st.HasTarget && st.Target == 1
	})
	if st.Up != 1 {
		t.Fatalf("retry status = %+v, want Up 1", st)
	}
}

// TestMonitorChannelClosed verifies Run returns an error when the netlink
// update channel closes.
func TestMonitorChannelClosed(t *testing.T) {
	m := &Monitor{
		Store:     BaselineStore{Dir: filepath.Join(t.TempDir(), "target-ports")},
		ListLinks: func() ([]netlink.Link, error) { return nil, nil },
		Subscribe: func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error {
			close(ch)
			return nil
		},
		PhysicalOnly:   true,
		SettlePeriod:   time.Hour,
		Debounce:       time.Hour,
		ResyncInterval: time.Hour,
	}
	err := m.Run()
	if err == nil || !strings.Contains(err.Error(), "netlink channel closed") {
		t.Fatalf("Run error = %v, want netlink channel closed", err)
	}
}

// TestMonitorSubscribeError verifies a Subscribe failure is returned from Run.
func TestMonitorSubscribeError(t *testing.T) {
	want := errors.New("subscribe boom")
	m := &Monitor{
		Store:     BaselineStore{Dir: filepath.Join(t.TempDir(), "target-ports")},
		ListLinks: func() ([]netlink.Link, error) { return nil, nil },
		Subscribe: func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error {
			return want
		},
		SettlePeriod:   time.Hour,
		Debounce:       time.Hour,
		ResyncInterval: time.Hour,
	}
	if err := m.Run(); !errors.Is(err, want) {
		t.Fatalf("Run error = %v, want %v", err, want)
	}
}

// TestMonitorLoadError verifies a baseline load failure (Dir is a file, so
// ReadDir errors with something other than not-exist) surfaces from Run.
func TestMonitorLoadError(t *testing.T) {
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Monitor{
		Store:     BaselineStore{Dir: f},
		ListLinks: func() ([]netlink.Link, error) { return nil, nil },
		Subscribe: func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error {
			return nil
		},
		SettlePeriod:   time.Hour,
		Debounce:       time.Hour,
		ResyncInterval: time.Hour,
	}
	if err := m.Run(); err == nil || !strings.Contains(err.Error(), "loading baseline") {
		t.Fatalf("Run error = %v, want loading baseline", err)
	}
}

// TestMonitorResyncError verifies a resync ListLinks failure returns from Run.
func TestMonitorResyncError(t *testing.T) {
	want := errors.New("list boom")
	m := &Monitor{
		Store:          BaselineStore{Dir: filepath.Join(t.TempDir(), "target-ports")},
		ListLinks:      func() ([]netlink.Link, error) { return nil, want },
		OnResync:       func(string) {},
		Subscribe:      func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error { return nil },
		SettlePeriod:   time.Hour,
		Debounce:       time.Hour,
		ResyncInterval: 10 * time.Millisecond,
	}
	if err := m.Run(); !errors.Is(err, want) {
		t.Fatalf("Run error = %v, want %v", err, want)
	}
}

func TestStatusDeficit(t *testing.T) {
	cases := []struct {
		description string
		status      Status
		want        int
	}{
		{"no target reports zero", Status{Up: 3, HasTarget: false}, 0},
		{"at baseline reports zero", Status{Up: 2, Target: 2, HasTarget: true}, 0},
		{"missing ports reports positive", Status{Up: 1, Target: 3, HasTarget: true}, 2},
		{"surplus ports reports negative", Status{Up: 4, Target: 3, HasTarget: true}, -1},
		{"no target ignores stray target field", Status{Up: 0, Target: 5, HasTarget: false}, 0},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			if got := c.status.Deficit(); got != c.want {
				t.Errorf("Deficit = %d, want %d", got, c.want)
			}
		})
	}
}
