# Architecture

## Runtime shape

The shipped product is still intentionally small. `cmd/parsergo/main.go` starts one HTTP server, wires `internal/api/analysis_handler.go` and `internal/api/report_handler.go`, and keeps job state in the in-process store from `internal/job/job.go`. There is no external database, broker, or remote asset dependency in the runtime path.

## Main code paths

### Service entrypoint

`cmd/parsergo/main.go` owns process startup, readiness, and shutdown. It reads `PARSERGO_ADDR`, starts the HTTP listener, and registers the analysis and report routes on a single `http.ServeMux`.

### Analysis engine

`internal/analysis/engine.go` is the parser and aggregation core. It accepts the currently declared `format=combined` surface, streams the input, and returns the canonical totals and ranked request list that later surfaces reuse.

### Job lifecycle and storage

`internal/job/job.go` tracks queued, running, succeeded, failed, and expired analyses. The API handlers update that store directly, which keeps the contract simple and makes the job state easy to cross-check in tests.

### Report surface

`internal/api/report_handler.go` serves `/reports` and `/reports/{id}` from the stored analysis results. The report surface is a presentation layer only. It renders the same totals and ranking order the canonical summary already computed.

### Benchmark harness

`cmd/bench/main.go` and `internal/bench/` run the legacy Python baseline and the Go rewrite against the same declared scenario files in `benchmark/scenarios/`. The committed evidence bundle under `evidence/benchmark-homelab-20260328/` comes from that harness.

## Data flow

1. A client submits a log corpus to `POST /v1/analyses`.
2. `internal/api/analysis_handler.go` validates the request and creates a server-owned job id.
3. `internal/analysis/engine.go` parses the corpus and computes the canonical summary.
4. The API stores the finished summary, exposes it at `/v1/analyses/{id}/summary`, and makes the matching report visible at `/reports/{id}`.
5. `cmd/bench/main.go` can run the same corpus through the benchmark harness so the summary, parity artifacts, and report-visible values stay traceable to one scenario id.

## Design rules that stay fixed

- Client filenames and paths do not choose where files land on disk.
- The canonical summary is the source of truth for API responses, reports, and benchmark parity.
- Reports stay self-contained. No CDN scripts, remote fonts, or late third-party fetches.
- Performance claims only count when the parity artifacts under `evidence/benchmark-homelab-20260328/*/parity/` say the rewrite and baseline did the same work.
