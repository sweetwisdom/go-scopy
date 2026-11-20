param([string]$Version = '0.1.9', [switch]$SkipChecks, [switch]$Installer, [string]$MakeNSIS = 'makensis')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
Push-Location $taskRoot
try {
    if (-not $SkipChecks) {
        go vet ./...
        if ($LASTEXITCODE) { throw 'go vet failed' }
    }
    $taskCommit = git rev-parse HEAD
    if ($LASTEXITCODE) { throw 'Cannot read source commit' }
    $packageArgs = @('--version', $Version, '--commit', $taskCommit)
    if ($Installer) { $packageArgs += @('--installer', '--makensis', $MakeNSIS) }
    go run ./cmd/package @packageArgs
    if ($LASTEXITCODE) { throw 'Release build failed' }
    Write-Host 'Release archives and SHA-256 files are ready in dist/'
} finally { Pop-Location }
