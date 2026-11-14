// Types and the Rokit service are generated from the Go code by `mygo generate`.
import { Rokit, type Opts } from "./mygo";

const $ = <T extends HTMLElement>(selector: string) => document.querySelector<T>(selector)!;

const pick = $<HTMLButtonElement>("#pick");
const urlForm = $<HTMLFormElement>("#url-form");
const urlInput = $<HTMLInputElement>("#url");
const fetchBtn = $<HTMLButtonElement>("#fetch");
const urlErr = $("#url-err");
const format = $<HTMLSelectElement>("#format");
const bitrate = $<HTMLSelectElement>("#bitrate");
const speed = $<HTMLInputElement>("#speed");
const gain = $<HTMLInputElement>("#gain");
const save = $<HTMLButtonElement>("#save");
const outPath = $("#out-path");
const convertBtn = $<HTMLButtonElement>("#convert");
const statusEl = $("#status");
const errorEl = $("#error");

let input = "";
let output = "";
let busy = false;

const LOSSLESS = new Set(["wav", "flac"]);

function setStatus(text: string, kind: "" | "ok" | "fail" | "busy" = "") {
  statusEl.textContent = text;
  statusEl.className = `status ${kind}`.trim();
}

function showError(msg: string) {
  errorEl.textContent = msg;
  errorEl.hidden = !msg;
}

function render() {
  const name = input.split(/[\\/]/).pop() ?? "";
  $("#pick-title").textContent = name || "Pilih file audio";
  $("#pick-sub").hidden = !input;
  pick.classList.toggle("has-file", !!input);
  outPath.textContent = output || "—";
  bitrate.disabled = busy || LOSSLESS.has(format.value);
  for (const el of [pick, urlInput, format, speed, gain]) (el as HTMLButtonElement).disabled = busy;
  for (const el of document.querySelectorAll<HTMLInputElement>("input[name=ch]")) el.disabled = busy;
  save.disabled = busy || !input;
  convertBtn.disabled = busy || !input || !output;
}

async function renderUrlState() {
  const v = urlInput.value.trim();
  const ok = v !== "" && (await Rokit.isURL(v));
  fetchBtn.disabled = busy || !ok;
  urlErr.hidden = v === "" || ok;
}

function opts(): Opts {
  return {
    format: format.value,
    bitrate: Number(bitrate.value),
    speed: Number(speed.value),
    gainDb: Number(gain.value),
    channels: document.querySelector<HTMLInputElement>("input[name=ch]:checked")!.value,
  };
}

async function guarded(label: string, fn: () => Promise<void>) {
  busy = true;
  showError("");
  setStatus(label, "busy");
  render();
  void renderUrlState();
  try {
    await fn();
  } catch (e) {
    setStatus("Gagal.", "fail");
    showError(e instanceof Error ? e.message : String(e));
  } finally {
    busy = false;
    render();
    void renderUrlState();
  }
}

async function setInput(path: string) {
  input = path;
  output = await Rokit.suggestOutput(path, format.value);
}

pick.addEventListener("click", async () => {
  const path = await Rokit.pickFile();
  if (!path) return;
  await setInput(path);
  setStatus("Siap.");
  render();
});

urlInput.addEventListener("input", () => void renderUrlState());
urlForm.addEventListener("submit", (e) => {
  e.preventDefault();
  if (fetchBtn.disabled) return;
  void guarded("Mengunduh…", async () => {
    const d = await Rokit.download(urlInput.value);
    await setInput(d.path);
    setStatus(`Siap (unduhan selesai): ${d.title}`);
  });
});

format.addEventListener("change", async () => {
  if (input) output = await Rokit.suggestOutput(input, format.value);
  render();
});

speed.addEventListener("input", async () => {
  const v = Number(speed.value);
  $("#speed-chip").textContent = `${v.toFixed(2)}×`;
  $("#speed-hint").textContent = await Rokit.hint(v);
});

gain.addEventListener("input", () => {
  const v = Number(gain.value);
  $("#gain-chip").textContent = `${v >= 0 ? "+" : "−"}${Math.abs(v).toFixed(1)} dB`;
});

save.addEventListener("click", async () => {
  const p = await Rokit.pickOutput(output);
  if (p) {
    output = p;
    render();
  }
});

convertBtn.addEventListener("click", () =>
  guarded("Mengonversi…", async () => {
    const wasDownload = await Rokit.isDownload(input);
    await Rokit.convert(input, output, opts());
    setStatus(`Selesai: ${output}`, "ok");
    // The backend deletes a downloaded temp input after success; a local input stays.
    if (wasDownload) input = "";
  }),
);

async function init() {
  render();
  const t = await Rokit.checkTools();
  const missing: string[] = [];
  if (!t.ffmpeg) missing.push("ffmpeg tidak ditemukan di PATH. Pasang dengan <code>winget install Gyan.FFmpeg</code> lalu restart aplikasi.");
  if (!t.ytdlp) missing.push("yt-dlp (untuk URL) tidak ditemukan. Pasang dengan <code>winget install yt-dlp.yt-dlp</code> lalu restart aplikasi.");
  const banner = $("#tools");
  banner.innerHTML = missing.join("<br>");
  banner.hidden = missing.length === 0;
}

void init();
