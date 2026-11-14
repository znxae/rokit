// Small helpers shared by the tabs.

export const $ = <T extends HTMLElement>(selector: string, root: ParentNode = document) =>
  root.querySelector<T>(selector)!;

const ICON_OK =
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="18" height="18" aria-hidden="true"><circle cx="12" cy="12" r="10"/><path d="m16 9-5.5 5.5L8 12"/></svg>';
const ICON_FAIL =
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="18" height="18" aria-hidden="true"><circle cx="12" cy="12" r="10"/><line x1="12" x2="12" y1="8" y2="12"/><line x1="12" x2="12.01" y1="16" y2="16"/></svg>';

export type Kind = "" | "ok" | "fail" | "busy";

/** Writes a status line: an icon (ok/fail) or spinner (busy) plus text. Text goes in as text, never HTML. */
export function setStatus(el: HTMLElement, kind: Kind, text: string) {
  el.className = `status ${kind}`.trim();
  el.replaceChildren();
  if (kind === "ok" || kind === "fail") {
    const icon = document.createElement("span");
    icon.innerHTML = kind === "ok" ? ICON_OK : ICON_FAIL; // constant markup above
    icon.style.display = "flex";
    el.append(icon);
  }
  const span = document.createElement("span");
  span.textContent = text;
  el.append(span);
}

/** Progress in the bottom bar: text on the left, text on the right, a bar below. Safe to call on every tick. */
export function setProgress(el: HTMLElement, pct: number, left: string, right = "") {
  let prog = el.querySelector<HTMLElement>(".prog");
  if (!prog) {
    el.className = "status";
    el.innerHTML = '<div class="prog"><div class="t"><span></span><b></b></div><div class="track"><i></i></div></div>'; // constant markup
    prog = el.querySelector<HTMLElement>(".prog")!;
  }
  prog.querySelector("span")!.textContent = left;
  prog.querySelector("b")!.textContent = right;
  prog.querySelector<HTMLElement>(".track i")!.style.width = `${Math.max(0, Math.min(100, pct))}%`;
}

export function showError(el: HTMLElement, msg: string) {
  el.textContent = msg;
  el.hidden = !msg;
}

/** Segmented control: marks the clicked button .on and returns the value through onChange. */
export function segmented(el: HTMLElement, onChange: (v: string) => void) {
  el.addEventListener("click", (e) => {
    const b = (e.target as HTMLElement).closest<HTMLButtonElement>("button");
    if (!b || b.disabled || !el.contains(b)) return;
    for (const x of el.querySelectorAll("button")) x.classList.toggle("on", x === b);
    onChange(b.dataset.v ?? b.textContent!.trim());
  });
}

export const segValue = (el: HTMLElement) =>
  el.querySelector<HTMLButtonElement>("button.on")?.dataset.v ?? "";

export function setSegDisabled(el: HTMLElement, disabled: boolean) {
  for (const b of el.querySelectorAll("button")) b.disabled = disabled;
}

export const baseName = (p: string) => p.split(/[\\/]/).pop() ?? p;

/** 1.5 -> "1.50×", dB with a real minus sign. */
export const fmtSpeed = (v: number) => `${v.toFixed(2)}×`;
export const fmtDb = (v: number) => `${v > 0 ? "+" : v < 0 ? "−" : ""}${Math.abs(v).toFixed(1)} dB`;
export const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

/** 1800 -> "1.800" (Indonesian grouping). */
export const fmtInt = (n: number) => n.toLocaleString("id-ID");
/** 222 -> "3:42", 3725 -> "1:02:05". */
export function fmtDuration(sec: number) {
  const s = Math.round(sec);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}
export const dirName = (p: string) => p.replace(/[\\/][^\\/]*$/, "");

/** Shows a tab by name and marks its sidebar row. */
export function showTab(name: string) {
  const panel = document.getElementById(`tab-${name}`);
  if (!panel) return;
  for (const r of document.querySelectorAll<HTMLButtonElement>("#nav .side-row")) {
    r.classList.toggle("sel", r.dataset.tab === name);
    r.toggleAttribute("aria-current", r.dataset.tab === name);
  }
  for (const t of document.querySelectorAll(".tab")) t.classList.toggle("active", t === panel);
}
