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

function Get-Sha256Hex {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

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
    param(
        [Parameter(Mandatory = $true)]
        [string]$Repo
    )

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
