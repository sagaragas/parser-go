# Web Log Parser (Go)

A clean-room Go rewrite of a legacy Python log analysis tool, built for speed, safety, and long-running service use.

## What this is

This repository contains a from-scratch Go implementation that analyzes web server access logs. The project was built using clean-room techniques: the legacy tool was studied for behavioral reference only, with no code, prose, or assets copied.

## Quick links

- [Clean room and legal](./Clean-Room-and-Legal.md) - How we kept this rewrite ethical and safe to publish
- [Architecture](./Architecture.md) - How the system is organized
- [Benchmark methodology](./Benchmark-Methodology.md) - How we measure correctness and performance
- [Corpus and sanitization](./Corpus-and-Sanitization.md) - Where our test data comes from and how we keep it safe to share
- [Evidence index](./Evidence-Index.md) - Links to measured results and benchmark bundles

## Project status

This is an active rewrite project. The wiki will fill in as implementation milestones complete and evidence becomes available.

## License

The Go implementation and all original content in this repository are released under the Apache License 2.0.
