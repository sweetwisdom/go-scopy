param(
    [Parameter(Mandatory = $true)]
    [string]$AgentJar
)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$source = (Resolve-Path -LiteralPath $AgentJar).Path
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [IO.Compression.ZipFile]::OpenRead($source)
try {
    if (-not ($archive.Entries | Where-Object { $_.FullName -eq 'classes.dex' })) {
        throw 'Expected an Android DEX JAR containing classes.dex'
    }
} finally { $archive.Dispose() }
$destination = Join-Path $root 'resources/extra/common/go-scrcpy-agent.jar'
if ($source -ne $destination) { Copy-Item -LiteralPath $source -Destination $destination -Force }
$hash = (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()
$manifestPath = Join-Path $root 'resources/manifest.json'
$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
$entry = $manifest.files | Where-Object { $_.path -eq 'common/go-scrcpy-agent.jar' }
if ($entry) { $entry.sha256 = $hash }
else { $manifest.files = @($manifest.files) + [pscustomobject]@{ path = 'common/go-scrcpy-agent.jar'; sha256 = $hash } }
# The additional Agent is built locally, not supplied by the imported tool repository.
$provenance = [pscustomobject]@{ project = 'go-scrcpy-agent'; package = 'io.github.goscrcpy.agent'; path = 'common/go-scrcpy-agent.jar'; sha256 = $hash }
$manifest | Add-Member -NotePropertyName agent -NotePropertyValue $provenance -Force
$json = $manifest | ConvertTo-Json -Depth 12
[IO.File]::WriteAllText($manifestPath, $json + "`n", [Text.UTF8Encoding]::new($false))
Write-Output "Agent synchronized: $destination"
Write-Output "SHA256: $hash"
