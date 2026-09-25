package prommetrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/randomizedcoder/go-link-monitor/pkg/linkmonitor"
)

func TestMetricsReport(t *testing.T) {
	cases := []struct {
		description string
		status      linkmonitor.Status
		wantUp      float64
		wantTarget  float64
		wantDeficit float64
	}{
		{
			description: "no target sets only up gauge",
			status:      linkmonitor.Status{Up: 3, HasTarget: false},
			wantUp:      3,
			wantTarget:  0, // never set
			wantDeficit: 0, // never set
		},
		{
			description: "at baseline reports zero deficit",
			status:      linkmonitor.Status{Up: 2, Target: 2, HasTarget: true},
			wantUp:      2,
			wantTarget:  2,
			wantDeficit: 0,
		},
		{
			description: "missing ports reports positive deficit",
			status:      linkmonitor.Status{Up: 1, Target: 3, HasTarget: true},
			wantUp:      1,
			wantTarget:  3,
			wantDeficit: 2,
		},
		{
			description: "surplus ports reports negative deficit",
			status:      linkmonitor.Status{Up: 4, Target: 3, HasTarget: true},
			wantUp:      4,
			wantTarget:  3,
			wantDeficit: -1,
		},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			m := New()
			m.Report(c.status)
			if got := testutil.ToFloat64(m.up); got != c.wantUp {
				t.Errorf("up = %v, want %v", got, c.wantUp)
			}
			if got := testutil.ToFloat64(m.target); got != c.wantTarget {
				t.Errorf("target = %v, want %v", got, c.wantTarget)
			}
			if got := testutil.ToFloat64(m.deficit); got != c.wantDeficit {
				t.Errorf("deficit = %v, want %v", got, c.wantDeficit)
			}
		})
	}
}

func TestMetricsOnResync(t *testing.T) {
	cases := []struct {
		description string
		reasons     []string
		wantByLabel map[string]float64
	}{
		{
			description: "counts by reason",
			reasons:     []string{"overrun", "overrun", "periodic"},
			wantByLabel: map[string]float64{"overrun": 2, "periodic": 1},
		},
		{
			description: "single periodic",
			reasons:     []string{"periodic"},
			wantByLabel: map[string]float64{"periodic": 1},
		},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			m := New()
			for _, r := range c.reasons {
				m.OnResync(r)
			}
			for label, want := range c.wantByLabel {
				if got := testutil.ToFloat64(m.resyncs.WithLabelValues(label)); got != want {
					t.Errorf("resyncs[%q] = %v, want %v", label, got, want)
				}
			}
		})
	}
}

func TestMetricsCollectorsRegister(t *testing.T) {
	// All collectors must be registerable together without conflict.
	m := New()
	cs := m.Collectors()
	if len(cs) != 4 {
		t.Fatalf("Collectors() returned %d, want 4", len(cs))
	}
}
