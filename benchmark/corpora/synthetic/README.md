# Synthetic corpora

Synthetic test fixtures used by the benchmark harness.

## Structure

- `small/` - Sanity check corpus (5 lines: 3 matched, 1 filtered, 1 malformed)

## Properties

Fixtures are built with controlled properties:
- Deterministic content for reproducibility
- Known line counts and byte sizes
- Controlled format variation (valid, filtered, malformed)
- Deterministic ranking tie scenarios

## Status

The `synthetic-small` scenario is operational and used by the benchmark harness. See `benchmark/scenarios/synthetic-small.json` for the scenario definition.
