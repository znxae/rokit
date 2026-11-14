import { Channel } from "mygo-runtime";
import { Rokit, type FrameOpts, type FrameProgress, type VideoInfo } from "./mygo";
import { $, baseName, errText, fmtDuration, fmtInt, segmented, segValue, setProgress, setSegDisabled, setStatus, showError } from "./ui";

export function initFrames() {
  const tab = $("#tab-frames");
  const pick = $<HTMLButtonElement>("#f-pick", tab);
  const urlInput = $<HTMLInputElement>("#f-url", tab);
  const fetchBtn = $<HTMLButtonElement>("#f-fetch", tab);
  const urlErr = $("#f-url-err", tab);
  const img = $("#f-img", tab);
  const rowQ = $("#f-row-q", tab);
  const quality = $<HTMLInputElement>("#f-q", tab);
  const fps = $<HTMLInputElement>("#f-fps", tab);
  const presets = $("#f-presets", tab);
  const est = $("#f-est", tab);
  const outPath = $("#f-out", tab);
  const folderBtn = $<HTMLButtonElement>("#f-folder", tab);
  const go = $<HTMLButtonElement>("#f-go", tab);
  const cancel = $<HTMLButtonElement>("#f-cancel", tab);
  const openBtn = $<HTMLButtonElement>("#f-open", tab);
  const status = $("#f-status", tab);
  const error = $("#f-error", tab);

  let input = "";
  let outDir = "";
  let info: VideoInfo | null = null;
  let busy = false;
  let urlOk = false;
  let progress: Channel<FrameProgress> | null = null;
  let resultDir = "";
  let cancelled = false;

  const opts = (): FrameOpts => ({
    format: segValue(img),
    quality: Number(quality.value),
    fps: Number(fps.value),
    allFrames: segValue(presets) === "all",
  });

  function estimate() {
    const o = opts();
    const rate = o.allFrames ? (info?.fps ?? 0) : o.fps;
    est.textContent = info && info.duration > 0 && rate > 0 ? `± ${fmtInt(Math.max(1, Math.ceil(info.duration * rate)))} gambar` : "—";
  }

  function render() {
    rowQ.classList.toggle("off", segValue(img) === "png");
    quality.disabled = busy || segValue(img) === "png";
    for (const el of [pick, urlInput, fps]) el.disabled = busy;
    setSegDisabled(img, busy);
    setSegDisabled(presets, busy);
    fetchBtn.disabled = busy || !urlOk;
    folderBtn.disabled = busy || !input;
    go.hidden = busy;
    cancel.hidden = !busy;
    go.disabled = !input || !outDir;
    openBtn.hidden = busy || !resultDir;
    outPath.textContent = outDir || "—";
    outPath.title = outDir;
    estimate();
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
    resultDir = "";
    outDir = await Rokit.suggestFramesDir(path, "");
    $("#f-name", tab).textContent = title || baseName(path);
    $("#f-meta", tab).textContent = "Membaca info video…";
    pick.textContent = "Ganti video";
    info = null;
    try {
      info = await Rokit.probeVideo(path);
      const parts = [info.duration > 0 ? fmtDuration(info.duration) : "", info.width ? `${info.width}×${info.height}` : "", info.fps ? `${Math.round(info.fps * 100) / 100} fps` : ""];
      $("#f-meta", tab).textContent = parts.filter(Boolean).join(" · ") || "Info video tidak tersedia";
      if (!info.hasVideo) throw new Error("File ini tidak punya gambar (video).");
    } finally {
      render();
    }
  }

  pick.addEventListener("click", () =>
    void run("Membuka video…", async () => {
      const path = await Rokit.pickVideo();
      if (!path) {
        setStatus(status, "", input ? "Siap." : "Pilih video atau tempel URL.");
        return;
      }
      await setInput(path);
      setStatus(status, "", "Siap diekstrak.");
    }),
  );

  urlInput.addEventListener("input", async () => {
    const v = urlInput.value.trim();
    urlOk = v !== "" && (await Rokit.isURL(v));
    urlErr.hidden = v === "" || urlOk;
    render();
  });
  $("#f-url-form", tab).addEventListener("submit", (e) => {
    e.preventDefault();
    if (fetchBtn.disabled) return;
    void run("Mengunduh video…", async () => {
      const d = await Rokit.downloadVideoFile(urlInput.value);
      await setInput(d.path, d.title);
      setStatus(status, "", "Siap diekstrak.");
    });
  });

  segmented(img, render);
  segmented(presets, (v) => {
    if (v !== "all") fps.value = v;
    render();
  });
  quality.addEventListener("input", () => {
    $("#f-q-val", tab).textContent = `${quality.value}%`;
  });
  fps.addEventListener("input", () => {
    for (const b of presets.querySelectorAll("button")) b.classList.remove("on");
    render();
  });

  folderBtn.addEventListener("click", async () => {
    const parent = await Rokit.pickFolder();
    if (!parent) return;
    outDir = await Rokit.suggestFramesDir(input, parent); // a new folder inside the chosen one
    render();
  });

  openBtn.addEventListener("click", () => void Rokit.openFolder(resultDir).catch((e) => showError(error, errText(e))));

  go.addEventListener("click", () =>
    void run("Mengekstrak…", async () => {
      cancelled = false;
      const wasDownload = await Rokit.isDownload(input);
      progress = new Channel<FrameProgress>((p) => {
        const pct = p.total > 0 ? Math.min(100, (p.done / p.total) * 100) : 0;
        const left = p.total > 0 ? `Mengekstrak… frame ${fmtInt(p.done)} / ${fmtInt(p.total)}` : `Mengekstrak… ${fmtInt(p.done)} frame`;
        setProgress(status, pct, left, p.total > 0 ? `${Math.round(pct)}%` : "");
      });
      setProgress(status, 0, "Mengekstrak…");
      try {
        const r = await Rokit.extractFrames(input, outDir, opts(), progress);
        resultDir = r.dir;
        setStatus(status, "ok", `Selesai — ${fmtInt(r.count)} gambar di ${r.dir}`);
        outDir = await Rokit.suggestFramesDir(input, ""); // the next run gets a fresh folder
        if (wasDownload) input = ""; // the backend deleted the temp download
      } catch (e) {
        if (!cancelled) throw e;
        setStatus(status, "", "Dibatalkan.");
      } finally {
        progress = null;
      }
    }),
  );
  cancel.addEventListener("click", () => {
    cancelled = true;
    progress?.close();
  });

  render();
}
