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
   - Unix: `curl -sSf https://raw.githubusercontent.com/ugzv/ublockdnsclient/main/install.sh | sh -s -- <profile-id>`
   - Windows (Admin PowerShell):
     - `iwr https://raw.githubusercontent.com/ugzv/ublockdnsclient/main/install.ps1 -OutFile install.ps1`
     - `powershell -ExecutionPolicy Bypass -File .\\install.ps1 -ProfileId <profile-id> -Version <tag>`
6. Validate runtime:
   - `ublockdns version`
   - `ublockdns status`
   - `ublockdns uninstall`

## Notes

- Internal package layout:
  - `internal/core`: shared helpers, validation, config constants, DNS probe/cache utilities
  - `internal/runtime`: proxy startup, upstream endpoint handling, rules stream/watcher logic
  - `internal/service`: install/uninstall, service control, status/readiness, privilege checks
  - `internal/state`: persisted token and install-state storage
- Windows token path: `%ProgramData%\\ublockdns`.
- Unix token path: `/etc/ublockdns`.
- Release builds are managed by GoReleaser (`.goreleaser.yml`).
