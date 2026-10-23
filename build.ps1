# build.ps1 - one self-contained iconplugin.exe; no runtime, no cgo.
$ErrorActionPreference = 'Stop'
$go = if (Test-Path 'S:\tools\go\bin\go.exe') { 'S:\tools\go\bin\go.exe' } else { 'go' }
$env:CGO_ENABLED = '0'
& $go test ./...
if ($LASTEXITCODE -ne 0) { throw 'tests failed' }
& $go build -trimpath -ldflags '-s -w' -o iconplugin.exe .
if ($LASTEXITCODE -ne 0) { throw 'build failed' }
$item = Get-Item iconplugin.exe
Write-Host ("iconplugin.exe  {0:n1} MB  {1}" -f ($item.Length / 1MB), $item.LastWriteTime)
