package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	promslogflag "github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

func main() {
	var (
		metricsPath  = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
		scrapeURI    = kingpin.Flag("kamailio.scrape-uri", `URI on which to scrape kamailio. E.g. "unix:/var/run/kamailio/kamailio_ctl" or "tcp://localhost:2049"`).Short('u').Default("unix:/var/run/kamailio/kamailio_ctl").String()
		methods      = kingpin.Flag("kamailio.methods", `Comma-separated list of methods to call. E.g. "tm.stats,sl.stats". Implemented: `+strings.Join(availableMethods, ",")).Short('m').Default("tm.stats,sl.stats,core.shmmem,core.uptime,core.tcp_info").String()
		timeout      = kingpin.Flag("kamailio.timeout", "Timeout for trying to get stats from kamailio.").Short('t').Default("5s").Duration()
		statsGroups  = kingpin.Flag("kamailio.stats-groups", `Comma-separated list of statistics groups to export with the "stats.fetch" method. E.g. "script,core". Only used if "stats.fetch" is present in --kamailio.methods.`).Default("script").String()
		toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, ":9494")
	)

	promslogConfig := &promslog.Config{}
	promslogflag.AddFlags(kingpin.CommandLine, promslogConfig)

	kingpin.Version(version.Print("kamailio_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promslog.New(promslogConfig)

	c, err := NewCollector(*scrapeURI, *timeout, *methods, *statsGroups, logger)

	if err != nil {
		logger.Error("cannot create collector", "error", err)
		os.Exit(1)
	}

	prometheus.MustRegister(c, versioncollector.NewCollector("kamailio_exporter"))

	http.Handle(*metricsPath, promhttp.Handler())

	if *metricsPath != "/" {
		landingConfig := web.LandingConfig{
			Name:        "Kamailio Exporter",
			Description: "Prometheus Exporter for Kamailio",
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{Address: *metricsPath, Text: "Metrics"},
			},
		}

		landingPage, err := web.NewLandingPage(landingConfig)

		if err != nil {
			logger.Error("cannot create landing page", "error", err)
			os.Exit(1)
		}

		http.Handle("/", landingPage)
	}

	if err := web.ListenAndServe(&http.Server{}, toolkitFlags, logger); err != nil {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}
