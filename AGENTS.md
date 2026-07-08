# kamailio_exporter — agent notes

Prometheus exporter for Kamailio, scraping over the ctl module's BINRPC
socket (via github.com/florentchauveau/go-kamailio-binrpc).

Single `package main`: `main.go` (flags, exporter-toolkit listener),
`collector.go` (prometheus.Collector), `collector_test.go`. Module path
is `/v2`.

## Logging

Logging goes through `github.com/prometheus/common/promslog` (Prometheus
exporter toolkit, logfmt output) — never import `log/slog` directly, even
in tests. `promslog.New()` returns a `*slog.Logger`, so fields typed
`*slog.Logger` are fine; only the construction must use promslog.

- Production: `promslog.New(&promslog.Config{})` (see main.go)
- Tests, discarded output: `promslog.NewNopLogger()`
- Tests, captured output: `promslog.New(&promslog.Config{Writer: &buf})`

## Collector conventions

- RPC methods are whitelisted in `availableMethods` and declared in
  `metricsList`; per-method response parsing lives in the switch in
  `scrapeMethod`. Metric names are `kamailio_<method>_<name>` (dots
  become underscores), counters get a `_total` suffix (`ExportedName`).
- Parse numeric values with `scanValue`/`Record.Scan` into float64 —
  never `Int()` directly. Kamailio returns ints, doubles, or strings
  depending on version (issue #30: shmmem doubles used to export as 0).
  Log and skip unparseable values; never export 0 for them.
- Dynamic names (custom stats, node addresses) go into labels, not
  metric names — user-defined strings may be invalid Prometheus
  identifiers, and duplicate label sets fail the whole scrape (include
  enough labels, e.g. dmq nodes need `port`).
- Response shapes: most methods return one struct record;
  `dmq.list_nodes` returns one record per node; `stats.fetch` takes RPC
  arguments (groups need a trailing colon, e.g. `script:`) and returns
  `group.name` keys (dot-separated). A `[500, message]` pair is an
  error response, handled before method parsing.
- `fetchBINRPC` is variadic: method first, then RPC arguments.

## Tests

- `go test -race ./...`; CI also enforces `go vet` and `gofmt -l`.
- Integration tests run against a fake BINRPC server
  (`startFakeKamailio`): responses are keyed by the full request —
  method and arguments joined with spaces (`"stats.fetch script:"`).
- Build struct responses with `encodeStructPayload` (int, float64,
  string values); concatenate payloads for multi-record responses.
  `tmStatsPayload` is a real captured response.
- When fixing a bug, prove the new test fails on the old code
  (`git stash push <file>` → test → `git stash pop`).

## Release flow

- Florent creates releases manually in the GitHub UI (pre-release box
  for rc tags). The release workflow triggers on `release: published`;
  goreleaser attaches binaries to the existing release and preserves
  its notes (keep-existing) and pre-release flag (`prerelease: auto`).
- Never switch the trigger back to tag push: it races with UI-created
  releases (two release writers → 422 tag already_exists).
- Versions are stamped into `github.com/prometheus/common/version` via
  ldflags in `.goreleaser.yaml` (there is no `main.version`). Verify a
  released binary with `--version` and `go version -m` (dep versions).
- Docker images are built by a separate buildx job in the same
  workflow (not by goreleaser).

## Git conventions

- No direct pushes to master: branch + PR, and Florent merges. Use
  merge commits (not squash) — commit SHAs get referenced in issue/PR
  replies, and goreleaser builds release changelogs from commits.
- Commit style: lowercase summaries, `ci:`/`docs:`/`chore(deps):`
  prefixes for non-feature changes.

## Sibling library

`go-kamailio-binrpc` lives at `~/code/go-kamailio-binrpc` (same
conventions). It is a zero-dependency library: keep its `go` directive
as low as possible (it is a floor forced on all importers). The BINRPC
wire format is documented by its tests; watch for int size limits —
values are variable-length big-endian, doubles are fixed-point ×1000.
