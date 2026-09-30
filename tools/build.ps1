param([switch]$Test)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Set-Location -LiteralPath $root
$go = Join-Path $root '.tools/go/bin/go.exe'
if (-not (Test-Path -LiteralPath $go)) { $go = (Get-Command go -ErrorAction Stop).Source }
if ($Test) {
    & $go test ./... -count=1 -timeout=60s
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }
    & $go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed.' }
}
New-Item -ItemType Directory -Path (Join-Path $root 'bin') -Force | Out-Null
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    # Linux tools are embedded as a compressed offline deployment kit.
    foreach ($arch in @('amd64', 'arm64')) {
        $env:GOOS = 'linux'
        $env:GOARCH = $arch
        & $go build -trimpath -ldflags '-s -w' -o "bin/nr-relay-linux-$arch" ./cmd/nr-relay
        if ($LASTEXITCODE -ne 0) { throw "Linux $arch build failed." }
    }
    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    New-Item -ItemType Directory -Path (Join-Path $root '.bundle') -Force | Out-Null
    $bundlePath = Join-Path $root '.bundle/linux-relay.zip'
    $stream = [IO.File]::Open($bundlePath, [IO.FileMode]::Create)
    $zip = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($file in @('bin/nr-relay-linux-amd64', 'bin/nr-relay-linux-arm64',
            'tools/install-relay.sh', 'tools/install-wireguard.sh', 'tools/setup-wireguard.md',
            'docs/QUICKSTART.md', 'docs/SELF_HOSTING.md', 'docs/THIRD_PARTY_NOTICES.md', 'docs/networkroute-relay.service')) {
            [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, (Join-Path $root $file), $file, [IO.Compression.CompressionLevel]::Optimal) | Out-Null
        }
    } finally { $zip.Dispose(); $stream.Dispose() }
    foreach ($arch in @('amd64', 'arm64')) {
        $env:GOOS = 'windows'
        $env:GOARCH = $arch
        & $go build -trimpath -ldflags '-s -w' -o "bin/networkroute-windows-$arch.exe" ./cmd/networkroute
        if ($LASTEXITCODE -ne 0) { throw "Windows $arch build failed." }
        $env:GOOS = 'linux'
        & $go build -trimpath -ldflags '-s -w' -o "bin/networkroute-linux-$arch" ./cmd/networkroute
        if ($LASTEXITCODE -ne 0) { throw "Linux client $arch build failed." }
    }
} finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
}
Get-FileHash -Algorithm SHA256 -Path bin/* | Format-Table -AutoSize
