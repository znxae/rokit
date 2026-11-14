// Types and the Rokit service are generated from the Go code by `mygo generate`.
import { Rokit } from "./mygo";
import { initAudio } from "./audio";
import { $ } from "./ui";

/** Sidebar rows switch tabs; a row whose panel does not exist yet is ignored. */
function initTabs() {
  const rows = document.querySelectorAll<HTMLButtonElement>("#nav .side-row");
  for (const row of rows) {
    row.addEventListener("click", () => {
      const panel = document.getElementById(`tab-${row.dataset.tab}`);
      if (!panel) return;
      for (const r of rows) {
        r.classList.toggle("sel", r === row);
        r.toggleAttribute("aria-current", r === row);
      }
      for (const t of document.querySelectorAll(".tab")) t.classList.toggle("active", t === panel);
    });
  }
}

async function checkTools() {
  const t = await Rokit.checkTools();
  const parts: string[] = [];
  if (!t.ffmpeg) parts.push("ffmpeg tidak ditemukan di PATH. Pasang dengan <code>winget install Gyan.FFmpeg</code>, lalu restart aplikasi.");
  if (!t.ytdlp) parts.push("yt-dlp (untuk URL) tidak ditemukan. Pasang dengan <code>winget install yt-dlp.yt-dlp</code>, lalu restart aplikasi.");
  const banner = $("#tools");
  banner.innerHTML = parts.join("<br>"); // constant strings above, no user data
  banner.hidden = parts.length === 0;
  $("#side-foot").textContent = parts.length === 0 ? "ffmpeg dan yt-dlp terdeteksi" : "Alat belum lengkap";
}

initTabs();
initAudio();
void checkTools();
