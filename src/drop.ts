// What a file dropped on the window means in each tab. Pure, so `node --test` can check it.
export type TabName = "audio" | "frames" | "download" | "roblox" | "clothing" | string;
export type DropItem = { path: string; name: string; ext: string; kind: string };
export type DropDecision = { use?: DropItem; message?: string; error?: boolean };

export const NOT_HERE = "File ini tidak bisa dipakai di tab ini";

const WANT: Record<string, { kinds: string[]; what: string }> = {
  audio: { kinds: ["audio", "video"], what: "audio atau video" },
  frames: { kinds: ["video"], what: "video" },
  clothing: { kinds: ["image"], what: "PNG" },
};

/** Tabs that take files at all (the overlay shows only there). */
export const takesFiles = (tab: TabName) => tab in WANT;

export function decideDrop(tab: TabName, items: DropItem[]): DropDecision {
  const w = WANT[tab];
  if (!w) return { message: NOT_HERE, error: true };
  const first = items[0];
  if (!first) return {};
  const note = items.length > 1 ? `Hanya 1 file diproses; dipakai: ${first.name}` : "";
  if (first.kind === "folder") return { message: `Itu folder, bukan file. Lepas satu file ${w.what}.`, error: true };
  if (first.kind === "missing") return { message: `File tidak ditemukan: ${first.name}`, error: true };
  if (w.kinds.includes(first.kind)) return { use: first, message: note };
  if (first.kind === "other") return { message: `File ini bukan ${w.what} (${first.ext || "tanpa ekstensi"}).`, error: true };
  return { message: NOT_HERE, error: true };
}
