param([string]$ToolsRoot = '', [string]$OutputDir = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
if (-not $ToolsRoot) { $ToolsRoot = $taskRoot }
if (-not $OutputDir) { $OutputDir = Join-Path $taskRoot 'artifacts/headless' }
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$taskData = Join-Path ([IO.Path]::GetTempPath()) ('goscopy-smoke-' + [guid]::NewGuid().ToString('N'))
$taskProcess = $null
function Read-Reply {
    $taskLines = [Collections.Generic.List[string]]::new()
    while ($true) {
        $taskLine = $taskProcess.StandardOutput.ReadLineAsync().WaitAsync([TimeSpan]::FromSeconds(20)).GetAwaiter().GetResult()
        if ($null -eq $taskLine) { throw "Headless process exited: $($taskProcess.StandardError.ReadToEnd())" }
        $taskLines.Add($taskLine)
        if ($taskLine -match '^error(?: |$)') { throw ($taskLines -join "`n") }
        if ($taskLine -match '^ok(?: |$)') { return ($taskLines -join "`n") }
    }
}
function Send-Command([string]$TaskCommand) {
    $taskProcess.StandardInput.WriteLine($TaskCommand)
    $taskProcess.StandardInput.Flush()
    return Read-Reply
}
function Check-Layout {
    $taskLayout = Send-Command 'layout'
    if ($taskLayout -notmatch '(?m)^ok 0 problems\s*$') { throw $taskLayout }
}
Push-Location $taskRoot
try {
    New-Item -ItemType Directory -Force build | Out-Null
    go build -tags vivahead -o build/go-scopy-headless.exe ./cmd/go-scopy
    if ($LASTEXITCODE) { throw 'Headless build failed' }
    $taskInfo = [Diagnostics.ProcessStartInfo]::new()
    $taskInfo.FileName = Join-Path $taskRoot 'build/go-scopy-headless.exe'
    $taskInfo.WorkingDirectory = $taskRoot
    $taskInfo.UseShellExecute = $false
    $taskInfo.CreateNoWindow = $true
    $taskInfo.RedirectStandardInput = $true
    $taskInfo.RedirectStandardOutput = $true
    $taskInfo.RedirectStandardError = $true
    $taskInfo.StandardInputEncoding = [Text.UTF8Encoding]::new($false)
    $taskInfo.StandardOutputEncoding = [Text.Encoding]::UTF8
    $taskInfo.Environment['VIVA_HEADLESS'] = '1'
    $taskInfo.Environment['VIVA_HEADLESS_SCALE'] = '1'
    foreach ($taskArg in @('--data-dir', $taskData, '--tools-root', $ToolsRoot)) { $taskInfo.ArgumentList.Add($taskArg) }
    $taskStartup = [Diagnostics.Stopwatch]::StartNew()
    $taskProcess = [Diagnostics.Process]::Start($taskInfo)
    $null = Read-Reply
    $null = Send-Command 'waitfor "设备监听已连接" 15000'
    $taskStartup.Stop()
    $null = Send-Command 'settle 100'
    $taskProcess.Refresh()
    [ordered]@{ mode='Viva headless with real tools, no Android device'; startupToTrackerMs=$taskStartup.Elapsed.TotalMilliseconds; workingSetBytes=$taskProcess.WorkingSet64; privateMemoryBytes=$taskProcess.PrivateMemorySize64 } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $OutputDir 'measurements.json') -Encoding utf8NoBOM
    Check-Layout
    $taskShot = (Join-Path $OutputDir 'real-tools-light.png').Replace('\','/')
    $null = Send-Command ('shot "' + $taskShot + '"')
    $null = Send-Command 'click "设置"'
    $null = Send-Command 'click "运行环境"'
    $taskEnvironment = Send-Command 'texts'
    if ($taskEnvironment -notmatch 'Android Debug Bridge version' -or $taskEnvironment -notmatch 'scrcpy 4\.1|scrcpy 5\.0\.1') { throw $taskEnvironment }
    $taskEnvironment | Set-Content -LiteralPath (Join-Path $OutputDir 'environment.txt') -Encoding utf8NoBOM
    Check-Layout
    $null = Send-Command 'click "常规"'
    $null = Send-Command 'click "切换为深色"'
    $null = Send-Command 'settle 100'
    $null = Send-Command 'click "设备"'
    Check-Layout
    $taskShot = (Join-Path $OutputDir 'real-tools-dark.png').Replace('\','/')
    $null = Send-Command ('shot "' + $taskShot + '"')
    $null = Send-Command 'quit'
    if (-not $taskProcess.WaitForExit(15000)) { throw 'Headless process did not cleanly exit' }
    if ($taskProcess.ExitCode -ne 0) { throw "Headless exited with code $($taskProcess.ExitCode)" }
    Write-Host "Headless smoke passed. Isolated configuration: $taskData"
} finally {
    if ($taskProcess) { if (-not $taskProcess.HasExited) { $taskProcess.Kill() }; $taskProcess.Dispose() }
    Pop-Location
}
