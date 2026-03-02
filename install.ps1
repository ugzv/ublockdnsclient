param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$ProfileId,

    [Parameter(Mandatory = $false, Position = 1)]
    [string]$AccountToken,

    [Parameter(Mandatory = $false)]
    [string]$Version
)

$ErrorActionPreference = "Stop"

function Test-Admin {
    $currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($currentIdentity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
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

Write-Host "Installing uBlock DNS client"
Write-Host "  Version: $Version"
Write-Host "  Arch:    $arch"
Write-Host "  Asset:   $asset"

New-Item -ItemType Directory -Path $installDir -Force | Out-Null

Write-Host "Downloading $url ..."
$downloaded = $false
for ($attempt = 1; $attempt -le 3; $attempt++) {
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

Move-Item -Path $tempExe -Destination $exePath -Force

$installArgs = @("install", "-profile", $ProfileId)
if ($AccountToken) {
    $installArgs += @("-token", $AccountToken)
}

Write-Host "Configuring service ..."
& $exePath @installArgs
if ($LASTEXITCODE -ne 0) {
    throw "Service installation failed with exit code $LASTEXITCODE."
}

$serviceName = "ublockdns"
$serviceReady = $false
$deadline = (Get-Date).AddSeconds(45)
do {
    $svc = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($null -ne $svc) {
        if ($svc.Status -eq [System.ServiceProcess.ServiceControllerStatus]::Running) {
            $serviceReady = $true
            break
        }
        if ($svc.Status -eq [System.ServiceProcess.ServiceControllerStatus]::Stopped) {
            break
        }
    }
    Start-Sleep -Seconds 1
} while ((Get-Date) -lt $deadline)

if (-not $serviceReady) {
    $lastStatus = if ($null -eq $svc) { "not-installed" } else { $svc.Status.ToString() }
    throw "Service '$serviceName' did not reach Running state (last status: $lastStatus). Check Windows Event Log -> System for details."
}

Write-Host "Done."
Write-Host "  Status:    $exePath status"
Write-Host "  Uninstall: $exePath uninstall"
