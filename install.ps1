param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$ProfileId,

    [Parameter(Mandatory = $false, Position = 1)]
    [string]$AccountToken,

    [Parameter(Mandatory = $false)]
    [string]$Version
)

$ErrorActionPreference = "Stop"
$commonPath = Join-Path $PSScriptRoot "scripts/windows/common.ps1"
if (Test-Path $commonPath) {
    . $commonPath
} else {
    function Test-Admin {
        $currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
        $principal = New-Object Security.Principal.WindowsPrincipal($currentIdentity)
        return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    }

    function Assert-SupportedWindowsVersion {
        $versionInfo = [System.Environment]::OSVersion.Version
        if ($versionInfo.Major -lt 10) {
            throw "Windows 10 or later is required. Current version detected: $($versionInfo.ToString()). The published uBlockDNS binaries are built with a Go toolchain that no longer supports Windows 7/8/8.1."
        }
    }

    function Enable-Tls12 {
        try {
            $protocolType = [System.Net.SecurityProtocolType]
            if ([Enum]::GetNames($protocolType) -contains "Tls12") {
                [System.Net.ServicePointManager]::SecurityProtocol = `
                    [System.Net.ServicePointManager]::SecurityProtocol -bor $protocolType::Tls12
            }
        } catch {}
    }

    function Invoke-DownloadFile {
        param(
            [string]$Uri,
            [string]$OutFile
        )

        Enable-Tls12

        $client = New-Object System.Net.WebClient
        try {
            $client.Headers.Add("User-Agent", "uBlockDNS-Installer")
            $client.DownloadFile($Uri, $OutFile)
        } finally {
            $client.Dispose()
        }
    }

    function Get-Sha256Hex {
        param([string]$Path)

        $stream = [System.IO.File]::OpenRead($Path)
        try {
            $sha256 = [System.Security.Cryptography.SHA256]::Create()
            try {
                $hashBytes = $sha256.ComputeHash($stream)
            } finally {
                $sha256.Dispose()
            }
        } finally {
            $stream.Dispose()
        }

        return ([System.BitConverter]::ToString($hashBytes).Replace("-", "").ToLowerInvariant())
    }

    function Resolve-GitHubLatestTag {
        param([string]$Repo)

        Enable-Tls12

        $request = [System.Net.HttpWebRequest]::Create("https://github.com/$Repo/releases/latest")
        $request.Method = "HEAD"
        $request.AllowAutoRedirect = $true
        $request.UserAgent = "uBlockDNS-Installer"

        $response = $request.GetResponse()
        try {
            $segments = $response.ResponseUri.AbsolutePath.TrimEnd('/').Split('/')
            if ($segments.Length -eq 0) {
                throw "GitHub latest release redirect did not include a tag."
            }
            return $segments[$segments.Length - 1]
        } finally {
            $response.Close()
        }
    }
}

if (-not (Test-Admin)) {
    throw "Please run this installer from an elevated PowerShell session (Run as Administrator)."
}

Assert-SupportedWindowsVersion

$repo = "ugzv/ublockdnsclient"
$binary = "ublockdns"

$arch = switch ($env:PROCESSOR_ARCHITECTURE.ToLowerInvariant()) {
    "amd64" { "amd64" }
    "x86"   { "amd64" }
    "arm64" { "arm64" }
    default  { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

if (-not $Version) {
    $Version = Resolve-GitHubLatestTag -Repo $repo
}

if (-not $Version) {
    throw "Could not determine release version."
}

$asset = "$binary-windows-$arch.exe"
$url = "https://github.com/$repo/releases/download/$Version/$asset"
$sumsUrl = "https://github.com/$repo/releases/download/$Version/SHA256SUMS"

$installDir = Join-Path $env:ProgramFiles "uBlockDNS"
$exePath = Join-Path $installDir "$binary.exe"
$tempExe = Join-Path $env:TEMP "$binary.$([Guid]::NewGuid().ToString('N')).exe"
$tempSums = Join-Path $env:TEMP "SHA256SUMS.$([Guid]::NewGuid().ToString('N'))"

function Stop-ExistingInstall {
    param(
        [string]$ExePath,
        [string]$BinaryName
    )

    if (-not (Test-Path $ExePath)) {
        return
    }

    Write-Host "Existing installation detected, stopping previous service ..."
    try {
        & $ExePath uninstall
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "Previous uninstall returned exit code $LASTEXITCODE. Trying stop fallback."
            & $ExePath stop
            if ($LASTEXITCODE -ne 0) {
                Write-Warning "Previous stop fallback returned exit code $LASTEXITCODE. Continuing with best-effort cleanup."
            }
        }
    } catch {
        Write-Warning "Previous uninstall failed: $($_.Exception.Message). Trying stop fallback."
        try {
            & $ExePath stop
            if ($LASTEXITCODE -ne 0) {
                Write-Warning "Previous stop fallback returned exit code $LASTEXITCODE. Continuing with best-effort cleanup."
            }
        } catch {
            Write-Warning "Previous stop fallback failed: $($_.Exception.Message). Continuing with best-effort cleanup."
        }
    }

    $deadline = (Get-Date).AddSeconds(20)
    do {
        $proc = Get-Process -Name $BinaryName -ErrorAction SilentlyContinue
        if ($null -eq $proc) {
            break
        }
        try { $proc | Stop-Process -Force -ErrorAction SilentlyContinue } catch {}
        Start-Sleep -Milliseconds 500
    } while ((Get-Date) -lt $deadline)
}

Write-Host "Installing uBlockDNS"
Write-Host "  Version: $Version"
Write-Host "  Arch:    $arch"
Write-Host "  Asset:   $asset"
if ($AccountToken) {
    Write-Host "  Account token: provided (instant rules updates enabled)"
}

New-Item -ItemType Directory -Path $installDir -Force | Out-Null

Enable-Tls12

Write-Host "Downloading $url ..."
$downloaded = $false
for ($attempt = 1; $attempt -le 3; $attempt++) {
    Write-Host "Download attempt $attempt/3 ..."
    try {
        Invoke-DownloadFile -Uri $url -OutFile $tempExe
        $downloaded = $true
        break
    } catch {
        if ($attempt -eq 3) {
            throw
        }
        Write-Host "Download attempt $attempt failed, retrying ..."
        Start-Sleep -Seconds 2
    }
}
if (-not $downloaded) {
    throw "Download failed for $url"
}

Write-Host "Downloading checksum manifest ..."
Invoke-DownloadFile -Uri $sumsUrl -OutFile $tempSums

$expectedHash = $null
foreach ($line in Get-Content -Path $tempSums) {
    if ($line -match '^([0-9a-fA-F]{64})\s+\*?(.+)$') {
        if ($Matches[2] -eq $asset) {
            $expectedHash = $Matches[1].ToLowerInvariant()
            break
        }
    }
}
if (-not $expectedHash) {
    throw "Could not find checksum for '$asset' in SHA256SUMS."
}

$actualHash = Get-Sha256Hex -Path $tempExe
if ($actualHash -ne $expectedHash) {
    throw "SHA-256 verification failed for '$asset'. Expected $expectedHash, got $actualHash."
}
Write-Host "Checksum verified for $asset."
Remove-Item -Path $tempSums -Force -ErrorAction SilentlyContinue

Stop-ExistingInstall -ExePath $exePath -BinaryName $binary

$replaced = $false
for ($attempt = 1; $attempt -le 5; $attempt++) {
    if ($attempt -gt 1) {
        Write-Host "Retrying binary replacement ($attempt/5) ..."
    }
    try {
        Copy-Item -Path $tempExe -Destination $exePath -Force
        Remove-Item -Path $tempExe -Force -ErrorAction SilentlyContinue
        $replaced = $true
        break
    } catch {
        if ($attempt -eq 5) {
            throw "Could not replace '$exePath' after $attempt attempts. Ensure ublockdns.exe is not running. Last error: $($_.Exception.Message)"
        }
        Start-Sleep -Seconds 1
    }
}
if (-not $replaced) {
    throw "Failed to install binary at '$exePath'."
}

$installArgs = @("install", "-profile", $ProfileId)
if ($AccountToken) {
    $installArgs += @("-token", $AccountToken)
}

Write-Host "Configuring service ..."
& $exePath @installArgs
if ($LASTEXITCODE -ne 0) {
    throw "Service installation failed with exit code $LASTEXITCODE."
}

Write-Host "Waiting for uBlockDNS to become ready ..."
$readyOutput = & $exePath wait-ready -timeout 45s -json 2>&1
$readyExitCode = $LASTEXITCODE
if ($readyExitCode -ne 0) {
    $statusOutput = & $exePath status -json 2>&1
    $statusSummary = (($statusOutput | ForEach-Object { "$_" }) -join "`n").Trim()
    if (-not $statusSummary) {
        $statusSummary = "(no status output)"
    }
    throw "uBlockDNS did not report ready status. Last machine status: $statusSummary"
}

Write-Host "Done."
Write-Host "  Next:      Protection is active. Run a status check."
Write-Host ("  Status:    & `"{0}`" status" -f $exePath)
Write-Host ("  Uninstall: & `"{0}`" uninstall" -f $exePath)
