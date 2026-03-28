# Benchmark methodology

## Goals

- Prove correctness before claiming performance wins
- Compare Go rewrite against legacy Python baseline on identical inputs
- Produce reproducible, publishable evidence bundles

## Scenario classes

We run benchmarks across several corpus types:

- Synthetic small corpus - sanity check for basic correctness
- Synthetic medium corpus - representative single-file workload
- Synthetic large corpus - stress test for memory and throughput
- Multi-file corpus - rotated log handling
- Anonymized homelab corpus - real-world validation

## Fairness rules

Both implementations run under identical conditions:

- Same input bytes, same declared settings
- Fresh temp workspace per timed run to avoid stale artifacts
- Dependency installation and one-time build costs excluded from timing
- Warmup policy, cache state, iteration count, and thread settings recorded symmetrically

## Parity gates

Before any performance comparison:

- Canonical summaries must match field-for-field
- Workload accounting (bytes, lines, matched, filtered, output rows) must agree
- Any mismatch blocks the speed claim until investigated

## Iteration and metrics

Each scenario runs multiple iterations. We capture:

- Wall time per iteration
- CPU time per iteration
- Maximum resident set size (RSS)

Aggregates report median, mean, and spread where applicable.

## Publishable bundles

A complete benchmark bundle contains:

- Scenario manifest with corpus, revisions, settings, environment
- Canonical normalized summaries for baseline and rewrite
- Workload accounting artifacts
- Raw per-iteration metrics
- Aggregate summaries
- Sanitized environment snapshot
- Anonymization report when homelab data is involved
- Claim-to-evidence index for traceability

Bundles exclude raw logs, temp files, caches, and machine-local secrets.

## Limitations and honesty

We disclose:

- Measured revisions and commit identifiers
- Corpus source and sanitization steps
- Host hardware, OS, and runtime versions
- Sample counts and statistical spread
- Whether homelab scenarios are illustrative or representative

This lets readers judge what the benchmark actually proves.
