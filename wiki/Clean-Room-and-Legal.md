# Clean room and legal

## Plain statement

The legacy Python repository was used as read-only research input. This repo was built in mission mode under a clean-room OSS process, and it does not copy upstream code, README text, report templates, screenshots, or other assets. The Go implementation, the wiki prose, and the committed benchmark evidence were written independently for publication.

## Why the boundary matters

The upstream project does not provide a clear license grant. That makes a line-by-line or asset-for-asset reuse story a bad idea. The safer path was to study behavior, restate requirements in original words, and build a new implementation that can be published on its own terms.

## What counted as allowed input

- Behavioral observations from the legacy tool
- Original code and tests in this repo
- Benchmark artifacts produced by this repo's own harness
- Sanitized corpora and redaction reports committed under `benchmark/` and `evidence/`

## What stayed out

- Upstream source code
- Upstream documentation prose
- Upstream templates, screenshots, or report assets
- Raw homelab logs
- Mission-only infrastructure such as `.factory/` in the generated public release candidate

## Licensing

This repository now ships the full Apache License 2.0 text in the root `LICENSE` file. That applies to the original Go implementation and the original documentation written in this repo.

## Publication safety

The public-facing corpus story is backed by committed redaction records, not by trust alone:

- `benchmark/corpora/homelab/jellyfin-illustrative/redaction-report.json` records the source label, bounded capture window, and the projection into sanitized combined-log lines.
- `evidence/benchmark-homelab-20260328/homelab-jellyfin-illustrative/redaction/report.json` and `redaction/scan.json` show the publishable bundle copy and the empty forbidden-match scan.
- `go run ./cmd/releasecandidate` generates a public release-candidate tree and archive that exclude `.factory/`, `HOMELAB_LOG_SOURCES.md`, local toolchains, and benchmark runtime output.
