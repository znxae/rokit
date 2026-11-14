#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use eframe::egui::{self, Color32, FontId, Margin, RichText, Rounding, Shadow, Stroke, TextStyle};
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::mpsc::{channel, Receiver, Sender};

const FORMATS: [&str; 6] = ["mp3", "wav", "flac", "ogg", "m4a", "opus"];
const BITRATES: [u32; 7] = [64, 96, 128, 160, 192, 256, 320];
const INSTALL_MSG: &str =
    "ffmpeg was not found on PATH. Install it (Windows: `winget install Gyan.FFmpeg`; macOS: `brew install ffmpeg`; Linux: your package manager), then restart this app.";

// Material 3 dark palette.
const BG: Color32 = Color32::from_rgb(0x1C, 0x1B, 0x1F);
const CARD: Color32 = Color32::from_rgb(0x2B, 0x29, 0x30);
const WIDGET: Color32 = Color32::from_rgb(0x36, 0x34, 0x3B);
const WIDGET_HOVER: Color32 = Color32::from_rgb(0x41, 0x3E, 0x48);
const OUTLINE: Color32 = Color32::from_rgb(0x49, 0x45, 0x4F);
const TEXT: Color32 = Color32::from_rgb(0xE6, 0xE1, 0xE5);
const PRIMARY: Color32 = Color32::from_rgb(0xD0, 0xBC, 0xFF);
const ON_PRIMARY: Color32 = Color32::from_rgb(0x38, 0x1E, 0x72);
const ERROR: Color32 = Color32::from_rgb(0xF2, 0xB8, 0xB5);

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

enum Msg {
    FfmpegMissing,
    Done(Result<(), String>),
}

fn run_ffmpeg(args: Vec<String>) -> Result<(), String> {
    match Command::new("ffmpeg").args(&args).output() {
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Err(INSTALL_MSG.into()),
        Err(e) => Err(format!("Could not start ffmpeg: {e}")),
        Ok(out) if out.status.success() => Ok(()),
        Ok(out) => Err(String::from_utf8_lossy(&out.stderr).into_owned()),
    }
}

fn apply_style(ctx: &egui::Context) {
    let mut s = (*ctx.style()).clone();
    s.text_styles = [
        (TextStyle::Heading, FontId::proportional(24.0)),
        (TextStyle::Body, FontId::proportional(16.0)),
        (TextStyle::Button, FontId::proportional(16.0)),
        (TextStyle::Small, FontId::proportional(13.0)),
        (TextStyle::Monospace, FontId::monospace(14.0)),
    ]
    .into();
    s.spacing.item_spacing = egui::vec2(10.0, 12.0);
    s.spacing.button_padding = egui::vec2(16.0, 9.0);
    s.spacing.interact_size.y = 36.0;
    s.spacing.slider_width = 240.0;
    let v = &mut s.visuals;
    *v = egui::Visuals::dark();
    v.override_text_color = Some(TEXT);
    v.panel_fill = BG;
    v.window_fill = CARD;
    v.extreme_bg_color = WIDGET;
    v.window_rounding = Rounding::same(16.0);
    v.menu_rounding = Rounding::same(12.0);
    v.popup_shadow = Shadow { offset: egui::vec2(0.0, 4.0), blur: 16.0, spread: 0.0, color: Color32::from_black_alpha(100) };
    v.selection.bg_fill = PRIMARY;
    v.selection.stroke = Stroke::new(1.0_f32, ON_PRIMARY);
    let r = Rounding::same(12.0);
    for (w, fill) in [
        (&mut v.widgets.noninteractive, CARD),
        (&mut v.widgets.inactive, WIDGET),
        (&mut v.widgets.hovered, WIDGET_HOVER),
        (&mut v.widgets.active, WIDGET_HOVER),
        (&mut v.widgets.open, WIDGET),
    ] {
        w.rounding = r;
        w.bg_fill = fill;
        w.weak_bg_fill = fill;
        w.bg_stroke = Stroke::new(1.0_f32, OUTLINE);
        w.fg_stroke = Stroke::new(1.0_f32, TEXT);
    }
    v.widgets.hovered.bg_stroke = Stroke::new(1.0_f32, PRIMARY);
    v.widgets.active.bg_stroke = Stroke::new(1.5_f32, PRIMARY);
    ctx.set_style(s);
}

/// A rounded, softly shadowed Material-style card with a section label.
fn card(ui: &mut egui::Ui, title: &str, add: impl FnOnce(&mut egui::Ui)) {
    let w = ui.available_width();
    egui::Frame::none()
        .fill(CARD)
        .rounding(Rounding::same(16.0))
        .stroke(Stroke::new(1.0_f32, OUTLINE))
        .inner_margin(Margin::same(16.0))
        .shadow(Shadow { offset: egui::vec2(0.0, 2.0), blur: 8.0, spread: 0.0, color: Color32::from_black_alpha(80) })
        .show(ui, |ui| {
            ui.set_width(w - 34.0);
            ui.label(RichText::new(title.to_uppercase()).size(12.0).strong().color(PRIMARY));
            add(ui);
        });
}

struct App {
    input: Option<PathBuf>,
    output: PathBuf,
    opts: Opts,
    busy: bool,
    status: String,
    error: String,
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
            if Command::new("ffmpeg").arg("-version").output().is_err() {
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
            tx,
            rx,
        }
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
                Msg::Done(r) => {
                    self.busy = false;
                    match r {
                        Ok(()) => self.status = format!("Done: {}", self.output.display()),
                        Err(e) => {
                            self.status = "Failed.".into();
                            self.error = e;
                        }
                    }
                }
            }
        }
        egui::CentralPanel::default()
            .frame(egui::Frame::none().fill(BG).inner_margin(Margin::same(20.0)))
            .show(ctx, |ui| {
                egui::ScrollArea::vertical().show(ui, |ui| {
                    ui.label(RichText::new("Audio Editor").heading().color(PRIMARY));
                    ui.add_enabled_ui(!self.busy, |ui| {
                        card(ui, "Input", |ui| {
                            ui.horizontal(|ui| {
                                if ui.button("Open file…").clicked() {
                                    if let Some(p) = rfd::FileDialog::new()
                                        .add_filter("Audio/video", &["mp3", "wav", "flac", "ogg", "m4a", "opus", "aac", "wma", "mp4", "mkv", "webm"])
                                        .pick_file()
                                    {
                                        self.output = default_output(&p, self.opts.format);
                                        self.status = "Ready.".into();
                                        self.input = Some(p);
                                    }
                                }
                                let name = self.input.as_ref().and_then(|p| p.file_name()).map(|n| n.to_string_lossy().into_owned());
                                ui.label(name.unwrap_or_else(|| "(none)".into()));
                            });
                        });
                        card(ui, "Format & bitrate", |ui| {
                            egui::ComboBox::from_label("Format").selected_text(self.opts.format).show_ui(ui, |ui| {
                                for f in FORMATS {
                                    if ui.selectable_value(&mut self.opts.format, f, f).clicked() && self.input.is_some() {
                                        self.output.set_extension(f);
                                    }
                                }
                            });
                            ui.add_enabled_ui(!is_lossless(self.opts.format), |ui| {
                                egui::ComboBox::from_label("Bitrate")
                                    .selected_text(format!("{} kbps", self.opts.bitrate))
                                    .show_ui(ui, |ui| {
                                        for b in BITRATES {
                                            ui.selectable_value(&mut self.opts.bitrate, b, format!("{b} kbps"));
                                        }
                                    });
                            });
                        });
                        card(ui, "Speed", |ui| {
                            ui.add(egui::Slider::new(&mut self.opts.speed, 0.25..=4.0).text("×").step_by(0.05));
                            ui.label(RichText::new(speed_hint(self.opts.speed)).small().color(PRIMARY));
                        });
                        card(ui, "Amplify", |ui| {
                            ui.add(egui::Slider::new(&mut self.opts.gain_db, -20.0..=20.0).text("dB").step_by(0.5));
                        });
                        card(ui, "Channels", |ui| {
                            ui.horizontal(|ui| {
                                ui.radio_value(&mut self.opts.channels, Channels::Keep, "Keep");
                                ui.radio_value(&mut self.opts.channels, Channels::Mono, "Mono");
                                ui.radio_value(&mut self.opts.channels, Channels::Stereo, "Stereo");
                            });
                        });
                        card(ui, "Output", |ui| {
                            if ui.button("Save as…").clicked() && self.input.is_some() {
                                if let Some(p) = rfd::FileDialog::new()
                                    .set_file_name(self.output.file_name().unwrap_or_default().to_string_lossy())
                                    .save_file()
                                {
                                    self.output = p;
                                }
                            }
                            ui.label(self.output.display().to_string());
                        });
                        let btn = egui::Button::new(RichText::new("Convert").size(18.0).strong().color(ON_PRIMARY))
                            .fill(PRIMARY)
                            .rounding(Rounding::same(24.0))
                            .min_size(egui::vec2(ui.available_width(), 48.0));
                        if ui.add_enabled(self.input.is_some(), btn).clicked() {
                            self.convert(ctx);
                        }
                    });
                    ui.horizontal(|ui| {
                        ui.label(&self.status);
                        if self.busy {
                            ui.spinner();
                        }
                    });
                    if !self.error.is_empty() {
                        ui.colored_label(ERROR, &self.error);
                    }
                });
            });
    }
}

fn main() -> eframe::Result {
    let opts = eframe::NativeOptions {
        viewport: egui::ViewportBuilder::default().with_inner_size([520.0, 900.0]),
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
