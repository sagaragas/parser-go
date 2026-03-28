# Synthetic corpora

Placeholder for synthetic test fixtures.

## Structure

- `small/` - Sanity check corpus (thousands of lines)
- `medium/` - Representative single-file workload (hundreds of thousands of lines)
- `large/` - Stress test corpus (millions of lines)

## Generation

Synthetic fixtures will be generated with controlled properties:
- Deterministic seed for reproducibility
- Known line counts and byte sizes
- Controlled format variation
- Deterministic ranking tie scenarios

## Status

Awaiting fixture generation implementation.
