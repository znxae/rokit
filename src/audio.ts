import { Channel } from "mygo-runtime";
import { Rokit, type ConvProgress, type Opts } from "./mygo";
import { $, baseName, errText, fmtDb, fmtDuration, fmtSpeed, segmented, segValue, setProgress, setSegDisabled, setStatus, showError } from "./ui";

const LOSSLESS = new Set(["wav", "flac"]);
const BARS = 88;

export function initAudio() {
  const tab = $("#tab-audio");
  const pick = $<HTMLButtonElement>("#pick", tab);
  const urlInput = $<HTMLInputElement>("#url", tab);
  const fetchBtn = $<HTMLButtonElement>("#fetch", tab);
  const urlErr = $("#url-err", tab);
  const format = $<HTMLSelectElement>("#format", tab);
  const bitrate = $<HTMLSelectElement>("#bitrate", tab);
  const rowBitrate = $("#row-bitrate", tab);
  const channels = $("#channels", tab);
  const speed = $<HTMLInputElement>("#speed", tab);
  const gain = $<HTMLInputElement>("#gain", tab);
  const reset = $<HTMLButtonElement>("#reset", tab);
  const save = $<HTMLButtonElement>("#save", tab);
  const outPath = $("#out-path", tab);
  const convert = $<HTMLButtonElement>("#convert", tab);
  const status = $("#status", tab);
  const error = $("#error", tab);
  const wave = $("#wave", tab);

  // A quiet, fixed waveform shape: it only shows progress, it is not the audio's real waveform.
  for (let i = 0; i < BARS; i++) {
    const h = 22 + 60 * Math.abs(Math.sin(i * 0.37) * Math.cos(i * 0.11 + 1)) + 14 * Math.abs(Math.sin(i * 1.9));
    const bar = document.createElement("i");
    bar.style.height = `${Math.min(100, h)}%`;
    bar.style.animationDelay = `${i * 6}ms`;
    wave.append(bar);
  }
  const fillWave = (pct: number) => {
    const n = Math.round((BARS * pct) / 100);
    [...wave.children].forEach((b, k) => b.classList.toggle("p", k < n));
  };

  let input = "";
  let output = "";
  let busy = false;
  let urlOk = false;

  function render() {
    const lossless = LOSSLESS.has(format.value);
    rowBitrate.classList.toggle("off", lossless);
    bitrate.disabled = busy || lossless;
    for (const el of [pick, urlInput, format, speed, gain, reset]) el.disabled = busy;
    setSegDisabled(channels, busy);
    reset.hidden = Number(speed.value) === 1 && Number(gain.value) === 0;
    fetchBtn.disabled = busy || !urlOk;
    save.disabled = busy || !input;
    convert.disabled = busy || !input || !output;
    outPath.textContent = output || "—";
    outPath.title = output;
  }

  /** Any change to the file or settings makes an earlier result stale. */
  function stale() {
    fillWave(0);
    if (!busy && input) setStatus(status, "", "Belum dikonversi");
  }

  async function refreshUrl() {
    const v = urlInput.value.trim();
    urlOk = v !== "" && (await Rokit.isURL(v));
    urlErr.hidden = v === "" || urlOk;
    render();
  }

  async function run(label: string, fn: () => Promise<void>) {
    busy = true;
    showError(error, "");
    setStatus(status, "busy", label);
    render();
    try {
      await fn();
    } catch (e) {
      setStatus(status, "fail", "Gagal.");
      showError(error, errText(e));
    } finally {
      busy = false;
      render();
    }
  }

  async function setInput(path: string, title = "") {
    input = path;
    output = await Rokit.suggestOutput(path, format.value);
    const name = title || baseName(path);
    const ext = (path.split(".").pop() ?? "").toUpperCase();
    $("#fname", tab).textContent = name;
    $("#fmeta", tab).textContent = ext;
    pick.textContent = "Ganti file";
    wave.hidden = false;
    fillWave(0);
    try {
      const info = await Rokit.probeVideo(path);
      if (info.duration > 0) $("#fmeta", tab).textContent = `${ext} · ${fmtDuration(info.duration)}`;
    } catch {
      /* the file still works without the extra line */
    }
  }

  pick.addEventListener("click", () =>
    void run("Membuka file…", async () => {
      const path = await Rokit.pickFile();
      if (!path) {
        setStatus(status, "", input ? "Belum dikonversi" : "Pilih file input atau tempel URL.");
        return;
      }
      await setInput(path);
      setStatus(status, "", "Belum dikonversi");
    }),
  );

  urlInput.addEventListener("input", () => void refreshUrl());
  $("#url-form", tab).addEventListener("submit", (e) => {
    e.preventDefault();
    if (fetchBtn.disabled) return;
    void run("Mengunduh…", async () => {
      const d = await Rokit.download(urlInput.value);
      await setInput(d.path, d.title);
      setStatus(status, "", "Belum dikonversi");
    });
  });

  format.addEventListener("change", async () => {
    if (input) output = await Rokit.suggestOutput(input, format.value);
    render();
    stale();
  });
  segmented(channels, stale);
  bitrate.addEventListener("change", stale);

  const syncValues = async () => {
    const v = Number(speed.value);
    $("#speed-val", tab).textContent = fmtSpeed(v);
    $("#gain-val", tab).textContent = fmtDb(Number(gain.value));
    $("#speed-hint", tab).textContent = await Rokit.hint(v); // the same rule the tests cover
    render();
    stale();
  };
  speed.addEventListener("input", () => void syncValues());
  gain.addEventListener("input", () => void syncValues());
  reset.addEventListener("click", () => {
    speed.value = "1";
    gain.value = "0";
    void syncValues();
  });

  save.addEventListener("click", async () => {
    const p = await Rokit.pickOutput(output);
    if (p) {
      output = p;
      render();
    }
  });

  convert.addEventListener("click", () =>
    void run("Mengonversi…", async () => {
      const opts: Opts = {
        format: format.value,
        bitrate: Number(bitrate.value),
        speed: Number(speed.value),
        gainDb: Number(gain.value),
        channels: segValue(channels),
      };
      const wasDownload = await Rokit.isDownload(input);
      const progress = new Channel<ConvProgress>((p) => {
        fillWave(p.percent);
        setProgress(status, p.percent, "Mengonversi…", `${Math.round(p.percent)}%`);
      });
      setProgress(status, 0, "Mengonversi…", "0%");
      await Rokit.convert(input, output, opts, progress);
      fillWave(100);
      setStatus(status, "ok", `Selesai — tersimpan di ${output}`);
      if (wasDownload) input = ""; // the backend deleted the temp download
    }),
  );

  render();
}
