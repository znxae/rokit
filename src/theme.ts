import { Rokit } from "./mygo";
import { $ } from "./ui";

export const CHOICES = ["auto", "apple", "dark", "material", "material-dark", "oled", "glass"] as const;
export type Choice = (typeof CHOICES)[number];

const KEY = "rokit.theme";
const dark = window.matchMedia("(prefers-color-scheme: dark)");

const isChoice = (v: unknown): v is Choice => CHOICES.includes(v as Choice);

/** "auto" follows the system: Apple Terang in light, Gelap in dark. */
export function resolve(choice: Choice, systemDark: boolean): Exclude<Choice, "auto"> {
  return choice === "auto" ? (systemDark ? "dark" : "apple") : choice;
}

export function applyTheme(choice: Choice) {
  document.documentElement.dataset.theme = resolve(choice, dark.matches);
}

/** localStorage is only a fast cache for the first paint; the Go settings file is the source of truth. */
export function cachedChoice(): Choice {
  try {
    const v = localStorage.getItem(KEY);
    return isChoice(v) ? v : "auto";
  } catch {
    return "auto";
  }
}

function cache(choice: Choice) {
  try {
    localStorage.setItem(KEY, choice);
  } catch {
    /* private mode or blocked storage: the Go file still persists it */
  }
}

export async function initTheme() {
  const select = $<HTMLSelectElement>("#theme");
  let current: Choice = cachedChoice();
  applyTheme(current);
  select.value = current;

  try {
    const saved = await Rokit.theme();
    if (isChoice(saved) && saved !== current) {
      current = saved;
      applyTheme(current);
      select.value = current;
      cache(current);
    }
  } catch {
    /* keep the cached choice */
  }

  select.addEventListener("change", () => {
    const v = select.value;
    if (!isChoice(v)) return;
    current = v;
    applyTheme(v);
    cache(v);
    void Rokit.setTheme(v).catch(() => {}); // the theme already changed on screen
  });
  dark.addEventListener("change", () => {
    if (current === "auto") applyTheme("auto");
  });
}
