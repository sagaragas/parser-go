# Benchmarks

Compares this Go implementation against a Python baseline on the same log input.

## Contents

- `scenarios/` -- Scenario definitions (JSON)
- `corpora/` -- Test log files
- `results/` -- Runtime output (gitignored)

## Running a benchmark

You need three things:

- Go on your `PATH` (or set `BENCH_GO_BINARY`)
- A Python interpreter for the baseline (`BENCH_BASELINE_PYTHON`)
- A checkout of the Python baseline repo (`BENCH_LEGACY_REPO`)

```sh
BENCH_BASELINE_PYTHON=/path/to/python \
BENCH_LEGACY_REPO=/path/to/web-log-parser \
go run ./cmd/bench run --scenario synthetic-small
```

Results are written to `results/` and are not tracked in git.
