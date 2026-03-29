# Contributing

Thanks for taking the time to help with `parser-go`.

This project started in mission mode under a clean-room process. Contributions should stay on that side of the line: write original code and docs for `github.com/sagaragas/parser-go`, and do not paste code, README text, templates, screenshots, or other assets from the legacy Python project that informed the rewrite.

## Before you start

- Check for an existing issue or pull request first.
- Use the security reporting process in `SECURITY.md` for vulnerabilities.
- Keep benchmark or evidence changes tied to committed artifacts instead of hand-written claims.

## Local validation

Run the normal Go checks before you open a pull request:

```sh
go test ./...
go vet ./...
go build ./...
```

If your change touches benchmark scenarios, evidence, or public-facing docs, re-run the relevant command paths and update the affected files in the same change.

## Pull request expectations

Please keep pull requests focused and include:

- a short summary of the problem and the fix
- tests or a clear explanation for why tests were not needed
- notes about any docs, evidence, or benchmark artifacts that changed
- redacted inputs only; do not attach raw homelab logs, secrets, or private infrastructure details

If you change behavior, update the tests in the same branch. If you change user-facing docs or benchmark claims, make sure the wording still matches the committed evidence.
