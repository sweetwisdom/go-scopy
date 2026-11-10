param([string]$Destination = '', [string]$Archive = '', [string]$EscrcpyRoot = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
if (-not $Destination) { $Destination = Join-Path $taskRoot 'tools/scrcpy' }
if ($EscrcpyRoot) {
    $taskLocalLock = Get-Content -Raw (Join-Path $PSScriptRoot 'escrcpy-tools.lock.json') | ConvertFrom-Json
    $taskResources = Join-Path $EscrcpyRoot 'desktop/electron/resources/extra'
    foreach ($taskFile in $taskLocalLock.files) {
        $taskInput = Join-Path $taskResources $taskFile.path
        if ((Get-FileHash -LiteralPath $taskInput -Algorithm SHA256).Hash.ToLowerInvariant() -ne $taskFile.sha256) {
            throw "Local escrcpy resource differs from the pinned manifest: $($taskFile.path)"
        }
    }
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    foreach ($taskFile in $taskLocalLock.files) {
        Copy-Item -LiteralPath (Join-Path $taskResources $taskFile.path) -Destination (Join-Path $Destination $taskFile.name) -Force
    }
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'escrcpy-tools.lock.json') -Destination (Join-Path $Destination 'source.json') -Force
    Write-Host "Reused verified local escrcpy resources (scrcpy $($taskLocalLock.version)): $Destination"
    return
}
$taskLock = Get-Content -Raw (Join-Path $PSScriptRoot 'tools.lock.json') | ConvertFrom-Json
$taskCache = Join-Path $taskRoot 'build/downloads'
New-Item -ItemType Directory -Force -Path $taskCache, $Destination | Out-Null
if (-not $Archive) {
    $Archive = Join-Path $taskCache $taskLock.file
    if (-not (Test-Path -LiteralPath $Archive)) {
        Write-Host "Downloading official scrcpy $($taskLock.version)..."
        $taskPartial = "$Archive.partial"
        Invoke-WebRequest -Uri $taskLock.url -OutFile $taskPartial
        if ((Get-FileHash -LiteralPath $taskPartial -Algorithm SHA256).Hash.ToLowerInvariant() -ne $taskLock.sha256) {
            throw 'Downloaded tool archive failed SHA-256 verification.'
        }
        Move-Item -LiteralPath $taskPartial -Destination $Archive -Force
    }
}
if ((Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $taskLock.sha256) {
    throw 'Tool archive failed SHA-256 verification. Supply the official pinned archive with -Archive.'
}
$taskExtract = Join-Path $taskCache "scrcpy-$($taskLock.version)"
Expand-Archive -LiteralPath $Archive -DestinationPath $taskExtract -Force
$taskSource = Join-Path $taskExtract "scrcpy-win64-v$($taskLock.version)"
foreach ($taskRequired in @('adb.exe', 'scrcpy.exe', 'scrcpy-server', 'AdbWinApi.dll', 'AdbWinUsbApi.dll')) {
    if (-not (Test-Path -LiteralPath (Join-Path $taskSource $taskRequired))) { throw "Missing official tool file: $taskRequired" }
}
Copy-Item -Path (Join-Path $taskSource '*') -Destination $Destination -Recurse -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'tools.lock.json') -Destination (Join-Path $Destination 'source.json') -Force
Write-Host "Tools ready: $Destination"
