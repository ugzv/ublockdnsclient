# uBlock DNS Client

[![CI](https://github.com/ugzv/ublockdnsclient/actions/workflows/ci.yml/badge.svg)](https://github.com/ugzv/ublockdnsclient/actions/workflows/ci.yml)

Cross-platform CLI client for uBlock DNS.

## Install

Use the hosted installer:

```sh
curl -sSf https://ublockdns.com/install.sh | sh -s -- <profile-id>
```

The installer currently supports Linux (`amd64`, `arm64`).

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
- `UBLOCKDNS_DOH_SERVER` environment variable for global override.

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
- Assets are uploaded as `ublockdns-<os>-<arch>` plus `SHA256SUMS`.

## Security

Report security issues privately. Do not open public issues for exploitable vulnerabilities.

## License

MIT
