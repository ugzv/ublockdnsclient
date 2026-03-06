# uBlockDNS Client

DNS-level ad and tracker blocking for your entire device. Uses community-maintained filter lists (EasyList, EasyPrivacy, and more), applied at the DNS layer so every app and browser is covered without per-device extensions.

**Website:** [ublockdns.com](https://ublockdns.com)

## Install

Create a free account at [ublockdns.com](https://ublockdns.com), then follow the setup guide in your dashboard. The guide covers all supported platforms with copy-paste commands and step-by-step instructions.

Quick install for macOS and Linux:

```sh
curl -sSf https://ublockdns.com/install.sh | sh -s -- <profile-id>
```

Verify installer script checksum (Linux):

```sh
curl -sSfLO https://ublockdns.com/install.sh
curl -sSfLO https://github.com/ugzv/ublockdnsclient/releases/latest/download/SCRIPT_SHA256SUMS
grep " install.sh$" SCRIPT_SHA256SUMS | sha256sum -c -
sh install.sh <profile-id>
```

Windows (PowerShell as Administrator):

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

Manage your filter lists, custom rules, and query log from the [dashboard](https://ublockdns.com).

## How it works

The client runs a local DNS proxy on `127.0.0.1:53` and forwards all queries to the uBlockDNS server over encrypted DNS-over-HTTPS. The server checks each query against your enabled filter lists and returns a block response for matched domains.

When you change filter lists or custom rules in the dashboard, the client receives the update in real time and flushes your local DNS cache automatically.

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
