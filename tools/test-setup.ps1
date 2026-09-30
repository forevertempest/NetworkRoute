# Local CLI smoke test. Generates temporary files only; never activates a VPN.
param()
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$client = Join-Path $root "bin/networkroute-windows-$arch.exe"
$go = Join-Path $root '.tools/go/bin/go.exe'
if (-not (Test-Path -LiteralPath $go)) { $go = (Get-Command go -ErrorAction Stop).Source }
$testRoot = Join-Path $root ('.tools/setup-smoke-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
function Invoke-Checked([string]$Executable, [string[]]$Arguments) {
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Setup command failed: $Executable $($Arguments[0])" }
}
try {
    $server = Join-Path $testRoot 'nr-relay-test.exe'
    Push-Location -LiteralPath $root
    try { Invoke-Checked $go @('build', '-o', $server, './cmd/nr-relay') } finally { Pop-Location }
    $standalone = Join-Path $testRoot 'standalone'
    & (Join-Path $root 'tools/setup.ps1') -Mode standalone -Directory $standalone
    Invoke-Checked $client @('doctor', '--config', (Join-Path $standalone 'config.local.json'))
    $relayClient = Join-Path $testRoot 'client'
    $relayServer = Join-Path $testRoot 'server'
    Invoke-Checked $client @('setup', '--mode', 'relay', '--directory', $relayClient)
    Invoke-Checked $server @('setup', '--directory', $relayServer, '--public-address', '127.0.0.1:7443', '--client-request', (Join-Path $relayClient 'client-request.json'))
    Invoke-Checked $client @('pair', '--directory', $relayClient, '--ticket', (Join-Path $relayServer 'relay-connection.json'))
    $vpnClient = Join-Path $testRoot 'vpn-client'
    $vpnServer = Join-Path $testRoot 'vpn-server'
    Invoke-Checked $client @('vpn-prepare', '--directory', $vpnClient)
    Invoke-Checked $server @('vpn-setup', '--directory', $vpnServer, '--endpoint', 'vpn.example.com:51820', '--client-request', (Join-Path $vpnClient 'client-wireguard.json'), '--full-tunnel', '--egress', 'eth0', '--dns', '1.1.1.1')
    $ticket = Join-Path $vpnServer 'wireguard-connection.json'
    # A full-tunnel profile must be refused without explicit acceptance.
    $rejected = Start-Process -FilePath $client -ArgumentList @('vpn-pair', '--directory', ('"' + $vpnClient + '"'), '--ticket', ('"' + $ticket + '"')) -WindowStyle Hidden -Wait -PassThru -RedirectStandardOutput (Join-Path $testRoot 'rejected.out') -RedirectStandardError (Join-Path $testRoot 'rejected.err')
    if ($rejected.ExitCode -eq 0) { throw 'Full tunnel unexpectedly accepted without opt-in.' }
    Invoke-Checked $client @('vpn-pair', '--directory', $vpnClient, '--ticket', $ticket, '--allow-full-tunnel')
    if (-not (Test-Path -LiteralPath (Join-Path $vpnClient 'nro-client.conf'))) { throw 'Client VPN profile was not created.' }
    Write-Output 'Setup CLI smoke checks passed; no VPN, routes or firewall were activated.'
} finally {
    $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
    $allowedRoot = [IO.Path]::GetFullPath((Join-Path $root '.tools')) + [IO.Path]::DirectorySeparatorChar
    if (-not $resolvedTestRoot.StartsWith($allowedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe temporary cleanup path.' }
    Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force
}
