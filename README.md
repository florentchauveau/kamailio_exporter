# Kamailio Exporter for Prometheus
[![Go Report Card](https://goreportcard.com/badge/github.com/florentchauveau/kamailio_exporter)](https://goreportcard.com/report/github.com/florentchauveau/kamailio_exporter)
[![CircleCI](https://circleci.com/gh/florentchauveau/kamailio_exporter.svg?style=shield)](https://circleci.com/gh/florentchauveau/kamailio_exporter)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://github.com/florentchauveau/kamailio_exporter/blob/master/LICENSE)

A [Kamailio](https://www.kamailio.org/) exporter for Prometheus.

It communicates with Kamailio using native [BINRPC](http://kamailio.org/docs/modules/stable/modules/ctl.html) via the `ctl` module. 

BINRPC is implemented in library https://github.com/florentchauveau/go-kamailio-binrpc.

## Getting Started

Pre-built binaries are available in [releases](https://github.com/florentchauveau/kamailio_exporter/releases).

To run it:
```bash
./kamailio_exporter [flags]
```

Help on flags:
```
./kamailio_exporter --help

Flags:
      --help                 Show context-sensitive help (also try --help-long
                             and --help-man).
  -l, --web.listen-address=":9494"
                             Address to listen on for web interface and
                             telemetry.
      --web.telemetry-path="/metrics"
                             Path under which to expose metrics.
  -u, --kamailio.scrape-uri="unix:/var/run/kamailio/kamailio_ctl"
                             URI on which to scrape kamailio. E.g.
                             "unix:/var/run/kamailio/kamailio_ctl" or
                             "tcp://localhost:2049"
  -m, --kamailio.methods="tm.stats,sl.stats,core.shmmem,core.uptime"
                             Comma-separated list of methods to call. E.g.
                             "tm.stats,sl.stats". Implemented:
                             tm.stats,sl.stats,core.shmmem,core.uptime,dispatcher.list
  -t, --kamailio.timeout=5s  Timeout for trying to get stats from kamailio.
  ```

## Usage

The [CTL](http://kamailio.org/docs/modules/stable/modules/ctl.html) module must be loaded by the Kamailio instance. If you are using `kamcmd` (and you probably are), the module is already loaded.

By default (if no parameters are changed in the config file), the `ctl` module exposes a Unix stream socket: `/var/run/kamailio/kamailio_ctl`. If you change it, specify the scrape URI with the `--kamailio.scrape-uri` flag. Example:

```
./kamailio_exporter -u "tcp://localhost:2049"
```

## Metrics

By default, the exporter will try to fetch values from the following commands:

- `tm.stats` (requires the [TM](http://kamailio.org/docs/modules/stable/modules/tm.html) module)
- `sl.stats` (requires the [SL](http://kamailio.org/docs/modules/stable/modules/sl.html) module)
- `core.shmmem`
- `core.uptime`

If you are using the [DISPATCHER](http://kamailio.org/docs/modules/stable/modules/dispatcher.html) module, you can enable `dispatcher.list` as well:

```bash
./kamailio_exporter -m "tm.stats,sl.stats,core.shmmem,core.uptime,dispatcher.list"
```

List of exposed metrics:
```
# HELP kamailio_core_shmmem_fragments Number of fragments in shared memory.
# TYPE kamailio_core_shmmem_fragments gauge
# HELP kamailio_core_shmmem_free Free shared memory.
# TYPE kamailio_core_shmmem_free gauge
# HELP kamailio_core_shmmem_max_used Max used shared memory.
# TYPE kamailio_core_shmmem_max_used gauge
# HELP kamailio_core_shmmem_real_used Real used shared memory.
# TYPE kamailio_core_shmmem_real_used gauge
# HELP kamailio_core_shmmem_total Total shared memory.
# TYPE kamailio_core_shmmem_total gauge
# HELP kamailio_core_shmmem_used Used shared memory.
# TYPE kamailio_core_shmmem_used gauge
# HELP kamailio_core_uptime_uptime_total Uptime in seconds.
# TYPE kamailio_core_uptime_uptime_total counter
# HELP kamailio_dispatcher_list_target Target status.
# TYPE kamailio_dispatcher_list_target gauge
# HELP kamailio_exporter_failed_scrapes Number of failed kamailio scrapes
# TYPE kamailio_exporter_failed_scrapes counter
# HELP kamailio_exporter_total_scrapes Number of total kamailio scrapes
# TYPE kamailio_exporter_total_scrapes counter
# HELP kamailio_sl_stats_codes_total Per-code counters.
# TYPE kamailio_sl_stats_codes_total counter
# HELP kamailio_tm_stats_codes_total Per-code counters.
# TYPE kamailio_tm_stats_codes_total counter
# HELP kamailio_tm_stats_created_total Created transactions.
# TYPE kamailio_tm_stats_created_total counter
# HELP kamailio_tm_stats_current Current transactions.
# TYPE kamailio_tm_stats_current gauge
# HELP kamailio_tm_stats_delayed_free_total Delayed free transactions.
# TYPE kamailio_tm_stats_delayed_free_total counter
# HELP kamailio_tm_stats_freed_total Freed transactions.
# TYPE kamailio_tm_stats_freed_total counter
# HELP kamailio_tm_stats_rpl_generated_total Number of reply generated.
# TYPE kamailio_tm_stats_rpl_generated_total counter
# HELP kamailio_tm_stats_rpl_received_total Number of reply received.
# TYPE kamailio_tm_stats_rpl_received_total counter
# HELP kamailio_tm_stats_rpl_sent_total Number of reply sent.
# TYPE kamailio_tm_stats_rpl_sent_total counter
# HELP kamailio_tm_stats_total_local_total Total local transactions.
# TYPE kamailio_tm_stats_total_local_total counter
# HELP kamailio_tm_stats_total_total Total transactions.
# TYPE kamailio_tm_stats_total_total counter
# HELP kamailio_tm_stats_waiting Waiting transactions.
# TYPE kamailio_tm_stats_waiting gauge
# HELP kamailio_up Was the last scrape successful.
# TYPE kamailio_up gauge
```

## Compiling

With go1.11+, clone the project and:
```
go build
```

Dependencies will be fetch automatically.

## Contributing

Feel free to send pull requests.
