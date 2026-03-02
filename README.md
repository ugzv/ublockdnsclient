# uBlock DNS Client

[![CI](https://github.com/ugzv/ublockdnsclient/actions/workflows/ci.yml/badge.svg)](https://github.com/ugzv/ublockdnsclient/actions/workflows/ci.yml)

Cross-platform CLI client for uBlock DNS.

## Install

Use the installer from GitHub (macOS/Linux):

```sh
curl -sSf https://raw.githubusercontent.com/ugzv/ublockdnsclient/main/install.sh | sh -s -- <profile-id>
```

Windows (PowerShell, Administrator):

```powershell
iwr https://raw.githubusercontent.com/ugzv/ublockdnsclient/main/install.ps1 -OutFile install.ps1
powershell -ExecutionPolicy Bypass -File .\install.ps1 -ProfileId <profile-id>
```

Prebuilt binaries currently target:
- `linux/amd64`
- `linux/arm64`
- `linux/arm/v7`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`
- `windows/arm64`
- `freebsd/amd64`

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

- On `install -token <account-key>`, token is stored in a restricted file and loaded at runtime:
  - Unix: `/etc/ublockdns/<profile>.token` (mode `0600`)
  - Windows: `%ProgramData%\\ublockdns\\<profile>.token`
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
- GitHub Actions uses GoReleaser to build and publish release assets.
- Assets are uploaded as `ublockdns-<os>-<arch>` (or `...-armv7`) plus `SHA256SUMS`, along with installer scripts (`install.sh`, `install.ps1`).

## Security

Report security issues privately. Do not open public issues for exploitable vulnerabilities.

## License

MIT
