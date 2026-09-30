# Refresh bundled dependency license notices from the installed Go/module sources.
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$go = Join-Path $root '.tools/go/bin/go.exe'
if (-not (Test-Path -LiteralPath $go)) { $go = (Get-Command go -ErrorAction Stop).Source }
Push-Location -LiteralPath $root
try {
    $moduleCache = (& $go env GOMODCACHE).Trim()
    $toolchainRoot = (& $go env GOROOT).Trim()
    $entries = @([pscustomobject]@{ Name = 'Go runtime / standard library'; Path = (Join-Path $toolchainRoot 'LICENSE') })
    foreach ($module in @('github.com/quic-go/quic-go', 'golang.org/x/sys', 'golang.org/x/crypto', 'golang.org/x/net')) {
        $version = (& $go list -m -f '{{.Version}}' $module).Trim()
        if ($LASTEXITCODE -ne 0) { throw "Cannot resolve $module" }
        $entries += [pscustomobject]@{ Name = "$module $version"; Path = (Join-Path $moduleCache "$module@$version/LICENSE") }
    }
    $text = "# Third-party notices`n`nThese notices cover bundled dependencies, including the embedded Linux relay.`n`n"
    foreach ($entry in $entries) {
        $text += "## $($entry.Name)`n`n"
        $text += [IO.File]::ReadAllText($entry.Path).Replace("`r`n", "`n") + "`n`n"
    }
    [IO.File]::WriteAllText((Join-Path $root 'docs/THIRD_PARTY_NOTICES.md'), $text, [Text.UTF8Encoding]::new($false))
} finally { Pop-Location }
