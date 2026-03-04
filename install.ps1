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
}

if (-not (Test-Admin)) {
    throw "Please run this installer from an elevated PowerShell session (Run as Administrator)."
}

$repo = "ugzv/ublockdnsclient"
$binary = "ublockdns"

$arch = switch ($env:PROCESSOR_ARCHITECTURE.ToLowerInvariant()) {
    "amd64" { "amd64" }
    "x86"   { "amd64" }
    "arm64" { "arm64" }
    default  { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

if (-not $Version) {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest"
    $Version = $release.tag_name
}

if (-not $Version) {
    throw "Could not determine release version."
}

$asset = "$binary-windows-$arch.exe"
$releaseTagApi = "https://api.github.com/repos/$repo/releases/tags/$Version"
$release = Invoke-RestMethod -Uri $releaseTagApi
$assetInfo = $release.assets | Where-Object { $_.name -eq $asset } | Select-Object -First 1
if (-not $assetInfo) {
    $available = ($release.assets | ForEach-Object { $_.name }) -join ", "
    throw "Release $Version does not contain asset '$asset'. Available assets: $available"
}
$url = $assetInfo.browser_download_url

$installDir = Join-Path $env:ProgramFiles "uBlockDNS"
$exePath = Join-Path $installDir "$binary.exe"
$tempExe = Join-Path $env:TEMP "$binary.$([Guid]::NewGuid().ToString('N')).exe"

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
            Write-Warning "Previous uninstall returned exit code $LASTEXITCODE. Continuing with best-effort cleanup."
        }
    } catch {
        Write-Warning "Previous uninstall failed: $($_.Exception.Message). Continuing with best-effort cleanup."
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

Write-Host "Downloading $url ..."
$downloaded = $false
for ($attempt = 1; $attempt -le 3; $attempt++) {
    Write-Host "Download attempt $attempt/3 ..."
    try {
        Invoke-WebRequest -Uri $url -OutFile $tempExe
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

$serviceReady = $false
$lastStatusText = ""
$lastServiceState = "unknown"
$maxChecks = 45
for ($check = 1; $check -le $maxChecks; $check++) {
    $statusLines = & $exePath status 2>&1
    $statusExitCode = $LASTEXITCODE
    $lastStatusText = (($statusLines | ForEach-Object { "$_" }) -join "`n").Trim()
    if ($lastStatusText -match '(?im)^Service:\s*(.+)$') {
        $lastServiceState = $Matches[1].Trim()
    }

    if ($statusExitCode -eq 0 -and $lastStatusText -match '(?im)^Status:\s*active\b') {
        $serviceReady = $true
        break
    }

    if ($check -eq 1 -or ($check % 5 -eq 0)) {
        Write-Host "Waiting for uBlockDNS to report active status ($check/$maxChecks) ..."
    }
    Start-Sleep -Seconds 1
}

if (-not $serviceReady) {
    $statusSummary = if ($lastStatusText) { $lastStatusText } else { "(no status output)" }
    throw "uBlockDNS did not report active status (service: $lastServiceState). Last status output: $statusSummary"
}

Write-Host "Done."
Write-Host "  Next:      Protection is active. Run a status check."
Write-Host ("  Status:    & `"{0}`" status" -f $exePath)
Write-Host ("  Uninstall: & `"{0}`" uninstall" -f $exePath)
