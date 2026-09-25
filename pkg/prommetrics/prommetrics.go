// Package prommetrics adapts linkmonitor.Status to Prometheus collectors.
//
// It is kept separate from package linkmonitor so the core library carries
// no Prometheus dependency: only callers that actually want Prometheus
// export (such as cmd/go-link-monitor) import this package.
package prommetrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/randomizedcoder/go-link-monitor/pkg/linkmonitor"
)

// Metrics holds the collectors exported by the monitor.
type Metrics struct {
	target  prometheus.Gauge
	up      prometheus.Gauge
	deficit prometheus.Gauge
	resyncs *prometheus.CounterVec
}

// New constructs the collectors. Register them with a registry (or the
// default one) via Collectors before use.
func New() *Metrics {
	return &Metrics{
		target: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "network_monitor_target_ports",
			Help: "Baseline number of up ports recorded at first inventory.",
		}),
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "network_monitor_up_ports",
			Help: "Current number of operationally up ports.",
		}),
		deficit: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "network_monitor_port_deficit",
			Help: "target - up. >0 means ports are missing vs. baseline.",
		}),
		resyncs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "network_monitor_resyncs_total",
			Help: "Full netlink dumps performed, by reason.",
		}, []string{"reason"}),
	}
}

// Collectors returns the collectors for registration, e.g.
// prometheus.MustRegister(m.Collectors()...).
func (m *Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{m.target, m.up, m.deficit, m.resyncs}
}

// Report adapts a linkmonitor.Status to the gauges. Suitable as
// Monitor.Report.
func (m *Metrics) Report(st linkmonitor.Status) {
	m.up.Set(float64(st.Up))
	if st.HasTarget {
		m.target.Set(float64(st.Target))
		m.deficit.Set(float64(st.Deficit()))
	}
}

// OnResync increments the resync counter for the given reason. Suitable as
// Monitor.OnResync.
func (m *Metrics) OnResync(reason string) {
	m.resyncs.WithLabelValues(reason).Inc()
}
