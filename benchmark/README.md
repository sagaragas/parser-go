# Benchmark directory

This directory holds the benchmark harness, committed scenario files, sanitized corpora, and gitignored runtime output.

## Contents

- `scenarios/` - Scenario manifest definitions (JSON/YAML)
- `corpora/synthetic/` - Synthetic test fixtures (small, medium, large)
- `results/` - Benchmark run outputs (created at runtime, gitignored)

## Running a committed scenario from a fresh clone

The public repo does not assume a sibling checkout or a mission-local toolchain. To rerun the baseline-vs-rewrite scenarios, point the harness at your own tools:

- `BENCH_GO_BINARY` or `--go-binary` if `go` is not already on your `PATH`
- `BENCH_BASELINE_PYTHON` or `--baseline-python` for the legacy baseline's compatibility environment
- `BENCH_LEGACY_REPO` or `--legacy-repo` for a separate checkout of the legacy Python repo

Example:

```sh
BENCH_BASELINE_PYTHON=/path/to/legacy-venv/bin/python \
BENCH_LEGACY_REPO=/path/to/web-log-parser \
go run ./cmd/bench run --scenario synthetic-small
```

Runtime output lands in `results/` and stays untracked.

## Safety

- Real homelab logs are never stored here
- Only sanitized or synthetic corpora may be committed
- `results/` is gitignored to prevent accidental commit of temp artifacts
