# Homelab validation

This page documents how the Go implementation is validated against real-world traffic in the homelab environment.

## Validation approach

Homelab validation complements synthetic benchmarks by testing the implementation against production-like traffic patterns. The validation focuses on correctness, resource usage, and integration with existing infrastructure.

## Data source

The primary validation corpus comes from Caddy reverse proxy access logs. These logs represent actual web traffic patterns including:

- Mixed HTTP methods (GET, POST, PUT, DELETE)
- Range of response codes (2xx success, 3xx redirects, 4xx client errors, 5xx server errors)
- Varied request paths and query parameters
- Diverse user-agent strings
- Realistic traffic timing and volume patterns

## Anonymization pipeline

Before any homelab data enters the validation workflow, it passes through a sanitization pipeline:

1. Client IPs are pseudonymized to RFC 5737 documentation ranges
2. Cookies and authorization headers are removed entirely
3. Query-string parameters containing tokens or identifiers are redacted
4. Referrer URLs are truncated to origin only
5. User-agent strings are simplified to browser family and version
6. Internal hostnames are replaced with generic placeholders

## Validation scenarios

### Correctness validation

The Go implementation is run against anonymized homelab logs and its output compared against the expected structure. Key checks include:

- Total request counts match input line counts
- Response code distributions are plausible
- Top-requested paths are extracted and ranked
- Timing metrics fall within expected ranges
- Error handling for malformed lines is consistent

### Resource validation

Memory and CPU usage are monitored during homelab validation runs:

- Peak memory usage under sustained load
- CPU time per thousand log lines
- Garbage collection frequency and pause times
- File descriptor usage patterns

### Integration validation

The validation includes testing the service deployment model:

- Service startup and shutdown behavior
- Health and readiness endpoint responses
- Concurrent request handling
- Report generation and retrieval
- Job lifecycle management (queued, running, completed, expired)

## Validation environment

The homelab validation runs on actual infrastructure:

- Target host: ansible (172.16.1.9)
- Log source: Caddy access logs from caddy (172.16.1.21)
- Deployment: Containerized Go service
- Monitoring: Native service metrics and system resource tracking

## Expected outcomes

A successful homelab validation demonstrates:

- Correct parsing of real-world log formats
- Stable resource usage without memory leaks
- Consistent performance across varied traffic patterns
- Proper handling of edge cases found in production traffic
- Clean integration with homelab deployment patterns

## Status

Awaiting service completion and homelab deployment readiness. Validation results and evidence bundles will be linked from the evidence index as they become available.
