# Benchmark methodology

The benchmark harness compares the legacy Python baseline and the Go rewrite on the same declared input, then refuses to make a performance claim unless parity passes first. The current committed evidence set is `evidence/benchmark-homelab-20260328/`.

## Committed scenarios

| scenario id | kind | representation | manifest | aggregate summary |
| --- | --- | --- | --- | --- |
| `synthetic-small` | synthetic | representative | `evidence/benchmark-homelab-20260328/synthetic-small/manifest.json` | `evidence/benchmark-homelab-20260328/synthetic-small/parity/aggregate-summary.json` |
| `homelab-jellyfin-illustrative` | homelab | illustrative | `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/manifest.json` | `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/parity/aggregate-summary.json` |

Both manifests pin the rewrite to `dc01cf104ef86c2d3a755b84bcae1203e1a4b15d` and the baseline to `904f838ddce5defc8715f2e444063520b7b0d612`.

## Fairness controls in the committed bundle

The two committed manifests use the same declared controls:

- `warmup_iterations = 1`
- `measured_iterations = 2`
- `cache_posture = "cold"`
- `concurrency = 1`
- `max_procs = 1`

The matching proof lives in each manifest's `fairness.control_evidence` block and in the copied `fairness.json` file beside it.

## What has to match before timing matters

For each scenario, the harness writes:

- a canonical summary for the baseline and the rewrite
- workload accounting for input bytes, total lines, matched lines, filtered lines, rejected lines, and output row count
- a parity record under `parity/parity.json`

If any tracked field drifts, the run is not claimable. The published index currently marks both committed scenarios as `parity_passed: true` in `evidence/benchmark-homelab-20260328/index.json`.

## What gets published

Each publishable bundle includes the manifest, fairness proof, normalized summaries, workload accounting, raw timing metrics, aggregate summary, and sanitized environment snapshot. The homelab bundle also carries `redaction/report.json`, `redaction/scan.json`, and the same-run service cross-check at `service-integration/cross-check.json`.

## Limits of the current result set

- The committed homelab-backed scenario is `homelab-jellyfin-illustrative`. It is useful because it proves the end-to-end path on real sanitized traffic, but it is still labeled illustrative rather than representative.
- The parser currently benchmarks `format=combined` only, so non-combined raw captures have to be sanitized and projected before they can enter a scenario.
- The current public wiki points readers to the measured artifacts. It does not treat the two-scenario bundle as the final word on every workload shape.
