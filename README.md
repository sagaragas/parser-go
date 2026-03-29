# parser-go

[![CI](https://github.com/sagaragas/parser-go/actions/workflows/ci.yml/badge.svg)](https://github.com/sagaragas/parser-go/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](./LICENSE)

A web server log parser and analysis service written in Go. Parses Combined Log Format access logs, computes traffic summaries, and serves results through an HTTP API with a built-in report viewer.

## Install

```sh
go install github.com/sagaragas/parser-go/cmd/parsergo@latest
```

Or build from source:

```sh
git clone https://github.com/sagaragas/parser-go.git
cd parser-go
go build -o parsergo ./cmd/parsergo
```

## Usage

### Start the service

```sh
parsergo serve
```

### Submit a log file

```sh
curl -X POST http://127.0.0.1:3120/v1/analyses \
  -F "format=combined" \
  -F "profile=default" \
  -F "file=@/path/to/access.log"
```

The response includes a job ID and polling URL. Once the job completes, view results at:

- **JSON summary:** `GET /v1/analyses/{id}/summary`
- **HTML report:** `http://127.0.0.1:3120/reports/{id}`

### Docker

```sh
docker build -t parsergo .
docker run -p 3120:3120 parsergo
```

The container defaults to binding `0.0.0.0:3120` so it is reachable from the host via the port mapping. To override, set `PARSERGO_ADDR`.

## Configuration

All configuration is through environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `PARSERGO_ADDR` | `127.0.0.1:3120` | Bind address |
| `PARSERGO_MAX_UPLOAD_BYTES` | `10485760` (10 MB) | Maximum upload size |
| `PARSERGO_QUEUE_LIMIT` | `2` | Maximum concurrent analysis jobs |
| `PARSERGO_RETENTION` | `24h` | How long completed jobs are kept |

## API

| Endpoint | Method | Description |
| --- | --- | --- |
| `/healthz` | GET | Liveness check |
| `/readyz` | GET | Readiness check (503 during startup) |
| `/v1/analyses` | POST | Submit a log file for analysis (202) |
| `/v1/analyses/{id}` | GET | Poll job status |
| `/v1/analyses/{id}/summary` | GET | JSON analysis results |
| `/v1/analyses/{id}/report` | GET | HTML report |
| `/reports` | GET | List completed reports |
| `/reports/{id}` | GET | View report in browser |

### Error responses

| Status | Meaning |
| --- | --- |
| 400 | Invalid request (bad format, missing fields) |
| 413 | Upload too large |
| 415 | Unsupported media type |
| 422 | Unprocessable input (unsupported format/profile) |
| 429 | Queue full, retry after `Retry-After` seconds |
| 503 | Service not ready (during startup) |

## Project layout

```
cmd/parsergo/       Service entrypoint
cmd/bench/          Benchmark harness CLI
internal/analysis/  Log parser and aggregation engine
internal/api/       HTTP handlers
internal/job/       Job lifecycle and storage
internal/server/    HTTP server setup
internal/summary/   Summary types
internal/bench/     Benchmark internals
benchmark/          Scenario definitions and test corpora
```

## Development

```sh
go test ./...
go vet ./...
```

## Benchmarks

Go-native benchmarks run on the [NASA Kennedy Space Center access logs](https://ita.ee.lbl.gov/html/contrib/NASA-HTTP.html) (1.89M lines, 196 MB). A 10K-line sample is included in the repo:

```sh
go test -bench=. -benchmem ./internal/analysis/
```

On the full NASA dataset (~52 MB/s, ~480K lines/sec). See [`benchmark/README.md`](./benchmark/README.md) for details.

A cross-language parity harness is also available for comparing against the Python baseline:

```sh
BENCH_BASELINE_PYTHON=/path/to/python \
BENCH_LEGACY_REPO=/path/to/web-log-parser \
go run ./cmd/bench run --scenario synthetic-small
```

## License

[Apache License 2.0](./LICENSE)
