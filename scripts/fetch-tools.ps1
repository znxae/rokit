# Downloads the pinned yt-dlp and ffmpeg into .tools\bin and verifies SHA256 against tools.lock.
# Official sources only; fails on any mismatch. Nothing is executed until every hash has matched,
# and then only `--version`. Usage: powershell -File scripts\fetch-tools.ps1
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$lock = Get-Content (Join-Path $root 'tools.lock') -Raw | ConvertFrom-Json
$dl = Join-Path $root '.tools\dl'; $bin = Join-Path $root '.tools\bin'; $lic = Join-Path $root '.tools\licenses'
New-Item -ItemType Directory -Force $dl, $bin, $lic | Out-Null

$allowed = @('https://github.com/yt-dlp/yt-dlp/releases/download/', 'https://github.com/FFmpeg-Builds/FFmpeg-Builds/releases/download/')
function Get-Pinned($tool) {
  if (-not ($allowed | Where-Object { $tool.url.StartsWith($_) })) { throw "URL not on the official allow-list: $($tool.url)" }
  $file = Join-Path $dl ([IO.Path]::GetFileName($tool.url))
  if (-not (Test-Path $file) -or (Get-FileHash $file -Algorithm SHA256).Hash -ne $tool.sha256.ToUpper()) {
    Write-Host "Downloading $($tool.url)"
    Invoke-WebRequest -Uri $tool.url -OutFile $file -UseBasicParsing
  }
  $got = (Get-FileHash $file -Algorithm SHA256).Hash
  if ($got -ne $tool.sha256.ToUpper()) { Remove-Item $file -Force; throw "SHA256 MISMATCH for $file`n expected $($tool.sha256)`n got      $got" }
  Write-Host "OK sha256 $([IO.Path]::GetFileName($file))"
  $file
}

$ytFile = Get-Pinned $lock.'yt-dlp'
$ffFile = Get-Pinned $lock.ffmpeg

# Everything verified: now place the files. Only the named zip entries are read (no blind extraction).
Copy-Item $ytFile (Join-Path $bin 'yt-dlp.exe') -Force
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [IO.Compression.ZipFile]::OpenRead($ffFile)
try {
  $top = ($zip.Entries | Select-Object -First 1).FullName.Split('/')[0]
  foreach ($want in $lock.ffmpeg.files) {
    $e = $zip.GetEntry("$top/$want")
    if (-not $e) { throw "missing in zip: $want" }
    $dest = if ($want -like 'bin/*') { Join-Path $bin ([IO.Path]::GetFileName($want)) } else { Join-Path $lic 'ffmpeg-LICENSE.txt' }
    [IO.Compression.ZipFileExtensions]::ExtractToFile($e, $dest, $true)
  }
} finally { $zip.Dispose() }

# Hashes matched: --version is the only execution.
$v = @()
foreach ($n in 'ffmpeg', 'ffprobe', 'yt-dlp') {
  $exe = Join-Path $bin "$n.exe"
  $ver = if ($n -eq 'yt-dlp') { & $exe --version } else { & $exe -version | Select-Object -First 1 }
  $v += "$n : $ver"
}
$v | Set-Content (Join-Path $root '.tools\TOOL_VERSIONS.txt') -Encoding utf8
$v
Write-Host "Tools ready in $bin"
