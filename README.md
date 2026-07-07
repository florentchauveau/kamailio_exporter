# Kamailio Exporter for Prometheus

[![Go Report Card](https://goreportcard.com/badge/github.com/florentchauveau/kamailio_exporter)](https://goreportcard.com/report/github.com/florentchauveau/kamailio_exporter)
![CI](https://github.com/florentchauveau/kamailio_exporter/actions/workflows/ci.yml/badge.svg)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://github.com/florentchauveau/kamailio_exporter/blob/master/LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/florentchauveau/kamailio_exporter/v2.svg)](https://pkg.go.dev/github.com/florentchauveau/kamailio_exporter/v2)

A [Kamailio](https://www.kamailio.org/) exporter for Prometheus.

Safe to use in production. It has been used in production for years at [Callr](https://www.callr.com).

It communicates with Kamailio using native [BINRPC](http://kamailio.org/docs/modules/stable/modules/ctl.html) via the `ctl` module.

BINRPC is implemented in library https://github.com/florentchauveau/go-kamailio-binrpc.

## Getting Started

Pre-built binaries are available in [releases](https://github.com/florentchauveau/kamailio_exporter/releases).

Docker images are also available on [DockerHub](https://hub.docker.com/r/florentchauveau/kamailio_exporter).

To run it:

```bash
./kamailio_exporter [flags]
```

Help on flags:

```
./kamailio_exporter --help

Flags:
  -h, --[no-]help                Show context-sensitive help (also try
                                 --help-long and --help-man).
      --web.telemetry-path="/metrics"
                                 Path under which to expose metrics.
  -u, --kamailio.scrape-uri="unix:/var/run/kamailio/kamailio_ctl"
                                 URI on which to scrape kamailio. E.g.
                                 "unix:/var/run/kamailio/kamailio_ctl" or
                                 "tcp://localhost:2049"
  -m, --kamailio.methods="tm.stats,sl.stats,core.shmmem,core.uptime,core.tcp_info"
                                 Comma-separated list of methods to call.
                                 E.g. "tm.stats,sl.stats". Implemented:
                                 tm.stats,sl.stats,core.shmmem,core.uptime,core.tcp_info,dispatcher.list,tls.info,dlg.stats_active,dmq.list_nodes,stats.fetch
  -t, --kamailio.timeout=5s      Timeout for trying to get stats from kamailio.
      --kamailio.stats-groups="script"
                                 Comma-separated list of statistics groups to
                                 export with the "stats.fetch" method. E.g.
                                 "script,core". Only used if "stats.fetch" is
                                 present in --kamailio.methods.
      --[no-]web.systemd-socket  Use systemd socket activation listeners instead
                                 of port listeners (Linux only).
      --web.listen-address=:9494 ...
                                 Addresses on which to expose metrics and web
                                 interface. Repeatable for multiple addresses.
      --web.config.file=""       Path to configuration file that can
                                 enable TLS or authentication. See:
                                 https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md
      --log.level=info           Only log messages with the given severity or
                                 above. One of: [debug, info, warn, error]
      --log.format=logfmt        Output format of log messages. One of: [logfmt,
                                 json]
      --[no-]version             Show application version.
```

## Upgrading from v1.x

Version 2.0.0 modernized the exporter. Exposed metrics are unchanged, but
there are some breaking changes:

- the `-l` short flag was removed: use `--web.listen-address`
  (which is now repeatable, to listen on multiple addresses)
- logs are now written in `logfmt` format, or JSON with `--log.format=json`
- the `--version` output format changed

New in this version: TLS and basic authentication support (see below),
`--log.level` and `--log.format` flags, and a `kamailio_exporter_build_info`
metric.

## TLS and basic authentication

The exporter uses the [Prometheus exporter-toolkit](https://github.com/prometheus/exporter-toolkit):
TLS, basic authentication and systemd socket activation are supported via the
`--web.config.file` flag. Example:

```yaml
# web-config.yml
tls_server_config:
  cert_file: server.crt
  key_file: server.key
```

```bash
./kamailio_exporter --web.config.file=web-config.yml
```

See the [web configuration documentation](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md) for all options.

## Usage

The [CTL](http://kamailio.org/docs/modules/stable/modules/ctl.html) module must be loaded by the Kamailio instance. If you are using `kamcmd` (and you probably are), the module is already loaded.

By default (if no parameters are changed in the config file), the `ctl` module exposes a Unix stream socket: `/var/run/kamailio/kamailio_ctl`. If you change it, specify the scrape URI with the `--kamailio.scrape-uri` flag. Example:

```
./kamailio_exporter -u "tcp://localhost:2049"
```

## Metrics

### Default metrics

By default, the exporter will try to fetch values from the following commands:

- `tm.stats` (requires the [TM](http://kamailio.org/docs/modules/stable/modules/tm.html) module)
- `sl.stats` (requires the [SL](http://kamailio.org/docs/modules/stable/modules/sl.html) module)
- `core.shmmem`
- `core.uptime`

### Module specific metrics

#### Dispatcher

If you are using the [DISPATCHER](http://kamailio.org/docs/modules/stable/modules/dispatcher.html) module, you can enable `dispatcher.list`.

#### TLS

For [TLS](https://kamailio.org/docs/modules/stable/modules/tls.html) you can enable `tls.info`.

#### Dialog

For [DIALOG](http://kamailio.org/docs/modules/stable/modules/dialog.html) module, you can enable `dlg.stats_active`.

#### DMQ

If you are using the [DMQ](https://kamailio.org/docs/modules/stable/modules/dmq.html) module, you can enable `dmq.list_nodes` to export the status of DMQ nodes:

```
kamailio_dmq_list_nodes_node{host="10.0.0.2",local="0",port="5090",status="active"} 1
```

### Custom statistics (script stats)

Statistics created in your Kamailio config with `update_stat()` live in the
`script` group of the statistics framework:

```
event_route[dispatcher:dst-down] {
    update_stat("destination_down", "+1");
}
```

Enable the `stats.fetch` method to export them:

```bash
./kamailio_exporter -m "tm.stats,sl.stats,core.shmmem,core.uptime,stats.fetch"
```

```
kamailio_stats_fetch_value{group="script",name="destination_down"} 4
```

The groups to fetch are controlled with `--kamailio.stats-groups` (default
`script`). Any group of the statistics framework works (e.g. `script,core`),
as well as `all`, and full statistic names (e.g. `script:destination_down`).

### Example for using non-default metrics

```bash
./kamailio_exporter -m "tm.stats,sl.stats,core.shmmem,core.uptime,dispatcher.list,tls.info,dlg.stats_active"
```

If you want more information regarding TCP and TLS connections, you can use `core.tcp_info` as well:

```bash
./kamailio_exporter -m "tm.stats,sl.stats,core.shmmem,core.uptime,core.tcp_info"
```

List of exposed metrics:

```bash
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
# HELP kamailio_core_tcp_info_readers Total TCP readers.
# TYPE kamailio_core_tcp_info_readers gauge
# HELP kamailio_core_tcp_info_max_connections Maximum TCP connections.
# TYPE kamailio_core_tcp_info_max_connections gauge
# HELP kamailio_core_tcp_info_max_tls_connections Maximum TLS connections.
# TYPE kamailio_core_tcp_info_max_tls_connections gauge
# HELP kamailio_core_tcp_info_max_opened_connections Opened TCP connections.
# TYPE kamailio_core_tcp_info_max_opened_connections gauge
# HELP kamailio_core_tcp_info_max_opened_tls_connections Opened TLS connections.
# TYPE kamailio_core_tcp_info_max_opened_tls_connections gauge
# HELP kamailio_core_tcp_info_max_write_queued_bytes Write queued bytes.
# TYPE kamailio_core_tcp_info_max_write_queued_bytes gauge
# HELP kamailio_tls_info_opened_connections Number of opened tls connections.
# TYPE kamailio_tls_info_opened_connections gauge
# HELP kamailio_tls_info_max_connections Number of max tls connections.
# TYPE kamailio_tls_info_max_connections gauge
# HELP kamailio_dlg_stats_active_all Dialogs all.
# TYPE kamailio_dlg_stats_active_all gauge
# HELP kamailio_dlg_stats_active_answering Dialogs answering.
# TYPE kamailio_dlg_stats_active_answering gauge
# HELP kamailio_dlg_stats_active_connecting Dialogs connecting.
# TYPE kamailio_dlg_stats_active_connecting gauge
# HELP kamailio_dlg_stats_active_ongoing Dialogs ongoing.
# TYPE kamailio_dlg_stats_active_ongoing gauge
# HELP kamailio_dlg_stats_active_starting Dialogs starting.
# TYPE kamailio_dlg_stats_active_starting gauge
```

## Compiling

With go1.26+, clone the project and:

```bash
go build
```

Dependencies will be fetched automatically.

## Contributing

Feel free to send pull requests.

## How it works

How we implemented the exporter is explained in this blog post: https://blog.callr.tech/kamailio-exporter-for-prometheus/
