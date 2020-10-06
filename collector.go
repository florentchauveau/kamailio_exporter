package main

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"

	binrpc "github.com/florentchauveau/go-kamailio-binrpc/v2"
	"github.com/prometheus/client_golang/prometheus"
)

/* Sample output

kamcmd> tm.stats
{
        current: 1
        waiting: 0
        total: 9514528
        total_local: 2794613
        rpl_received: 19902190
        rpl_generated: 4965793
        rpl_sent: 19908572
        6xx: 7782
        5xx: 2286589
        4xx: 961055
        3xx: 0
        2xx: 6267549
        created: 9514528
        freed: 9514527
        delayed_free: 0
}
kamcmd> sl.stats
{
        200: 666263
        202: 0
        2xx: 0
        300: 0
        301: 0
        302: 0
        400: 5883
        401: 0
        403: 0
        404: 0
        407: 0
        408: 0
        483: 0
        4xx: 5621
        500: 0
        5xx: 0
        6xx: 0
		xxx: 0
}
kamcmd> core.shmmem
{
        total: 67108864
        free: 61189608
        used: 2590984
        real_used: 5919256
        max_used: 13323296
        fragments: 44546
}
kamcmd> core.tcp_info
{
	readers: 8
	max_connections: 4096
	max_tls_connections: 2048
	opened_connections: 595
	opened_tls_connections: 401
	write_queued_bytes: 0
}
kamcmd dlg.stats_active
{
	starting: 152
	connecting: 674
	answering: 0
	ongoing: 512
	all: 1338
}
*/

// Collector implements prometheus.Collector (see below).
// it also contains the config of the exporter.
type Collector struct {
	URI     string
	Timeout time.Duration
	Methods []string

	url   *url.URL
	mutex sync.Mutex
	conn  net.Conn

	up            prometheus.Gauge
	failedScrapes prometheus.Counter
	totalScrapes  prometheus.Counter
}

// Metric is the definition of a metric.
type Metric struct {
	Kind   prometheus.ValueType
	Name   string
	Help   string
	Method string // kamailio method associated with the metric
}

// MetricValue is the value of a metric, with its labels.
type MetricValue struct {
	Value  float64
	Labels map[string]string
}

// DispatcherTarget is a target of the dispatcher module.
type DispatcherTarget struct {
	URI   string
	Flags string
	SetID int
}

// DMQPeer is a peer for the dmq module
type DMQPeer struct {
	Host   string
	Status string
	Local int
}

const (
	namespace = "kamailio"
)

var (
	// this is used to match codes returned by Kamailio
	// examples: "200" or "6xx" or even "xxx"
	codeRegex = regexp.MustCompile("^[0-9x]{3}$")

	// implemented RPC methods
	availableMethods = []string{
		"tm.stats",
		"sl.stats",
		"core.shmmem",
		"core.uptime",
		"core.tcp_info",
		"dispatcher.list",
		"tls.info",
		"dlg.stats_active",
		"dmq.list_nodes"
	}

	metricsList = map[string][]Metric{
		"tm.stats": {
			NewMetricGauge("current", "Current transactions.", "tm.stats"),
			NewMetricGauge("waiting", "Waiting transactions.", "tm.stats"),
			NewMetricCounter("total", "Total transactions.", "tm.stats"),
			NewMetricCounter("total_local", "Total local transactions.", "tm.stats"),
			NewMetricCounter("rpl_received", "Number of reply received.", "tm.stats"),
			NewMetricCounter("rpl_generated", "Number of reply generated.", "tm.stats"),
			NewMetricCounter("rpl_sent", "Number of reply sent.", "tm.stats"),
			NewMetricCounter("created", "Created transactions.", "tm.stats"),
			NewMetricCounter("freed", "Freed transactions.", "tm.stats"),
			NewMetricCounter("delayed_free", "Delayed free transactions.", "tm.stats"),
			NewMetricCounter("codes", "Per-code counters.", "tm.stats"),
		},
		"sl.stats": {
			NewMetricCounter("codes", "Per-code counters.", "sl.stats"),
		},
		"core.shmmem": {
			NewMetricGauge("total", "Total shared memory.", "core.shmmem"),
			NewMetricGauge("free", "Free shared memory.", "core.shmmem"),
			NewMetricGauge("used", "Used shared memory.", "core.shmmem"),
			NewMetricGauge("real_used", "Real used shared memory.", "core.shmmem"),
			NewMetricGauge("max_used", "Max used shared memory.", "core.shmmem"),
			NewMetricGauge("fragments", "Number of fragments in shared memory.", "core.shmmem"),
		},
		"core.uptime": {
			NewMetricCounter("uptime", "Uptime in seconds.", "core.uptime"),
		},
		"core.tcp_info": {
			NewMetricGauge("readers", "Total TCP readers.", "core.tcp_info"),
			NewMetricGauge("max_connections", "Maximum TCP connections", "core.tcp_info"),
			NewMetricGauge("max_tls_connections", "Maximum TLS connections.", "core.tcp_info"),
			NewMetricGauge("opened_connections", "Opened TCP connections.", "core.tcp_info"),
			NewMetricGauge("opened_tls_connections", "Opened TLS connections.", "core.tcp_info"),
			NewMetricGauge("write_queued_bytes", "Write queued bytes.", "core.tcp_info"),
		},
		"dispatcher.list": {
			NewMetricGauge("target", "Target status.", "dispatcher.list"),
		},
		"tls.info": {
			NewMetricGauge("opened_connections", "TLS Opened Connections.", "tls.info"),
			NewMetricGauge("max_connections", "TLS Max Connections.", "tls.info"),
		},
		"dlg.stats_active": {
			NewMetricGauge("starting", "Dialogs starting.", "dlg.stats_active"),
			NewMetricGauge("connecting", "Dialogs connecting.", "dlg.stats_active"),
			NewMetricGauge("answering", "Dialogs answering.", "dlg.stats_active"),
			NewMetricGauge("ongoing", "Dialogs ongoing.", "dlg.stats_active"),
			NewMetricGauge("all", "Dialogs all.", "dlg.stats_active"),
		},
		"dmq.list_nodes": {
			NewMetricGauge("status", "DMQ peer Status", "dmq.list_nodes"),
			NewMetricGauge("local", "DMQ local", "dmq.list_nodes"),
		},
	}
)

// NewMetricGauge is a helper function to create a gauge.
func NewMetricGauge(name string, help string, method string, labels ...string) Metric {
	return Metric{
		prometheus.GaugeValue,
		name,
		help,
		method,
	}
}

// NewMetricCounter is a helper function to create a counter.
func NewMetricCounter(name string, help string, method string, labels ...string) Metric {
	return Metric{
		prometheus.CounterValue,
		name,
		help,
		method,
	}
}

// NewCollector processes uri, timeout and methods and returns a new Collector.
func NewCollector(uri string, timeout time.Duration, methods string) (*Collector, error) {
	c := Collector{}

	c.URI = uri
	c.Timeout = timeout

	var url *url.URL
	var err error

	if url, err = url.Parse(c.URI); err != nil {
		return nil, errors.Wrap(err, "cannot parse URI")
	}

	c.url = url

	c.Methods = strings.Split(methods, ",")

	for _, method := range c.Methods {
		found := false

		for _, m := range availableMethods {
			if m == method {
				found = true
				break
			}
		}

		if !found {
			return nil, errors.Errorf(
				`invalid method "%s". available methods are: %s.`,
				method,
				strings.Join(availableMethods, ","),
			)
		}
	}

	c.up = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "up",
		Help:      "Was the last scrape successful.",
	})

	c.totalScrapes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "exporter_total_scrapes",
		Help:      "Number of total kamailio scrapes",
	})

	c.failedScrapes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "exporter_failed_scrapes",
		Help:      "Number of failed kamailio scrapes",
	})

	return &c, nil
}

// ExportedName returns a formatted Prometheus metric name, in the form:
// "namespace_method_metric" for gauge
// "namespace_method_metric_total" for counters
// "meth.od" is transformed into "meth_od"
//
// examples: "kamailio_tm_stats_current"
//           "kamailio_tm_stats_created_total"
//           "kamailio_sl_stats_200_total"
func (m *Metric) ExportedName() string {
	suffix := m.Name

	if m.Kind == prometheus.CounterValue {
		suffix = m.Name + "_total"
	}

	return fmt.Sprintf("%s_%s_%s",
		namespace,
		strings.Replace(m.Method, ".", "_", -1),
		suffix,
	)
}

// LabelKeys returns the keys of the labels of m
func (m *MetricValue) LabelKeys() []string {
	if len(m.Labels) == 0 {
		return nil
	}

	var list []string

	for key := range m.Labels {
		list = append(list, key)
	}

	// we need to keep the keys and values in a consistent order
	// (a go map does have an order)
	sort.Strings(list)

	return list
}

// LabelValues returns the values of the labels of m
func (m *MetricValue) LabelValues() []string {
	if len(m.Labels) == 0 {
		return nil
	}

	var list []string

	for _, key := range m.LabelKeys() {
		list = append(list, m.Labels[key])
	}

	return list
}

// scrape will connect to the kamailio instance if needed, and push metrics to the Prometheus channel.
func (c *Collector) scrape(ch chan<- prometheus.Metric) error {
	c.totalScrapes.Inc()

	var err error

	address := c.url.Host
	if c.url.Scheme == "unix" {
		address = c.url.Path
	}

	c.conn, err = net.DialTimeout(c.url.Scheme, address, c.Timeout)

	if err != nil {
		return err
	}

	c.conn.SetDeadline(time.Now().Add(c.Timeout))

	defer c.conn.Close()

	for _, method := range c.Methods {
		if _, found := metricsList[method]; !found {
			panic("invalid method requested")
		}

		metricsScraped, err := c.scrapeMethod(method)

		if err != nil {
			return err
		}

		for _, metricDef := range metricsList[method] {
			metricValues, found := metricsScraped[metricDef.Name]

			if !found {
				continue
			}

			for _, metricValue := range metricValues {
				metric, err := prometheus.NewConstMetric(
					prometheus.NewDesc(metricDef.ExportedName(), metricDef.Help, metricValue.LabelKeys(), nil),
					metricDef.Kind,
					metricValue.Value,
					metricValue.LabelValues()...,
				)

				if err != nil {
					return err
				}

				ch <- metric
			}
		}
	}

	return nil
}

// scrapeMethod will return metrics for one method.
func (c *Collector) scrapeMethod(method string) (map[string][]MetricValue, error) {
	records, err := c.fetchBINRPC(method)

	if err != nil {
		return nil, err
	}

	// we expect just 1 record of type map
	if len(records) == 2 && records[0].Type == binrpc.TypeInt && records[0].Value.(int) == 500 {
		return nil, fmt.Errorf(`invalid response for method "%s": [500] %s`, method, records[1].Value.(string))
	} else if len(records) != 1 {
		return nil, fmt.Errorf(`invalid response for method "%s", expected %d record, got %d`,
			method, 1, len(records),
		)
	}

	// all methods implemented in this exporter return a struct
	items, err := records[0].StructItems()

	if err != nil {
		return nil, err
	}

	metrics := make(map[string][]MetricValue)

	switch method {
	case "sl.stats":
		fallthrough
	case "tm.stats":
		for _, item := range items {
			i, _ := item.Value.Int()

			if codeRegex.MatchString(item.Key) {
				// this item is a "code" statistic, eg "200" or "6xx"
				metrics["codes"] = append(metrics["codes"],
					MetricValue{
						Value: float64(i),
						Labels: map[string]string{
							"code": item.Key,
						},
					},
				)
			} else {
				metrics[item.Key] = []MetricValue{{Value: float64(i)}}
			}
		}
	case "tls.info":
		fallthrough
	case "core.shmmem":
		fallthrough
	case "core.tcp_info":
		fallthrough
	case "dlg.stats_active":
		fallthrough
	case "dmq.list_nodes":
		peers, err := parseDMQPeers(items)

		if err != nil {
			return nil, err
		}

		if len(peers) == 0 {
			break
		}
		for _, peer := range peers {
			mv := MetricValue{
				Value: 1,
				Labels: map[string]string{
					"host":   peer.Host,
					"status": peer.Status,
					"local":  peer.Local,
				},
			}

			metrics["peer"] = append(metrics["peer"], mv)
		}
	}

	return metrics, nil
}

	case "core.uptime":
		for _, item := range items {
			i, _ := item.Value.Int()
			metrics[item.Key] = []MetricValue{{Value: float64(i)}}
		}
	case "dispatcher.list":
		targets, err := parseDispatcherTargets(items)

		if err != nil {
			return nil, err
		}

		if len(targets) == 0 {
			break
		}

		for _, target := range targets {
			mv := MetricValue{
				Value: 1,
				Labels: map[string]string{
					"uri":   target.URI,
					"flags": target.Flags,
					"setid": strconv.Itoa(target.SetID),
				},
			}

			metrics["target"] = append(metrics["target"], mv)
		}
	}

	return metrics, nil
}

// parseDispatcherTargets parses the "dispatcher.list" result and returns a list of targets.
func parseDispatcherTargets(items []binrpc.StructItem) ([]DispatcherTarget, error) {
	var result []DispatcherTarget

	for _, item := range items {
		if item.Key != "RECORDS" {
			continue
		}

		sets, err := item.Value.StructItems()

		if err != nil {
			return nil, err
		}

		for _, item = range sets {
			if item.Key != "SET" {
				continue
			}

			setItems, err := item.Value.StructItems()

			if err != nil {
				return nil, err
			}

			var setID int
			var targets []DispatcherTarget

			for _, set := range setItems {
				if set.Key == "ID" {
					if setID, err = set.Value.Int(); err != nil {
						return nil, err
					}
				} else if set.Key == "TARGETS" {
					destinations, err := set.Value.StructItems()

					if err != nil {
						return nil, err
					}

					for _, destination := range destinations {
						if destination.Key != "DEST" {
							continue
						}

						props, err := destination.Value.StructItems()

						if err != nil {
							return nil, err
						}

						target := DispatcherTarget{}

						for _, prop := range props {
							switch prop.Key {
							case "URI":
								target.URI, _ = prop.Value.String()
							case "FLAGS":
								target.Flags, _ = prop.Value.String()
							}
						}

						targets = append(targets, target)
					}
				}
			}

			if setID == 0 {
				return nil, errors.New("missing set ID while parsing dispatcher.list")
			}

			if len(targets) == 0 {
				continue
			}

			for _, target := range targets {
				target.SetID = setID
				result = append(result, target)
			}
		}
	}

	return result, nil
}


// parseDispatcherTargets parses the "dispatcher.list" result and returns a list of targets.
func parseDMQPeers(items []binrpc.StructItem) ([]DMQPeer, error) {
	var result []DMQPeer

	for _, item := range items {
		if item.Key != "RECORDS" {
			continue
		}

		sets, err := item.Value.StructItems()

		if err != nil {
			return nil, err
		}

		for _, item = range sets {
			if item.Key != "SET" {
				continue
			}

			setItems, err := item.Value.StructItems()

			if err != nil {
				return nil, err
			}

			var setID int
			var targets []DispatcherTarget

			for _, set := range setItems {
				if set.Key == "ID" {
					if setID, err = set.Value.Int(); err != nil {
						return nil, err
					}
				} else if set.Key == "TARGETS" {
					destinations, err := set.Value.StructItems()

					if err != nil {
						return nil, err
					}

					for _, destination := range destinations {
						if destination.Key != "DEST" {
							continue
						}

						props, err := destination.Value.StructItems()

						if err != nil {
							return nil, err
						}

						target := DispatcherTarget{}

						for _, prop := range props {
							switch prop.Key {
							case "URI":
								target.URI, _ = prop.Value.String()
							case "FLAGS":
								target.Flags, _ = prop.Value.String()
							}
						}

						targets = append(targets, target)
					}
				}
			}

			if setID == 0 {
				return nil, errors.New("missing set ID while parsing dispatcher.list")
			}

			if len(targets) == 0 {
				continue
			}

			for _, target := range targets {
				target.SetID = setID
				result = append(result, target)
			}
		}
	}

	return result, nil
}

// fetchBINRPC talks to kamailio using the BINRPC protocol.
func (c *Collector) fetchBINRPC(method string) ([]binrpc.Record, error) {
	// WritePacket returns the cookie generated
	cookie, err := binrpc.WritePacket(c.conn, method)

	if err != nil {
		return nil, err
	}

	// the cookie is passed again for verification
	// we receive records in response
	records, err := binrpc.ReadPacket(c.conn, cookie)

	if err != nil {
		return nil, err
	}

	return records, nil
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	err := c.scrape(ch)

	if err != nil {
		c.failedScrapes.Inc()
		c.up.Set(0)
		log.Println("[error]", err)
	} else {
		c.up.Set(1)
	}

	ch <- c.up
	ch <- c.totalScrapes
	ch <- c.failedScrapes
}
