import test from "node:test";
import assert from "node:assert/strict";
import { decideDrop, NOT_HERE, takesFiles } from "../src/drop.ts";

const it = (name: string, kind: string) => ({ path: "C:/x/" + name, name, ext: name.includes(".") ? "." + name.split(".").pop() : "", kind });

test("audio tab takes audio and video", () => {
  assert.equal(decideDrop("audio", [it("a.mp3", "audio")]).use?.name, "a.mp3");
  assert.equal(decideDrop("audio", [it("v.mp4", "video")]).use?.name, "v.mp4");
});
test("frames tab takes video only", () => {
  assert.equal(decideDrop("frames", [it("v.mp4", "video")]).use?.name, "v.mp4");
  assert.equal(decideDrop("frames", [it("a.mp3", "audio")]).message, NOT_HERE);
});
test("clothing tab takes png only", () => {
  assert.equal(decideDrop("clothing", [it("s.png", "image")]).use?.name, "s.png");
  assert.equal(decideDrop("clothing", [it("a.mp3", "audio")]).message, NOT_HERE);
});
test("other tabs take nothing", () => {
  for (const t of ["download", "roblox"]) {
    assert.equal(decideDrop(t, [it("a.mp3", "audio")]).message, NOT_HERE);
    assert.equal(takesFiles(t), false);
  }
});
test("several files: first one, with a note", () => {
  const d = decideDrop("audio", [it("a.mp3", "audio"), it("b.mp3", "audio"), it("c.mp3", "audio")]);
  assert.equal(d.use?.name, "a.mp3");
  assert.equal(d.message, "Hanya 1 file diproses; dipakai: a.mp3");
});
test("folder, text file, missing file", () => {
  assert.match(decideDrop("audio", [it("dir", "folder")]).message!, /folder/);
  assert.match(decideDrop("audio", [it("n.txt", "other")]).message!, /bukan audio atau video \(\.txt\)/);
  assert.match(decideDrop("clothing", [it("n.txt", "other")]).message!, /bukan PNG/);
  assert.match(decideDrop("audio", [it("gone.mp3", "missing")]).message!, /tidak ditemukan/);
  assert.equal(decideDrop("audio", []).use, undefined);
});
