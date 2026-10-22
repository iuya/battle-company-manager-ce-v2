// 3D dice for the roll panels: three.js draws them, cannon-es rolls them. Only what this file imports ends up in
// static/js/dice3d.js (see build.mjs), so the bundle holds a small part of both libraries.
//
// Dice3DTray has the interface of the 2D DiceTray of dice.js: new Dice3DTray(canvas, { count, values, groups, labels, onStop }),
// roll(), shake(), destroy(). Two ways to throw:
//   - values given (the server rolled): the throw is simulated first, without drawing, and the faces of each die are relabelled so
//     that the face that lands on top shows the wanted number (a die is symmetric, so any valid labelling is an honest die). The
//     recorded throw is then played back. Nothing can change the outcome, so shake() does nothing.
//   - values null (the browser decides): the physics run live and the number is read from the face that ends up on top. shake()
//     jolts the table: every die, also one that has come to rest, is thrown up and spun again.
import {
  WebGLRenderer, Scene, PerspectiveCamera, AmbientLight, DirectionalLight, Mesh, PlaneGeometry, MeshStandardMaterial,
  ShadowMaterial, CanvasTexture, SRGBColorSpace, PCFSoftShadowMap, Color, Vector3, Quaternion, Euler, MeshBasicMaterial,
} from 'three';
import { RoundedBoxGeometry } from 'three/examples/jsm/geometries/RoundedBoxGeometry.js';
import { World, Body, Box, Plane, Vec3, Material, ContactMaterial, Quaternion as CQuaternion } from 'cannon-es';

const rand = (a, b) => a + Math.random() * (b - a);
const STEP = 1 / 120;            // physics step, small so that fast dice do not pass through the floor
const TABLE_W = 10;
const laneDepth = lanes => lanes > 1 ? 3.1 : 2.4; // two rolls get more room each: dice thrown across a lane hit its divider
const GUTTER = 2.4; // the right edge of the table is kept free for the names and totals of the lanes // world units: the table's width and the depth of one lane
const TILT = 70 * Math.PI / 180;  // camera angle above the table
const PIPS = { 1: [[0, 0]], 2: [[-1, -1], [1, 1]], 3: [[-1, -1], [0, 0], [1, 1]], 4: [[-1, -1], [1, -1], [-1, 1], [1, 1]],
  5: [[-1, -1], [1, -1], [0, 0], [-1, 1], [1, 1]], 6: [[-1, -1], [1, -1], [-1, 0], [1, 0], [-1, 1], [1, 1]] };
// The faces of a die in the order of BoxGeometry (+x, -x, +y, -y, +z, -z) and their normals; opposite faces add up to 7.
const NORMALS = [new Vector3(1, 0, 0), new Vector3(-1, 0, 0), new Vector3(0, 1, 0), new Vector3(0, -1, 0), new Vector3(0, 0, 1), new Vector3(0, 0, -1)];
const BASE_LABELS = [1, 6, 2, 5, 3, 4];
const opposite = i => i ^ 1;
const reducedMotion = () => window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches;

// One renderer per canvas: a canvas keeps its WebGL context, so a second throw on it reuses everything but the dice.
const stages = new WeakMap();

function faceTexture(value, ivory, ink) {
  const c = document.createElement('canvas');
  c.width = c.height = 128;
  const g = c.getContext('2d');
  g.fillStyle = ivory; g.fillRect(0, 0, 128, 128);
  g.fillStyle = ink;
  for (const [px, py] of PIPS[value]) { g.beginPath(); g.arc(64 + px * 30, 64 + py * 30, value === 1 ? 17 : 12, 0, Math.PI * 2); g.fill(); }
  const t = new CanvasTexture(c);
  t.colorSpace = SRGBColorSpace;
  return t;
}

function labelMesh(text, color, size, align) {
  const c = document.createElement('canvas');
  c.width = 512; c.height = 128;
  const g = c.getContext('2d');
  g.fillStyle = color; g.font = `600 ${size}px system-ui, sans-serif`; g.textBaseline = 'middle'; g.textAlign = align;
  g.fillText(text, align === 'left' ? 8 : align === 'center' ? 256 : 504, 64);
  const t = new CanvasTexture(c);
  t.colorSpace = SRGBColorSpace;
  const m = new Mesh(new PlaneGeometry(2.2, 0.55), new MeshBasicMaterial({ map: t, transparent: true, depthWrite: false }));
  m.rotation.x = -Math.PI / 2;
  m.userData.dispose = () => { t.dispose(); m.geometry.dispose(); m.material.dispose(); };
  return m;
}

class Stage { // what stays between throws: renderer, scene, camera, lights, textures
  constructor(canvas) {
    this.canvas = canvas;
    this.renderer = new WebGLRenderer({ canvas, antialias: true });
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = PCFSoftShadowMap;
    this.scene = new Scene();
    this.camera = new PerspectiveCamera(30, 2, 0.1, 100);
    this.scene.add(new AmbientLight(0xffffff, 1.6));
    const sun = new DirectionalLight(0xffffff, 2.2);
    sun.position.set(-3, 9, 4);
    sun.castShadow = true;
    sun.shadow.mapSize.set(1024, 1024);
    Object.assign(sun.shadow.camera, { left: -8, right: 8, top: 8, bottom: -8, near: 1, far: 30 });
    this.scene.add(sun);
    this.textures = new Map();
    this.floor = new Mesh(new PlaneGeometry(40, 40), new MeshStandardMaterial({ roughness: 0.95 }));
    this.floor.rotation.x = -Math.PI / 2;
    this.floor.receiveShadow = true;
    this.scene.add(this.floor);
    this.dice = [];     // meshes of the current throw
    this.extras = [];   // lane lines and labels of the current throw
  }
  material(value) {
    if (!this.textures.has(value)) {
      this.textures.set(value, new MeshStandardMaterial({ map: faceTexture(value, '#f1ecd9', '#1d1a14'), roughness: 0.4, metalness: 0.05 }));
    }
    return this.textures.get(value);
  }
  clear() {
    for (const m of this.dice) { this.scene.remove(m); m.geometry.dispose(); }
    for (const m of this.extras) { this.scene.remove(m); m.userData.dispose && m.userData.dispose(); if (!m.userData.dispose) { m.geometry.dispose(); m.material.dispose(); } }
    this.dice = []; this.extras = [];
  }
}

// How tall a canvas of this width should be for a table of this many lanes.
function heightFor(width, lanes) {
  return Math.round(width * (lanes * laneDepth(lanes) * Math.sin(TILT) + 1.1) / (TABLE_W + 0.8));
}

class Dice3DTray {
  constructor(canvas, opts = {}) {
    this.canvas = canvas;
    this.groups = opts.groups && opts.groups.length ? opts.groups : [opts.count || 1];
    this.labels = opts.labels || [];
    this.count = this.groups.reduce((a, b) => a + b, 0);
    this.laneOf = this.groups.flatMap((n, lane) => Array(n).fill(lane));
    this.values = opts.values || null;
    this.onStop = opts.onStop || (() => {});
    this.rolling = false;
    this.shaken = false; // a roll may be shaken once
    this.raf = 0;
    if (!stages.has(canvas)) stages.set(canvas, new Stage(canvas)); // throws when WebGL is not available
    this.stage = stages.get(canvas);
  }

  static heightFor(width, lanes) { return heightFor(width, lanes); }
  static supported() {
    try { const c = document.createElement('canvas'); return !!(c.getContext('webgl2') || c.getContext('webgl')); } catch (e) { return false; }
  }

  // ---- setting up a throw ----
  setup() {
    const st = this.stage, canvas = this.canvas, lanes = this.groups.length, LANE_D = laneDepth(lanes), depth = lanes * LANE_D;
    const style = getComputedStyle(canvas);
    const w = canvas.clientWidth || 600, h = canvas.clientHeight || heightFor(w, lanes);
    st.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    st.renderer.setSize(w, h, false);
    st.renderer.setClearColor(new Color(style.backgroundColor));
    st.floor.material.color.set(style.backgroundColor);
    // the camera looks down at the table from the front, far enough to see all of it
    const cam = st.camera;
    cam.aspect = w / h;
    const vHalf = (cam.fov * Math.PI / 360), hHalf = Math.atan(Math.tan(vHalf) * cam.aspect);
    const dist = Math.max((TABLE_W / 2 + 0.4) / Math.tan(hHalf), (depth * Math.sin(TILT) / 2 + 0.7) / Math.tan(vHalf));
    cam.position.set(0, dist * Math.sin(TILT), dist * Math.cos(TILT));
    cam.lookAt(0, 0, 0);
    cam.updateProjectionMatrix();

    st.clear();
    // lane lines and names
    const line = style.borderTopColor, ink = style.color;
    for (let l = 1; l < lanes; l++) {
      const m = new Mesh(new PlaneGeometry(TABLE_W, 0.04), new MeshBasicMaterial({ color: line }));
      m.rotation.x = -Math.PI / 2; m.position.set(0, 0.005, -depth / 2 + l * LANE_D);
      st.scene.add(m); st.extras.push(m);
    }
    if (lanes > 1) {
      this.names = this.groups.map((_, l) => {
        const m = labelMesh(this.labels[l] || '', ink, 56, 'center');
        m.material.opacity = 0.6;
        m.position.set(TABLE_W / 2 - GUTTER / 2, 0.01, -depth / 2 + l * LANE_D + 0.55);
        st.scene.add(m); st.extras.push(m);
        return m;
      });
    }
    // physics: floor, ceiling, walls and lane dividers
    const world = new World({ gravity: new Vec3(0, -32, 0) });
    world.allowSleep = true;
    const mat = new Material('table'), dieMat = new Material('die');
    world.addContactMaterial(new ContactMaterial(dieMat, mat, { friction: 0.3, restitution: 0.3 }));
    world.addContactMaterial(new ContactMaterial(dieMat, dieMat, { friction: 0.2, restitution: 0.3 }));
    const wall = (x, y, z, hx, hy, hz) => {
      const b = new Body({ mass: 0, material: mat, shape: new Box(new Vec3(hx, hy, hz)), position: new Vec3(x, y, z) });
      world.addBody(b);
    };
    const floor = new Body({ mass: 0, material: mat, shape: new Plane() });
    floor.quaternion.setFromEuler(-Math.PI / 2, 0, 0);
    world.addBody(floor);
    const ceiling = new Body({ mass: 0, material: mat, shape: new Plane(), position: new Vec3(0, 6, 0) });
    ceiling.quaternion.setFromEuler(Math.PI / 2, 0, 0);
    world.addBody(ceiling);
    wall(-TABLE_W / 2 - 0.5, 3, 0, 0.5, 3, depth); wall(TABLE_W / 2 - (lanes > 1 ? GUTTER : 0) + 0.5, 3, 0, 0.5, 3, depth);
    wall(0, 3, -depth / 2 - 0.5, TABLE_W, 3, 0.5); wall(0, 3, depth / 2 + 0.5, TABLE_W, 3, 0.5);
    for (let l = 1; l < lanes; l++) wall(0, 3, -depth / 2 + l * LANE_D, TABLE_W, 3, 0.03); // up to the ceiling: a shaken die cannot cross lanes
    this.world = world;

    // dice: thrown from the left of their lane
    const geometry = () => new RoundedBoxGeometry(1, 1, 1, 3, 0.12);
    this.bodies = []; this.meshes = []; this.faceLabels = [];
    const inLane = this.laneOf.map((lane, i) => this.laneOf.slice(0, i).filter(l => l === lane).length);
    for (let i = 0; i < this.count; i++) {
      const lane = this.laneOf[i], zc = -depth / 2 + (lane + 0.5) * LANE_D;
      const body = new Body({ mass: 1, material: dieMat, shape: new Box(new Vec3(0.5, 0.5, 0.5)), linearDamping: 0.2, angularDamping: 0.25,
        position: new Vec3(-TABLE_W / 2 + 0.9 + inLane[i] * 0.2, 1.4 + rand(0, 1) + inLane[i] * 0.3, zc + rand(-0.35, 0.35)),
        sleepSpeedLimit: 0.2, sleepTimeLimit: 0.35, allowSleep: true });
      body.quaternion.setFromEuler(rand(0, 6.28), rand(0, 6.28), rand(0, 6.28));
      body.velocity.set(rand(12, 17), rand(-1, 2), rand(-1.8, 1.8));
      body.angularVelocity.set(rand(-18, 18), rand(-18, 18), rand(-18, 18));
      world.addBody(body);
      const mesh = new Mesh(geometry(), BASE_LABELS.map(v => st.material(v)));
      mesh.castShadow = true;
      st.scene.add(mesh); st.dice.push(mesh);
      this.bodies.push(body); this.meshes.push(mesh); this.faceLabels.push(BASE_LABELS.slice());
    }
    this.sync();
  }

  sync() { // puts the meshes where the bodies are
    this.bodies.forEach((b, i) => { this.meshes[i].position.copy(b.position); this.meshes[i].quaternion.set(b.quaternion.x, b.quaternion.y, b.quaternion.z, b.quaternion.w); });
  }

  upFace(quat) { // the index of the face that points up
    const q = new Quaternion(quat.x, quat.y, quat.z, quat.w);
    let best = 0, bestDot = -2;
    NORMALS.forEach((n, i) => { const d = n.clone().applyQuaternion(q).y; if (d > bestDot) { bestDot = d; best = i; } });
    return { index: best, dot: bestDot };
  }

  // After about two seconds the table gets stickier, so that a roll never drags on.
  drag(step) {
    if (step < 240) return;
    for (const b of this.bodies) { b.velocity.scale(0.97, b.velocity); b.angularVelocity.scale(0.95, b.angularVelocity); }
  }

  // The orientation nearest to q in which a face points straight up.
  flatten(q) {
    const quat = new Quaternion(q[0], q[1], q[2], q[3]);
    const n = NORMALS[this.upFace(quat).index].clone().applyQuaternion(quat);
    return new Quaternion().setFromUnitVectors(n, new Vector3(0, 1, 0)).multiply(quat);
  }

  settled() { return this.bodies.every(b => b.sleepState === Body.SLEEPING); }

  // ---- throwing ----
  roll() {
    cancelAnimationFrame(this.raf);
    this.setup();
    this.rolling = true;
    if (reducedMotion()) { this.finishNow(); return; }
    if (this.values && this.values.length >= this.count) this.simulate(); else this.live();
  }

  // The server rolled: simulate (not drawn) until the dice rest, relabel the faces, and play the recording back.
  simulate() {
    let frames = null;
    for (let attempt = 0; attempt < 4 && !frames; attempt++) {
      if (attempt > 0) this.setup();
      frames = this.record();
    }
    if (!frames) frames = this.record(true);
    const last = frames[frames.length - 1];
    this.bodies.forEach((b, i) => { // relabel so that the face that came up shows the wanted number
      const { index } = this.upFace({ x: last[i].q[0], y: last[i].q[1], z: last[i].q[2], w: last[i].q[3] });
      const labels = BASE_LABELS.slice(), want = this.values[i];
      if (labels[index] !== want) {
        const g = labels.indexOf(want);
        if (g === opposite(index)) { [labels[index], labels[g]] = [labels[g], labels[index]]; }
        else {
          [labels[index], labels[g]] = [labels[g], labels[index]];
          [labels[opposite(index)], labels[opposite(g)]] = [labels[opposite(g)], labels[opposite(index)]];
        }
      }
      this.faceLabels[i] = labels;
      this.meshes[i].material = labels.map(v => this.stage.material(v));
    });
    this.frames = frames;
    this.start = performance.now();
    this.mode = 'playback';
    this.raf = requestAnimationFrame(t => this.play(t));
  }

  record(force) { // runs the throw to the end; null when a die ended up leaning (the caller throws again)
    const frames = [];
    for (let step = 0, rest = 0; step < 120 * 9; step++) {
      this.drag(step);
      this.world.step(STEP);
      if (step % 2 === 0) frames.push(this.bodies.map(b => ({ p: [b.position.x, b.position.y, b.position.z], q: [b.quaternion.x, b.quaternion.y, b.quaternion.z, b.quaternion.w] })));
      rest = this.settled() ? rest + 1 : 0;
      if (rest > 12 && step > 120) break;
    }
    const cocked = this.bodies.some(b => this.upFace(b.quaternion).dot < 0.97 || b.position.y > 0.75); // leaning, or lying on another die
    if (cocked && !force) return null;
    // a die that rests slightly off true is eased flat over the last moments
    const last = frames[frames.length - 1], flat = last.map(f => this.flatten(f.q));
    for (let k = 1; k <= 10; k++) {
      frames.push(last.map((f, d) => ({ p: [f.p[0], f.p[1] + (0.5 - f.p[1]) * k / 10, f.p[2]],
        q: new Quaternion(...f.q).slerp(flat[d], k / 10).toArray() })));
    }
    return frames;
  }

  play(now) {
    const t = Math.max(0, now - this.start) / 1000, f = t * 60, i = Math.min(Math.floor(f), this.frames.length - 1), j = Math.min(i + 1, this.frames.length - 1), k = f - Math.floor(f);
    this.bodies.forEach((_, d) => {
      const a = this.frames[i][d], b = this.frames[j][d], m = this.meshes[d];
      m.position.set(a.p[0] + (b.p[0] - a.p[0]) * k, a.p[1] + (b.p[1] - a.p[1]) * k, a.p[2] + (b.p[2] - a.p[2]) * k);
      m.quaternion.set(a.q[0], a.q[1], a.q[2], a.q[3]).slerp(new Quaternion(b.q[0], b.q[1], b.q[2], b.q[3]), k);
    });
    this.stage.renderer.render(this.stage.scene, this.stage.camera);
    if (f >= this.frames.length - 1) { this.finish(this.values.slice(0, this.count)); return; }
    this.raf = requestAnimationFrame(t2 => this.play(t2));
  }

  // The browser decides: the physics run in front of the player and the top face is read when everything has stopped.
  live() {
    this.mode = 'live';
    this.last = performance.now(); this.acc = 0; this.elapsed = 0;
    this.raf = requestAnimationFrame(t => this.tick(t));
  }

  tick(now) {
    const dt = Math.max(0, Math.min(0.1, (now - this.last) / 1000));
    this.last = now; this.acc += dt; this.elapsed += dt;
    while (this.acc >= STEP) { this.drag(Math.round(this.elapsed * 120)); this.world.step(STEP); this.acc -= STEP; }
    this.sync();
    this.stage.renderer.render(this.stage.scene, this.stage.camera);
    const calm = this.settled() || this.elapsed > 12;
    if (calm && this.elapsed > 0.8) {
      const ups = this.bodies.map(b => this.upFace(b.quaternion));
      const stacked = this.bodies.filter(b => b.position.y > 0.75);
      if (stacked.length && this.elapsed < 12) { // a die lying on another one is knocked off
        for (const b of stacked) { b.wakeUp(); b.velocity.set(rand(-3, 3), 6, rand(-1.2, 1.2)); b.angularVelocity.set(rand(-10, 10), rand(-10, 10), rand(-10, 10)); }
        this.elapsed = Math.min(this.elapsed, 1.5);
      } else if (ups.every(u => u.dot > 0.95) || this.elapsed > 12) { this.finish(ups.map(u => BASE_LABELS[u.index])); return; }
    }
    this.raf = requestAnimationFrame(t => this.tick(t));
  }

  // Shake the table (only when the browser decides): every die is woken, thrown up and spun again.
  shake() {
    if (!this.rolling || this.mode !== 'live' || this.shaken) return false;
    this.shaken = true; // only one shake per roll, or a player could shake until the dice show what they want
    for (const b of this.bodies) {
      b.wakeUp();
      b.velocity.x += rand(-7, 7); b.velocity.z += rand(-4, 4); b.velocity.y += rand(10, 15);
      b.angularVelocity.set(rand(-24, 24), rand(-24, 24), rand(-24, 24));
    }
    this.elapsed = Math.min(this.elapsed, 4); // a shaken table gets time to settle again
    this.canvas.classList.remove('dice-shake');
    void this.canvas.offsetWidth;
    this.canvas.classList.add('dice-shake');
    return true;
  }

  finishNow() { // reduced motion: the dice lie in a row, showing their numbers
    const LANE_D = laneDepth(this.groups.length), lane = i => this.laneOf[i], depth = this.groups.length * LANE_D;
    const inLane = this.laneOf.map((l, i) => this.laneOf.slice(0, i).filter(x => x === l).length);
    const values = this.bodies.map((_, i) => (this.values && this.values[i]) || 1 + Math.floor(Math.random() * 6));
    this.bodies.forEach((b, i) => {
      b.position.set(-TABLE_W / 2 + 1.6 + inLane[i] * 1.4, 0.5, -depth / 2 + (lane(i) + 0.5) * LANE_D);
      b.quaternion.setFromEuler(0, rand(0, 6.28), 0); // face +y is up: it shows BASE_LABELS[2]
      const labels = BASE_LABELS.slice(), g = labels.indexOf(values[i]);
      if (g !== 2) {
        if (g === opposite(2)) [labels[2], labels[g]] = [labels[g], labels[2]];
        else { [labels[2], labels[g]] = [labels[g], labels[2]]; [labels[opposite(2)], labels[opposite(g)]] = [labels[opposite(g)], labels[opposite(2)]]; }
      }
      this.meshes[i].material = labels.map(v => this.stage.material(v));
    });
    this.sync();
    this.stage.renderer.render(this.stage.scene, this.stage.camera);
    this.finish(values);
  }

  finish(values) {
    if (this.mode === 'live') { // the dice rest exactly flat
      this.bodies.forEach((b, i) => { const f = this.flatten([b.quaternion.x, b.quaternion.y, b.quaternion.z, b.quaternion.w]); this.meshes[i].quaternion.copy(f); this.meshes[i].position.y = 0.5; });
    }
    this.rolling = false;
    this.mode = 'done';
    const lanes = this.groups.length;
    if (lanes > 1) { // each lane's total
      const LANE_D = laneDepth(lanes), depth = lanes * LANE_D, ink = getComputedStyle(this.canvas).color;
      this.groups.forEach((n, l) => {
        const sum = values.filter((_, i) => this.laneOf[i] === l).reduce((a, b) => a + b, 0);
        const m = labelMesh('= ' + sum, ink, 92, 'center');
        m.position.set(TABLE_W / 2 - GUTTER / 2, 0.01, -depth / 2 + (l + 0.5) * LANE_D + 0.3);
        this.stage.scene.add(m); this.stage.extras.push(m);
      });
    }
    this.stage.renderer.render(this.stage.scene, this.stage.camera);
    this.onStop(values);
  }

  destroy() {
    cancelAnimationFrame(this.raf);
    this.rolling = false;
    if (this.stage) this.stage.clear();
  }
}

window.Dice3D = { Tray: Dice3DTray, heightFor, supported: Dice3DTray.supported };
