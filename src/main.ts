// Types and the Rokit service are generated from the Go code by `mygo generate`.
import { currentWindow } from "mygo-runtime";
import { Rokit, type ToolInfo } from "./mygo";
import { initAudio } from "./audio";
import { initRoblox } from "./roblox";
import { initDownload } from "./download";
import { initFrames } from "./frames";
import { initClothing } from "./clothing";
import { initTheme } from "./theme";
import { initDropzone } from "./dropzone";
import { $, errText, showTab } from "./ui";

function initTabs() {
  for (const row of document.querySelectorAll<HTMLButtonElement>("#nav .side-row")) {
    row.addEventListener("click", () => showTab(row.dataset.tab ?? ""));
  }
}

async function checkTools() {
  const t = await Rokit.checkTools();
  const parts: string[] = [];
  if (!t.ffmpeg) parts.push("Alat bawaan (ffmpeg) tidak ditemukan; pasang ulang aplikasi.");
  if (!t.ytdlp) parts.push("Alat bawaan (yt-dlp) tidak ditemukan; pasang ulang aplikasi.");
  const banner = $("#tools");
  banner.innerHTML = parts.join("<br>"); // constant strings above, no user data
  banner.hidden = parts.length === 0;
  $("#side-foot").textContent = parts.length === 0 ? "ffmpeg dan yt-dlp siap" : "Alat belum lengkap";
  $("#t-info").textContent = `ffmpeg ${toolText(t.ffmpegInfo)} · yt-dlp ${toolText(t.ytdlpInfo)}`;
}

const toolText = (i: ToolInfo) => (i.found ? `${i.version || "?"} (${i.source})` : "tidak ada");

function initToolUpdate() {
  const btn = $<HTMLButtonElement>("#t-update");
  btn.addEventListener("click", async () => {
    btn.disabled = true;
    $("#t-info").textContent = "Memperbarui yt-dlp…";
    try {
      const i = await Rokit.updateYtDlp();
      $("#t-info").textContent = `yt-dlp ${i.version} (${i.source}) — sudah terbaru`;
    } catch (e) {
      $("#t-info").textContent = errText(e);
    } finally {
      btn.disabled = false;
    }
  });
}

/** The page draws the window buttons (HideWindowButtons); double-clicking a drag region maximizes, MyGo does that itself. */
function initWindowButtons() {
  $("#w-close").addEventListener("click", () => void currentWindow.close());
  $("#w-min").addEventListener("click", () => void currentWindow.minimize());
  $("#w-zoom").addEventListener("click", () => void currentWindow.toggleMaximize());
}

initWindowButtons();
initTabs();
initToolUpdate();
void initTheme();
initAudio();
initFrames();
initDownload();
initClothing();
initDropzone();
initRoblox();
void checkTools();
