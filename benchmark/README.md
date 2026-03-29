# Benchmarks

## Go-native benchmarks

The primary benchmarks use Go's `testing.B` framework and run on real data from the [NASA Kennedy Space Center HTTP access logs](https://ita.ee.lbl.gov/html/contrib/NASA-HTTP.html) (July 1995).

A 10,000-line sample is committed to the repo at `corpora/nasa/nasa_10k.log`. To run:

```sh
go test -bench=. -benchmem ./internal/analysis/
```

For extended benchmarks on the full 1.89M-line dataset (196 MB), download it:

```sh
curl -o /tmp/NASA_access_log_Jul95.gz ftp://ita.ee.lbl.gov/traces/NASA_access_log_Jul95.gz
gunzip /tmp/NASA_access_log_Jul95.gz
mv /tmp/NASA_access_log_Jul95 /tmp/nasa_jul95
go test -bench=BenchmarkParse_NASAFull -benchmem ./internal/analysis/
```

## Cross-language parity harness

A separate harness in `cmd/bench` compares Go against a Python baseline on the same input, enforcing output parity before allowing performance comparison. You need:

- Go on your `PATH` (or set `BENCH_GO_BINARY`)
- A Python interpreter for the baseline (`BENCH_BASELINE_PYTHON`)
- A checkout of the Python baseline repo (`BENCH_LEGACY_REPO`)

```sh
BENCH_BASELINE_PYTHON=/path/to/python \
BENCH_LEGACY_REPO=/path/to/web-log-parser \
go run ./cmd/bench run --scenario synthetic-small
```

## Contents

- `corpora/nasa/` -- NASA KSC access log sample (real data)
- `corpora/synthetic/` -- Small synthetic fixtures
- `scenarios/` -- Cross-language scenario definitions (JSON)
- `results/` -- Runtime output (gitignored)
