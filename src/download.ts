import { Channel } from "mygo-runtime";
import { Rokit, type DLProgress } from "./mygo";
import { $, dirName, errText, fmtDuration, segmented, segValue, setProgress, setSegDisabled, setStatus, showError } from "./ui";

export function initDownload() {
  const tab = $("#tab-download");
  const urlInput = $<HTMLInputElement>("#d-url", tab);
  const check = $<HTMLButtonElement>("#d-check", tab);
  const urlErr = $("#d-url-err", tab);
  const vid = $("#d-vid", tab);
  const quality = $("#d-quality", tab);
  const container = $("#d-container", tab);
  const go = $<HTMLButtonElement>("#d-go", tab);
  const cancel = $<HTMLButtonElement>("#d-cancel", tab);
  const openBtn = $<HTMLButtonElement>("#d-open", tab);
  const status = $("#d-status", tab);
  const error = $("#d-error", tab);

  let busy = false;
  let urlOk = false;
  let progress: Channel<DLProgress> | null = null;
  let cancelled = false;
  let savedPath = "";

  function render() {
    urlInput.disabled = busy;
    check.disabled = busy || !urlOk;
    setSegDisabled(quality, busy);
    setSegDisabled(container, busy);
    go.hidden = busy;
    go.disabled = !urlOk;
    cancel.hidden = !busy;
    openBtn.hidden = busy || !savedPath;
    $("#d-q", tab).textContent = quality.querySelector("button.on")?.textContent ?? "";
  }

  urlInput.addEventListener("input", async () => {
    const v = urlInput.value.trim();
    urlOk = v !== "" && (await Rokit.isURL(v));
    urlErr.hidden = v === "" || urlOk;
    vid.hidden = true; // the card belongs to the previously checked link
    savedPath = "";
    render();
  });

  async function guarded(label: string, fn: () => Promise<void>) {
    busy = true;
    showError(error, "");
    setStatus(status, "busy", label);
    render();
    try {
      await fn();
    } catch (e) {
      if (cancelled) setStatus(status, "", "Dibatalkan.");
      else {
        setStatus(status, "fail", "Gagal.");
        showError(error, errText(e));
      }
    } finally {
      busy = false;
      render();
    }
  }

  $("#d-form", tab).addEventListener("submit", (e) => {
    e.preventDefault();
    if (check.disabled) return;
    cancelled = false;
    void guarded("Memeriksa tautan…", async () => {
      const m = await Rokit.info(urlInput.value);
      $("#d-title", tab).textContent = m.title;
      $("#d-by", tab).textContent = m.uploader || "—";
      $("#d-dur", tab).textContent = m.duration > 0 ? fmtDuration(m.duration) : "";
      $("#d-dur", tab).hidden = m.duration <= 0;
      const thumb = $<HTMLImageElement>("#d-thumb", tab);
      thumb.hidden = !m.thumbnail;
      thumb.src = m.thumbnail || "";
      try {
        $("#d-src", tab).textContent = new URL(urlInput.value.trim()).hostname.replace(/^www\./, "");
      } catch {
        $("#d-src", tab).textContent = "—";
      }
      vid.hidden = false;
      setStatus(status, "", "Siap diunduh.");
    });
  });

  segmented(quality, render);
  segmented(container, render);

  go.addEventListener("click", () => {
    cancelled = false;
    void guarded("Mengunduh…", async () => {
      progress = new Channel<DLProgress>((p) => {
        setProgress(status, p.percent, `Mengunduh… ${Math.round(p.percent)}%${p.speed ? ` · ${p.speed}` : ""}`, p.eta ? `sisa ${p.eta}` : "");
      });
      setProgress(status, 0, "Mengunduh…");
      try {
        const r = await Rokit.downloadVideo(urlInput.value, { quality: segValue(quality), container: segValue(container) }, progress);
        savedPath = r.path;
        setStatus(status, "ok", `Selesai — tersimpan di ${r.path}`);
      } finally {
        progress = null;
      }
    });
  });
  cancel.addEventListener("click", () => {
    cancelled = true;
    progress?.close();
  });
  openBtn.addEventListener("click", () => void Rokit.openFolder(dirName(savedPath)).catch((e) => showError(error, errText(e))));

  render();
}
