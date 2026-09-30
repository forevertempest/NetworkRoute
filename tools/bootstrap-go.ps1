$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$toolDir = Join-Path $root '.tools'
New-Item -ItemType Directory -Path $toolDir -Force | Out-Null
$release = (Invoke-RestMethod 'https://go.dev/dl/?mode=json')[0]
$archive = $release.files | Where-Object { $_.os -eq 'windows' -and $_.arch -eq 'amd64' -and $_.kind -eq 'archive' } | Select-Object -First 1
if (-not $archive) { throw 'Official Go release has no Windows amd64 archive.' }
$zip = Join-Path $toolDir $archive.filename
Invoke-WebRequest -UseBasicParsing -Uri ('https://go.dev/dl/' + $archive.filename) -OutFile $zip
if ((Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant() -ne $archive.sha256) { throw 'Go archive SHA256 mismatch.' }
Expand-Archive -LiteralPath $zip -DestinationPath $toolDir -Force
& (Join-Path $toolDir 'go\bin\go.exe') version
if ($LASTEXITCODE -ne 0) { throw 'Go bootstrap failed.' }
