import test from "node:test";
import assert from "node:assert/strict";
import { allRegions, BOX, checkClothingFile, faceSize, overlaps, pixelAt, PX_PER_STUD, readPngHeader, TEMPLATE_H, TEMPLATE_W, UV, validateClothingBytes } from "../src/clothing_uv.ts";

test("18 regions: 6 faces for each of 3 parts", () => {
  assert.equal(allRegions().length, 18);
  for (const p of Object.keys(UV)) assert.equal(Object.keys((UV as any)[p]).length, 6);
});

test("every region lies inside 585x559", () => {
  for (const { part, face, rect } of allRegions()) {
    assert.ok(rect.x >= 0 && rect.y >= 0 && rect.x + rect.w <= TEMPLATE_W && rect.y + rect.h <= TEMPLATE_H, `${part}.${face} outside`);
  }
});

test("no two regions overlap", () => {
  const r = allRegions();
  for (let i = 0; i < r.length; i++) for (let j = i + 1; j < r.length; j++) assert.ok(!overlaps(r[i]!.rect, r[j]!.rect), `${r[i]!.part}.${r[i]!.face} overlaps ${r[j]!.part}.${r[j]!.face}`);
});

test("region pixel size matches its box face (64 px per stud)", () => {
  for (const { part, face, rect } of allRegions()) {
    const [fw, fh] = faceSize(part, face);
    assert.equal(rect.w, fw * PX_PER_STUD, `${part}.${face} width`);
    assert.equal(rect.h, fh * PX_PER_STUD, `${part}.${face} height`);
  }
  assert.deepEqual(BOX.torso, [2, 2, 1]);
});

test("sizes follow the documented table (128x128, 64x128, 128x64, 64x64)", () => {
  assert.deepEqual([UV.torso.front.w, UV.torso.front.h, UV.torso.back.w, UV.torso.back.h], [128, 128, 128, 128]);
  assert.deepEqual([UV.torso.right.w, UV.torso.right.h, UV.torso.left.w, UV.torso.left.h], [64, 128, 64, 128]);
  assert.deepEqual([UV.torso.up.w, UV.torso.up.h, UV.torso.down.w, UV.torso.down.h], [128, 64, 128, 64]);
  assert.deepEqual([UV.rightLimb.up.w, UV.rightLimb.up.h, UV.leftLimb.down.w, UV.leftLimb.down.h], [64, 64, 64, 64]);
});

test("strip order matches the template: torso R F L B, right arm L B R F, left arm F L B R", () => {
  const xs = (p: "torso" | "rightLimb" | "leftLimb", f: string[]) => f.map((n) => (UV as any)[p][n].x);
  const inc = (a: number[]) => a.every((v, i) => i === 0 || v > a[i - 1]!);
  assert.ok(inc(xs("torso", ["right", "front", "left", "back"])));
  assert.ok(inc(xs("rightLimb", ["left", "back", "right", "front"])));
  assert.ok(inc(xs("leftLimb", ["front", "left", "back", "right"])));
  // torso up sits above front, down below it, same column
  assert.equal(UV.torso.up.x, UV.torso.front.x);
  assert.equal(UV.torso.down.x, UV.torso.front.x);
  assert.ok(UV.torso.up.y + UV.torso.up.h <= UV.torso.front.y && UV.torso.down.y >= UV.torso.front.y + UV.torso.front.h);
});

test("pixelAt maps face corners to the region corners", () => {
  assert.deepEqual(pixelAt("torso", "front", 0, 0), [231, 74]);
  assert.deepEqual(pixelAt("torso", "front", 1, 1), [359, 202]);
  assert.deepEqual(pixelAt("leftLimb", "down", 0.5, 0.5), [340, 517]);
});

function png(w: number, h: number): Uint8Array {
  const b = new Uint8Array(33);
  b.set([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52]);
  new DataView(b.buffer).setUint32(16, w);
  new DataView(b.buffer).setUint32(20, h);
  return b;
}

test("PNG validator: size, format, limit and the Indonesian messages", () => {
  assert.deepEqual(readPngHeader(png(585, 559)), { isPng: true, width: 585, height: 559 });
  assert.equal(validateClothingBytes(png(585, 559)), "");
  assert.equal(validateClothingBytes(png(128, 128)), "ukuran harus 585x559, gambar Anda 128x128");
  assert.equal(validateClothingBytes(png(585, 560)), "ukuran harus 585x559, gambar Anda 585x560");
  assert.equal(validateClothingBytes(new TextEncoder().encode("GIF89a................................")), "file harus berformat PNG");
  assert.equal(validateClothingBytes(new Uint8Array(0)), "file harus berformat PNG");
  assert.equal(checkClothingFile({ size: 5 * 1024 * 1024, width: 585, height: 559, isPng: true }), "file terlalu besar (5.0 MB); batas 4 MB");
  assert.equal(checkClothingFile({ size: 4 * 1024 * 1024, width: 585, height: 559, isPng: true }), "");
});
