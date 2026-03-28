# Corpus and sanitization

## Data sources

### Synthetic corpora

Generated log files with known properties: fixed line counts, controlled format variation, and deterministic content. These serve as ground truth for correctness testing and baseline performance measurement.

### Homelab sources

Real-world validation uses anonymized Caddy access logs from internal infrastructure. The primary source is the Caddy reverse proxy handling production-like traffic.

## Sanitization requirements

Before any real log data enters a publishable bundle:

- Client IP addresses removed or pseudonymized
- Cookies and authorization headers stripped
- Query-string secrets redacted
- Referrer URLs sanitized
- User-agent tokens that could identify internal systems removed
- Internal-only hostnames and paths generalized

## Redaction reporting

Each sanitized corpus carries a redaction report documenting:

- What fields were removed or transformed
- Hash of the original corpus (for internal traceability)
- Hash of the sanitized corpus (for evidence bundle reference)
- Any edge cases or manual review steps

## What we never commit

- Raw unsanitized logs
- Production credentials or tokens
- Internal network details or hostnames
- Session identifiers or cookie values

## Corpus provenance

Synthetic fixtures are checked into the repository. Sanitized homelab corpora are tracked by hash in benchmark manifests. The original unsanitized sources remain in their source systems and are never copied to development or publication repositories.
