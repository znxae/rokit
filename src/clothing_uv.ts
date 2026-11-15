// Pure data and checks for the classic clothing maker (no DOM, no three.js), so they can be tested with `node --test`.
//
// The rectangles were measured from Roblox's published "Classic clothing" template (585x559, create.roblox.com/docs/avatar/classic-clothing,
// whose size table gives 128x128 FRONT/BACK, 64x128 sides, 128x64 UP/DOWN, 64x64 arm/leg U/D). Roblox's template image is not shipped here.

export const TEMPLATE_W = 585;
export const TEMPLATE_H = 559;
export const MAX_BYTES = 4 * 1024 * 1024;

export type Rect = { x: number; y: number; w: number; h: number };
export type FaceName = "front" | "back" | "right" | "left" | "up" | "down";
export type PartName = "torso" | "rightLimb" | "leftLimb";

/** Faces are named as the avatar's own sides: "right" is the avatar's right (the template's R/L labels). */
export const UV: Record<PartName, Record<FaceName, Rect>> = {
  torso: {
    up: { x: 231, y: 8, w: 128, h: 64 },
    right: { x: 165, y: 74, w: 64, h: 128 },
    front: { x: 231, y: 74, w: 128, h: 128 },
    left: { x: 361, y: 74, w: 64, h: 128 },
    back: { x: 427, y: 74, w: 128, h: 128 },
    down: { x: 231, y: 204, w: 128, h: 64 },
  },
  // The template's "RIGHT ARM" strip (also the right leg of a pants image): L, B, R, F.
  rightLimb: {
    up: { x: 217, y: 289, w: 64, h: 64 },
    left: { x: 19, y: 355, w: 64, h: 128 },
    back: { x: 85, y: 355, w: 64, h: 128 },
    right: { x: 151, y: 355, w: 64, h: 128 },
    front: { x: 217, y: 355, w: 64, h: 128 },
    down: { x: 217, y: 485, w: 64, h: 64 },
  },
  // The template's "LEFT ARM" strip (left leg): F, L, B, R.
  leftLimb: {
    up: { x: 308, y: 289, w: 64, h: 64 },
    front: { x: 308, y: 355, w: 64, h: 128 },
    left: { x: 374, y: 355, w: 64, h: 128 },
    back: { x: 440, y: 355, w: 64, h: 128 },
    right: { x: 506, y: 355, w: 64, h: 128 },
    down: { x: 308, y: 485, w: 64, h: 64 },
  },
};

/** Box size (width, height, depth) of each part in R6 studs. */
export const BOX: Record<PartName, [number, number, number]> = { torso: [2, 2, 1], rightLimb: [1, 2, 1], leftLimb: [1, 2, 1] };

/** What a face of a box is in the template: its pixel size must match the box face (64 px per stud). */
export function faceSize(part: PartName, face: FaceName): [number, number] {
  const [w, h, d] = BOX[part];
  switch (face) {
    case "front":
    case "back":
      return [w, h];
    case "right":
    case "left":
      return [d, h];
    case "up":
    case "down":
      return [w, d];
  }
}

export const PX_PER_STUD = 64;

export function allRegions(): { part: PartName; face: FaceName; rect: Rect }[] {
  const out: { part: PartName; face: FaceName; rect: Rect }[] = [];
  for (const part of Object.keys(UV) as PartName[]) {
    for (const face of Object.keys(UV[part]) as FaceName[]) out.push({ part, face, rect: UV[part][face] });
  }
  return out;
}

export function overlaps(a: Rect, b: Rect): boolean {
  return a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
}

/** Why a picked file cannot be used, in Indonesian, or "" when it is fine. width/height come from the PNG header. */
export function checkClothingFile(f: { type?: string; size: number; width: number; height: number; isPng: boolean }): string {
  if (f.size > MAX_BYTES) return `file terlalu besar (${(f.size / 1048576).toFixed(1)} MB); batas 4 MB`;
  if (!f.isPng) return "file harus berformat PNG";
  if (f.width !== TEMPLATE_W || f.height !== TEMPLATE_H) return `ukuran harus ${TEMPLATE_W}x${TEMPLATE_H}, gambar Anda ${f.width}x${f.height}`;
  return "";
}

const SIG = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];

/** Reads the signature and IHDR of a PNG. width/height are 0 when it is not a PNG. */
export function readPngHeader(b: Uint8Array): { isPng: boolean; width: number; height: number } {
  if (b.length < 24 || SIG.some((v, i) => b[i] !== v) || b[12] !== 0x49 || b[13] !== 0x48 || b[14] !== 0x44 || b[15] !== 0x52) {
    return { isPng: false, width: 0, height: 0 };
  }
  const u32 = (o: number) => (((b[o]! << 24) | (b[o + 1]! << 16) | (b[o + 2]! << 8) | b[o + 3]!) >>> 0);
  return { isPng: true, width: u32(16), height: u32(20) };
}

/** Full check of the bytes of a picked or dropped file. */
export function validateClothingBytes(b: Uint8Array): string {
  const h = readPngHeader(b);
  return checkClothingFile({ size: b.length, width: h.width, height: h.height, isPng: h.isPng });
}

/** Which pixel of the template a (face, u, v) point maps to, for tests: u,v in 0..1 across the face, v down. */
export function pixelAt(part: PartName, face: FaceName, u: number, v: number): [number, number] {
  const r = UV[part][face];
  return [r.x + u * r.w, r.y + v * r.h];
}
