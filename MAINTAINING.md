# Maintaining This Repo

## Release Checklist

1. Ensure CI passes on `main`.
2. Bump version by creating a tag (`vX.Y.Z`) on the release commit.
3. Push tag to GitHub to trigger `.github/workflows/release.yml`.
4. Verify release assets include:
   - `ublockdns-linux-amd64`
   - `ublockdns-linux-arm64`
   - `ublockdns-linux-armv7`
   - `ublockdns-darwin-amd64`
   - `ublockdns-darwin-arm64`
   - `ublockdns-windows-amd64.exe`
   - `ublockdns-windows-arm64.exe`
   - `uBlockDNS-Setup-vX.Y.Z-windows-amd64.exe`
   - `ublockdns-freebsd-amd64`
   - `SHA256SUMS`
   - `SCRIPT_SHA256SUMS`
   - `install.sh`
   - `install.ps1`
   - `setup.ps1`
5. Smoke test installers:
   - Use the tagged release assets for the exact version being validated, not raw `main`.
   - Unix: `curl -sSfL https://github.com/ugzv/ublockdnsclient/releases/download/vX.Y.Z/install.sh | sh -s -- <profile-id>`
   - Hosted Unix (served by backend): `curl -sSfL https://ublockdns.com/install.sh | sh -s -- <profile-id>`
   - Windows 10+ (Admin PowerShell, dashboard flow): `irm https://ublockdns.com/install?id=<profile-id> | iex`
   - Windows 10+ (manual script download): use `/install-script`, not `/install.ps1` — Cloudflare can redirect `.ps1` URLs to an HTML page.
     - `iwr https://ublockdns.com/install-script -OutFile install.ps1`
     - `powershell -ExecutionPolicy Bypass -File .\\install.ps1 -ProfileId <profile-id> -Version vX.Y.Z`
6. Validate runtime:
   - `ublockdns version`
   - `ublockdns status`
   - `ublockdns status -json`
   - `ublockdns uninstall`
7. On Linux, validate DNS install/uninstall round-trip:
   - After install: `/etc/resolv.conf` contains `127.0.0.1`; optional drop-ins exist under `/etc/NetworkManager/conf.d/ublockdns.conf` or `/etc/systemd/resolved.conf.d/ublockdns.conf` depending on the host
   - After uninstall: `ublockdns status` shows service not installed; `/etc/resolv.conf` no longer contains `Managed by uBlockDNS`; `chattr +i` is not set on `/etc/resolv.conf`
   - Note: uninstall restores DNS before removing the service registration. If service removal fails, check warnings printed by the CLI and rerun `sudo ublockdns uninstall`.

## Notes

- Hosted install URLs (`https://ublockdns.com/...`) are served by the backend from embedded copies of `install.sh` and `install.ps1` in `ublockdns/internal/api/`. Sync those files from this repo when installer behavior changes.
- Cloudflare sits in front of `ublockdns.com`. Use `curl -sSfL` for Unix pipe installs so redirects are followed. On Windows, prefer the dashboard bootstrap (`/install?id=...`) or download from `/install-script` instead of `/install.ps1` directly.
- Internal package layout:
  - `internal/core`: shared helpers, validation, config constants, DNS probe/cache utilities; Linux durable DNS lives in `linux_dns_*.go`
  - `internal/runtime`: proxy startup, upstream endpoint handling, rules stream/watcher logic
  - `internal/service`: install/uninstall, service control, status/readiness, privilege checks
  - `internal/state`: persisted token and install-state storage
- Windows token path: `%ProgramData%\\ublockdns`.
- Unix token path: `/etc/ublockdns`.
- Release builds are managed by GoReleaser (`.goreleaser.yml`).
