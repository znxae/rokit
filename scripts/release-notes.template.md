# Rokit {{VERSION}}

Dibangun dari branch `{{BRANCH}}` (commit `{{COMMIT}}`).

## Unduhan / Downloads
| File | SHA256 |
|---|---|
| `rokit-audio-editor-setup-{{VERSION}}-amd64.exe` (installer) | `{{SETUP_SHA}}` |
| `Rokit-{{VERSION}}-win64.zip` (portabel / portable) | `{{ZIP_SHA}}` |

**Installer:** instal per-pengguna (tanpa admin) ke `%LOCALAPPDATA%\Programs\Rokit`, sudah termasuk ffmpeg, ffprobe, yt-dlp; membuat pintasan Start menu dan (opsional, centang) Desktop, serta uninstaller di Pengaturan > Aplikasi. Penghapus tidak menghapus `%APPDATA%\Rokit` kecuali Anda memilih Ya.
**Zip:** ekstrak, jalankan `Rokit Audio Editor.exe`, biarkan folder `bin` di sebelahnya.

## Peringatan jujur / Honest warnings
- Installer dan exe **tidak ditandatangani**: Windows SmartScreen menampilkan "Windows protected your PC" (More info > Run anyway), peringatannya lebih keras untuk file setup.exe; sebagian antivirus bisa menandainya. Zip adalah alternatifnya. Cocokkan SHA256 di atas sebelum menjalankan. / Both are unsigned: SmartScreen warns, more strongly for the setup.exe; some antivirus may flag them. Verify the SHA256.
- Login Roblox opsional menyimpan cookie sesi di Windows Credential Manager (lihat README, bagian Aset Roblox). Unduh hanya konten milik Anda atau yang boleh Anda unduh.
- ffmpeg berlisensi LGPL; sumber: <https://git.ffmpeg.org/gitweb/ffmpeg.git/shortlog/refs/tags/n8.1.3>. Lihat `THIRD_PARTY_NOTICES.md`.
- Belum diverifikasi / Unverified: {{UNVERIFIED}}
