# Clean room and legal

## The clean-room boundary

This Go implementation was built using clean-room software engineering practices. The upstream Python tool served only as behavioral reference during initial research. No code, README text, templates, screenshots, or other assets were copied from the original repository.

## Why clean-room matters

The legacy project appears to lack a clear license grant. To ensure this rewrite can be published safely:

- All implementation decisions were made independently
- All prose in this wiki and related articles is original
- No mechanical translation of Python logic into Go
- The original repository remains read-only reference material

## What we did study

- Behavioral observations: what inputs the tool accepts, what outputs it produces
- Performance characteristics for benchmark comparison purposes
- Format and profile support as a baseline for declaring our own surface

## What we did not copy

- Source code
- Documentation or tutorial text
- Configuration schemas or templates
- Report HTML or styling
- Test fixtures or example data

## License

This Go implementation and all original content are released under:

**Apache License, Version 2.0**

See the LICENSE file at the repository root for the full text.

## Publication safety

- No private logs or credentials appear in committed evidence
- All real-world corpora are anonymized before entering publishable bundles
- The release-candidate tree excludes mission infrastructure and temporary artifacts
