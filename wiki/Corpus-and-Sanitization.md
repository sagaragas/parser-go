# Corpus and sanitization

## Synthetic corpus

The representative synthetic fixture in the current public bundle is `benchmark/corpora/synthetic/small/access.log`. Its committed manifest at `evidence/benchmark-homelab-20260328/synthetic-small/manifest.json` records corpus hash `cfb8103d89c4bb1cb69732e643177357e3bae1faf0a9cd304c2fb4966e52540d`, `335` input bytes, and the declared `format=combined` / `profile=default` settings.

## Homelab-derived corpus

The current public homelab-backed corpus is `benchmark/corpora/homelab/jellyfin-illustrative/access.log`. It is not a raw capture. It is a sanitized combined-log derivative whose hash is `27d75747c984391a52fba754c8f9bde1cc83cb6626d6832236fbd5378c0a9a87`, matching the manifest and index entries under `evidence/benchmark-homelab-20260328/`.

## What changed during sanitization

`benchmark/corpora/homelab/jellyfin-illustrative/redaction-report.json` records four concrete transformations:

- host prefix removal
- client IP pseudonymization into documentation ranges
- path-token pseudonymization to `session-a` and `session-b`
- projection into parser-compatible combined-log lines

The report also records `line_count_before = 18`, `line_count_after = 18`, and `structure_preserved = true`.

## What never enters git

- raw unsanitized log slices
- cookies or authorization headers
- query-string secrets
- internal-only hostnames or paths copied verbatim from source systems
- ad hoc temp output from benchmark runs

## Why the current homelab corpus is labeled illustrative

The scenario file `benchmark/scenarios/homelab-jellyfin-illustrative.json` and the public index both mark this corpus as illustrative. It proves the benchmark and service can agree on a real sanitized slice, but it is still a fallback media-service window rather than a broad ingress sample.
