param(
    [Parameter(Mandatory = $false)]
    [string]$ProfileId,

    [Parameter(Mandatory = $false)]
    [string]$AccountToken,

    [Parameter(Mandatory = $false)]
    [string]$Version,

    [Parameter(Mandatory = $false)]
    [switch]$KeepOpen
)

$ErrorActionPreference = "Stop"

function Test-Admin {
    $currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($currentIdentity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

if (-not (Test-Admin)) {
    Write-Host "Requesting administrator privileges ..."
    $args = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", ('"{0}"' -f $PSCommandPath),
        "-KeepOpen"
    )
    if ($ProfileId) { $args += @("-ProfileId", ('"{0}"' -f $ProfileId)) }
    if ($AccountToken) { $args += @("-AccountToken", ('"{0}"' -f $AccountToken)) }
    if ($Version) { $args += @("-Version", ('"{0}"' -f $Version)) }

    Start-Process -FilePath "powershell" -Verb RunAs -ArgumentList ($args -join " ") | Out-Null
    exit 0
}

Write-Host "uBlock DNS Setup (Windows)"
Write-Host "---------------------------"

if (-not $ProfileId) {
    $ProfileId = Read-Host "Enter your uBlock DNS profile ID"
}
if (-not $ProfileId) {
    throw "Profile ID is required."
}

if (-not $AccountToken) {
    $tokenPrompt = Read-Host "Enter account token for instant rule updates (optional, press Enter to skip)"
    if ($tokenPrompt) {
        $AccountToken = $tokenPrompt
    }
}

$repoRoot = Split-Path -Parent $PSCommandPath
$installerPath = Join-Path $repoRoot "install.ps1"

if (-not (Test-Path $installerPath)) {
    Write-Host "Downloading install.ps1 from GitHub ..."
    $installerUrl = "https://raw.githubusercontent.com/ugzv/ublockdnsclient/main/install.ps1"
    Invoke-WebRequest -Uri $installerUrl -OutFile $installerPath
}

$installArgs = @("-ProfileId", $ProfileId)
if ($AccountToken) { $installArgs += @("-AccountToken", $AccountToken) }
if ($Version) { $installArgs += @("-Version", $Version) }

Write-Host "Running installer ..."
& powershell -ExecutionPolicy Bypass -File $installerPath @installArgs
if ($LASTEXITCODE -ne 0) {
    throw "Installer failed with exit code $LASTEXITCODE."
}

Write-Host ""
Write-Host "Setup complete."
Write-Host "Run this anytime to check status:"
Write-Host "  $env:ProgramFiles\uBlockDNS\ublockdns.exe status"

$statusExe = Join-Path $env:ProgramFiles "uBlockDNS\ublockdns.exe"
if (Test-Path $statusExe) {
    Write-Host ""
    Write-Host "Current status:"
    & $statusExe status
}

if ($KeepOpen) {
    Write-Host ""
    [void](Read-Host "Press Enter to close")
}
