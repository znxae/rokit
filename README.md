# Rokit

A small Windows desktop app built with [MyGo](https://github.com/egoist/mygo) (Go backend, TypeScript page, system WebView2).
It converts audio with `ffmpeg`, extracts video frames, downloads videos with `yt-dlp`, and downloads Roblox assets.
The Rust/egui version of the audio editor lives on the `main` branch.

## Tabs

| Tab | What it does |
|---|---|
| Audio | mp3, wav, flac, ogg, m4a, opus. Bitrate 64–320 kbps (off for wav/flac), speed 0.25×–4× (pitch kept, chained `atempo`), volume ±20 dB, mono/stereo/keep. Input: a file or a URL (yt-dlp). Progress from `ffmpeg -progress`. |
| Video ke Gambar | PNG or JPG frames (JPG quality 1–100), 0.5–60 fps or every frame, into a new folder (never overwrites). Cancel removes the partial folder. |
| Unduh Video | One video via yt-dlp: best/1080/720/480/360, mp4 or as-is, saved to Downloads under its title. |
| Aset Roblox | Anonymous check and download of a Roblox asset, or a pasted list. See below. |

Themes (picker in the sidebar, saved in `%APPDATA%\Rokit\settings.json`): Otomatis, Apple Terang, Gelap, Material Terang, Material Gelap, OLED, Kaca.

## Aset Roblox

Paste an asset **ID**, `rbxassetid://ID`, or a URL on `roblox.com`, `www.roblox.com`, `create.roblox.com` or `assetdelivery.roblox.com`
(`/library/ID`, `/catalog/ID`, `/marketplace/asset/ID`, `/store/asset/ID`, `?id=ID`). Any other host or scheme is rejected.

- **No login, ever.** The app only calls Roblox's public endpoints without cookies or tokens. It never asks for, reads or sends a `.ROBLOSECURITY` cookie.
  Private, age-gated or removed assets stop with a clear message; nothing tries to get around access controls.
- Endpoints: `economy.roblox.com/v2/assets/ID/details` (name, type, creator) and `assetdelivery.roblox.com/v2/assetId/ID` (download location).
  Roblox reports delivery errors as HTTP 200 with an `errors` list (401 login needed, 403/409 not authorized, 404 not found).
- Downloads are followed only to `https://*.rbxcdn.com`.
- Saved to `Downloads\Rokit-Roblox\<id>-<name>.<ext>`, never overwriting (`(1)`, `(2)`, …).
- Extension comes from the file's real bytes: audio `.ogg`/`.mp3`, images `.png`, meshes `.mesh` (raw Roblox format), models/hats/accessories `.rbxmx` or `.rbxm` (raw, open in Roblox Studio).
- **Decal / Face / Shirt / Pants / T-Shirt** are small public XML wrappers. The app reads the image ID inside and downloads that image as PNG, and says "Decal -> gambar ID xxx". A Decal's ID is not its Image's ID.
- Audio: "Buka di tab Audio" loads the saved file as the Audio tab's input.
- Use only assets you own or whose license allows it, and follow Roblox's Terms of Use.

### Limits

| Limit | Value |
|---|---|
| Asset size | 200 MB |
| Metadata/delivery request timeout | 20 s each |
| One asset, all requests | 3 min |
| Wrapper XML read for Decal etc. | 1 MB |
| "Unduh semua" list | max 50 lines, one per line, sequential |
| "Unduh semua" pace | 1 request per second (each asset needs about 3 requests) |
| HTTP 429 | single download shows "Terlalu banyak permintaan, coba lagi nanti"; a list stops and marks the rest skipped |
| User-Agent | `Rokit-Asset-Downloader/1.0` |

What works without login (checked live): Audio, Image, Mesh, public Decals, Shirt/Pants templates, Hats and accessories.
Many Decals, Images, Models and Places answer 401/403/409 and cannot be downloaded without login.

## Requirements

- Windows 10/11 with WebView2 (preinstalled on Windows 11).
- `ffmpeg` and `ffprobe` on PATH: `winget install Gyan.FFmpeg`
- `yt-dlp` on PATH (URL download, Unduh Video): `winget install yt-dlp.yt-dlp`

## Run the built app

```
build\windows-amd64\Rokit Audio Editor.exe          (portable)
build\windows-amd64\rokit-audio-editor-setup-0.1.0-amd64.exe   (installer)
```

## Develop and build

Needs Go 1.27+, Node 24+.

```
npm install
npx mygo dev        # live reload window
npx mygo build      # writes build\windows-amd64\
go test ./...       # unit tests
npm run typecheck
ROKIT_LIVE=1 go test -run TestLive -v ./...   # real Roblox endpoints (needs network)
```

`npx mygo generate` rewrites `src/mygo.ts` from the Go service when its methods change.

The window has no native title bar: the page draws macOS-style buttons (MyGo `TitleBarHidden` + `HideWindowButtons`), which keeps the native resize edge, shadow and corners.
