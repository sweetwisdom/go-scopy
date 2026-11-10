param([string]$DataDir = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
Push-Location $taskRoot
try {
    $taskArguments = @('run', './cmd/go-scopy', '--tools-root', $taskRoot)
    if ($DataDir) { $taskArguments += @('--data-dir', $DataDir) }
    & go @taskArguments
    if ($LASTEXITCODE) { throw 'Go-Scopy exited with an error' }
} finally { Pop-Location }
