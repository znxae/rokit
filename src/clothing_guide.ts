// Our own labelled guide for the 585x559 classic clothing layout (drawn here, not Roblox's template image).
// Each region has its own color and an Indonesian label, so a mistake in orientation is easy to see on the avatar.
import { allRegions, TEMPLATE_H, TEMPLATE_W, type FaceName, type PartName } from "./clothing_uv";

const FACE_LABEL: Record<FaceName, string> = { front: "DEPAN", back: "BELAKANG", right: "KANAN", left: "KIRI", up: "ATAS", down: "BAWAH" };
const FACE_HUE: Record<FaceName, number> = { front: 8, back: 205, right: 150, left: 40, up: 275, down: 330 };
const PART_TITLE: Record<PartName, string> = { torso: "BADAN", rightLimb: "LENGAN / KAKI KANAN", leftLimb: "LENGAN / KAKI KIRI" };

export function drawGuide(canvas: HTMLCanvasElement, opts: { notes?: boolean } = { notes: true }) {
  canvas.width = TEMPLATE_W;
  canvas.height = TEMPLATE_H;
  const c = canvas.getContext("2d")!;
  c.clearRect(0, 0, TEMPLATE_W, TEMPLATE_H);
  if (opts.notes) {
    c.fillStyle = "#f4f4f6";
    c.fillRect(0, 0, TEMPLATE_W, TEMPLATE_H);
  }
  c.textAlign = "center";
  c.textBaseline = "middle";
  for (const { part, face, rect } of allRegions()) {
    c.fillStyle = `hsl(${FACE_HUE[face]} 70% ${face === "up" || face === "down" ? 74 : 68}%)`;
    c.fillRect(rect.x, rect.y, rect.w, rect.h);
    c.strokeStyle = "rgba(0,0,0,0.55)";
    c.lineWidth = 2;
    c.strokeRect(rect.x + 1, rect.y + 1, rect.w - 2, rect.h - 2);
    c.fillStyle = "#1d1d1f";
    const small = rect.w < 100;
    c.font = `700 ${small ? 11 : 17}px "Segoe UI", system-ui, sans-serif`;
    const word = FACE_LABEL[face];
    c.fillText(word, rect.x + rect.w / 2, rect.y + rect.h / 2);
    if (rect.h >= 100 && rect.w >= 100) {
      c.font = '600 11px "Segoe UI", system-ui, sans-serif';
      c.fillText(PART_TITLE[part], rect.x + rect.w / 2, rect.y + rect.h / 2 + 20);
    }
  }
  if (opts.notes) {
    c.fillStyle = "#4a4a50";
    c.textAlign = "left";
    c.font = '700 13px "Segoe UI", system-ui, sans-serif';
    c.fillText("PANDUAN", 14, 24);
    c.font = '12px "Segoe UI", system-ui, sans-serif';
    ["585 x 559 px", "", "Gambar di atas bagian", "berwarna, simpan", "sebagai PNG.", "", "Transparan = warna", "tubuh avatar.", "", "Kemeja: badan +", "2 lengan.", "Celana: badan bawah", "+ 2 kaki.", "", "ATAS/BAWAH lengan", "dan kaki: 64 x 64 px."].forEach((t, i) => c.fillText(t, 14, 46 + i * 16));
    c.fillText("Panduan buatan Rokit", 16, TEMPLATE_H - 14);
  }
}
