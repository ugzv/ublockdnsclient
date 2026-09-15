param(
    [Parameter(Mandatory = $false)]
    [string]$ProfileId,

    [Parameter(Mandatory = $false)]
    [string]$AccountToken,

    [Parameter(Mandatory = $false)]
    [string]$AccountTokenFile,

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
    # BEGIN GENERATED HELPERS
    function Test-Admin {
        $currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
        $principal = New-Object Security.Principal.WindowsPrincipal($currentIdentity)
        return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    }

    function Assert-SupportedWindowsVersion {
        $version = [System.Environment]::OSVersion.Version
        if ($version.Major -lt 10) {
            throw "Windows 10 or later is required. Current version detected: $($version.ToString()). The published uBlockDNS binaries are built with a Go toolchain that no longer supports Windows 7/8/8.1."
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
            [Parameter(Mandatory = $true)]
            [string]$Uri,

            [Parameter(Mandatory = $true)]
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

    function New-AccountTokenFile {
        param([Parameter(Mandatory = $true)][string]$Token)

        $directory = Join-Path ([IO.Path]::GetTempPath()) ("ublockdns-token-" + [Guid]::NewGuid().ToString('N'))
        try {
            New-Item -ItemType Directory -Path $directory | Out-Null
            $acl = New-Object Security.AccessControl.DirectorySecurity
            $acl.SetAccessRuleProtection($true, $false)
            $identities = @(
                [Security.Principal.WindowsIdentity]::GetCurrent().User,
                [Security.Principal.SecurityIdentifier]::new('S-1-5-32-544'),
                [Security.Principal.SecurityIdentifier]::new('S-1-5-18')
            )
            foreach ($identity in $identities) {
                $rule = [Security.AccessControl.FileSystemAccessRule]::new(
                    $identity, 'FullControl', 'ContainerInherit, ObjectInherit', 'None', 'Allow')
                $acl.AddAccessRule($rule)
            }
            Set-Acl -LiteralPath $directory -AclObject $acl
            $path = Join-Path $directory "token"
            [IO.File]::WriteAllText($path, $Token)
            return $path
        } catch {
            Remove-Item -LiteralPath $directory -Recurse -Force -ErrorAction SilentlyContinue
            throw
        }
    }
    # END GENERATED HELPERS
}

if ($AccountTokenFile) {
    if ($AccountToken) {
        throw "Specify AccountToken or AccountTokenFile, not both."
    }
    $AccountToken = [IO.File]::ReadAllText((Resolve-Path -LiteralPath $AccountTokenFile).Path).Trim()
}

if (-not (Test-Admin)) {
    Write-Host "Requesting administrator privileges ..."
    $elevationArgs = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", ('"{0}"' -f $PSCommandPath)
    )
    if ($KeepOpen) { $elevationArgs += "-KeepOpen" }
    if ($NoPause) { $elevationArgs += "-NoPause" }
    if ($ProfileId) { $elevationArgs += @("-ProfileId", ('"{0}"' -f $ProfileId)) }
    if ($Version) { $elevationArgs += @("-Version", ('"{0}"' -f $Version)) }

    $startParams = @{
        FilePath     = "powershell"
        Verb         = "RunAs"
        ArgumentList = ($elevationArgs -join " ")
        Wait         = $true
        PassThru     = $true
    }
    $tokenPath = $null
    try {
        if ($AccountToken) {
            $tokenPath = New-AccountTokenFile -Token $AccountToken
            $elevationArgs += @("-AccountTokenFile", ('"{0}"' -f $tokenPath))
            $startParams["ArgumentList"] = $elevationArgs -join " "
        }
        $proc = Start-Process @startParams
        exit $proc.ExitCode
    } catch {
        throw "Failed to start elevated setup: $($_.Exception.Message)"
    } finally {
        if ($tokenPath) {
            Remove-Item -LiteralPath (Split-Path -Parent $tokenPath) -Recurse -Force
        }
    }
}

$transcriptStarted = $false
try {
    # A transcript header includes the host command line, including legacy token arguments.
    if (-not $PSBoundParameters.ContainsKey("AccountToken")) {
        Start-Transcript -Path $logPath -Force | Out-Null
        $transcriptStarted = $true
    }
} catch {}

try {
    Assert-SupportedWindowsVersion
    Enable-Tls12

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
        $tokenPrompt = Read-Host "Enter account token for instant rule updates (optional, press Enter to skip)" -AsSecureString
        $AccountToken = [Net.NetworkCredential]::new("", $tokenPrompt).Password
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
                Invoke-DownloadFile -Uri $installerUrl -OutFile $installerPath
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

    $installArgs = @{ ProfileId = $ProfileId }
    if ($AccountToken) { $installArgs["AccountToken"] = $AccountToken }
    if ($Version) { $installArgs["Version"] = $Version }

    Write-Host "Running installer ..."
    & $installerPath @installArgs

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
    if ($transcriptStarted) { Write-Host "See log: $logPath" }
} finally {
    if ($transcriptStarted) {
        try { Stop-Transcript | Out-Null } catch {}
    }
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
