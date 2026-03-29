# parsergo wiki

parsergo is a clean-room Go rewrite of a legacy Python web log parser. The current public evidence set lives in `evidence/benchmark-homelab-20260328/` and is pinned to rewrite revision `dc01cf104ef86c2d3a755b84bcae1203e1a4b15d`, as recorded in `evidence/benchmark-homelab-20260328/index.json`.

## Start here

- [Clean room and legal](./Clean-Room-and-Legal.md) explains the licensing posture, the clean-room boundary, and why the repo ships Apache-2.0.
- [Architecture](./Architecture.md) maps the runtime pieces to the actual source tree.
- [Benchmark methodology](./Benchmark-Methodology.md) describes the committed scenarios, fairness controls, parity gate, and limits of the current results.
- [Corpus and sanitization](./Corpus-and-Sanitization.md) explains which corpora are synthetic, which one is homelab-derived, and how the publishable corpus was redacted.
- [Homelab validation](./Homelab-Validation.md) covers the same-run benchmark-to-service cross-check for the committed homelab scenario.
- [Evidence index](./Evidence-Index.md) is the shortest path to manifests, parity diffs, aggregate summaries, and redaction reports.

## What is already measured

- `synthetic-small` is the representative synthetic fixture in the committed bundle. Its manifest, parity results, and aggregate summary live under `evidence/benchmark-homelab-20260328/synthetic-small/`.
- `homelab-jellyfin-illustrative` is the current sanitized homelab-backed scenario. It is explicitly marked illustrative in `evidence/benchmark-homelab-20260328/index.json`, and its same-run benchmark-to-service comparison lives at `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/service-integration/cross-check.json`.

## How to read this wiki

Start with the legal and methodology pages if you need the publication boundary first. Start with the architecture page if you want the code layout. If you only need the measured artifacts, go straight to the evidence index and follow the bundle paths from there.
