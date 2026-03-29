# parser-go

parsergo is a clean-room Go rewrite of a legacy Python web log parser. The shipped surface is a long-running HTTP service plus a benchmark harness and a publishable evidence set.

Public module path: `github.com/sagaragas/parser-go`

## Start here

- [Wiki home](./wiki/Home.md)
- [Clean room and legal](./wiki/Clean-Room-and-Legal.md)
- [Architecture](./wiki/Architecture.md)
- [Benchmark methodology](./wiki/Benchmark-Methodology.md)
- [Evidence index](./wiki/Evidence-Index.md)
- [Apache-2.0 license](./LICENSE)

## Quick start

```sh
go test ./...
go build ./...
```

## Project provenance

`parser-go` started as a mission-mode clean-room rewrite of a legacy Python log parser that did not ship a clear license grant. The legacy repository was used as read-only research input; this repo, its docs, and its benchmark artifacts were written independently for publication at `github.com/sagaragas/parser-go`.

## Community

- [Contributing guide](./CONTRIBUTING.md)
- [Security policy](./SECURITY.md)
- [Code of conduct](./CODE_OF_CONDUCT.md)

## Benchmark prerequisites and overrides

The benchmark harness compares this repo with the legacy Python baseline. A fresh public clone needs three explicit inputs before the baseline side can run:

- Go 1.26 or newer on your `PATH`, or `BENCH_GO_BINARY` / `--go-binary`
- a compatible Python interpreter for the legacy baseline via `BENCH_BASELINE_PYTHON` / `--baseline-python`
- a separate checkout of the legacy repository via `BENCH_LEGACY_REPO` / `--legacy-repo`

Example:

```sh
BENCH_BASELINE_PYTHON=/path/to/legacy-venv/bin/python \
BENCH_LEGACY_REPO=/path/to/web-log-parser \
go run ./cmd/bench run --scenario synthetic-small
```

If `go` is not on your `PATH`, add `BENCH_GO_BINARY=/path/to/go` or pass `--go-binary /path/to/go`.

## Current measured bundle

The committed benchmark bundle is `evidence/benchmark-homelab-20260328/`. Its index records two parity-passing scenarios, `synthetic-small` and `homelab-jellyfin-illustrative`, and pins both to rewrite revision `dc01cf104ef86c2d3a755b84bcae1203e1a4b15d`.

If you want the exact manifests, parity diffs, or aggregate summaries, start with [`evidence/benchmark-homelab-20260328/index.json`](./evidence/benchmark-homelab-20260328/index.json) and follow the bundle paths listed there.

## Public release candidate

This repo now includes a generator for a public-safe release-candidate tree and archive.

```sh
go run ./cmd/releasecandidate
```

From the internal mission checkout, the standalone public launch root is `dist/release-candidate/tree/parser-go/`. The generated tree and archive exclude `.factory/`, `HOMELAB_LOG_SOURCES.md`, `benchmark/results/`, local toolchains, and other mission-only or temporary paths.
