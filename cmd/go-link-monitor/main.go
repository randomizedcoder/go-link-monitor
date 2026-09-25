// Command go-link-monitor watches the live count of operationally-up NIC
// ports via rtnetlink, persists a filename-encoded baseline, and exports
// the state as Prometheus metrics.
//
// It requires CAP_NET_ADMIN (typically root) for the rtnetlink
// subscription. Send SIGUSR1 to re-record the baseline to the current
// count.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vishvananda/netlink"

	"github.com/randomizedcoder/go-link-monitor/pkg/linkmonitor"
	"github.com/randomizedcoder/go-link-monitor/pkg/prommetrics"
)

// Build metadata, stamped at build time via -ldflags -X (see nix/lib/mkGoBinary.nix).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var (
		metricsAddr  = flag.String("metrics-addr", ":9101", "address for the Prometheus /metrics endpoint")
		baselineDir  = flag.String("baseline-dir", "/etc/network-monitor/target-ports", "directory holding the filename-encoded baseline")
		settle       = flag.Duration("settle", 30*time.Second, "count must be stable this long before the first baseline is recorded")
		debounce     = flag.Duration("debounce", 200*time.Millisecond, "quiet period after a change before publishing")
		resync       = flag.Duration("resync", time.Hour, "interval between safety-net full netlink dumps")
		physicalOnly = flag.Bool("physical-only", true, "count only physical NIC ports (exclude lo, veth, bridge, vlan, ...)")
		showVersion  = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("go-link-monitor %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	metrics := prommetrics.New()
	prometheus.MustRegister(metrics.Collectors()...)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{
		Addr:              *metricsAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { log.Fatal(srv.ListenAndServe()) }()

	rebaseline := make(chan os.Signal, 1)
	signal.Notify(rebaseline, syscall.SIGUSR1)

	m := &linkmonitor.Monitor{
		Store:     linkmonitor.BaselineStore{Dir: *baselineDir},
		ListLinks: netlink.LinkList,
		Subscribe: func(ch chan<- netlink.LinkUpdate, done <-chan struct{}, onErr func(error)) error {
			return netlink.LinkSubscribeWithOptions(ch, done, netlink.LinkSubscribeOptions{
				ListExisting:  true, // replays a full dump through ch after subscribing
				ErrorCallback: onErr,
			})
		},
		Report:         metrics.Report,
		OnResync:       metrics.OnResync,
		Rebaseline:     rebaseline,
		PhysicalOnly:   *physicalOnly,
		SettlePeriod:   *settle,
		Debounce:       *debounce,
		ResyncInterval: *resync,
	}

	for {
		if err := m.Run(); err != nil {
			log.Printf("watcher exited: %v; restarting in 1s", err)
		}
		time.Sleep(time.Second)
	}
}
