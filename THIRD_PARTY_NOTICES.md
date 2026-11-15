# Third-party notices

Rokit starts these programs from the `bin` folder next to `Rokit Audio Editor.exe`. They are separate programs,
not linked into Rokit. Exact versions and SHA-256 hashes are pinned in `tools.lock`.

## FFmpeg (`bin\ffmpeg.exe`, `bin\ffprobe.exe`)

- Build: FFmpeg-Builds (win64, **lgpl**, static) `autobuild-2026-11-14-12-37`, FFmpeg `n8.1.3-35-gf1acb0a266`.
- License: **GNU Lesser General Public License, version 3 or later** (the build is configured with `--enable-version3` and
  without `--enable-gpl` or `--enable-nonfree`). The full text is in `licenses\ffmpeg-LICENSE.txt`.
- The build statically includes other libraries (for example libmp3lame, libopus, libvorbis, libvpx, dav1d), each under its own
  license, all permissive or LGPL-compatible for this configuration. The complete configure line is printed by `bin\ffmpeg.exe -version`.
- **Source offer.** You may obtain the corresponding source and the build scripts, and replace the FFmpeg programs with your own build
  (Rokit only runs whatever `ffmpeg.exe` and `ffprobe.exe` are in `bin`):
  - FFmpeg source for this version: <https://git.ffmpeg.org/gitweb/ffmpeg.git/shortlog/refs/tags/n8.1.3> and <https://ffmpeg.org/download.html>
  - Build scripts, patches and library versions: <https://github.com/FFmpeg-Builds/FFmpeg-Builds> (release `autobuild-2026-11-14-12-37`)
- FFmpeg is a trademark of Fabrice Bellard. Project: <https://ffmpeg.org/>, license notes: <https://ffmpeg.org/legal.html>

## yt-dlp (`bin\yt-dlp.exe`)

- Version: see `tools.lock` (release <https://github.com/yt-dlp/yt-dlp/releases>).
- License: **The Unlicense** (public domain dedication): <https://github.com/yt-dlp/yt-dlp/blob/master/LICENSE>
- The single-file `yt-dlp.exe` bundles Python and libraries under their own licenses, listed in
  <https://github.com/yt-dlp/yt-dlp/blob/master/THIRD_PARTY_LICENSES.txt>
- "Perbarui yt-dlp" in the app replaces this file with the latest official release after checking its SHA-256
  against the release's own `SHA2-256SUMS`.

## Rokit itself

Built with [MyGo](https://github.com/egoist/mygo) (see its license) and the Go standard library.
