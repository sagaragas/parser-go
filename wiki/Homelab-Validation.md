# Homelab validation

The current committed homelab-backed proof is the scenario `homelab-jellyfin-illustrative`. It is a bounded, sanitized retry window that was used in three places at once: the benchmark bundle, the API submission flow, and the browser-visible report surface.

## What the committed cross-check proves

`evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/service-integration/cross-check.json` ties one sanitized corpus hash to:

- benchmark summary values
- API job id `job_1774742902_48035203`
- report URL `/reports/job_1774742902_48035203`
- visible report metrics and ranked request rows

That file records `matches: true`, `requests_total: 18`, `matched_lines: 18`, and the same ranked order in both the benchmark summary and the service summary.

## Visible ranking in the committed run

The same cross-check file records two ranked request rows:

1. `GET /videos/session-a/live.m3u8` with count `12`
2. `GET /videos/session-b/live.m3u8` with count `6`

That matters because it shows the benchmark path and the live service path agree on ordering, not just on totals.

## Sanitization boundary

The publishable corpus and the publishable evidence copy both point back to the same anonymized source story:

- `benchmark/corpora/homelab/jellyfin-illustrative/redaction-report.json`
- `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/redaction/report.json`
- `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/redaction/scan.json`

Those files record the bounded capture window, pseudonymized client addresses, rewritten path tokens, and an empty forbidden-match scan.

## Limits

This is still an illustrative fallback slice, not the final representative ingress dataset. The public result set proves the end-to-end path on sanitized homelab traffic. It does not claim that one short media-service retry window captures every production pattern the parser will ever see.
