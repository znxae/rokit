import { Rokit } from "./mygo";
import { ClothingViewer, DEFAULT_LOOK, type Look } from "./clothing_scene";
import { drawGuide } from "./clothing_guide";
import { checkClothingFile, MAX_BYTES, TEMPLATE_H, TEMPLATE_W, validateClothingBytes } from "./clothing_uv";
import { $, errText, segmented, segValue, setStatus, showError } from "./ui";

type Kind = "shirt" | "pants";
const NAME: Record<Kind, string> = { shirt: "kemeja", pants: "celana" };

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function bytesToB64(b: Uint8Array): string {
  let s = "";
  for (let i = 0; i < b.length; i += 0x8000) s += String.fromCharCode(...b.subarray(i, i + 0x8000));
  return btoa(s);
}

export function initClothing() {
  const tab = $("#tab-clothing");
  const stage = $("#c-stage", tab);
  const status = $("#c-status", tab);
  const err = $("#c-err", tab);
  const kindSeg = $("#c-kind", tab);
  const saveBtn = $<HTMLButtonElement>("#c-save", tab);
  const whoNow = $("#c-who-now", tab);
  const who = $<HTMLInputElement>("#c-who", tab);

  let viewer: ClothingViewer | null = null;
  let look: Look = DEFAULT_LOOK;
  const bytes: Record<Kind, Uint8Array | null> = { shirt: null, pants: null };

  const active = (): Kind => (segValue(kindSeg) === "pants" ? "pants" : "shirt");

  function ensureViewer(): ClothingViewer | null {
    if (viewer) return viewer;
    const v = new ClothingViewer(stage);
    if (!v.ok) {
      $("#c-nogl", tab).hidden = false;
      return null;
    }
    viewer = v;
    v.setLook(look);
    v.setAutoRotate(segValue($("#c-rot", tab)) === "on");
    v.setOverlay(segValue($("#c-lines", tab)) === "on");
    return v;
  }

  function renderButtons() {
    saveBtn.disabled = !bytes[active()];
    for (const k of ["shirt", "pants"] as Kind[]) $(`#c-clear-${k}`, tab).hidden = !bytes[k];
  }

  async function load(kind: Kind, data: Uint8Array, name: string) {
    const msg = validateClothingBytes(data);
    if (msg) {
      showError(err, `${name || NAME[kind]}: ${msg}`);
      setStatus(status, "fail", "Gambar ditolak.");
      return;
    }
    showError(err, "");
    const bmp = await createImageBitmap(new Blob([data as BlobPart], { type: "image/png" }));
    bytes[kind] = data;
    const th = $<HTMLCanvasElement>(`#c-th-${kind}`, tab);
    th.getContext("2d")!.clearRect(0, 0, th.width, th.height);
    th.getContext("2d")!.drawImage(bmp, 0, 0, th.width, th.height);
    th.classList.add("has");
    $(`#c-nm-${kind}`, tab).textContent = `${name} · ${TEMPLATE_W}x${TEMPLATE_H} ✓`;
    ensureViewer()?.setLayer(kind, bmp);
    setStatus(status, "ok", `${NAME[kind][0]!.toUpperCase()}${NAME[kind].slice(1)} dimuat.`);
    renderButtons();
  }

  function clear(kind: Kind) {
    bytes[kind] = null;
    const th = $<HTMLCanvasElement>(`#c-th-${kind}`, tab);
    th.getContext("2d")!.clearRect(0, 0, th.width, th.height);
    th.classList.remove("has");
    $(`#c-nm-${kind}`, tab).textContent = "Seret PNG ke sini atau pilih file";
    viewer?.setLayer(kind, null);
    showError(err, "");
    renderButtons();
  }

  for (const kind of ["shirt", "pants"] as Kind[]) {
    $(`#c-pick-${kind}`, tab).addEventListener("click", async () => {
      try {
        const f = await Rokit.pickClothingImage();
        if (f.name) await load(kind, b64ToBytes(f.data), f.name);
      } catch (e) {
        showError(err, errText(e));
        setStatus(status, "fail", "Gambar ditolak.");
      }
    });
    $(`#c-clear-${kind}`, tab).addEventListener("click", () => clear(kind));
    const drop = $(`#c-drop-${kind}`, tab);
    drop.addEventListener("dragover", (e) => {
      e.preventDefault();
      drop.classList.add("over");
    });
    drop.addEventListener("dragleave", () => drop.classList.remove("over"));
    drop.addEventListener("drop", async (e) => {
      e.preventDefault();
      drop.classList.remove("over");
      const f = e.dataTransfer?.files?.[0];
      if (!f) return;
      if (f.size > MAX_BYTES) {
        showError(err, `${f.name}: ${checkClothingFile({ size: f.size, width: 0, height: 0, isPng: true })}`);
        return;
      }
      await load(kind, new Uint8Array(await f.arrayBuffer()), f.name);
    });
  }
  // A file dropped outside a drop zone must not make the web view navigate to it.
  for (const ev of ["dragover", "drop"]) tab.addEventListener(ev, (e) => e.preventDefault());

  segmented(kindSeg, renderButtons);
  segmented($("#c-lines", tab), (v) => viewer?.setOverlay(v === "on"));
  segmented($("#c-rot", tab), (v) => viewer?.setAutoRotate(v === "on"));
  segmented($("#c-bg", tab), (v) => stage.setAttribute("data-bg", v));

  $("#c-reset", tab).addEventListener("click", () => viewer?.resetCamera());

  async function save(kind: "kemeja" | "celana" | "panduan" | "preview", b64: string) {
    try {
      const p = await Rokit.saveClothingImage(kind, b64);
      setStatus(status, "ok", `Tersimpan di ${p}`);
    } catch (e) {
      setStatus(status, "fail", "Gagal menyimpan.");
      showError(err, errText(e));
    }
  }

  saveBtn.addEventListener("click", () => {
    const k = active();
    const b = bytes[k];
    if (b) void save(NAME[k] as "kemeja" | "celana", bytesToB64(b));
  });
  $("#c-guide", tab).addEventListener("click", () => {
    const c = document.createElement("canvas");
    drawGuide(c);
    void save("panduan", c.toDataURL("image/png").split(",")[1]!);
  });
  $("#c-shot", tab).addEventListener("click", () => {
    const v = ensureViewer();
    if (!v) return;
    const bg = segValue($("#c-bg", tab)) as "tema" | "kotak" | "bening";
    const themed = getComputedStyle(stage).backgroundColor;
    void save("preview", v.snapshot(bg, themed).split(",")[1]!);
  });
  $("#c-upload", tab).addEventListener("click", () => void Rokit.openUploadPage().catch((e) => showError(err, errText(e))));

  // ---- Avatar kita ----
  function applyLook(l: Look, label: string) {
    look = l;
    viewer?.setLook(l);
    whoNow.textContent = label;
  }
  async function lookup(fn: () => Promise<{ name: string; avatarType: string } & Look>) {
    setStatus(status, "busy", "Mengambil avatar…");
    showError(err, "");
    try {
      const a = await fn();
      applyLook(a, `${a.name} (${a.avatarType}) — pratinjau memakai bentuk R6`);
      setStatus(status, "ok", `Avatar ${a.name} dipakai.`);
    } catch (e) {
      setStatus(status, "fail", "Avatar tidak dimuat.");
      showError(err, errText(e));
    }
  }
  $("#c-who-form", tab).addEventListener("submit", (e) => {
    e.preventDefault();
    const v = who.value.trim();
    if (!v) {
      applyLook(DEFAULT_LOOK, "Avatar bawaan (R6)");
      return;
    }
    void lookup(() => Rokit.robloxAvatarLook(v));
  });
  $("#c-me", tab).addEventListener("click", () => void lookup(() => Rokit.robloxMyAvatar()));
  $("#c-default", tab).addEventListener("click", () => {
    who.value = "";
    applyLook(DEFAULT_LOOK, "Avatar bawaan (R6)");
  });

  // The viewport is created when the tab is first opened (it has no size while hidden).
  for (const row of document.querySelectorAll<HTMLButtonElement>('#nav .side-row[data-tab="clothing"]')) row.addEventListener("click", () => ensureViewer());

  renderButtons();
  if (import.meta.env.DEV) (window as unknown as Record<string, unknown>).__clothing = { ensureViewer, load, applyLook, drawGuide };
}
