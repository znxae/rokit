// Files dropped on the window. MyGo hands over real paths (onFileDrop); the DOM events only drive the overlay
// and make sure the web view never navigates to a dropped file.
import { onFileDrop } from "mygo-runtime";
import { Rokit } from "./mygo";
import { decideDrop, takesFiles, type DropItem } from "./drop";
import { dropAudio } from "./audio";
import { dropVideo } from "./frames";
import { dropClothing } from "./clothing";
import { $, errText } from "./ui";

const activeTab = () => document.querySelector(".tab.active")?.id.replace(/^tab-/, "") ?? "";

let toastTimer = 0;
export function toast(msg: string, err = false) {
  const t = $("#toast");
  t.textContent = msg;
  t.classList.toggle("err", err);
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => (t.hidden = true), 5000);
}

export function initDropzone() {
  const veil = $("#dropveil");
  let depth = 0;
  const hide = () => {
    depth = 0;
    veil.hidden = true;
  };
  const hasFiles = (e: DragEvent) => !!e.dataTransfer && Array.from(e.dataTransfer.types).includes("Files");

  window.addEventListener("dragenter", (e) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    depth++;
    veil.hidden = !takesFiles(activeTab());
  });
  window.addEventListener("dragover", (e) => {
    if (hasFiles(e)) e.preventDefault(); // otherwise the web view would open the file
  });
  window.addEventListener("dragleave", (e) => {
    if (hasFiles(e) && --depth <= 0) hide();
  });
  window.addEventListener("drop", (e) => {
    if (hasFiles(e)) e.preventDefault();
    hide();
  });
  window.addEventListener("keydown", (e) => e.key === "Escape" && hide());
  window.addEventListener("blur", hide);

  onFileDrop(({ paths, x, y }) => {
    hide();
    void handle(paths, x, y);
  });

  async function handle(paths: string[], x: number, y: number) {
    const tab = activeTab();
    let items: DropItem[];
    try {
      items = await Rokit.classifyDrop(paths);
    } catch (e) {
      toast(errText(e), true);
      return;
    }
    const d = decideDrop(tab, items);
    if (!d.use) {
      if (d.message) toast(d.message, d.error);
      return;
    }
    try {
      if (tab === "audio") await dropAudio(d.use.path);
      else if (tab === "frames") await dropVideo(d.use.path);
      else await dropClothing(d.use.path, x, y);
      if (d.message) toast(d.message);
    } catch (e) {
      toast([errText(e), d.message].filter(Boolean).join(" "), true);
    }
  }
}
