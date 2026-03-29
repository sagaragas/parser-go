# Evidence index

The current public benchmark set is `evidence/benchmark-homelab-20260328/`. Its top-level index is `evidence/benchmark-homelab-20260328/index.json`.

## Published scenarios

### `synthetic-small`

- Kind: synthetic
- Representation: representative
- Corpus hash: `cfb8103d89c4bb1cb69732e643177357e3bae1faf0a9cd304c2fb4966e52540d`
- Rewrite revision: `dc01cf104ef86c2d3a755b84bcae1203e1a4b15d`
- Key files:
  - `evidence/benchmark-homelab-20260328/synthetic-small/manifest.json`
  - `evidence/benchmark-homelab-20260328/synthetic-small/parity/parity.json`
  - `evidence/benchmark-homelab-20260328/synthetic-small/parity/aggregate-summary.json`
  - `evidence/benchmark-homelab-20260328/synthetic-small/rewrite/normalized-summary.json`

### `homelab-jellyfin-illustrative`

- Kind: homelab
- Representation: illustrative
- Corpus hash: `27d75747c984391a52fba754c8f9bde1cc83cb6626d6832236fbd5378c0a9a87`
- Rewrite revision: `dc01cf104ef86c2d3a755b84bcae1203e1a4b15d`
- Capture window: `2026-03-27T22:35:03-07:00/2026-03-27T22:41:23-07:00`
- Key files:
  - `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/manifest.json`
  - `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/parity/parity.json`
  - `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/parity/aggregate-summary.json`
  - `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/redaction/report.json`
  - `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/service-integration/cross-check.json`

## How to trace a claim

1. Start at `evidence/benchmark-homelab-20260328/index.json` for the scenario id, corpus hash, representation, and measured rewrite revision.
2. Open that scenario's `manifest.json` for the declared corpus, normalization rules, host snapshot, and fairness controls.
3. Check `parity/parity.json` and `parity/aggregate-summary.json` before using any timing number.
4. For the homelab-backed scenario, read the matching `redaction/` files and `service-integration/cross-check.json` so the benchmark, API, and report surfaces stay tied to the same run.
