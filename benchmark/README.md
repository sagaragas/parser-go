# Benchmark directory

This directory contains benchmark harness code, scenario definitions, and temporary run outputs.

## Contents

- `scenarios/` - Scenario manifest definitions (JSON/YAML)
- `corpora/synthetic/` - Synthetic test fixtures (small, medium, large)
- `results/` - Benchmark run outputs (created at runtime, gitignored)

## Safety

- Real homelab logs are never stored here
- Only sanitized or synthetic corpora may be committed
- `results/` is gitignored to prevent accidental commit of temp artifacts
