package main

import (
	"bytes"
	"encoding/hex"
	"io"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	binrpc "github.com/florentchauveau/go-kamailio-binrpc/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// tmStatsPayload is a real "tm.stats" BINRPC response payload (a single
// struct record), captured from a Kamailio instance. It decodes to:
// current=1, waiting=1, total=8989173, total_local=2331335,
// rpl_received=19361652, rpl_generated=4951553, rpl_sent=19365758,
// 6xx=7402, 5xx=2286546, 4xx=956666, 3xx=0, 2xx=5746744,
// created=8989173, freed=8989172, delayed_free=0
const tmStatsPayload = "03950863757272656e74001001950877616974696e6700100165746f74616c00308929f5950c746f74616c5f6c6f63616c00302396c7950d72706c5f7265636569766564004001276f74950e72706c5f67656e65726174656400304b8e01950972706c5f73656e74004001277f7e4536787800201cea45357878003022e3d24534787800300e98fa45337878000045327878003057b03895086372656174656400308929f565667265656400308929f4950d64656c617965645f66726565000083"

func TestExportedName(t *testing.T) {
	tests := []struct {
		metric   Metric
		expected string
	}{
		{Metric{Kind: prometheus.GaugeValue, Name: "current", Method: "tm.stats"}, "kamailio_tm_stats_current"},
		{Metric{Kind: prometheus.CounterValue, Name: "created", Method: "tm.stats"}, "kamailio_tm_stats_created_total"},
		{Metric{Kind: prometheus.CounterValue, Name: "codes", Method: "sl.stats"}, "kamailio_sl_stats_codes_total"},
		{Metric{Kind: prometheus.GaugeValue, Name: "readers", Method: "core.tcp_info"}, "kamailio_core_tcp_info_readers"},
	}

	for _, test := range tests {
		if name := test.metric.ExportedName(); name != test.expected {
			t.Errorf(`expected "%s", got "%s"`, test.expected, name)
		}
	}
}

func TestMetricValueLabels(t *testing.T) {
	empty := MetricValue{Value: 1}

	if keys := empty.LabelKeys(); keys != nil {
		t.Errorf("expected nil keys for empty labels, got %v", keys)
	}

	if values := empty.LabelValues(); values != nil {
		t.Errorf("expected nil values for empty labels, got %v", values)
	}

	m := MetricValue{
		Value: 1,
		Labels: map[string]string{
			"uri":   "sip:10.0.0.1:5060",
			"flags": "AP",
			"setid": "2",
		},
	}

	// keys must be sorted, values must follow the same order
	if keys := m.LabelKeys(); !reflect.DeepEqual(keys, []string{"flags", "setid", "uri"}) {
		t.Errorf("unexpected keys: %v", keys)
	}

	if values := m.LabelValues(); !reflect.DeepEqual(values, []string{"AP", "2", "sip:10.0.0.1:5060"}) {
		t.Errorf("unexpected values: %v", values)
	}
}

func TestNewCollector(t *testing.T) {
	c, err := NewCollector("tcp://localhost:2049", time.Second, "tm.stats,sl.stats")

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(c.Methods, []string{"tm.stats", "sl.stats"}) {
		t.Errorf("unexpected methods: %v", c.Methods)
	}

	if _, err = NewCollector("tcp://localhost:2049", time.Second, "invalid.method"); err == nil {
		t.Error("expected an error for an invalid method")
	}

	if _, err = NewCollector("://invalid", time.Second, "tm.stats"); err == nil {
		t.Error("expected an error for an invalid URI")
	}
}

func TestParseDispatcherTargets(t *testing.T) {
	newString := func(s string) binrpc.Record {
		return binrpc.Record{Type: binrpc.TypeString, Value: s}
	}
	newStruct := func(items []binrpc.StructItem) binrpc.Record {
		return binrpc.Record{Type: binrpc.TypeStruct, Value: items}
	}

	newDest := func(uri string, flags string) binrpc.StructItem {
		return binrpc.StructItem{Key: "DEST", Value: newStruct([]binrpc.StructItem{
			{Key: "URI", Value: newString(uri)},
			{Key: "FLAGS", Value: newString(flags)},
		})}
	}

	items := []binrpc.StructItem{
		{Key: "NR_SETS", Value: newString("1")},
		{Key: "RECORDS", Value: newStruct([]binrpc.StructItem{
			{Key: "SET", Value: newStruct([]binrpc.StructItem{
				{Key: "ID", Value: binrpc.Record{Type: binrpc.TypeInt, Value: 2}},
				{Key: "TARGETS", Value: newStruct([]binrpc.StructItem{
					newDest("sip:10.0.0.1:5060", "AP"),
					newDest("sip:10.0.0.2:5060", "IP"),
				})},
			})},
		})},
	}

	targets, err := parseDispatcherTargets(items)

	if err != nil {
		t.Fatal(err)
	}

	expected := []DispatcherTarget{
		{URI: "sip:10.0.0.1:5060", Flags: "AP", SetID: 2},
		{URI: "sip:10.0.0.2:5060", Flags: "IP", SetID: 2},
	}

	if !reflect.DeepEqual(targets, expected) {
		t.Errorf("unexpected targets: %v", targets)
	}

	// a SET without an ID must return an error
	missingID := []binrpc.StructItem{
		{Key: "RECORDS", Value: newStruct([]binrpc.StructItem{
			{Key: "SET", Value: newStruct([]binrpc.StructItem{
				{Key: "TARGETS", Value: newStruct([]binrpc.StructItem{
					newDest("sip:10.0.0.1:5060", "AP"),
				})},
			})},
		})},
	}

	if _, err = parseDispatcherTargets(missingID); err == nil {
		t.Error("expected an error for a SET without ID")
	}
}

func TestCollectorScrapeTCP(t *testing.T) {
	payload, _ := hex.DecodeString(tmStatsPayload)
	address := startFakeKamailio(t, "tcp", "127.0.0.1:0", map[string][]byte{
		"tm.stats": payload,
	})

	c, err := NewCollector("tcp://"+address, time.Second, "tm.stats")

	if err != nil {
		t.Fatal(err)
	}

	expected := `
		# HELP kamailio_tm_stats_codes_total Per-code counters.
		# TYPE kamailio_tm_stats_codes_total counter
		kamailio_tm_stats_codes_total{code="2xx"} 5746744
		kamailio_tm_stats_codes_total{code="3xx"} 0
		kamailio_tm_stats_codes_total{code="4xx"} 956666
		kamailio_tm_stats_codes_total{code="5xx"} 2286546
		kamailio_tm_stats_codes_total{code="6xx"} 7402
		# HELP kamailio_tm_stats_current Current transactions.
		# TYPE kamailio_tm_stats_current gauge
		kamailio_tm_stats_current 1
		# HELP kamailio_tm_stats_total_total Total transactions.
		# TYPE kamailio_tm_stats_total_total counter
		kamailio_tm_stats_total_total 8989173
		# HELP kamailio_up Was the last scrape successful.
		# TYPE kamailio_up gauge
		kamailio_up 1
	`

	err = testutil.CollectAndCompare(c, strings.NewReader(expected),
		"kamailio_tm_stats_codes_total",
		"kamailio_tm_stats_current",
		"kamailio_tm_stats_total_total",
		"kamailio_up",
	)

	if err != nil {
		t.Error(err)
	}
}

func TestCollectorScrapeUnixSocket(t *testing.T) {
	payload, _ := hex.DecodeString(tmStatsPayload)
	socket := filepath.Join(t.TempDir(), "kamailio_ctl")

	startFakeKamailio(t, "unix", socket, map[string][]byte{
		"tm.stats": payload,
	})

	c, err := NewCollector("unix:"+socket, time.Second, "tm.stats")

	if err != nil {
		t.Fatal(err)
	}

	expected := `
		# HELP kamailio_up Was the last scrape successful.
		# TYPE kamailio_up gauge
		kamailio_up 1
	`

	if err = testutil.CollectAndCompare(c, strings.NewReader(expected), "kamailio_up"); err != nil {
		t.Error(err)
	}
}

func TestCollectorScrapeErrorResponse(t *testing.T) {
	// a BINRPC error response: an int code and a string message
	var payload bytes.Buffer

	code, _ := binrpc.CreateRecord(500)
	message, _ := binrpc.CreateRecord("command tm.stats not found")

	if err := code.Encode(&payload); err != nil {
		t.Fatal(err)
	}
	if err := message.Encode(&payload); err != nil {
		t.Fatal(err)
	}

	address := startFakeKamailio(t, "tcp", "127.0.0.1:0", map[string][]byte{
		"tm.stats": payload.Bytes(),
	})

	c, err := NewCollector("tcp://"+address, time.Second, "tm.stats")

	if err != nil {
		t.Fatal(err)
	}

	expectUp(t, c, 0)
}

func TestCollectorScrapeConnectionRefused(t *testing.T) {
	// grab a port that nothing is listening on
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	address := listener.Addr().String()
	listener.Close()

	c, err := NewCollector("tcp://"+address, time.Second, "tm.stats")

	if err != nil {
		t.Fatal(err)
	}

	expectUp(t, c, 0)
}

// expectUp collects c and verifies the value of kamailio_up.
func expectUp(t *testing.T, c *Collector, up int) {
	t.Helper()

	expected := `
		# HELP kamailio_up Was the last scrape successful.
		# TYPE kamailio_up gauge
		kamailio_up ` + map[int]string{0: "0", 1: "1"}[up] + `
	`

	if err := testutil.CollectAndCompare(c, strings.NewReader(expected), "kamailio_up"); err != nil {
		t.Error(err)
	}
}

// startFakeKamailio starts a BINRPC server that responds to each request
// with the payload registered for the requested method.
// It returns the address the server is listening on.
func startFakeKamailio(t *testing.T, network string, address string, payloads map[string][]byte) string {
	t.Helper()

	listener, err := net.Listen(network, address)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()

			if err != nil {
				return
			}

			go serveBINRPC(conn, payloads)
		}
	}()

	return listener.Addr().String()
}

// serveBINRPC answers BINRPC requests on conn until an error occurs
// or an unknown method is requested.
func serveBINRPC(conn net.Conn, payloads map[string][]byte) {
	defer conn.Close()

	for {
		header, err := binrpc.ReadHeader(conn)

		if err != nil {
			return
		}

		record, err := binrpc.ReadRecord(conn)

		if err != nil {
			return
		}

		method, err := record.String()

		if err != nil {
			return
		}

		payload, found := payloads[method]

		if !found {
			return
		}

		if err = writeRawPacket(conn, header.Cookie, payload); err != nil {
			return
		}
	}
}

// writeRawPacket writes a BINRPC packet with the given cookie and payload.
// The binrpc package cannot encode struct records, so tests provide raw
// payload bytes (see tmStatsPayload).
func writeRawPacket(w io.Writer, cookie uint32, payload []byte) error {
	length := minBigEndian(len(payload))
	cookieBytes := minBigEndian(int(cookie))

	packet := []byte{
		binrpc.BinRPCMagic<<4 | binrpc.BinRPCVersion,
		byte((len(length)-1)<<2 | (len(cookieBytes) - 1)),
	}
	packet = append(packet, length...)
	packet = append(packet, cookieBytes...)
	packet = append(packet, payload...)

	_, err := w.Write(packet)

	return err
}

// minBigEndian returns the big-endian representation of n,
// without leading zero bytes.
func minBigEndian(n int) []byte {
	b := []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}

	for len(b) > 1 && b[0] == 0 {
		b = b[1:]
	}

	return b
}
