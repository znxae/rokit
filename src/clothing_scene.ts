// 3D preview of classic clothing on a blocky R6 avatar. three.js (MIT, bundled, works offline).
// The idea (a viewport that shows the uploaded image on an avatar) is general; the code here is our own.
import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { allRegions, BOX, TEMPLATE_H, TEMPLATE_W, UV, type FaceName, type PartName } from "./clothing_uv";

export type Look = { head: string; torso: string; leftArm: string; rightArm: string; leftLeg: string; rightLeg: string };

/** Neutral default avatar: skin tone for head and arms, grey torso, blue-grey legs. */
export const DEFAULT_LOOK: Look = { head: "#E8BF98", torso: "#B9BCC5", leftArm: "#E8BF98", rightArm: "#E8BF98", leftLeg: "#8794AA", rightLeg: "#8794AA" };

export type Background = "tema" | "kotak" | "bening";
export type Layer = "shirt" | "pants";
export type ImageSrc = HTMLImageElement | HTMLCanvasElement | ImageBitmap;

type V3 = [number, number, number];

/** [top-left, top-right, bottom-left] of a face as seen from outside; the avatar faces +Z, its right side is -X. */
function corners(face: FaceName, hx: number, hy: number, hz: number): [V3, V3, V3] {
  switch (face) {
    case "front": return [[-hx, hy, hz], [hx, hy, hz], [-hx, -hy, hz]];
    case "back": return [[hx, hy, -hz], [-hx, hy, -hz], [hx, -hy, -hz]];
    case "right": return [[-hx, hy, -hz], [-hx, hy, hz], [-hx, -hy, -hz]];
    case "left": return [[hx, hy, hz], [hx, hy, -hz], [hx, -hy, hz]];
    case "up": return [[-hx, hy, -hz], [hx, hy, -hz], [-hx, hy, hz]];
    case "down": return [[-hx, -hy, hz], [hx, -hy, hz], [-hx, -hy, -hz]];
  }
}

const FACES: FaceName[] = ["front", "back", "right", "left", "up", "down"];

/** A box whose faces are mapped to the classic template rectangles of `part` (UV table in clothing_uv.ts). */
export function partGeometry(part: PartName): THREE.BufferGeometry {
  const [w, h, d] = BOX[part];
  const [hx, hy, hz] = [w / 2, h / 2, d / 2];
  const pos: number[] = [];
  const uv: number[] = [];
  const nrm: number[] = [];
  const idx: number[] = [];
  FACES.forEach((face, i) => {
    const [tl, tr, bl] = corners(face, hx, hy, hz);
    const br: V3 = [tr[0] + bl[0] - tl[0], tr[1] + bl[1] - tl[1], tr[2] + bl[2] - tl[2]];
    const down = new THREE.Vector3(bl[0] - tl[0], bl[1] - tl[1], bl[2] - tl[2]);
    const right = new THREE.Vector3(tr[0] - tl[0], tr[1] - tl[1], tr[2] - tl[2]);
    const n = down.cross(right).normalize();
    const r = UV[part][face];
    const u0 = r.x / TEMPLATE_W, u1 = (r.x + r.w) / TEMPLATE_W;
    const vT = 1 - r.y / TEMPLATE_H, vB = 1 - (r.y + r.h) / TEMPLATE_H;
    for (const [p, u, v] of [[tl, u0, vT], [tr, u1, vT], [br, u1, vB], [bl, u0, vB]] as [V3, number, number][]) {
      pos.push(...p);
      uv.push(u, v);
      nrm.push(n.x, n.y, n.z);
    }
    const o = i * 4;
    idx.push(o, o + 3, o + 2, o, o + 2, o + 1); // TL, BL, BR / TL, BR, TR: counter-clockwise from outside
  });
  const g = new THREE.BufferGeometry();
  g.setAttribute("position", new THREE.Float32BufferAttribute(pos, 3));
  g.setAttribute("uv", new THREE.Float32BufferAttribute(uv, 2));
  g.setAttribute("normal", new THREE.Float32BufferAttribute(nrm, 3));
  g.setIndex(idx);
  return g;
}

/** Our own simple face (two eyes and a smile), drawn on a canvas. No Roblox assets. */
function faceTexture(): THREE.CanvasTexture {
  const c = document.createElement("canvas");
  c.width = 256;
  c.height = 128;
  const g = c.getContext("2d")!;
  g.fillStyle = "#1d1d1f";
  g.fillRect(78, 40, 18, 26);
  g.fillRect(160, 40, 18, 26);
  g.strokeStyle = "#1d1d1f";
  g.lineWidth = 8;
  g.lineCap = "round";
  g.beginPath();
  g.arc(128, 62, 44, 0.25 * Math.PI, 0.75 * Math.PI);
  g.stroke();
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  return t;
}

/** Image plus (optionally) the region outlines, as one 585x559 canvas. */
function composeTexture(src: ImageSrc, lines: boolean): THREE.CanvasTexture {
  const c = document.createElement("canvas");
  c.width = TEMPLATE_W;
  c.height = TEMPLATE_H;
  const g = c.getContext("2d")!;
  g.drawImage(src, 0, 0, TEMPLATE_W, TEMPLATE_H);
  if (lines) {
    g.textAlign = "center";
    g.textBaseline = "middle";
    g.font = '700 12px "Segoe UI", system-ui, sans-serif';
    const L: Record<FaceName, string> = { front: "D", back: "B", right: "Kn", left: "Ki", up: "A", down: "W" };
    for (const { face, rect } of allRegions()) {
      g.strokeStyle = "rgba(255,255,255,0.9)";
      g.lineWidth = 3;
      g.strokeRect(rect.x + 1.5, rect.y + 1.5, rect.w - 3, rect.h - 3);
      g.strokeStyle = "rgba(0,0,0,0.85)";
      g.lineWidth = 1;
      g.strokeRect(rect.x + 1.5, rect.y + 1.5, rect.w - 3, rect.h - 3);
      g.lineWidth = 3;
      g.strokeStyle = "rgba(0,0,0,0.7)";
      g.strokeText(L[face], rect.x + rect.w / 2, rect.y + 12);
      g.fillStyle = "#fff";
      g.fillText(L[face], rect.x + rect.w / 2, rect.y + 12);
    }
  }
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  t.anisotropy = 8;
  return t;
}


// R6 layout in studs: legs 0-2, torso 2-4, head 4-5. The avatar's right limbs are at -X.
const BODY: { key: keyof Look; size: V3; pos: V3 }[] = [
  { key: "head", size: [2, 1, 1], pos: [0, 4.5, 0] },
  { key: "torso", size: [2, 2, 1], pos: [0, 3, 0] },
  { key: "rightArm", size: [1, 2, 1], pos: [-1.5, 3, 0] },
  { key: "leftArm", size: [1, 2, 1], pos: [1.5, 3, 0] },
  { key: "rightLeg", size: [1, 2, 1], pos: [-0.5, 1, 0] },
  { key: "leftLeg", size: [1, 2, 1], pos: [0.5, 1, 0] },
];

const LAYER_PARTS: Record<Layer, { uv: PartName; at: V3; scale: number }[]> = {
  shirt: [
    { uv: "torso", at: [0, 3, 0], scale: 1.03 },
    { uv: "rightLimb", at: [-1.5, 3, 0], scale: 1.03 },
    { uv: "leftLimb", at: [1.5, 3, 0], scale: 1.03 },
  ],
  pants: [
    { uv: "torso", at: [0, 3, 0], scale: 1.015 },
    { uv: "rightLimb", at: [-0.5, 1, 0], scale: 1.015 },
    { uv: "leftLimb", at: [0.5, 1, 0], scale: 1.015 },
  ],
};

export class ClothingViewer {
  readonly ok: boolean;
  private renderer?: THREE.WebGLRenderer;
  private scene = new THREE.Scene();
  private camera = new THREE.PerspectiveCamera(32, 1, 0.1, 100);
  private controls?: OrbitControls;
  private bodyMats = new Map<keyof Look, THREE.MeshLambertMaterial>();
  private layers: Record<Layer, { group: THREE.Group; src: ImageSrc | null }> = { shirt: { group: new THREE.Group(), src: null }, pants: { group: new THREE.Group(), src: null } };
  private overlay = false;
  private ro?: ResizeObserver;
  private raf = 0;
  private geoms = new Map<PartName, THREE.BufferGeometry>();

  constructor(private host: HTMLElement) {
    try {
      this.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: false });
    } catch {
      this.ok = false;
      return;
    }
    this.ok = true;
    const r = this.renderer;
    r.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    r.setClearColor(0x000000, 0);
    r.domElement.style.cssText = "position:absolute;inset:0;width:100%;height:100%;display:block;touch-action:none";
    r.domElement.setAttribute("aria-label", "Pratinjau 3D avatar");
    host.append(r.domElement);

    this.scene.add(new THREE.AmbientLight(0xffffff, 1.5));
    const key = new THREE.DirectionalLight(0xffffff, 2.2);
    key.position.set(3, 6, 8);
    const fill = new THREE.DirectionalLight(0xffffff, 0.8);
    fill.position.set(-5, 2, -6);
    this.scene.add(key, fill);

    const avatar = new THREE.Group();
    for (const b of BODY) {
      const mat = new THREE.MeshLambertMaterial({ color: DEFAULT_LOOK[b.key] });
      this.bodyMats.set(b.key, mat);
      const m = new THREE.Mesh(new THREE.BoxGeometry(...b.size), mat);
      m.position.set(...b.pos);
      avatar.add(m);
    }
    const face = new THREE.Mesh(new THREE.PlaneGeometry(2, 1), new THREE.MeshBasicMaterial({ map: faceTexture(), transparent: true, depthWrite: false }));
    face.position.set(0, 4.5, 0.505);
    avatar.add(face);
    this.layers.pants.group.renderOrder = 1;
    this.layers.shirt.group.renderOrder = 2;
    avatar.add(this.layers.pants.group, this.layers.shirt.group);
    this.scene.add(avatar);

    this.controls = new OrbitControls(this.camera, r.domElement);
    this.controls.enableDamping = true;
    this.controls.enablePan = false;
    this.controls.minDistance = 6;
    this.controls.maxDistance = 24;
    this.controls.autoRotateSpeed = 2.2;
    this.resetCamera();

    this.ro = new ResizeObserver(() => this.resize());
    this.ro.observe(host);
    this.resize();
    const loop = () => {
      this.raf = requestAnimationFrame(loop);
      if (host.clientWidth === 0 || host.clientHeight === 0 || document.hidden) return;
      this.controls?.update();
      r.render(this.scene, this.camera);
    };
    loop();
  }

  private resize() {
    const w = this.host.clientWidth, h = this.host.clientHeight;
    if (!this.renderer || w === 0 || h === 0) return;
    this.renderer.setSize(w, h, false);
    this.camera.aspect = w / h;
    this.camera.updateProjectionMatrix();
  }

  setLook(look: Look) {
    for (const [k, m] of this.bodyMats) m.color.set(look[k]);
  }

  setLayer(kind: Layer, src: ImageSrc | null) {
    const L = this.layers[kind];
    L.src = src;
    this.rebuild(kind);
  }

  setOverlay(on: boolean) {
    this.overlay = on;
    this.rebuild("shirt");
    this.rebuild("pants");
  }

  private rebuild(kind: Layer) {
    const L = this.layers[kind];
    for (const ch of [...L.group.children]) {
      L.group.remove(ch);
      const m = ch as THREE.Mesh;
      (m.material as THREE.MeshLambertMaterial).map?.dispose();
      (m.material as THREE.Material).dispose();
    }
    if (!L.src) return;
    for (const p of LAYER_PARTS[kind]) {
      let g = this.geoms.get(p.uv);
      if (!g) this.geoms.set(p.uv, (g = partGeometry(p.uv)));
      const mat = new THREE.MeshLambertMaterial({ map: composeTexture(L.src, this.overlay), transparent: true, depthWrite: false, polygonOffset: true, polygonOffsetFactor: -1, polygonOffsetUnits: -1 });
      const mesh = new THREE.Mesh(g, mat);
      mesh.position.set(...p.at);
      mesh.scale.setScalar(p.scale);
      mesh.renderOrder = kind === "shirt" ? 2 : 1;
      L.group.add(mesh);
    }
  }

  setAutoRotate(on: boolean) {
    if (this.controls) this.controls.autoRotate = on;
  }

  resetCamera() {
    this.camera.position.set(0, 3.6, 12.5);
    this.controls?.target.set(0, 2.6, 0);
    this.controls?.update();
  }

  /** Turns the avatar to a given angle (radians around Y) for tests and screenshots. */
  orbitTo(azimuth: number, polar = Math.PI / 2.3, dist = 12.5) {
    const t = this.controls?.target ?? new THREE.Vector3(0, 2.6, 0);
    this.camera.position.set(t.x + dist * Math.sin(polar) * Math.sin(azimuth), t.y + dist * Math.cos(polar), t.z + dist * Math.sin(polar) * Math.cos(azimuth));
    this.controls?.update();
  }

  /** PNG data URL of the viewport on the chosen background ("bening" keeps it transparent). */
  snapshot(bg: Background, themeColor = "#ffffff"): string {
    const r = this.renderer!;
    r.render(this.scene, this.camera);
    const src = r.domElement;
    const out = document.createElement("canvas");
    out.width = src.width;
    out.height = src.height;
    const g = out.getContext("2d")!;
    if (bg === "tema") {
      g.fillStyle = themeColor;
      g.fillRect(0, 0, out.width, out.height);
    } else if (bg === "kotak") {
      const s = 16 * (src.width / Math.max(1, this.host.clientWidth));
      for (let y = 0; y < out.height; y += s) for (let x = 0; x < out.width; x += s) {
        g.fillStyle = ((x / s + y / s) & 1) === 0 ? "#d9d9de" : "#f1f1f4";
        g.fillRect(x, y, s, s);
      }
    }
    g.drawImage(src, 0, 0);
    return out.toDataURL("image/png");
  }

  dispose() {
    cancelAnimationFrame(this.raf);
    this.ro?.disconnect();
    this.controls?.dispose();
    this.renderer?.dispose();
    this.renderer?.domElement.remove();
  }
}
