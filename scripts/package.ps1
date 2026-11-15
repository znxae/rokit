# Builds the portable zip: exe + bin\ (ffmpeg, ffprobe, yt-dlp) + notices. Run after `npx mygo build`
# and `scripts\fetch-tools.ps1`. Output: release\Rokit-<version>-win64.zip (this is what gets distributed).
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$exe = Join-Path $root 'build\windows-amd64\Rokit Audio Editor.exe'
$bin = Join-Path $root '.tools\bin'
if (-not (Test-Path $exe)) { throw "Build the app first: npx mygo build" }
foreach ($n in 'ffmpeg.exe', 'ffprobe.exe', 'yt-dlp.exe') { if (-not (Test-Path (Join-Path $bin $n))) { throw "Missing .tools\bin\$n; run scripts\fetch-tools.ps1" } }
$ver = [regex]::Match((Get-Content (Join-Path $root 'mygo.config.ts') -Raw), 'version:\s*"([^"]+)"').Groups[1].Value
if (-not $ver) { throw 'no version in mygo.config.ts' }
$name = "Rokit-$ver-win64"
$stage = Join-Path $root "release\$name"
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Force $stage, "$stage\bin", "$stage\licenses" | Out-Null
Copy-Item $exe $stage
Copy-Item "$bin\*.exe" "$stage\bin"
Copy-Item (Join-Path $root '.tools\licenses\*') "$stage\licenses"
Copy-Item (Join-Path $root 'THIRD_PARTY_NOTICES.md'), (Join-Path $root 'tools.lock'), (Join-Path $root 'README.md') $stage
$zip = Join-Path $root "release\$name.zip"
if (Test-Path $zip) { Remove-Item $zip -Force }
Add-Type -AssemblyName System.IO.Compression.FileSystem
[IO.Compression.ZipFile]::CreateFromDirectory($stage, $zip, [IO.Compression.CompressionLevel]::Optimal, $true)
"{0}  {1:N1} MB zip" -f $zip, ((Get-Item $zip).Length / 1MB)
