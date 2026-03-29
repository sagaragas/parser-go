# parser-go

A web server log parser and analysis service written in Go. Parses Combined Log Format access logs, computes traffic summaries, and serves results through an HTTP API with a built-in report viewer.

## Quick start

```sh
go build ./...
go test ./...
```

### Run the service

```sh
go run ./cmd/parsergo serve
```

The server listens on `127.0.0.1:3120` by default. Set `PARSERGO_ADDR` to change the bind address.

### Submit a log file for analysis

```sh
curl -X POST http://127.0.0.1:3120/v1/analyses \
  -F "format=combined" \
  -F "profile=default" \
  -F "dataset=@/path/to/access.log"
```

Poll the returned location until the job completes, then retrieve the summary at `/v1/analyses/{id}/summary` or view the report at `/reports/{id}`.

## API

| Endpoint | Method | Description |
| --- | --- | --- |
| `/healthz` | GET | Liveness check |
| `/readyz` | GET | Readiness check |
| `/v1/analyses` | POST | Submit a log file for analysis |
| `/v1/analyses/{id}` | GET | Poll job status |
| `/v1/analyses/{id}/summary` | GET | Retrieve analysis results |
| `/v1/analyses/{id}/report` | GET | HTML report (redirects to `/reports/{id}`) |
| `/reports` | GET | List all completed reports |
| `/reports/{id}` | GET | View a report in the browser |

## Project layout

```
cmd/parsergo/       Service entrypoint
cmd/bench/          Benchmark harness CLI
internal/analysis/  Log parser and aggregation engine
internal/api/       HTTP handlers
internal/job/       Job lifecycle and storage
internal/server/    HTTP server setup
internal/summary/   Canonical summary types
internal/bench/     Benchmark harness internals
benchmark/          Scenario definitions and test corpora
```

## Benchmarks

The benchmark harness compares this Go implementation against a Python baseline on the same input. To run a benchmark:

```sh
BENCH_BASELINE_PYTHON=/path/to/python \
BENCH_LEGACY_REPO=/path/to/web-log-parser \
go run ./cmd/bench run --scenario synthetic-small
```

See [`benchmark/README.md`](./benchmark/README.md) for details.

## License

[Apache License 2.0](./LICENSE)
