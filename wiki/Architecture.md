# Architecture

## System overview

The parser is a single-purpose log analysis system centered on a long-running Go service. It accepts log files, analyzes them asynchronously, and produces both machine-readable summaries and self-contained HTML reports.

## Core components

### Service API

The HTTP control plane handles liveness and readiness checks, accepts analysis submissions, exposes job status polling, and serves completed results. It validates requests, enforces limits, and returns structured errors for client mistakes.

### Analysis engine

The core value path ingests log data, parses a deliberately narrow set of formats, normalizes records into a canonical internal model, and computes metrics and ranked outputs. Unsupported inputs fail explicitly. The engine streams data to bound memory use and produces deterministic results.

### Job lifecycle

Analysis runs asynchronously. The job layer tracks work through queued, running, succeeded, failed, and expired states. It enforces queue limits, handles duplicate submissions safely, and manages retention. Workspaces are server-controlled: client filenames cannot steer filesystem layout.

### Report generation

Completed analyses produce self-contained HTML reports and a browsable index. Reports display the same metrics and rankings as the canonical summary, using only local assets with no third-party network dependence.

### Benchmark harness

A separate subsystem runs the Go rewrite and legacy Python baseline against identical corpora. It normalizes outputs, enforces parity gates, records iteration-level metrics, and emits publishable result bundles.

## Data flow

1. Client submits analysis request via HTTP
2. Service validates and creates a job with opaque server-controlled identity
3. Engine parses input, normalizes records, computes aggregates
4. System writes canonical summary and self-contained report
5. Service exposes job state, summary JSON, and report HTML
6. Failed jobs surface sanitized errors without stack traces or path leaks
7. Expired resources return explicit responses rather than silent disappearance

## Key invariants

- Safety over convenience: client-controlled inputs cannot escape workspace boundaries
- Determinism: identical input produces identical output and stable ranking order
- Self-containment: reports render offline with no external network requests
- Parity before speed: benchmark claims require workload accounting and output correctness
