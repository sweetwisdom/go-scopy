param([switch]$SkipChecks, [string]$ToolArchive = '', [string]$EscrcpyRoot = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
Push-Location $taskRoot
try {
    if ((go env GOOS) -ne 'windows' -or (go env GOARCH) -ne 'amd64') { throw 'This release script targets Windows x64.' }
    if (-not $SkipChecks) {
        go vet ./...
        if ($LASTEXITCODE) { throw 'go vet failed' }
        go test ./... -count=1
        if ($LASTEXITCODE) { throw 'go test failed' }
    }
    $taskName = 'Go-Scopy-0.1.1-windows-x64'
    $taskPackage = Join-Path $taskRoot "dist/$taskName"
    New-Item -ItemType Directory -Force -Path $taskPackage | Out-Null
    & (Join-Path $PSScriptRoot 'fetch-tools.ps1') -Destination (Join-Path $taskPackage 'tools/scrcpy') -Archive $ToolArchive -EscrcpyRoot $EscrcpyRoot
    go build -trimpath -ldflags '-s -w -H windowsgui' -o (Join-Path $taskPackage 'Go-Scopy.exe') ./cmd/go-scopy
    if ($LASTEXITCODE) { throw 'Release build failed' }
    go run ./cmd/assets -stamp-windows (Join-Path $taskPackage 'Go-Scopy.exe')
    if ($LASTEXITCODE) { throw 'Windows icon resource stamping failed' }
    Copy-Item -LiteralPath (Join-Path $taskRoot 'internal/brand/assets/icon.ico') -Destination (Join-Path $taskPackage 'Go-Scopy.ico') -Force
    foreach ($taskDoc in @('README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.md')) {
        Copy-Item -LiteralPath (Join-Path $taskRoot $taskDoc) -Destination $taskPackage -Force
    }
    Copy-Item -LiteralPath (Join-Path $taskRoot 'licenses') -Destination $taskPackage -Recurse -Force
    Copy-Item -LiteralPath (Join-Path $taskRoot 'docs') -Destination $taskPackage -Recurse -Force
    $taskHashes = Get-ChildItem -LiteralPath $taskPackage -File -Recurse | Sort-Object FullName | ForEach-Object {
        $taskRelative = [IO.Path]::GetRelativePath($taskPackage, $_.FullName).Replace('\', '/')
        if ($taskRelative -ne 'SHA256SUMS.txt') { '{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $taskRelative }
    }
    $taskHashes | Set-Content -LiteralPath (Join-Path $taskPackage 'SHA256SUMS.txt') -Encoding utf8NoBOM
    $taskZip = Join-Path $taskRoot "dist/$taskName.zip"
    Compress-Archive -LiteralPath $taskPackage -DestinationPath $taskZip -Force
    Write-Host "Release ready: $taskZip"
    Get-FileHash -LiteralPath $taskZip -Algorithm SHA256
} finally { Pop-Location }
