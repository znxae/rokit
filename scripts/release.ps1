# One command for a release: the installer (NSIS, built by `mygo build`) AND the portable zip, plus
# release\SHA256SUMS.txt for both. Run:  powershell -ExecutionPolicy Bypass -File scripts\release.ps1
# Needs the app closed (build\ is replaced) and, the first time, internet for scripts\fetch-tools.ps1
# (ffmpeg/yt-dlp, hash-pinned) and for MyGo to fetch NSIS. Nothing built here is committed.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
if (Get-Process -Name 'Rokit Audio Editor' -ErrorAction SilentlyContinue | Where-Object { $_.Path -like "$root\build\*" }) { throw 'Close Rokit first: it locks build\.' }
$bin = Join-Path $root '.tools\bin'
if (-not (Test-Path "$bin\ffmpeg.exe")) { & "$PSScriptRoot\fetch-tools.ps1" }
$ver = [regex]::Match((Get-Content mygo.config.ts -Raw), 'version:\s*"([^"]+)"').Groups[1].Value
$res = Join-Path $root 'resources'
# `mygo build` copies resources\ next to the exe and the installer installs everything there, so bin\ ships.
$staged = 'bin', 'licenses', 'THIRD_PARTY_NOTICES.md', 'README.md', 'tools.lock', 'LICENSE', 'LICENSE.md', 'LICENSE.txt'
try {
  New-Item -ItemType Directory -Force "$res\bin" | Out-Null
  Copy-Item "$bin\*.exe" "$res\bin" -Force
  Copy-Item "$root\.tools\licenses" "$res\licenses" -Recurse -Force
  foreach ($f in 'THIRD_PARTY_NOTICES.md', 'README.md', 'tools.lock', 'LICENSE', 'LICENSE.md', 'LICENSE.txt') {
    if (Test-Path "$root\$f") { Copy-Item "$root\$f" $res -Force }
  }
  npx mygo build
  if ($LASTEXITCODE -ne 0) { throw 'mygo build failed' }
} finally {
  foreach ($n in $staged) { Remove-Item "$res\$n" -Recurse -Force -ErrorAction SilentlyContinue }
}
& "$PSScriptRoot\package.ps1"
$setup = Get-ChildItem "$root\build\windows-amd64\*-setup-$ver-*.exe" | Select-Object -First 1
if (-not $setup) { throw 'installer not found in build\windows-amd64' }
$out = Join-Path $root 'release'
Copy-Item $setup.FullName $out -Force
$lines = foreach ($f in "Rokit-$ver-win64.zip", $setup.Name) {
  $h = (Get-FileHash "$out\$f" -Algorithm SHA256).Hash.ToLower()
  "$h *$f"
}
[IO.File]::WriteAllText("$out\SHA256SUMS.txt", (($lines -join "`n") + "`n"))
Get-Content "$out\SHA256SUMS.txt"
Get-ChildItem "$out\Rokit-$ver-win64.zip", "$out\$($setup.Name)" | ForEach-Object { '{0}  {1:N1} MB' -f $_.Name, ($_.Length / 1MB) }
