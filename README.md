# rokit

A small desktop audio editor in Rust (egui/eframe). It converts one file at a time by calling `ffmpeg`.

- Output formats: mp3, wav, flac, ogg (vorbis), m4a (aac), opus
- Bitrate 64–320 kbps (disabled for lossless wav/flac)
- Speed 0.25x–4x with pitch kept (chained `atempo`), amplify ±20 dB, mono/stereo/keep channels
- Under the Speed slider the app shows the inverse speed (1/speed, 3 decimals) that brings converted audio back to normal, e.g. 1.5x → 0.667x
- ffmpeg runs on a background thread; its error output is shown in the app

## Requirements

`ffmpeg` on PATH. Windows: `winget install Gyan.FFmpeg`.

## Build (Windows, GNU toolchain, no MSVC needed)

`rust-toolchain.toml` pins `stable-x86_64-pc-windows-gnu` (install with `rustup toolchain install stable-x86_64-pc-windows-gnu`; needs mingw gcc).
Run cargo from PowerShell, not Git Bash.

```
cargo test
cargo run --release
```
