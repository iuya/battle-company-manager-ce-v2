// Rolling dice, for any number of D6.
//
// DiceTray throws dice on a canvas: they slide and tumble across the tray, bump into the walls and each other, show faces
// flickering past while they move, slow down and stop on a number. The numbers can be decided beforehand (the server rolled:
// the animation only shows it) or left to the tray (the browser decides: client authoritative, which a campaign must
// switch on). While the dice have not all stopped, shake() sends them moving again ("shake the table").
//
//   const tray = new DiceTray(canvas, { count: 2, values: [3, 5] /* or null */, onStop: values => { ... } });
//   tray.roll();
//   tray.shake();   // only while rolling
//
// Users who prefer reduced motion get the result at once, without the animation.
(function () {
  const TAU = Math.PI * 2;
  const rand = (a, b) => a + Math.random() * (b - a);
  const PIPS = { // pip positions on a 3x3 grid, as [column, row] in -1..1
    1: [[0, 0]], 2: [[-1, -1], [1, 1]], 3: [[-1, -1], [0, 0], [1, 1]], 4: [[-1, -1], [1, -1], [-1, 1], [1, 1]],
    5: [[-1, -1], [1, -1], [0, 0], [-1, 1], [1, 1]], 6: [[-1, -1], [1, -1], [-1, 0], [1, 0], [-1, 1], [1, 1]],
  };
  const randomFace = () => 1 + Math.floor(Math.random() * 6);
  const reducedMotion = () => window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches;

  class DiceTray {
    constructor(canvas, opts = {}) {
      this.canvas = canvas;
      this.groups = opts.groups && opts.groups.length ? opts.groups : [opts.count || 1]; // dice per lane: [2, 2] gives two lanes of two
      this.labels = opts.labels || [];
      this.count = this.groups.reduce((a, b) => a + b, 0);
      this.laneOf = this.groups.flatMap((n, lane) => Array(n).fill(lane));
      this.values = opts.values || null; // predetermined results, one per die; null: decided here
      this.onStop = opts.onStop || (() => {});
      this.size = opts.size || 52;
      this.rolling = false;
      this.dice = [];
      this.raf = 0;
      this.last = 0;
      this.fit();
    }

    // Matches the canvas to its size on screen, sharp on dense displays.
    fit() {
      const dpr = window.devicePixelRatio || 1;
      this.w = this.canvas.clientWidth || 300;
      this.h = this.canvas.clientHeight || 140;
      this.canvas.width = Math.round(this.w * dpr);
      this.canvas.height = Math.round(this.h * dpr);
      this.ctx = this.canvas.getContext('2d');
      this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    roll() {
      cancelAnimationFrame(this.raf);
      this.fit();
      const half = this.size / 2, n = this.count, laneH = this.h / this.groups.length;
      const inLane = this.laneOf.map((lane, i) => this.laneOf.slice(0, i).filter(l => l === lane).length);
      this.dice = Array.from({ length: n }, (_, i) => ({
        lane: this.laneOf[i], x: half + 6 + inLane[i] * 14, y: laneH * (this.laneOf[i] + 0.5) + rand(-8, 8) * (this.groups[this.laneOf[i]] > 1 ? 1 : 0.5),
        vx: rand(650, 1000), vy: rand(-320, 320), angle: rand(0, TAU), spin: rand(9, 15) * (Math.random() < 0.5 ? -1 : 1),
        face: randomFace(), flicker: 0, rest: false, settling: false, final: this.values ? this.values[i] : randomFace(),
      }));
      this.rolling = true;
      if (reducedMotion()) { this.finishNow(); return; }
      this.last = performance.now();
      this.raf = requestAnimationFrame(t => this.frame(t));
    }

    // The table is shaken: every die gets a kick, also those that had already stopped. Only while the roll is on.
    shake() {
      if (!this.rolling || this.shaken) return false; // once per roll: shaking until the dice show what you want would be cheating
      this.shaken = true;
      for (const d of this.dice) {
        d.vx += rand(-1100, 1100); d.vy += rand(-900, 900); d.spin += rand(-12, 12);
        d.rest = d.settling = false;
      }
      this.canvas.classList.remove('dice-shake');
      void this.canvas.offsetWidth; // restart the CSS animation
      this.canvas.classList.add('dice-shake');
      return true;
    }

    frame(now) {
      const dt = Math.min(0.05, (now - this.last) / 1000);
      this.last = now;
      this.step(dt);
      this.draw();
      if (this.dice.every(d => d.rest)) {
        this.rolling = false;
        this.onStop(this.dice.map(d => d.final));
        return;
      }
      this.raf = requestAnimationFrame(t => this.frame(t));
    }

    step(dt) {
      const half = this.size / 2, laneH = this.h / this.groups.length, friction = Math.pow(0.12, dt), spinFriction = Math.pow(0.15, dt);
      for (const d of this.dice) {
        if (d.rest) continue;
        if (d.settling) { // stopped sliding: turn to the nearest quarter turn and show the result
          const target = Math.round(d.angle / (TAU / 4)) * (TAU / 4);
          d.angle += (target - d.angle) * Math.min(1, dt * 16);
          d.face = d.final;
          if (Math.abs(target - d.angle) < 0.01) { d.angle = target; d.rest = true; }
          continue;
        }
        d.x += d.vx * dt; d.y += d.vy * dt; d.angle += d.spin * dt;
        d.vx *= friction; d.vy *= friction; d.spin *= spinFriction;
        let bounced = false;
        if (d.x < half) { d.x = half; d.vx = Math.abs(d.vx) * 0.7; bounced = true; }
        if (d.x > this.w - half) { d.x = this.w - half; d.vx = -Math.abs(d.vx) * 0.7; bounced = true; }
        const top = d.lane * laneH + half, bottom = (d.lane + 1) * laneH - half; // a die stays in its own lane
        if (d.y < top) { d.y = top; d.vy = Math.abs(d.vy) * 0.7; bounced = true; }
        if (d.y > bottom) { d.y = bottom; d.vy = -Math.abs(d.vy) * 0.7; bounced = true; }
        const speed = Math.hypot(d.vx, d.vy);
        if (bounced) { d.spin += rand(-4, 4); d.face = randomFace(); d.flicker = 0; }
        d.flicker += dt;
        if (d.flicker > 0.05 + Math.max(0, 1 - speed / 600) * 0.18) { d.flicker = 0; d.face = randomFace(); } // slower as it slows
        if (speed < 28 && Math.abs(d.spin) < 0.8) d.settling = true;
      }
      for (let i = 0; i < this.dice.length; i++) { // dice do not overlap: push them apart and swap their pace
        for (let j = i + 1; j < this.dice.length; j++) {
          const a = this.dice[i], b = this.dice[j];
          if (a.lane !== b.lane) continue;
          const dx = b.x - a.x, dy = b.y - a.y, dist = Math.hypot(dx, dy) || 0.01, min = this.size * 0.95;
          if (dist >= min) continue;
          const nx = dx / dist, ny = dy / dist, push = (min - dist) / 2;
          a.x -= nx * push; a.y -= ny * push; b.x += nx * push; b.y += ny * push;
          const rel = (b.vx - a.vx) * nx + (b.vy - a.vy) * ny;
          if (rel < 0) { a.vx += rel * nx; a.vy += rel * ny; b.vx -= rel * nx; b.vy -= rel * ny; }
          for (const d of [a, b]) { d.rest = d.settling = false; d.face = randomFace(); }
        }
      }
    }

    draw() {
      const c = this.ctx, s = this.size, r = s * 0.18;
      c.clearRect(0, 0, this.w, this.h);
      this.drawLanes();
      for (const d of this.dice) {
        c.save();
        c.fillStyle = 'rgba(0,0,0,.22)'; // its shadow
        c.beginPath(); c.ellipse(d.x + 3, d.y + 5, s * 0.52, s * 0.5, 0, 0, TAU); c.fill();
        c.translate(d.x, d.y); c.rotate(d.angle);
        const g = c.createLinearGradient(-s / 2, -s / 2, s / 2, s / 2);
        g.addColorStop(0, '#fffdf4'); g.addColorStop(1, '#ddd6c0');
        c.fillStyle = g; c.strokeStyle = '#8c8470'; c.lineWidth = 1.5;
        c.beginPath(); c.roundRect(-s / 2, -s / 2, s, s, r); c.fill(); c.stroke();
        c.fillStyle = '#1d1a14';
        const step = s * 0.26, pip = s * 0.075;
        for (const [px, py] of PIPS[d.face]) { c.beginPath(); c.arc(px * step, py * step, pip, 0, TAU); c.fill(); }
        c.restore();
      }
    }

    // Several lanes (a hero's two rolls): a line between them, each named, and its total once the dice have stopped.
    drawLanes() {
      if (this.groups.length < 2) return;
      const c = this.ctx, laneH = this.h / this.groups.length, style = getComputedStyle(this.canvas);
      const settled = this.dice.length > 0 && this.dice.every(d => d.rest);
      c.save();
      c.strokeStyle = style.borderTopColor; c.fillStyle = style.color; c.font = '600 12px system-ui, sans-serif'; c.textBaseline = 'top';
      for (let lane = 0; lane < this.groups.length; lane++) {
        if (lane > 0) { c.beginPath(); c.moveTo(0, lane * laneH); c.lineTo(this.w, lane * laneH); c.stroke(); }
        c.globalAlpha = 0.6;
        if (this.labels[lane]) c.fillText(this.labels[lane], 10, lane * laneH + 7);
        if (settled) {
          const sum = this.dice.filter(d => d.lane === lane).reduce((a, d) => a + d.final, 0);
          c.globalAlpha = 1; c.font = '700 22px system-ui, sans-serif'; c.textAlign = 'right';
          c.fillText('= ' + sum, this.w - 12, lane * laneH + laneH / 2 - 11);
          c.textAlign = 'left'; c.font = '600 12px system-ui, sans-serif';
        }
      }
      c.restore();
    }

    finishNow() { // reduced motion: no animation, just the result
      this.dice.forEach(d => { d.face = d.final; d.angle = 0; d.rest = true; });
      const laneH = this.h / this.groups.length;
      this.dice.forEach((d, i) => { const k = this.laneOf.slice(0, i).filter(l => l === d.lane).length; d.x = this.size / 2 + 8 + k * (this.size + 10); d.y = laneH * (d.lane + 0.5); });
      this.draw();
      this.rolling = false;
      this.onStop(this.dice.map(d => d.final));
    }

    destroy() { cancelAnimationFrame(this.raf); this.rolling = false; }
  }
  window.DiceTray = DiceTray;

  // What a page needs to throw dice with Alpine: spread it into a component with its own x-ref="canvas".
  //   state: dice (the numbers once stopped), rolling, stopped; methods: throwDice(opts), shake(), onSpace(event)
  // opts: count, values (predetermined or null), canShake, then(values) when they stop.
  // The 3D dice (dice3d.js: three.js and cannon-es) load the first time a roll needs them, so other pages never pay for them.
  // Where WebGL or the file is missing, the 2D tray above is used.
  let loading3D = null;
  const load3D = () => {
    if (window.Dice3D) return Promise.resolve(window.Dice3D.supported() ? window.Dice3D : null);
    if (!loading3D) {
      loading3D = new Promise(resolve => {
        const s = document.createElement('script');
        s.src = '/static/js/dice3d.js';
        s.onload = () => resolve(window.Dice3D && window.Dice3D.supported() ? window.Dice3D : null);
        s.onerror = () => resolve(null);
        document.head.append(s);
      });
    }
    return loading3D;
  };

  window.diceMixin = () => ({
    dice: [], rolling: false, stopped: false, canShake: false, shaken: false, tray: null,
    async throwDice({ count = 1, values = null, canShake = false, groups = null, labels = [], then = () => {} } = {}) {
      if (this.tray) this.tray.destroy();
      this.tray = null;
      this.canShake = canShake && !values; // a roll the server decided cannot be shaken: the number is already settled
      const ticket = this.ticket = (this.ticket || 0) + 1;
      this.dice = []; this.stopped = false; this.rolling = true; this.shaken = false;
      if (document.activeElement && document.activeElement.blur) document.activeElement.blur(); // a space must not press a button
      const onStop = v => { this.dice = v; this.rolling = false; this.stopped = true; then(v); };
      const canvas = this.$refs.canvas, lanes = groups && groups.length > 1 ? groups.length : 1;
      const lib = await load3D();
      if (ticket !== this.ticket) return; // another throw (or the panel's end) came first
      let tray = null;
      if (lib) {
        try {
          canvas.style.height = lib.heightFor(canvas.clientWidth || 600, lanes) + 'px';
          tray = new lib.Tray(canvas, { count, values, groups, labels, onStop });
        } catch (e) { tray = null; canvas.style.height = ''; }
      }
      if (!tray) { // no WebGL: the flat tray
        canvas.style.height = lanes > 1 ? lanes * 84 + 'px' : '';
        tray = new DiceTray(canvas, { count, values, groups, labels, onStop });
      }
      this.tray = tray;
      tray.roll();
    },
    shake() { const ok = this.canShake && this.tray ? this.tray.shake() : false; if (ok) this.shaken = true; return ok; },
    // The space bar shakes the table while the dice are rolling (and only if the campaign allows it).
    onSpace(e) {
      if (!this.rolling) return;
      const t = e.target;
      if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
      e.preventDefault(); // the space bar must not scroll the page while the dice roll
      if (this.canShake) this.shake();
    },
    destroy() { this.ticket = (this.ticket || 0) + 1; if (this.tray) this.tray.destroy(); },
  });

  // A tray on its own (the style guide): throw 1 to 6 dice.
  document.addEventListener('alpine:init', () => {
    Alpine.data('diceDemo', () => ({
      ...window.diceMixin(), count: 2, canShakeDemo: true, serverValues: false, lanes: false,
      go() { this.throwDice({ count: this.count, canShake: this.canShakeDemo, groups: this.lanes && this.count > 1 ? [Math.ceil(this.count / 2), Math.floor(this.count / 2)] : null, labels: ['First roll', 'Second roll'], values: this.serverValues ? Array.from({ length: this.count }, () => 1 + Math.floor(Math.random() * 6)) : null }); },
    }));
  });
})();
