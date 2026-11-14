import { Rokit, type AssetMeta, type RobloxSaved } from "./mygo";
import { loadAudioFile } from "./audio";
import { $, dirName, errText, setStatus, showError, showTab } from "./ui";

// Lucide icons per kind; constant markup, no user data.
const ICON = (d: string) =>
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="44" height="44" aria-hidden="true">${d}</svg>`;
const ICON_AUDIO = ICON('<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>');
const ICON_IMAGE = ICON('<rect width="18" height="18" x="3" y="3" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21"/>');
const ICON_BOX = ICON('<path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5"/><path d="M12 22V12"/>');

export function initRoblox() {
  const tab = $("#tab-roblox");
  const input = $<HTMLInputElement>("#r-url", tab);
  const check = $<HTMLButtonElement>("#r-check", tab);
  const card = $("#r-card", tab);
  const note = $("#r-note", tab);
  const go = $<HTMLButtonElement>("#r-go", tab);
  const openBtn = $<HTMLButtonElement>("#r-open", tab);
  const audioBtn = $<HTMLButtonElement>("#r-audio", tab);
  const out = $("#r-out", tab);
  const status = $("#r-status", tab);
  const error = $("#r-error", tab);

  let busy = false;
  let meta: AssetMeta | null = null;
  let saved: RobloxSaved | null = null;

  function render() {
    input.disabled = busy;
    const has = input.value.trim() !== "";
    check.disabled = busy || !has;
    go.disabled = busy || !has || !!meta?.blocked;
    openBtn.hidden = busy || !saved;
    audioBtn.hidden = busy || !saved?.isAudio;
  }

  /** What was checked or saved no longer matches the field. */
  function reset() {
    meta = null;
    saved = null;
    card.hidden = true;
    note.hidden = true;
    out.textContent = "Downloads\Rokit-Roblox";
    out.title = "";
    showError(error, "");
    setStatus(status, "", "Tempel URL atau ID aset, lalu klik Cek.");
  }

  async function guarded(label: string, fn: () => Promise<void>) {
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

  input.addEventListener("input", () => {
    reset();
    render();
  });

  $("#r-form", tab).addEventListener("submit", (e) => {
    e.preventDefault();
    if (check.disabled) return;
    void guarded("Memeriksa aset…", async () => {
      const m = await Rokit.robloxCheck(input.value);
      meta = m;
      saved = null;
      $("#r-name", tab).textContent = m.name || `Aset ${m.id}`;
      $("#r-id", tab).textContent = String(m.id);
      $("#r-type", tab).textContent = m.type;
      $("#r-by", tab).textContent = m.creator || "—";
      $("#r-created", tab).textContent = m.created || "—";
      $("#r-tile", tab).innerHTML = m.typeId === 3 ? ICON_AUDIO : m.typeId === 1 ? ICON_IMAGE : ICON_BOX;
      card.hidden = false;
      note.hidden = false;
      note.classList.toggle("warn", !!m.blocked);
      note.textContent = m.blocked || m.note;
      setStatus(status, m.blocked ? "fail" : "", m.blocked ? "Tidak bisa diunduh." : "Siap diunduh.");
    });
  });

  go.addEventListener("click", () =>
    void guarded("Mengunduh…", async () => {
      const s = await Rokit.robloxDownload(input.value);
      saved = s;
      out.textContent = s.path;
      out.title = s.path;
      setStatus(status, "ok", `Selesai — tersimpan di ${s.path}${s.raw ? " (format mentah Roblox)" : ""}`);
      if (s.raw && !note.hidden) note.textContent = s.note;
    }),
  );

  openBtn.addEventListener("click", () => saved && void Rokit.openFolder(dirName(saved.path)).catch((e) => showError(error, errText(e))));
  audioBtn.addEventListener("click", async () => {
    if (!saved?.isAudio) return;
    try {
      await loadAudioFile(saved.path, meta?.name ?? "");
      showTab("audio");
    } catch (e) {
      showError(error, errText(e));
    }
  });

  render();
}
