#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use eframe::egui::{self, Align2, Color32, FontId, Margin, RichText, Rounding, Sense, Shadow, Shape, Stroke, TextStyle};
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::mpsc::{channel, Receiver, Sender};

const FORMATS: [&str; 6] = ["mp3", "wav", "flac", "ogg", "m4a", "opus"];
const BITRATES: [u32; 7] = [64, 96, 128, 160, 192, 256, 320];
const INSTALL_MSG: &str =
    "ffmpeg was not found on PATH. Install it (Windows: `winget install Gyan.FFmpeg`; macOS: `brew install ffmpeg`; Linux: your package manager), then restart this app.";

// Light theme, single teal accent.
const BG: Color32 = Color32::from_rgb(0xF4, 0xF6, 0xF8);
const CARD: Color32 = Color32::WHITE;
const CHIP: Color32 = Color32::from_rgb(0xF2, 0xF4, 0xF7);
const BORDER: Color32 = Color32::from_rgb(0xE3, 0xE7, 0xEB);
const TEXT: Color32 = Color32::from_rgb(0x10, 0x18, 0x28);
const MUTED: Color32 = Color32::from_rgb(0x66, 0x70, 0x85);
const ACCENT: Color32 = Color32::from_rgb(0x0F, 0x76, 0x6E);
const ACCENT_HOVER: Color32 = Color32::from_rgb(0x11, 0x5E, 0x59);
const ERROR: Color32 = Color32::from_rgb(0xB4, 0x23, 0x18);
const OK: Color32 = Color32::from_rgb(0x06, 0x76, 0x47);


#[derive(Clone, Copy, PartialEq, Debug)]
enum Channels {
    Keep,
    Mono,
    Stereo,
}

#[derive(Clone, Debug)]
struct Opts {
    format: &'static str,
    bitrate: u32,
    speed: f64,
    gain_db: f64,
    channels: Channels,
}

fn is_lossless(fmt: &str) -> bool {
    matches!(fmt, "wav" | "flac")
}

fn codec(fmt: &str) -> &'static str {
    match fmt {
        "mp3" => "libmp3lame",
        "wav" => "pcm_s16le",
        "flac" => "flac",
        "ogg" => "libvorbis",
        "m4a" => "aac",
        _ => "libopus",
    }
}

/// atempo only accepts 0.5..=2.0 per instance, so larger/smaller speeds are chained.
fn atempo_chain(mut speed: f64) -> Vec<String> {
    let mut v = Vec::new();
    while speed > 2.0 {
        v.push("atempo=2".to_string());
        speed /= 2.0;
    }
    while speed < 0.5 {
        v.push("atempo=0.5".to_string());
        speed /= 0.5;
    }
    v.push(format!("atempo={}", speed));
    v
}

/// Pure: the full ffmpeg argument list, in order (stdin is closed via -nostdin).
fn build_args(input: &str, output: &str, o: &Opts) -> Vec<String> {
    let mut a: Vec<String> = ["-hide_banner", "-nostdin", "-y", "-i", input, "-vn"]
        .iter()
        .map(|s| s.to_string())
        .collect();
    let mut filters = Vec::new();
    if (o.speed - 1.0).abs() > 1e-9 {
        filters.extend(atempo_chain(o.speed));
    }
    if o.gain_db.abs() > 1e-9 {
        filters.push(format!("volume={}dB", o.gain_db));
    }
    if !filters.is_empty() {
        a.push("-af".into());
        a.push(filters.join(","));
    }
    match o.channels {
        Channels::Keep => {}
        Channels::Mono => a.extend(["-ac".into(), "1".into()]),
        Channels::Stereo => a.extend(["-ac".into(), "2".into()]),
    }
    a.extend(["-c:a".into(), codec(o.format).into()]);
    if !is_lossless(o.format) {
        a.extend(["-b:a".into(), format!("{}k", o.bitrate)]);
    }
    a.push(output.into());
    a
}

/// Pure: hint telling which speed undoes `speed` (1/speed, 3 decimals).
fn speed_hint(speed: f64) -> String {
    if (speed - 1.0).abs() < 5e-4 {
        return "Speed normal (1x)".into();
    }
    let inv = 1.0 / speed;
    let dir = if inv > 1.0 { "lebih cepat" } else { "lebih lambat" };
    format!("Untuk kembali normal: ubah speed ke {inv:.3}x ({dir})")
}

fn default_output(input: &Path, fmt: &str) -> PathBuf {
    let mut p = input.with_extension(fmt);
    if p == input {
        let stem = input.file_stem().unwrap_or_default().to_string_lossy();
        p = input.with_file_name(format!("{stem}_out.{fmt}"));
    }
    p
}

const YTDLP_MSG: &str =
    "yt-dlp was not found on PATH. Install it (Windows: `winget install yt-dlp.yt-dlp`), then restart this app.";

/// Child processes must not flash a console window in the GUI build.
fn cmd(name: &str) -> Command {
    let mut c = Command::new(name);
    #[cfg(windows)]
    std::os::windows::process::CommandExt::creation_flags(&mut c, 0x0800_0000);
    c
}

/// Only plain http(s) URLs without whitespace/control chars are handed to yt-dlp.
fn valid_url(s: &str) -> bool {
    let s = s.trim();
    (s.starts_with("http://") || s.starts_with("https://"))
        && s.len() > 8
        && !s.chars().any(|c| c.is_whitespace() || c.is_control())
}

/// shortcut: Windows reserved names (CON, NUL, ...) are not special-cased; add if titles ever hit them.
fn sanitize_filename(s: &str) -> String {
    let c: String = s
        .chars()
        .map(|c| if c.is_control() || "<>:\"/\\|?*".contains(c) { '_' } else { c })
        .collect();
    let t: String = c.trim().trim_matches('.').chars().take(80).collect();
    let t = t.trim().trim_end_matches('.').to_string();
    if t.is_empty() { "audio".into() } else { t }
}

/// dir/stem.ext, or dir/stem (1).ext, (2)... so nothing existing is overwritten.
fn unique_path(dir: &Path, stem: &str, ext: &str) -> PathBuf {
    let p = dir.join(format!("{stem}.{ext}"));
    if !p.exists() {
        return p;
    }
    (1..)
        .map(|n| dir.join(format!("{stem} ({n}).{ext}")))
        .find(|c| !c.exists())
        .unwrap()
}

fn downloads_dir() -> PathBuf {
    std::env::var_os("USERPROFILE")
        .or_else(|| std::env::var_os("HOME"))
        .map(|h| PathBuf::from(h).join("Downloads"))
        .filter(|p| p.is_dir())
        .unwrap_or_else(std::env::temp_dir)
}

/// Downloads best audio of `url` into a fresh temp dir; returns the file path.
fn run_ytdlp(url: &str) -> Result<PathBuf, String> {
    let nanos = std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).map(|d| d.as_nanos()).unwrap_or(0);
    let dir = std::env::temp_dir().join(format!("rokit_dl_{}_{}", std::process::id(), nanos));
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    let template = format!("{}/%(title).80s.%(ext)s", dir.to_string_lossy());
    let out = match cmd("yt-dlp")
        .env("PYTHONIOENCODING", "utf-8")
        .args(["-f", "bestaudio/best", "--no-playlist", "--no-simulate", "--windows-filenames", "-o", &template, "--print", "after_move:filepath", "--", url])
        .output()
    {
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Err(YTDLP_MSG.into()),
        Err(e) => return Err(format!("Could not start yt-dlp: {e}")),
        Ok(o) => o,
    };
    if !out.status.success() {
        let _ = std::fs::remove_dir_all(&dir);
        return Err(String::from_utf8_lossy(&out.stderr).into_owned());
    }
    let printed = String::from_utf8_lossy(&out.stdout).lines().rev().find(|l| !l.trim().is_empty()).map(|l| PathBuf::from(l.trim()));
    // Fall back to the dir's only file if the printed path was mangled by console encoding.
    printed
        .filter(|p| p.is_file())
        .or_else(|| std::fs::read_dir(&dir).ok()?.flatten().map(|e| e.path()).find(|p| p.is_file()))
        .ok_or_else(|| "yt-dlp finished but produced no file.".to_string())
}

enum Msg {
    FfmpegMissing,
    Downloaded(Result<PathBuf, String>),
    Done(Result<(), String>),
}

fn run_ffmpeg(args: Vec<String>) -> Result<(), String> {
    match cmd("ffmpeg").args(&args).output() {
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Err(INSTALL_MSG.into()),
        Err(e) => Err(format!("Could not start ffmpeg: {e}")),
        Ok(out) if out.status.success() => Ok(()),
        Ok(out) => Err(String::from_utf8_lossy(&out.stderr).into_owned()),
    }
}

fn apply_style(ctx: &egui::Context) {
    let mut s = (*ctx.style()).clone();
    s.text_styles = [
        (TextStyle::Heading, FontId::proportional(22.0)),
        (TextStyle::Body, FontId::proportional(15.0)),
        (TextStyle::Button, FontId::proportional(15.0)),
        (TextStyle::Small, FontId::proportional(13.0)),
        (TextStyle::Monospace, FontId::monospace(14.0)),
    ]
    .into();
    s.spacing.item_spacing = egui::vec2(10.0, 10.0);
    s.spacing.button_padding = egui::vec2(14.0, 8.0);
    s.spacing.interact_size.y = 34.0;
    let v = &mut s.visuals;
    *v = egui::Visuals::light();
    v.override_text_color = Some(TEXT);
    v.panel_fill = BG;
    v.window_fill = CARD;
    v.extreme_bg_color = CARD;
    v.window_rounding = Rounding::same(14.0);
    v.menu_rounding = Rounding::same(10.0);
    v.window_shadow = Shadow { offset: egui::vec2(0.0, 2.0), blur: 8.0, spread: 0.0, color: Color32::from_black_alpha(20) };
    v.popup_shadow = v.window_shadow;
    v.slider_trailing_fill = true;
    v.selection.bg_fill = ACCENT;
    v.selection.stroke = Stroke::new(1.0_f32, Color32::WHITE);
    for (w, fill) in [
        (&mut v.widgets.noninteractive, CARD),
        (&mut v.widgets.inactive, CARD),
        (&mut v.widgets.hovered, CHIP),
        (&mut v.widgets.active, CHIP),
        (&mut v.widgets.open, CHIP),
    ] {
        w.rounding = Rounding::same(10.0);
        w.bg_fill = fill;
        w.weak_bg_fill = fill;
        w.bg_stroke = Stroke::new(1.0_f32, BORDER);
        w.fg_stroke = Stroke::new(1.0_f32, TEXT);
    }
    // bg_fill is the slider rail; buttons use weak_bg_fill.
    v.widgets.inactive.bg_fill = BORDER;
    v.widgets.hovered.bg_fill = BORDER;
    v.widgets.active.bg_fill = BORDER;
    v.widgets.hovered.bg_stroke = Stroke::new(1.0_f32, ACCENT);
    v.widgets.active.bg_stroke = Stroke::new(1.5_f32, ACCENT);
    ctx.set_style(s);
}

/// White rounded card with a very soft shadow and a small uppercase muted label.
fn card(ui: &mut egui::Ui, title: &str, add: impl FnOnce(&mut egui::Ui)) {
    let w = ui.available_width();
    egui::Frame::none()
        .fill(CARD)
        .rounding(Rounding::same(14.0))
        .stroke(Stroke::new(1.0_f32, BORDER))
        .inner_margin(Margin::same(16.0))
        .shadow(Shadow { offset: egui::vec2(0.0, 1.0), blur: 3.0, spread: 0.0, color: Color32::from_black_alpha(14) })
        .show(ui, |ui| {
            ui.set_width(w - 34.0);
            ui.label(RichText::new(title.to_uppercase()).size(13.0).color(MUTED));
            add(ui);
        });
}

fn chip(ui: &mut egui::Ui, text: String) {
    egui::Frame::none()
        .fill(CHIP)
        .rounding(Rounding::same(8.0))
        .inner_margin(Margin::symmetric(10.0, 4.0))
        .show(ui, |ui| ui.label(RichText::new(text).strong()));
}

/// Dashed, clickable drop-zone style button; returns true when clicked.
fn file_button(ui: &mut egui::Ui, name: Option<&str>) -> bool {
    let (rect, resp) = ui.allocate_exact_size(egui::vec2(ui.available_width(), 72.0), Sense::click());
    let hot = resp.hovered() && ui.is_enabled();
    let color = if hot { ACCENT } else { BORDER };
    ui.painter().rect_filled(rect, Rounding::same(10.0), if hot { CHIP } else { CARD });
    let r = rect.shrink(0.5);
    let pts = [r.left_top(), r.right_top(), r.right_bottom(), r.left_bottom(), r.left_top()];
    ui.painter().extend(Shape::dashed_line(&pts, Stroke::new(1.5_f32, color), 6.0, 4.0));
    let c = rect.center();
    match name {
        None => {
            ui.painter().text(c, Align2::CENTER_CENTER, "Pilih file audio", FontId::proportional(16.0), ACCENT);
        }
        Some(n) => {
            ui.painter().text(c - egui::vec2(0.0, 10.0), Align2::CENTER_CENTER, n, FontId::proportional(16.0), TEXT);
            ui.painter().text(c + egui::vec2(0.0, 12.0), Align2::CENTER_CENTER, "Klik untuk mengganti file", FontId::proportional(13.0), MUTED);
        }
    }
    resp.on_hover_cursor(egui::CursorIcon::PointingHand).clicked()
}

/// One segment of a segmented control.
fn segment(ui: &mut egui::Ui, cur: &mut Channels, val: Channels, label: &str) {
    let sel = *cur == val;
    let (fg, fill) = if sel { (Color32::WHITE, ACCENT) } else { (TEXT, CARD) };
    let b = egui::Button::new(RichText::new(label).color(fg))
        .fill(fill)
        .stroke(Stroke::new(1.0_f32, if sel { ACCENT } else { BORDER }))
        .min_size(egui::vec2(ui.available_width(), 36.0));
    if ui.add(b).clicked() {
        *cur = val;
    }
}


struct App {
    input: Option<PathBuf>,
    output: PathBuf,
    opts: Opts,
    busy: bool,
    status: String,
    error: String,
    url: String,
    /// Downloaded file we own and delete after a successful convert.
    temp_input: Option<PathBuf>,
    tx: Sender<Msg>,
    rx: Receiver<Msg>,
}

impl App {
    fn new(ctx: egui::Context) -> Self {
        apply_style(&ctx);
        let (tx, rx) = channel();
        // Early check so the install hint shows before the first conversion attempt.
        let (t, c) = (tx.clone(), ctx);
        std::thread::spawn(move || {
            if cmd("ffmpeg").arg("-version").output().is_err() {
                let _ = t.send(Msg::FfmpegMissing);
                c.request_repaint();
            }
        });
        Self {
            input: None,
            output: PathBuf::new(),
            opts: Opts { format: "mp3", bitrate: 192, speed: 1.0, gain_db: 0.0, channels: Channels::Keep },
            busy: false,
            status: "Pick an input file.".into(),
            error: String::new(),
            url: String::new(),
            temp_input: None,
            tx,
            rx,
        }
    }

    fn remove_temp(&mut self) {
        if let Some(p) = self.temp_input.take() {
            let _ = std::fs::remove_file(&p);
            if let Some(d) = p.parent() {
                let _ = std::fs::remove_dir(d); // only succeeds when empty
            }
        }
    }

    fn set_local_input(&mut self, p: PathBuf) {
        self.remove_temp();
        self.output = default_output(&p, self.opts.format);
        self.status = "Siap.".into();
        self.input = Some(p);
    }

    /// Output name after a format change; downloaded files get a fresh unique name in Downloads.
    fn refresh_output(&mut self) {
        match (&self.temp_input, &self.input) {
            (Some(_), Some(i)) => {
                let stem = i.file_stem().unwrap_or_default().to_string_lossy().into_owned();
                self.output = unique_path(&downloads_dir(), &sanitize_filename(&stem), self.opts.format);
            }
            _ => {
                self.output.set_extension(self.opts.format);
            }
        }
    }

    fn fetch(&mut self, ctx: &egui::Context) {
        let url = self.url.trim().to_string();
        if !valid_url(&url) {
            return;
        }
        self.busy = true;
        self.error.clear();
        self.status = "Mengunduh…".into();
        let (tx, ctx) = (self.tx.clone(), ctx.clone());
        std::thread::spawn(move || {
            let _ = tx.send(Msg::Downloaded(run_ytdlp(&url)));
            ctx.request_repaint();
        });
    }

    fn convert(&mut self, ctx: &egui::Context) {
        let Some(input) = self.input.clone() else { return };
        if self.output == input {
            self.error = "Output path equals the input file; choose a different one.".into();
            return;
        }
        let args = build_args(&input.to_string_lossy(), &self.output.to_string_lossy(), &self.opts);
        self.busy = true;
        self.error.clear();
        self.status = "Converting…".into();
        let (tx, ctx) = (self.tx.clone(), ctx.clone());
        std::thread::spawn(move || {
            let _ = tx.send(Msg::Done(run_ffmpeg(args)));
            ctx.request_repaint();
        });
    }
}

impl eframe::App for App {
    fn update(&mut self, ctx: &egui::Context, _: &mut eframe::Frame) {
        while let Ok(m) = self.rx.try_recv() {
            match m {
                Msg::FfmpegMissing => self.error = INSTALL_MSG.into(),
                Msg::Downloaded(r) => {
                    self.busy = false;
                    match r {
                        Ok(p) => {
                            self.remove_temp();
                            self.temp_input = Some(p.clone());
                            self.input = Some(p);
                            self.refresh_output();
                            self.status = "Siap (unduhan selesai).".into();
                        }
                        Err(e) => {
                            self.status = "Failed.".into();
                            self.error = e;
                        }
                    }
                }
                Msg::Done(r) => {
                    self.busy = false;
                    match r {
                        Ok(()) => {
                            self.status = format!("Done: {}", self.output.display());
                            if self.temp_input.is_some() {
                                self.remove_temp(); // temp download no longer needed
                                self.input = None;
                            }
                        }
                        Err(e) => {
                            self.status = "Failed.".into();
                            self.error = e;
                        }
                    }
                }
            }
        }
        // Single centered column, max 560px wide.
        let pad = ((ctx.screen_rect().width() - 560.0) / 2.0).max(20.0);
        egui::TopBottomPanel::bottom("go")
            .frame(egui::Frame::none().fill(BG).inner_margin(Margin { left: pad, right: pad, top: 8.0, bottom: 16.0 }))
            .show(ctx, |ui| {
                ui.spacing_mut().item_spacing.y = 10.0;
                    let go = ui
                        .add_enabled_ui(self.input.is_some() && !self.busy, |ui| {
                            let (rect, resp) = ui.allocate_exact_size(egui::vec2(ui.available_width(), 44.0), Sense::click());
                            let on = ui.is_enabled();
                            let fill = if !on {
                                BORDER
                            } else if resp.hovered() {
                                ACCENT_HOVER
                            } else {
                                ACCENT
                            };
                            ui.painter().rect_filled(rect, Rounding::same(10.0), fill);
                            let fg = if on { Color32::WHITE } else { MUTED };
                            ui.painter().text(rect.center(), Align2::CENTER_CENTER, "Convert", FontId::proportional(16.0), fg);
                            resp.clicked()
                        })
                        .inner;
                    if go {
                        self.convert(ctx);
                    }
                    ui.horizontal(|ui| {
                        let color = if self.status.starts_with("Done") {
                            OK
                        } else if self.status == "Failed." {
                            ERROR
                        } else {
                            MUTED
                        };
                        ui.label(RichText::new(&self.status).color(color));
                        if self.busy {
                            ui.spinner();
                        }
                    });
                    if !self.error.is_empty() {
                        ui.label(RichText::new(&self.error).color(ERROR));
                    }
            });
        egui::CentralPanel::default()
            .frame(egui::Frame::none().fill(BG).inner_margin(Margin { left: pad, right: pad, top: 24.0, bottom: 24.0 }))
            .show(ctx, |ui| {
                egui::ScrollArea::vertical().auto_shrink([false, false]).show(ui, |ui| {
                    ui.spacing_mut().item_spacing.y = 16.0;
                    ui.vertical(|ui| {
                        ui.spacing_mut().item_spacing.y = 2.0;
                        ui.label(RichText::new("Rokit Audio Editor").heading().strong());
                        ui.label(RichText::new("Konversi, ubah kecepatan, dan perkeras audio dengan ffmpeg").size(13.0).color(MUTED));
                    });
                    ui.add_enabled_ui(!self.busy, |ui| {
                        card(ui, "File input", |ui| {
                            let name = self.input.as_ref().and_then(|p| p.file_name()).map(|n| n.to_string_lossy().into_owned());
                            if file_button(ui, name.as_deref()) {
                                if let Some(p) = rfd::FileDialog::new()
                                    .add_filter("Audio/video", &["mp3", "wav", "flac", "ogg", "m4a", "opus", "aac", "wma", "mp4", "mkv", "webm"])
                                    .pick_file()
                                {
                                    self.set_local_input(p);
                                }
                            }
                            ui.horizontal(|ui| {
                                let w = (ui.available_width() - 90.0).max(80.0);
                                ui.add(
                                    egui::TextEdit::singleline(&mut self.url)
                                        .hint_text("atau tempel URL (YouTube, TikTok, …)")
                                        .desired_width(w),
                                );
                                if ui.add_enabled(valid_url(&self.url), egui::Button::new("Ambil")).clicked() {
                                    self.fetch(ctx);
                                }
                            });
                            if !self.url.trim().is_empty() && !valid_url(&self.url) {
                                ui.label(RichText::new("URL harus diawali http:// atau https://").size(13.0).color(ERROR));
                            }
                        });
                        card(ui, "Format & bitrate", |ui| {
                            ui.columns(2, |cols| {
                                cols[0].label(RichText::new("Format").size(13.0).color(MUTED));
                                egui::ComboBox::from_id_salt("fmt")
                                    .width(cols[0].available_width())
                                    .selected_text(self.opts.format)
                                    .show_ui(&mut cols[0], |ui| {
                                        for f in FORMATS {
                                            if ui.selectable_value(&mut self.opts.format, f, f).clicked() && self.input.is_some() {
                                                self.refresh_output();
                                            }
                                        }
                                    });
                                cols[1].label(RichText::new("Bitrate").size(13.0).color(MUTED));
                                cols[1].add_enabled_ui(!is_lossless(self.opts.format), |ui| {
                                    egui::ComboBox::from_id_salt("br")
                                        .width(ui.available_width())
                                        .selected_text(format!("{} kbps", self.opts.bitrate))
                                        .show_ui(ui, |ui| {
                                            for b in BITRATES {
                                                ui.selectable_value(&mut self.opts.bitrate, b, format!("{b} kbps"));
                                            }
                                        });
                                });
                            });
                        });
                        card(ui, "Kecepatan", |ui| {
                            ui.horizontal(|ui| {
                                ui.spacing_mut().slider_width = (ui.available_width() - 90.0).max(80.0);
                                ui.add(egui::Slider::new(&mut self.opts.speed, 0.25..=4.0).show_value(false).step_by(0.05));
                                chip(ui, format!("{:.2}×", self.opts.speed));
                            });
                            ui.label(RichText::new(speed_hint(self.opts.speed)).size(13.0).color(MUTED));
                        });
                        card(ui, "Perkeras suara", |ui| {
                            ui.horizontal(|ui| {
                                ui.spacing_mut().slider_width = (ui.available_width() - 90.0).max(80.0);
                                ui.add(egui::Slider::new(&mut self.opts.gain_db, -20.0..=20.0).show_value(false).step_by(0.5));
                                chip(ui, format!("{:+.1} dB", self.opts.gain_db));
                            });
                        });
                        card(ui, "Channel", |ui| {
                            ui.columns(3, |c| {
                                segment(&mut c[0], &mut self.opts.channels, Channels::Keep, "Asli");
                                segment(&mut c[1], &mut self.opts.channels, Channels::Mono, "Mono");
                                segment(&mut c[2], &mut self.opts.channels, Channels::Stereo, "Stereo");
                            });
                        });
                        card(ui, "File output", |ui| {
                            ui.horizontal_wrapped(|ui| {
                                if ui.button("Simpan sebagai…").clicked() && self.input.is_some() {
                                    if let Some(p) = rfd::FileDialog::new()
                                        .set_file_name(self.output.file_name().unwrap_or_default().to_string_lossy())
                                        .save_file()
                                    {
                                        self.output = p;
                                    }
                                }
                                ui.label(RichText::new(self.output.display().to_string()).size(13.0).color(MUTED));
                            });
                        });
                    });
                });
            });
    }
}


fn main() -> eframe::Result {
    let opts = eframe::NativeOptions {
        viewport: egui::ViewportBuilder::default().with_inner_size([620.0, 900.0]),
        ..Default::default()
    };
    eframe::run_native("Audio Editor", opts, Box::new(|cc| Ok(Box::new(App::new(cc.egui_ctx.clone())))))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn o(format: &'static str) -> Opts {
        Opts { format, bitrate: 128, speed: 1.0, gain_db: 0.0, channels: Channels::Keep }
    }
    fn has(a: &[String], pair: [&str; 2]) -> bool {
        a.windows(2).any(|w| w[0] == pair[0] && w[1] == pair[1])
    }

    #[test]
    fn codecs_per_format() {
        for (f, c) in [("mp3", "libmp3lame"), ("wav", "pcm_s16le"), ("flac", "flac"), ("ogg", "libvorbis"), ("m4a", "aac"), ("opus", "libopus")] {
            assert!(has(&build_args("i", "o", &o(f)), ["-c:a", c]), "{f}");
        }
    }

    #[test]
    fn bitrate_omitted_for_lossless() {
        assert!(!build_args("i", "o", &o("wav")).contains(&"-b:a".to_string()));
        assert!(!build_args("i", "o", &o("flac")).contains(&"-b:a".to_string()));
        assert!(has(&build_args("i", "o", &o("mp3")), ["-b:a", "128k"]));
    }

    #[test]
    fn atempo_chaining() {
        assert_eq!(atempo_chain(1.5), ["atempo=1.5"]);
        assert_eq!(atempo_chain(4.0), ["atempo=2", "atempo=2"]);
        assert_eq!(atempo_chain(3.0), ["atempo=2", "atempo=1.5"]);
        assert_eq!(atempo_chain(0.25), ["atempo=0.5", "atempo=0.5"]);
        let mut x = o("mp3");
        x.speed = 4.0;
        assert!(has(&build_args("i", "o", &x), ["-af", "atempo=2,atempo=2"]));
        x.speed = 0.25;
        assert!(has(&build_args("i", "o", &x), ["-af", "atempo=0.5,atempo=0.5"]));
    }

    #[test]
    fn volume_and_no_filter_by_default() {
        assert!(!build_args("i", "o", &o("mp3")).contains(&"-af".to_string()));
        let mut x = o("mp3");
        x.gain_db = -6.5;
        x.speed = 0.5;
        assert!(has(&build_args("i", "o", &x), ["-af", "atempo=0.5,volume=-6.5dB"]));
    }

    #[test]
    fn channels() {
        assert!(!build_args("i", "o", &o("mp3")).contains(&"-ac".to_string()));
        let mut x = o("mp3");
        x.channels = Channels::Mono;
        assert!(has(&build_args("i", "o", &x), ["-ac", "1"]));
        x.channels = Channels::Stereo;
        assert!(has(&build_args("i", "o", &x), ["-ac", "2"]));
    }

    #[test]
    fn default_output_never_input() {
        assert_eq!(default_output(Path::new("a/b.mp3"), "wav"), PathBuf::from("a/b.wav"));
        assert_eq!(default_output(Path::new("a/b.mp3"), "mp3"), PathBuf::from("a/b_out.mp3"));
    }

    #[test]
    fn speed_hint_inverse() {
        assert_eq!(speed_hint(1.0), "Speed normal (1x)");
        assert_eq!(speed_hint(1.5), "Untuk kembali normal: ubah speed ke 0.667x (lebih lambat)");
        assert_eq!(speed_hint(0.5), "Untuk kembali normal: ubah speed ke 2.000x (lebih cepat)");
        assert_eq!(speed_hint(4.0), "Untuk kembali normal: ubah speed ke 0.250x (lebih lambat)");
    }

    #[test]
    fn url_validation() {
        assert!(valid_url("https://www.youtube.com/watch?v=abc"));
        assert!(valid_url("  http://example.com/a  "));
        for bad in ["", "https://", "ftp://x.com/a", "javascript:alert(1)", "https://a.com/x y", "file:///C:/a.mp3", "www.youtube.com", "-o evil"] {
            assert!(!valid_url(bad), "{bad:?}");
        }
    }

    #[test]
    fn filename_sanitizing() {
        assert_eq!(sanitize_filename("a<b>:c\"d/e\\f|g?h*i"), "a_b__c_d_e_f_g_h_i");
        assert_eq!(sanitize_filename("  ..Title.. "), "Title");
        assert_eq!(sanitize_filename("???"), "___");
        assert_eq!(sanitize_filename("  . "), "audio");
        assert_eq!(sanitize_filename(&"x".repeat(200)).len(), 80);
        assert_eq!(sanitize_filename("tab\there"), "tab_here");
    }

    #[test]
    fn unique_path_never_exists() {
        let d = std::env::temp_dir().join(format!("ae_unique_{}", std::process::id()));
        std::fs::create_dir_all(&d).unwrap();
        assert_eq!(unique_path(&d, "t", "mp3"), d.join("t.mp3"));
        std::fs::write(d.join("t.mp3"), b"x").unwrap();
        assert_eq!(unique_path(&d, "t", "mp3"), d.join("t (1).mp3"));
        std::fs::write(d.join("t (1).mp3"), b"x").unwrap();
        assert_eq!(unique_path(&d, "t", "mp3"), d.join("t (2).mp3"));
        let _ = std::fs::remove_dir_all(&d);
    }

    fn probe(path: &str, sel: &str) -> f64 {
        let out = Command::new("ffprobe")
            .args(["-v", "error", "-select_streams", "a:0", "-show_entries", sel, "-of", "csv=p=0", path])
            .output()
            .expect("ffprobe");
        String::from_utf8_lossy(&out.stdout).trim().parse().unwrap_or_else(|_| panic!("probe {path}: {out:?}"))
    }

    /// Needs ffmpeg/ffprobe: `AE_TEST_INPUT=file cargo test -- --ignored`.
    #[test]
    #[ignore]
    fn real_ffmpeg_pipeline() {
        let input = std::env::var("AE_TEST_INPUT").expect("set AE_TEST_INPUT");
        let dir = std::env::temp_dir().join("ae_pipeline");
        std::fs::create_dir_all(&dir).unwrap();
        let src = probe(&input, "format=duration");
        let cases: [(&'static str, f64, f64, Channels, f64); 9] = [
            ("mp3", 1.0, 0.0, Channels::Keep, 2.0),
            ("wav", 1.0, 0.0, Channels::Keep, 2.0),
            ("flac", 1.0, 0.0, Channels::Keep, 2.0),
            ("ogg", 1.0, 0.0, Channels::Keep, 2.0),
            ("m4a", 1.0, 0.0, Channels::Keep, 2.0),
            ("opus", 1.0, 0.0, Channels::Keep, 2.0),
            ("mp3", 0.25, 0.0, Channels::Keep, 2.0),
            ("wav", 4.0, -6.0, Channels::Keep, 2.0),
            ("mp3", 1.5, 3.0, Channels::Mono, 1.0),
        ];
        for (i, (fmt, speed, gain_db, ch, want_ch)) in cases.into_iter().enumerate() {
            let out = dir.join(format!("case{i}.{fmt}")).to_string_lossy().into_owned();
            let o = Opts { format: fmt, bitrate: 128, speed, gain_db, channels: ch };
            run_ffmpeg(build_args(&input, &out, &o)).unwrap_or_else(|e| panic!("{fmt} x{speed}: {e}"));
            let dur = probe(&out, "format=duration");
            assert!((dur - src / speed).abs() < 0.4, "{fmt} x{speed}: duration {dur}, want {}", src / speed);
            assert_eq!(probe(&out, "stream=channels"), want_ch, "{fmt} channels");
            println!("ok {fmt} x{speed} {gain_db}dB -> {dur:.2}s");
        }
    }
}
