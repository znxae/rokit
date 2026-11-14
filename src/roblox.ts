import { Channel } from "mygo-runtime";
import { Rokit, type AssetMeta, type BatchItem, type RobloxAccount, type RobloxOpts, type RobloxSaved } from "./mygo";
import { loadAudioFile } from "./audio";
import { $, dirName, errText, segmented, segValue, setProgress, setSegDisabled, setStatus, showError, showTab } from "./ui";

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

  const mode = $("#r-mode");
  const single = $("#r-single", tab);
  const batch = $("#r-batch", tab);
  const list = $<HTMLTextAreaElement>("#r-list", tab);
  const items = $("#r-items", tab);
  const stop = $<HTMLButtonElement>("#r-stop", tab);
  const many = () => segValue(mode) === "many";
  let batchProgress: Channel<BatchItem> | null = null;
  let cancelled = false;
  let batchDir = "";

  const opts = $("#r-opts", tab);
  const ver = $<HTMLInputElement>("#r-ver", tab);
  const av = $("#r-av", tab);
  const obj = $("#r-obj", tab);
  const avRow = $("#r-avrow", tab);
  const objRow = $("#r-objrow", tab);
  const ACCESSORY = [8, 17, 41, 42, 43, 44, 45, 46, 47, 79];
  const avatar = () => segValue(av) === "avatar";
  const options = (): RobloxOpts => ({ version: Math.trunc(Number(ver.value)) || 0, avatar: avatar() && !avRow.hidden, obj: segValue(obj) === "obj" && !objRow.hidden });

  const acct = $("#r-acct", tab);
  const loginBtn = $<HTMLButtonElement>("#r-login", tab);
  const ACCT_HELP = acct.textContent ?? "";
  let account: RobloxAccount | null = null;

  let busy = false;
  let meta: AssetMeta | null = null;
  let saved: RobloxSaved | null = null;

  function render() {
    single.hidden = many();
    batch.hidden = !many();
    setSegDisabled(mode, busy);
    input.disabled = busy;
    list.disabled = busy;
    loginBtn.disabled = busy;
    loginBtn.textContent = account?.loggedIn ? "Keluar" : "Masuk Roblox";
    acct.textContent = account?.loggedIn ? `Masuk sebagai ${account.name}. Login hanya dikirim ke API Roblox; Keluar menghapusnya dari app ini.` : ACCT_HELP;
    ver.disabled = busy || avatar();
    setSegDisabled(av, busy || !!ver.value.trim());
    setSegDisabled(obj, busy);
    opts.hidden = many() || !meta;
    avRow.hidden = !meta || !ACCESSORY.includes(meta.typeId);
    objRow.hidden = !meta || (meta.typeId !== 4 && meta.typeId !== 40);
    const has = many() ? list.value.trim() !== "" : input.value.trim() !== "";
    check.disabled = busy || !input.value.trim();
    go.textContent = many() ? "Unduh semua" : "Unduh";
    go.hidden = busy && many();
    stop.hidden = !(busy && many());
    go.disabled = busy || !has || (!many() && !!meta?.blocked);
    openBtn.hidden = busy || !(many() ? batchDir : saved);
    audioBtn.hidden = busy || many() || !saved?.isAudio;
  }

  /** What was checked or saved no longer matches the field. */
  function reset() {
    meta = null;
    saved = null;
    card.hidden = true;
    ver.value = "";
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
    many() ? undefined : void guarded("Mengunduh…", async () => {
      const s = await Rokit.robloxDownload(input.value, options());
      saved = s;
      out.textContent = s.path;
      out.title = s.path;
      setStatus(status, "ok", `Selesai — tersimpan di ${s.path}${s.raw ? " (format mentah Roblox)" : ""}`);
      if (s.raw && !note.hidden) note.textContent = s.note;
    }),
  );

  openBtn.addEventListener("click", () => !many() && saved && void Rokit.openFolder(dirName(saved.path)).catch((e) => showError(error, errText(e))));
  audioBtn.addEventListener("click", async () => {
    if (!saved?.isAudio) return;
    try {
      await loadAudioFile(saved.path, meta?.name ?? "");
      showTab("audio");
    } catch (e) {
      showError(error, errText(e));
    }
  });

    // ---- Daftar: many assets, one at a time ----
  segmented(mode, () => {
    showError(error, "");
    setStatus(status, "", many() ? "Tempel daftar ID atau URL, satu per baris." : "Tempel URL atau ID aset, lalu klik Cek.");
    render();
  });
  loginBtn.addEventListener("click", () => {
    const out = !!account?.loggedIn;
    void guarded(out ? "Keluar…" : "Menunggu login di jendela Roblox…", async () => {
      if (out) {
        await Rokit.robloxLogout();
        account = null;
      } else {
        account = await Rokit.robloxLogin();
      }
      reset(); // earlier Cek results no longer match what this account can open
      setStatus(status, "ok", out ? "Keluar dari akun Roblox." : `Masuk sebagai ${account!.name}.`);
    });
  });
  Rokit.robloxAccountInfo().then((a) => {
    account = a;
    render();
  }, () => {});

  ver.addEventListener("input", render);
  segmented(av, render);
  segmented(obj, render);
  list.addEventListener("input", () => {
    batchDir = "";
    render();
  });

  function paint(it: BatchItem) {
    let li = items.children[it.index] as HTMLLIElement | undefined;
    if (!li) {
      li = document.createElement("li");
      li.innerHTML = '<span class="in"></span><span class="st"></span>'; // constant markup
      items.append(li);
    }
    li.className = it.status;
    li.querySelector(".in")!.textContent = it.input;
    li.querySelector(".st")!.textContent = it.message ? `${it.status} — ${it.message}` : it.status;
  }

  async function runMany() {
    items.replaceChildren();
    batchDir = "";
    cancelled = false;
    busy = true;
    showError(error, "");
    render();
    let total = 0;
    let done = 0;
    batchProgress = new Channel<BatchItem>((it) => {
      paint(it);
      total = Math.max(total, it.index + 1);
      if (it.status !== "menunggu" && it.status !== "jalan") done = Math.max(done, it.index + 1);
      setProgress(status, total ? (done / total) * 100 : 0, `Mengunduh… ${done} / ${total}`, "");
    });
    setProgress(status, 0, "Mengunduh…", "");
    try {
      const res = await Rokit.robloxDownloadMany(list.value, batchProgress);
      const ok = res.filter((i) => i.status === "selesai");
      const bad = res.filter((i) => i.status === "gagal").length;
      const skipped = res.filter((i) => i.status === "dilewati").length;
      if (ok[0]) batchDir = dirName(ok[0].path);
      setStatus(status, ok.length ? "ok" : "fail", `${cancelled ? "Dibatalkan — " : "Selesai — "}${ok.length} berhasil, ${bad} gagal${skipped ? `, ${skipped} dilewati` : ""}`);
    } catch (e) {
      setStatus(status, "fail", "Gagal.");
      showError(error, errText(e));
    } finally {
      batchProgress = null;
      busy = false;
      render();
    }
  }
  stop.addEventListener("click", () => {
    cancelled = true;
    batchProgress?.close();
  });
  go.addEventListener("click", () => {
    if (many()) void runMany();
  });
  openBtn.addEventListener("click", () => {
    if (many() && batchDir) void Rokit.openFolder(batchDir).catch((e) => showError(error, errText(e)));
  });

  render();
}
