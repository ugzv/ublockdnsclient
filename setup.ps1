param(
    [Parameter(Mandatory = $false)]
    [string]$ProfileId,

    [Parameter(Mandatory = $false)]
    [string]$AccountToken,

    [Parameter(Mandatory = $false)]
    [string]$Version,

    [Parameter(Mandatory = $false)]
    [switch]$KeepOpen,

    [Parameter(Mandatory = $false)]
    [switch]$NoPause
)

$ErrorActionPreference = "Stop"
$logPath = Join-Path $env:TEMP "ublockdns-setup.log"
$setupOk = $false
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
    Write-Host "Requesting administrator privileges ..."
    $args = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", ('"{0}"' -f $PSCommandPath)
    )
    if ($KeepOpen) { $args += "-KeepOpen" }
    if ($NoPause) { $args += "-NoPause" }
    if ($ProfileId) { $args += @("-ProfileId", ('"{0}"' -f $ProfileId)) }
    if ($AccountToken) { $args += @("-AccountToken", ('"{0}"' -f $AccountToken)) }
    if ($Version) { $args += @("-Version", ('"{0}"' -f $Version)) }

    $startParams = @{
        FilePath     = "powershell"
        Verb         = "RunAs"
        ArgumentList = ($args -join " ")
    }
    if ($NoPause) {
        $startParams["Wait"] = $true
        $startParams["PassThru"] = $true
    }

    try {
        $proc = Start-Process @startParams
        if ($NoPause -and $proc) {
            exit $proc.ExitCode
        }
    } catch {
        throw "Failed to start elevated setup: $($_.Exception.Message)"
    }
    exit 0
}

try {
    Start-Transcript -Path $logPath -Force | Out-Null
} catch {}

try {
    Write-Host "uBlockDNS Setup (Windows)"
    Write-Host "---------------------------"
    if ($Version) {
        Write-Host "Requested version: $Version"
    }

    if (-not $ProfileId) {
        $ProfileId = Read-Host "Enter your uBlockDNS profile ID"
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
        $installerUrls = @()
        if ($Version) {
            $installerUrls += "https://github.com/ugzv/ublockdnsclient/releases/download/$Version/install.ps1"
        }
        $installerUrls += "https://github.com/ugzv/ublockdnsclient/releases/latest/download/install.ps1"

        $downloadedInstaller = $false
        foreach ($installerUrl in $installerUrls) {
            try {
                Write-Host "Downloading install.ps1 from $installerUrl ..."
                Invoke-WebRequest -Uri $installerUrl -OutFile $installerPath
                $downloadedInstaller = $true
                break
            } catch {
                Write-Warning "Failed to download install.ps1 from $installerUrl"
            }
        }
        if (-not $downloadedInstaller) {
            throw "Could not download install.ps1."
        }
    }

    $installArgs = @("-ProfileId", $ProfileId)
    if ($AccountToken) { $installArgs += @("-AccountToken", $AccountToken) }
    if ($Version) { $installArgs += @("-Version", $Version) }

    Write-Host "Running installer ..."
    & powershell -NoProfile -ExecutionPolicy Bypass -File $installerPath @installArgs
    if ($LASTEXITCODE -ne 0) {
        throw "Installer failed with exit code $LASTEXITCODE."
    }

    Write-Host ""
    Write-Host "Setup complete."
    Write-Host "Run this anytime to check status:"
    Write-Host "  & `"$env:ProgramFiles\uBlockDNS\ublockdns.exe`" status"

    $statusExe = Join-Path $env:ProgramFiles "uBlockDNS\ublockdns.exe"
    if (Test-Path $statusExe) {
        Write-Host ""
        Write-Host "Current status:"
        & $statusExe status
    } else {
        Write-Warning "Binary not found at $statusExe"
    }
    $setupOk = $true
} catch {
    Write-Host ""
    Write-Error "Setup failed: $($_.Exception.Message)"
    Write-Host "See log: $logPath"
} finally {
    try { Stop-Transcript | Out-Null } catch {}
    $exitCode = 0
    if (-not $setupOk) {
        $exitCode = 1
    }
    if ($KeepOpen -or -not $NoPause) {
        Write-Host ""
        [void](Read-Host "Press Enter to close")
    }
    exit $exitCode
}
