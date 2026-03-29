# parsergo

parsergo is a clean-room Go rewrite of a legacy Python web log parser. The shipped surface is a long-running HTTP service plus a benchmark harness and a publishable evidence set.

## Start here

- [Wiki home](./wiki/Home.md)
- [Clean room and legal](./wiki/Clean-Room-and-Legal.md)
- [Architecture](./wiki/Architecture.md)
- [Benchmark methodology](./wiki/Benchmark-Methodology.md)
- [Evidence index](./wiki/Evidence-Index.md)
- [Apache-2.0 license](./LICENSE)

## Current measured bundle

The committed benchmark bundle is `evidence/benchmark-homelab-20260328/`. Its index records two parity-passing scenarios, `synthetic-small` and `homelab-jellyfin-illustrative`, and pins both to rewrite revision `dc01cf104ef86c2d3a755b84bcae1203e1a4b15d`.

If you want the exact manifests, parity diffs, or aggregate summaries, start with [`evidence/benchmark-homelab-20260328/index.json`](./evidence/benchmark-homelab-20260328/index.json) and follow the bundle paths listed there.

## Public release candidate

This repo now includes a generator for a public-safe release-candidate tree and archive.

```sh
go run ./cmd/releasecandidate
```

By default it writes `dist/release-candidate/tree/parser-go/`, `dist/release-candidate/parser-go-release-candidate.tar.gz`, and `dist/release-candidate/manifest.json`. The generated output excludes `.factory/`, `HOMELAB_LOG_SOURCES.md`, `benchmark/results/`, local toolchains, and other mission-only or temp paths.
