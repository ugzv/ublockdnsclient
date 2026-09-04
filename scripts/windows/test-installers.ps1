$ErrorActionPreference = "Stop"
$repoRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path (Join-Path $testRoot "scripts/windows") -Force | Out-Null
$previousTemp = $env:TEMP
$previousProgramFiles = $env:ProgramFiles
$previousToken = $env:UBLOCKDNS_ACCOUNT_TOKEN
$env:TEMP = $testRoot
$env:ProgramFiles = $testRoot
$global:installerTestsFailed = 0

function Assert-True($Condition, $Message) {
    if (-not $Condition) { throw $Message }
}

function Test-Case($Name, [scriptblock]$Test) {
    try {
        & $Test
        Write-Host "PASS: $Name"
    } catch {
        $global:installerTestsFailed++
        Write-Host "FAIL: $Name - $($_.Exception.Message)"
    }
}

function Start-Transcript { $global:testTranscriptStarted = $true }
function Stop-Transcript {}
function Read-Host {
    param($Prompt, [switch]$AsSecureString)
    Assert-True $AsSecureString "Token prompt must hide input"
    return ConvertTo-SecureString $global:testToken -AsPlainText -Force
}
function powershell {
    Assert-True (-not ($args -contains $global:testToken)) "Token exposed in child process arguments"
    throw "Unexpected child PowerShell process"
}
function Start-Process {
    param($FilePath, $Verb, $ArgumentList, [switch]$Wait, [switch]$PassThru)
    Assert-True (-not $ArgumentList.Contains($global:testToken)) "Token exposed in elevation arguments"
    Assert-True ($Wait -and $PassThru) "Elevation must wait to clean up its token file"
    Assert-True (Test-Path $global:testTokenPath) "Token file missing during elevation"
    Assert-True ([IO.File]::ReadAllText($global:testTokenPath) -eq $global:testToken) "Token changed during handoff"
    if ($global:testElevationFails) { throw "UAC cancelled" }
    return [pscustomobject]@{ ExitCode = 7 }
}

try {
    Copy-Item (Join-Path $repoRoot "setup.ps1") $testRoot
    @'
function Test-Admin { return $global:testIsAdmin }
function Assert-SupportedWindowsVersion {}
function Enable-Tls12 {}
function New-AccountTokenFile {
    param([string]$Token)
    $directory = New-Item -ItemType Directory -Path (Join-Path $env:TEMP ([Guid]::NewGuid().ToString('N')))
    $global:testTokenPath = Join-Path $directory.FullName "token"
    [IO.File]::WriteAllText($global:testTokenPath, $Token)
    return $global:testTokenPath
}
'@ | Set-Content (Join-Path $testRoot "scripts/windows/common.ps1")
    @'
param([string]$ProfileId, [string]$AccountToken, [string]$Version)
if ($ProfileId -ne 'profile' -or $AccountToken -ne $global:testToken -or $Version -ne 'v1.0') {
    throw "Installer parameters changed"
}
$global:testInstallerCalled = $true
'@ | Set-Content (Join-Path $testRoot "install.ps1")
    $global:testToken = 'secret-token-with spaces-and-$characters'

    Test-Case "Guided setup does not launch a child with the token in argv" {
        $global:testIsAdmin = $true
        $global:testInstallerCalled = $false
        $global:testTranscriptStarted = $false
        & (Join-Path $testRoot "setup.ps1") -ProfileId profile -AccountToken $global:testToken -Version v1.0 -NoPause
        Assert-True ($LASTEXITCODE -eq 0 -and $global:testInstallerCalled) "Installer did not complete"
        Assert-True (-not $global:testTranscriptStarted) "Transcript can record the raw token from the host command line"
    }

    Test-Case "Installer failure exits setup without reporting success" {
        $global:testIsAdmin = $true
        $global:testInstallerCalled = $false
        $installerPath = Join-Path $testRoot "install.ps1"
        $fixture = [IO.File]::ReadAllText($installerPath)
        try {
            [IO.File]::WriteAllText($installerPath, $fixture + "`nthrow 'Service installation failed'`n")
            $output = & (Join-Path $testRoot "setup.ps1") -ProfileId profile -AccountToken $global:testToken -Version v1.0 -NoPause 6>&1 | Out-String
            Assert-True $global:testInstallerCalled "Failing installer was not invoked"
            Assert-True ($LASTEXITCODE -eq 1) "Setup did not report installer failure"
            Assert-True ($output -notmatch 'Setup complete\.') "Setup reported success after installer failure"
        } finally {
            [IO.File]::WriteAllText($installerPath, $fixture)
        }
    }

    Test-Case "Guided setup reads a caller-owned token file" {
        $global:testIsAdmin = $true
        $global:testInstallerCalled = $false
        $path = Join-Path $testRoot "provided-token"
        [IO.File]::WriteAllText($path, $global:testToken)
        & (Join-Path $testRoot "setup.ps1") -ProfileId profile -AccountTokenFile $path -Version v1.0 -NoPause
        Assert-True ($LASTEXITCODE -eq 0 -and $global:testInstallerCalled) "Installer did not receive the token"
        Assert-True (Test-Path $path) "Caller-owned token file was removed"
    }

    Test-Case "Interactive token entry hides input and retains setup logging" {
        $global:testIsAdmin = $true
        $global:testInstallerCalled = $false
        $global:testTranscriptStarted = $false
        & (Join-Path $testRoot "setup.ps1") -ProfileId profile -Version v1.0 -NoPause
        Assert-True ($LASTEXITCODE -eq 0 -and $global:testInstallerCalled) "Installer did not receive prompted token"
        Assert-True $global:testTranscriptStarted "Interactive setup did not start logging"
    }

    Test-Case "Conflicting token parameters are rejected" {
        $rejected = $false
        try {
            & (Join-Path $testRoot "setup.ps1") -AccountToken $global:testToken -AccountTokenFile missing -NoPause
        } catch {
            $rejected = $_.Exception.Message.Contains('not both')
        }
        Assert-True $rejected "Ambiguous token input was accepted"
    }

    Test-Case "Elevation waits, preserves the exit code and removes the token file" {
        $global:testIsAdmin = $false
        $global:testElevationFails = $false
        $global:testTokenPath = $null
        & (Join-Path $testRoot "setup.ps1") -ProfileId profile -AccountToken $global:testToken -NoPause
        Assert-True ($LASTEXITCODE -eq 7) "Elevated exit code was lost"
        Assert-True ($global:testTokenPath -and -not (Test-Path $global:testTokenPath)) "Token file left behind"
    }

    Test-Case "Cancelled elevation removes the token file" {
        $global:testElevationFails = $true
        $global:testTokenPath = $null
        try {
            & (Join-Path $testRoot "setup.ps1") -ProfileId profile -AccountToken $global:testToken -NoPause
        } catch {
            Assert-True ($_.Exception.Message.Contains('UAC cancelled')) "Unexpected elevation error"
        }
        Assert-True ($global:testTokenPath -and -not (Test-Path $global:testTokenPath)) "Token file left behind"
    }

    Test-Case "Installer restores the caller's account-token environment" {
        $source = [IO.File]::ReadAllText((Join-Path $repoRoot "install.ps1"))
        $start = $source.IndexOf('$installArgs = @("install", "-profile", $ProfileId)')
        $end = $source.IndexOf('Write-Host "Waiting for uBlockDNS to become ready ..."')
        Assert-True ($start -ge 0 -and $end -gt $start) "Service invocation block missing"
        function Invoke-TestBinary {
            Assert-True ($env:UBLOCKDNS_ACCOUNT_TOKEN -eq $global:testToken) "Token missing from child environment"
            $global:LASTEXITCODE = 0
        }
        $exePath = 'Invoke-TestBinary'
        $ProfileId = 'profile'
        $AccountToken = $global:testToken
        $env:UBLOCKDNS_ACCOUNT_TOKEN = 'previous-token'
        & ([scriptblock]::Create($source.Substring($start, $end - $start)))
        Assert-True ($env:UBLOCKDNS_ACCOUNT_TOKEN -eq 'previous-token') "Caller environment was changed"

        function Invoke-TestBinary { throw "Binary failed" }
        try {
            & ([scriptblock]::Create($source.Substring($start, $end - $start)))
        } catch {
            Assert-True ($_.Exception.Message -eq 'Binary failed') "Unexpected installer error"
        }
        Assert-True ($env:UBLOCKDNS_ACCOUNT_TOKEN -eq 'previous-token') "Failure changed caller environment"
    }
    if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
        Test-Case "Token files grant access only to the user, administrators and SYSTEM" {
            . (Join-Path $PSScriptRoot "common.ps1")
            $path = New-AccountTokenFile -Token $global:testToken
            try {
                Assert-True ([IO.File]::ReadAllText($path) -eq $global:testToken) "Token changed on disk"
                $directoryAcl = Get-Acl -LiteralPath (Split-Path -Parent $path)
                Assert-True $directoryAcl.AreAccessRulesProtected "Token directory inherits permissions"
                $allowed = @([Security.Principal.WindowsIdentity]::GetCurrent().User.Value, 'S-1-5-32-544', 'S-1-5-18')
                $rules = (Get-Acl -LiteralPath $path).GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])
                foreach ($rule in $rules) {
                    Assert-True ($allowed -contains $rule.IdentityReference.Value) "Unexpected token-file principal"
                }
                foreach ($identity in $allowed) {
                    Assert-True ($rules.IdentityReference.Value -contains $identity) "Required principal cannot read token"
                }
            } finally {
                Remove-Item -LiteralPath (Split-Path -Parent $path) -Recurse -Force
            }
        }

        Test-Case "ACL failure removes the directory before a token is written" {
            . (Join-Path $PSScriptRoot "common.ps1")
            function Set-Acl {
                param($LiteralPath, $AclObject)
                $global:failedTokenDirectory = $LiteralPath
                throw "ACL denied"
            }
            $global:failedTokenDirectory = $null
            try { New-AccountTokenFile -Token $global:testToken } catch {
                Assert-True ($_.Exception.Message -eq 'ACL denied') "Unexpected ACL failure"
            }
            Assert-True ($global:failedTokenDirectory -and -not (Test-Path $global:failedTokenDirectory)) "Failed token directory left behind"
        }
    } else {
        Write-Host "SKIP: Windows token-file ACL checks require Windows"
    }
} finally {
    $env:TEMP = $previousTemp
    $env:ProgramFiles = $previousProgramFiles
    $env:UBLOCKDNS_ACCOUNT_TOKEN = $previousToken
    Remove-Item $testRoot -Recurse -Force
}
if ($global:installerTestsFailed) { throw "$global:installerTestsFailed installer test(s) failed" }
