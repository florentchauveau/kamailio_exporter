package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	var (
		listenAddress = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").Short('l').Default(":9494").String()
		metricsPath   = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
		scrapeURI     = kingpin.Flag("kamailio.scrape-uri", `URI on which to scrape kamailio. E.g. "unix:/var/run/kamailio/kamailio_ctl" or "tcp://localhost:2049"`).Short('u').Default("unix:/var/run/kamailio/kamailio_ctl").String()
		methods       = kingpin.Flag("kamailio.methods", `Comma-separated list of methods to call. E.g. "tm.stats,sl.stats". Implemented: `+strings.Join(availableMethods, ",")).Short('m').Default("tm.stats,sl.stats,core.shmmem,core.uptime").String()
		timeout       = kingpin.Flag("kamailio.timeout", "Timeout for trying to get stats from kamailio.").Short('t').Default("5s").Duration()
	)

	kingpin.Parse()

	c, err := NewCollector(*scrapeURI, *timeout, *methods)

	if err != nil {
		panic(err)
	}

	prometheus.MustRegister(c)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>Kamailio Exporter</title></head>
			<body>
			<h1>Kamailio Exporter</h1>
			<p><a href="` + *metricsPath + `">Metrics</a></p>
			</body>
			</html>`))
	})
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
