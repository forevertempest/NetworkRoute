param()
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$dist = Join-Path $root 'dist'
New-Item -ItemType Directory -Path $dist -Force | Out-Null
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
# Explicit allowlist: never package local config, generated profiles or keys.
$common = @('README.md', 'ARCHITECTURE.md', 'config.example.json', 'docs/CONSOLE.md', 'docs/LOCAL_CONTROL.md',
    'docs/QUICKSTART.md', 'docs/SELF_HOSTING.md', 'docs/SECURITY.md',
    'docs/VALIDATION.md', 'docs/BENCHMARKS.md', 'docs/THIRD_PARTY_NOTICES.md', 'docs/networkroute-relay.service', 'tools/setup-wireguard.md')
$archives = @()
foreach ($arch in @('amd64', 'arm64')) {
    foreach ($platform in @('windows', 'linux')) {
        $files = $common
        if ($platform -eq 'windows') {
            $files += @("bin/networkroute-windows-$arch.exe", 'tools/setup.ps1')
        } else {
            $files += @("bin/networkroute-linux-$arch", "bin/nr-relay-linux-$arch", 'tools/install-relay.sh', 'tools/install-wireguard.sh')
        }
        foreach ($file in $files) {
            if (-not (Test-Path -LiteralPath (Join-Path $root $file) -PathType Leaf)) { throw "Missing release file: $file. Run tools/build.ps1 first." }
        }
        $archivePath = Join-Path $dist "networkroute-$platform-$arch.zip"
        $stream = [IO.File]::Open($archivePath, [IO.FileMode]::Create)
        $zip = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create)
        try {
            foreach ($file in $files) {
                [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, (Join-Path $root $file), $file, [IO.Compression.CompressionLevel]::Optimal) | Out-Null
            }
        } finally { $zip.Dispose(); $stream.Dispose() }
        # Reopen and validate the exact allowlist, including directory separators.
        $zip = [IO.Compression.ZipFile]::OpenRead($archivePath)
        try {
            $actual = @($zip.Entries | ForEach-Object { $_.FullName })
            if (Compare-Object ($files | Sort-Object) ($actual | Sort-Object)) { throw 'Unexpected files in release archive.' }
        } finally { $zip.Dispose() }
        $archives += $archivePath
    }
}
Copy-Item -LiteralPath (Join-Path $root 'bin/networkroute-windows-amd64.exe') -Destination (Join-Path $dist 'NetworkRoute.exe') -Force
Copy-Item -LiteralPath (Join-Path $root 'bin/networkroute-windows-arm64.exe') -Destination (Join-Path $dist 'NetworkRoute-arm64.exe') -Force
$archives += @((Join-Path $dist 'NetworkRoute.exe'), (Join-Path $dist 'NetworkRoute-arm64.exe'))
foreach ($arch in @('amd64', 'arm64')) {
    foreach ($name in @('networkroute', 'nr-relay')) {
        $single = Join-Path $dist "$name-linux-$arch"
        Copy-Item -LiteralPath (Join-Path $root "bin/$name-linux-$arch") -Destination $single -Force
        $archives += $single
    }
}
$checksums = foreach ($archivePath in $archives) {
    $hash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $([IO.Path]::GetFileName($archivePath))"
}
[IO.File]::WriteAllLines((Join-Path $dist 'SHA256SUMS'), $checksums, [Text.Encoding]::ASCII)
Get-Item -LiteralPath $archives | Select-Object Name, Length
