# uBlock DNS Client

[![CI](https://github.com/ugzv/ublockdnsclient/actions/workflows/ci.yml/badge.svg)](https://github.com/ugzv/ublockdnsclient/actions/workflows/ci.yml)

Cross-platform CLI client for uBlock DNS.

## Install

Use the hosted installer:

```sh
curl -sSf https://ublockdns.com/install.sh | sh -s -- <profile-id>
```

Supports macOS and Linux (`amd64`, `arm64`).

Or build locally:

```sh
go build -o ublockdns .
sudo ./ublockdns install -profile <profile-id>
```

## Usage

```text
ublockdns install   -profile <id>
ublockdns uninstall
ublockdns start
ublockdns stop
ublockdns run       -profile <id>
ublockdns status
ublockdns version
```

Optional overrides:
- `-server <url>` on `install` / `run` for development DoH endpoints.
- `-api-server <url>` on `install` / `run` for development API endpoints.
- `-token <account-key>` on `install` / `run` to enable instant rules-update signal handling and automatic local DNS cache flush.
- `UBLOCKDNS_DOH_SERVER` environment variable for global override.
- `UBLOCKDNS_API_SERVER` environment variable for API override.
- `UBLOCKDNS_ACCOUNT_TOKEN` environment variable for runtime token (when not passed by flag).

## Instant Rule Updates

When a token is available, the client subscribes to backend rules-update events and flushes local DNS cache automatically after list or custom-rule changes.

- On `install -token <account-key>`, token is stored as root-only file (`/etc/ublockdns/<profile>.token`, mode `0600`) and loaded at runtime.
- Token is not printed in logs.
- Service arguments do not include token material.

## Development

Requirements:
- Go 1.23+

Commands:

```sh
go test ./...
go build ./...
```

Release builds (local):

```sh
./scripts/build-release.sh
```

## Releases

- Tag a commit as `vX.Y.Z`.
- GitHub Actions builds binaries for:
  - `linux/amd64`
  - `linux/arm64`
  - `darwin/amd64`
  - `darwin/arm64`
- Assets are uploaded as `ublockdns-<os>-<arch>` plus `SHA256SUMS`.

## Security

Report security issues privately. Do not open public issues for exploitable vulnerabilities.

## License

MIT
