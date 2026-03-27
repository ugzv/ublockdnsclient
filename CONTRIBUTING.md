# Contributing

## Development setup

```sh
go test ./...
go build ./...
```

## Pull request checklist

- Keep changes scoped and documented.
- Run `go test ./...` locally.
- Keep installers aligned with release artifact naming.
- Do not commit local binaries or `dist/` artifacts.
- Manually review security-sensitive code paths before opening the PR.

## Versioning

This project uses SemVer tags (`vX.Y.Z`).

- `main` can contain unreleased changes.
- Releases are created from tags.
