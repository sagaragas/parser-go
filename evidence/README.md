# Evidence directory

This directory contains publishable benchmark evidence bundles.

## Contents

Each evidence bundle is a self-contained directory with:

- `manifest.json` - Scenario definitions and environment metadata
- `baseline/` - Baseline (legacy Python) normalized summaries and metrics
- `rewrite/` - Go rewrite normalized summaries and metrics
- `parity/` - Parity diff results and workload accounting comparisons
- `environment/` - Sanitized environment snapshot with coarse host classes instead of exact hardware fingerprints
- `redaction/` - Anonymization reports (for homelab scenarios)

## Safety

- Only intentionally curated, sanitized content is committed here
- No raw logs, temp files, or machine-local secrets
- Each bundle is validated for publication safety before commit

## Index

Bundles will be indexed in the wiki [Evidence Index](../wiki/Evidence-Index.md) as they are produced.
