#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use eframe::egui;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::mpsc::{channel, Receiver, Sender};

const FORMATS: [&str; 6] = ["mp3", "wav", "flac", "ogg", "m4a", "opus"];
const BITRATES: [u32; 7] = [64, 96, 128, 160, 192, 256, 320];
const INSTALL_MSG: &str =
    "ffmpeg was not found on PATH. Install it (Windows: `winget install Gyan.FFmpeg`; macOS: `brew install ffmpeg`; Linux: your package manager), then restart this app.";

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

/// atempo only accepts 0.5..=2.0 per instance, so larger speeds are chained.
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

/// Pure: everything ffmpeg needs, in order. No `-y`-free prompting: stdin is closed by the caller.
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
        egui::CentralPanel::default().show(ctx, |ui| {
            ui.add_enabled_ui(!self.busy, |ui| {
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
                ui.separator();
                egui::ComboBox::from_label("Format")
                    .selected_text(self.opts.format)
                    .show_ui(ui, |ui| {
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
                ui.add(egui::Slider::new(&mut self.opts.speed, 0.5..=4.0).text("Speed ×").step_by(0.05));
                ui.add(egui::Slider::new(&mut self.opts.gain_db, -20.0..=20.0).text("Amplify dB").step_by(0.5));
                ui.horizontal(|ui| {
                    ui.label("Channels:");
                    ui.radio_value(&mut self.opts.channels, Channels::Keep, "Keep");
                    ui.radio_value(&mut self.opts.channels, Channels::Mono, "Mono");
                    ui.radio_value(&mut self.opts.channels, Channels::Stereo, "Stereo");
                });
                ui.separator();
                ui.horizontal(|ui| {
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
                if ui.add_enabled(self.input.is_some(), egui::Button::new("Convert")).clicked() {
                    self.convert(ctx);
                }
            });
            ui.label(&self.status);
            if self.busy {
                ui.spinner();
            }
            if !self.error.is_empty() {
                egui::ScrollArea::vertical().show(ui, |ui| {
                    ui.colored_label(egui::Color32::LIGHT_RED, &self.error);
                });
            }
        });
    }
}

fn main() -> eframe::Result {
    let opts = eframe::NativeOptions {
        viewport: egui::ViewportBuilder::default().with_inner_size([520.0, 420.0]),
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
        let mut x = o("mp3");
        x.speed = 4.0;
        assert!(has(&build_args("i", "o", &x), ["-af", "atempo=2,atempo=2"]));
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
}
