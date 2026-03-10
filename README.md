# uBlockDNS Client

uBlockDNS Client brings DNS-level ad and tracker blocking to your entire device. It uses community-maintained filter lists such as EasyList and EasyPrivacy, applied at the DNS layer so apps, browsers, and background services are covered without per-browser extensions.

**Website:** [ublockdns.com](https://ublockdns.com)

uBlockDNS Client is intended for desktop and server environments where you want one install point, system-wide protection, and remote configuration through the uBlockDNS dashboard.

## Highlights

- System-wide DNS filtering for browsers, apps, and background traffic
- Encrypted upstream DNS-over-HTTPS connection to the uBlockDNS service
- Real-time filter and custom rule updates from your dashboard
- Local service model with status checks and service management commands
- Cross-platform support for macOS, Linux, Windows, and FreeBSD

## Install

Create a free account at [ublockdns.com](https://ublockdns.com), then follow the setup guide in your dashboard. The guide covers all supported platforms with copy-paste commands and step-by-step instructions.

Quick install for macOS and Linux:

```sh
curl -sSf https://ublockdns.com/install.sh | sh -s -- <profile-id>
```

During installation, you may be prompted for your Mac or Linux administrator password to update system DNS settings.

Verify installer script checksum (Linux):

```sh
curl -sSfLO https://ublockdns.com/install.sh
curl -sSfLO https://github.com/ugzv/ublockdnsclient/releases/latest/download/SCRIPT_SHA256SUMS
grep " install.sh$" SCRIPT_SHA256SUMS | sha256sum -c -
sh install.sh <profile-id>
```

Windows 10 or later (PowerShell as Administrator):

```powershell
iwr https://ublockdns.com/install.ps1 -OutFile install.ps1; powershell -ExecutionPolicy Bypass -File .\install.ps1 -ProfileId <profile-id>
```

Verify installer script checksum (PowerShell):

```powershell
iwr https://ublockdns.com/install.ps1 -OutFile install.ps1
iwr https://github.com/ugzv/ublockdnsclient/releases/latest/download/SCRIPT_SHA256SUMS -OutFile SCRIPT_SHA256SUMS
$expected = (Select-String -Path .\SCRIPT_SHA256SUMS -Pattern " install.ps1$").Line.Split()[0].ToLower()
$actual = (Get-FileHash .\install.ps1 -Algorithm SHA256).Hash.ToLower()
if ($actual -ne $expected) { throw "install.ps1 checksum mismatch" }
powershell -ExecutionPolicy Bypass -File .\install.ps1 -ProfileId <profile-id>
```

A Windows GUI installer (.exe) is also available on the [releases page](https://github.com/ugzv/ublockdnsclient/releases).
Windows 7, Windows 8, and Windows 8.1 are not supported by the published binaries.

### Other platforms

The dashboard setup guide also covers Chrome, Firefox, iOS, Android, and routers. These use DNS-over-HTTPS directly and don't require this client.

### Supported architectures

linux/amd64, linux/arm64, linux/armv7, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64, freebsd/amd64

## Usage

```
ublockdns install   -profile <profile-id>   Install as system service
ublockdns uninstall                          Remove service and restore DNS
ublockdns start                              Start the service
ublockdns stop                               Stop the service
ublockdns status                             Show service state and DNS config
ublockdns status -json                       Show machine-readable status
ublockdns wait-ready -timeout 45s            Wait until service and DNS are active
ublockdns version                            Print version
```

Manage your filter lists, custom rules, and query log from the [dashboard](https://ublockdns.com). The CLI is intentionally narrow: install the local service, verify it is healthy, and let the dashboard handle policy changes.

## How it works

The client runs a local DNS proxy on `127.0.0.1:53` and forwards all queries to the uBlockDNS service over encrypted DNS-over-HTTPS. The service evaluates each query against the filter lists and custom rules enabled for your profile, then returns either the normal DNS answer or a block response.

When you update filter lists or custom rules in the dashboard, the client receives those changes in real time and flushes the local DNS cache automatically so new decisions take effect quickly.

## Build from source

Requires Go 1.23 or later.

```sh
go build -o ublockdns .
sudo ./ublockdns install -profile <profile-id>
```

## Feedback and issues

Found a bug or have a suggestion? [Open an issue](https://github.com/ugzv/ublockdnsclient/issues/new/choose).

For blocking problems (ads getting through or a site wrongly blocked), check which filter lists you have enabled in your [dashboard](https://ublockdns.com) first. uBlockDNS uses community-maintained lists and does not control their contents.

## Security

Report security vulnerabilities privately through [GitHub Security Advisories](https://github.com/ugzv/ublockdnsclient/security/advisories/new). Do not open public issues for security problems.

## License

MIT
