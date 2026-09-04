param([switch]$Check)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile(
    (Join-Path $PSScriptRoot "common.ps1"), [ref]$null, [ref]$errors)
if ($errors) { throw ($errors | Out-String) }
$functions = @{}
foreach ($statement in $ast.EndBlock.Statements) {
    if ($statement -is [Management.Automation.Language.FunctionDefinitionAst]) {
        $functions[$statement.Name] = $statement.Extent.Text.Replace("`r`n", "`n")
    }
}
$shared = @('Test-Admin', 'Assert-SupportedWindowsVersion', 'Enable-Tls12', 'Invoke-DownloadFile')
$targets = @{
    'install.ps1' = $shared + @('Get-Sha256Hex', 'Resolve-GitHubLatestTag')
    'setup.ps1' = $shared + @('New-AccountTokenFile')
}
foreach ($name in $targets.Keys) {
    $path = Join-Path $repoRoot $name
    $source = [IO.File]::ReadAllText($path).Replace("`r`n", "`n")
    $body = ($targets[$name] | ForEach-Object {
        if (-not $functions.ContainsKey($_)) { throw "Missing helper: $_" }
        $functions[$_]
    }) -join "`n`n"
    $body = (($body -split "`n") | ForEach-Object { if ($_) { '    ' + $_ } else { '' } }) -join "`n"
    $pattern = '(?ms)(    # BEGIN GENERATED HELPERS\n).*?(    # END GENERATED HELPERS)'
    if (-not [regex]::IsMatch($source, $pattern)) { throw "Missing helper markers in $name" }
    $updated = [regex]::Replace($source, $pattern, [Text.RegularExpressions.MatchEvaluator]{
        param($match)
        $match.Groups[1].Value + $body + "`n" + $match.Groups[2].Value
    })
    if ($updated -ne $source) {
        if ($Check) { throw "$name helpers are stale. Run scripts/windows/sync-helpers.ps1." }
        [IO.File]::WriteAllText($path, $updated, [Text.UTF8Encoding]::new($false))
    }
}
