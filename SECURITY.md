# Security and Transparency

## Non-affiliation

uBlockDNS Client is an independent project. It is not affiliated with, endorsed by, or maintained by the uBlock Origin project or its maintainer.

## Reporting a vulnerability

Please report security issues privately through [GitHub Security Advisories](https://github.com/ugzv/ublockdnsclient/security/advisories/new). Do not open public issues for vulnerabilities that could put users at risk.

## Current review status

As of March 27, 2026, this project has **not** completed an independent third-party security audit.

That means users should evaluate it as they would any other network-path software: read the source, verify release artifacts, and decide whether the trust model fits their environment.

## Release expectations

The current baseline for changes that affect DNS handling, local service behavior, installers, credentials, or network security is:

- maintainers review the change manually before release
- the Go test suite passes in CI
- release artifacts are published with checksums

These steps improve confidence, but they are not a substitute for a dedicated security assessment.
