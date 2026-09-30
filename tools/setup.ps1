param(
    [ValidateSet('standalone', 'vpn', 'relay')][string]$Mode,
    [string]$Directory = 'state/connection'
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$binary = Join-Path $root "bin/networkroute-windows-$arch.exe"
if (-not (Test-Path -LiteralPath $binary)) { throw 'Binary missing. Download/extract the GitHub build artifact, or run tools/build.ps1.' }
$arguments = @('setup', '--directory', $Directory)
if ($Mode) { $arguments += @('--mode', $Mode) }
& $binary @arguments
if ($LASTEXITCODE -ne 0) { throw 'Setup did not complete. Existing configurations were preserved.' }
