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
