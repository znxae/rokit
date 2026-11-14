import { Rokit, type Opts } from "./mygo";
import { $, baseName, errText, fmtDb, fmtSpeed, segmented, segValue, setSegDisabled, setStatus, showError } from "./ui";

const LOSSLESS = new Set(["wav", "flac"]);

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
  const save = $<HTMLButtonElement>("#save", tab);
  const outPath = $("#out-path", tab);
  const convert = $<HTMLButtonElement>("#convert", tab);
  const status = $("#status", tab);
  const error = $("#error", tab);

  let input = "";
  let output = "";
  let busy = false;
  let urlOk = false;

  function render() {
    const name = input ? baseName(input) : "";
    $("#pick-title", tab).textContent = name || "Pilih file audio atau video";
    $("#pick-sub", tab).textContent = name ? "Klik untuk mengganti file" : "MP3, WAV, FLAC, MP4, MKV, …";
    pick.classList.toggle("has", !!name);
    outPath.textContent = output || "—";
    outPath.title = output;
    const lossless = LOSSLESS.has(format.value);
    rowBitrate.classList.toggle("off", lossless);
    bitrate.disabled = busy || lossless;
    for (const el of [pick, urlInput, format, speed, gain]) el.disabled = busy;
    setSegDisabled(channels, busy);
    fetchBtn.disabled = busy || !urlOk;
    save.disabled = busy || !input;
    convert.disabled = busy || !input || !output;
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

  async function setInput(path: string) {
    input = path;
    output = await Rokit.suggestOutput(path, format.value);
  }

  pick.addEventListener("click", () =>
    void run("Membuka file…", async () => {
      const path = await Rokit.pickFile();
      if (!path) {
        setStatus(status, "", input ? "Siap." : "Pilih file input atau tempel URL.");
        return;
      }
      await setInput(path);
      setStatus(status, "", "Siap.");
    }),
  );

  urlInput.addEventListener("input", () => void refreshUrl());
  $("#url-form", tab).addEventListener("submit", (e) => {
    e.preventDefault();
    if (fetchBtn.disabled) return;
    void run("Mengunduh…", async () => {
      const d = await Rokit.download(urlInput.value);
      await setInput(d.path);
      setStatus(status, "", `Siap — unduhan selesai: ${d.title}`);
    });
  });

  format.addEventListener("change", async () => {
    if (input) output = await Rokit.suggestOutput(input, format.value);
    render();
  });
  segmented(channels, () => {});

  speed.addEventListener("input", async () => {
    const v = Number(speed.value);
    $("#speed-val", tab).textContent = fmtSpeed(v);
    $("#speed-hint", tab).textContent = await Rokit.hint(v); // the same rule the tests cover
  });
  gain.addEventListener("input", () => {
    $("#gain-val", tab).textContent = fmtDb(Number(gain.value));
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
      await Rokit.convert(input, output, opts);
      setStatus(status, "ok", `Selesai — tersimpan di ${output}`);
      if (wasDownload) input = ""; // the backend deleted the temp download
    }),
  );

  render();
}
