// The dice animation (static/js/dice.js, and the 3D dice of static/js/dice3d.js that load on the first roll): headless Chrome has
// WebGL through SwiftShader (the harness passes the flags), so the real 3D path runs. Server-rolled dice are random at run time,
// so the checks compare what the server sent (data-roll) with what the animation reports and what the page then shows; the
// style guide's dice demo takes values decided beforehand and is read through a spy on throwDice.
import { assert, campaignId, layout } from '../lib/util.mjs';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const PANEL = '#modal-body [x-data="rollPanel"]';
const DEMO = '[x-data="diceDemo"]';

export default async function (h) {
  // Éowyn's company of that campaign is locked by an aftermath still to play, so Sauron (whose aftermath is finished) rolls both times.
  const eowyn = await h.session('sauron@example.com', { login: false, height: 1000 });
  const sauron = eowyn;
  // A browser without WebGL: asking a canvas for a webgl context answers null (2d keeps working). Installed before any page script.
  await sauron.page.evaluateOnNewDocument(() => {
    const get = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (type, ...rest) { return sessionStorage.getItem('noWebGL') && /webgl/i.test(String(type)) ? null : get.call(this, type, ...rest); };
  });
  await h.login(sauron, 'sauron@example.com');
  const noGL = (on) => sauron.page.evaluate((on) => { on ? sessionStorage.setItem('noWebGL', '1') : sessionStorage.removeItem('noWebGL'); }, on);

  const state = (s, sel) => s.page.evaluate((sel) => {
    const el = document.querySelector(sel), d = el && window.Alpine ? Alpine.$data(el) : null;
    return d ? { rolling: d.rolling, stopped: d.stopped, dice: [...d.dice], tray: d.tray ? (window.Dice3D && d.tray instanceof Dice3D.Tray ? '3d' : d.tray instanceof DiceTray ? '2d' : 'other') : null } : null;
  }, sel);
  const waitStopped = (s, sel, ms = 25000) => s.page.waitForFunction((sel) => { const el = document.querySelector(sel); return el && Alpine.$data(el).stopped === true; }, { timeout: ms }, sel)
    .catch(() => { throw new Error(`the dice did not stop within ${ms} ms`); });
  const script3d = (s) => s.page.evaluate(() => ({ tag: !!document.querySelector('script[src*="dice3d.js"]'), lib: typeof window.Dice3D }));
  // Samples, every animation frame, what a player could see of the roll window: the dice canvas and the parts that wait for the dice
  // (x-show="revealed": the result, the raise buttons, Confirm; the same gate hides the decision form of an aftermath roll).
  const sampler = (s) => s.page.evaluate(() => {
    const vis = (e) => !!e && e.getClientRects().length > 0 && getComputedStyle(e).visibility !== 'hidden';
    window.__samples = [];
    const t0 = performance.now();
    const tick = () => {
      const panel = document.querySelector('#modal-body [x-data="rollPanel"]');
      const gated = panel ? [...panel.querySelectorAll('[x-show="revealed"]')] : [];
      window.__samples.push({ t: Math.round(performance.now() - t0), canvas: vis(document.querySelector('#modal-body canvas.dice-tray')), gated: gated.length, shown: gated.filter(vis).length });
      if (performance.now() - t0 < 30000) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
  const samples = (s) => s.page.evaluate(() => { const r = window.__samples; window.__samples = []; return r; });

  // The server's own roll in a campaign where the server rolls every die (the Roll button alone), recruit window.
  const recruitRoll = async (s, campaignName) => {
    const cid = await campaignId(s, campaignName);
    await s.goto(`/campaigns/${cid}/my-company`);
    await s.click('main button', 'Recruit');
    await s.page.waitForSelector(PANEL);
    await sampler(s);
    const t0 = Date.now();
    await s.click('#modal-body button', 'Roll ·');
    return { cid, t0 };
  };

  // Ends the recruit in progress whatever the dice gave: confirm, choose a lower result, or roll on the special chart.
  const finishRecruit = async (s) => {
    const trace = [];
    for (let i = 0; i < 5; i++) {
      const kind = await s.page.evaluate(() => {
        const q = (x) => document.querySelector('#modal-body ' + x);
        if (q('form[action$="/recruit/confirm"]')) return 'confirm';
        if (q('form[action$="/recruit/lower"]')) return 'lower';
        if (q('form[action$="/recruit/choose"]')) return 'choose';
        if (q('form[action$="/recruit/officers"]')) return 'officers';
        const b = [...document.querySelectorAll('#modal-body form[action$="/recruit/roll"] button')].find((x) => /special chart/.test(x.innerText));
        return b ? 'special' : 'done';
      });
      trace.push(kind + ' | ' + await s.page.evaluate(() => (document.querySelector('#modal-body') || document.body).innerText.replace(/\s+/g, ' ').slice(0, 160)));
      if (kind === 'done' || kind === 'officers') return kind;
      if (kind === 'special') { await sampler(s); await s.click('#modal-body button', 'Roll on the special chart'); await waitStopped(s, PANEL); continue; }
      await s.page.$$eval(`#modal-body form[action$="/recruit/${kind}"] input[type=radio]:not(:disabled)`, (r) => { const seen = new Set(); r.forEach((x) => { if (!seen.has(x.name) && !r.some((y) => y.name === x.name && y.checked)) x.click(); seen.add(x.name); }); });
      await s.click(`#modal-body form[action$="/recruit/${kind}"] button.btn-primary`, '');
    }
    return 'unfinished:\n' + trace.join('\n') + '\nrequests:\n' + s.reqs.slice(-24).join('\n') + '\nalerts: ' + (await s.page.evaluate(() => [...document.querySelectorAll('#modal-body .alert')].map((a) => a.innerText).join(' / ')));
  };

  // After a server roll in the recruit window: the animation reports the number the server sent, the page shows it, and what waits
  // for the dice (result, raise buttons, Confirm) shows only after they stop and stays.
  const verifyRoll = async (s, tray) => {
    await waitStopped(s, PANEL);
    const st = await state(s, PANEL);
    assert.equal(st.tray, tray, `the ${tray} tray is used`);
    assert.equal(st.rolling, false);
    const sent = Number(await s.page.$eval(PANEL, (e) => e.dataset.roll));
    assert.ok(sent >= 1 && sent <= 6, `the server sent a die (${sent})`);
    assert.deepEqual(st.dice, [sent], 'the animation reports the number the server sent');
    await s.settle(200);
    const text = await s.page.$eval(PANEL, (e) => e.innerText);
    assert.match(text, new RegExp(`You rolled a\\s*${sent}`), `the page tells the roll the server made (${sent})`);
    const f = await samples(s);
    const firstCanvas = f.findIndex((x) => x.canvas), firstShown = f.findIndex((x) => x.shown > 0);
    assert.ok(firstCanvas >= 0, 'the dice canvas was visible');
    assert.ok(f.some((x) => x.gated > 0), 'the roll window has parts that wait for the dice');
    assert.ok(firstShown > firstCanvas, 'the result showed after the dice appeared');
    assert.ok(f.slice(0, firstShown).every((x) => x.shown === 0), 'nothing that depends on the roll showed before the dice stopped');
    assert.ok(f.slice(firstShown).every((x) => x.shown === x.gated), 'once shown it stays');
    assert.ok(f.filter((x) => x.canvas && x.shown === 0).length >= 3, 'the dice were on screen for several frames before the result');
    return sent;
  };

  await h.step('dice: a server roll loads the 3D dice on demand, shows them rolling, and reveals the result the server sent only when they stop', async () => {
    const campaign = 'Settings: the server rolls every die';
    const cid = await campaignId(eowyn, campaign);
    await eowyn.goto(`/campaigns/${cid}/my-company`);
    assert.deepEqual(await script3d(eowyn), { tag: false, lib: 'undefined' }, 'a page without a roll does not load the 3D dice');
    assert.equal(await eowyn.page.evaluate(() => { const c = document.createElement('canvas'); return !!(c.getContext('webgl2') || c.getContext('webgl')); }), true, 'this Chrome has WebGL (SwiftShader); the 3D assertions need it');
    const { t0 } = await recruitRoll(eowyn, campaign);
    await waitStopped(eowyn, PANEL);
    const took = Date.now() - t0;
    assert.ok(took < 20000, `the animation ended (${took} ms)`);
    assert.deepEqual(await script3d(eowyn), { tag: true, lib: 'object' }, 'the first roll loaded the 3D dice script');
    assert.equal(await eowyn.page.evaluate(() => Dice3D.supported()), true);
    await verifyRoll(eowyn, '3d');
  });

  await h.step('dice: after the animation the roll window is usable (the canvas keeps the dice, the space bar does nothing, the recruit can be finished)', async () => {
    const png = await (await eowyn.page.$('#modal-body canvas.dice-tray')).screenshot();
    assert.ok(png.length > 4000, `the canvas shows the dice, not an empty box (${png.length} bytes)`);
    await eowyn.page.keyboard.press('Space'); // a space after the dice stopped does nothing (a server roll cannot be shaken)
    assert.equal((await state(eowyn, PANEL)).stopped, true);
    const raise = await eowyn.page.$$eval('#modal-body [aria-label="Raise the roll"] button', (b) => b.map((x) => ({ t: x.innerText.trim(), off: x.disabled })));
    const usable = raise.find((b) => b.t.startsWith('+') && !b.off);
    if (usable) {
      await eowyn.click('#modal-body [aria-label="Raise the roll"] button', usable.t);
      assert.equal(await eowyn.page.$eval('#modal-body [aria-label="Raise the roll"] button[aria-pressed=true]', (b) => b.innerText.trim()), usable.t, 'the raise button took effect');
      await eowyn.click('#modal-body [aria-label="Raise the roll"] button', 'As rolled');
    }
    const end = await finishRecruit(eowyn);
    assert.ok(['done', 'officers'].includes(end), `the recruit could be finished (${end})`);
  });

  await h.step('dice: without WebGL the flat dice roll instead, the result still shows, nothing is stuck and the page logs no error', async () => {
    const campaign = 'Settings: the server rolls every die';
    await noGL(true);
    await sauron.page.reload({ waitUntil: 'load' });
    await recruitRoll(sauron, campaign);
    await waitStopped(sauron, PANEL);
    assert.equal(await sauron.page.evaluate(() => typeof window.Dice3D), 'object', 'the 3D script is there');
    assert.equal(await sauron.page.evaluate(() => Dice3D.supported()), false, 'it reports that WebGL is missing');
    await verifyRoll(sauron, '2d');
    await finishRecruit(sauron);
    await noGL(false);
  });

  const demo = await h.session('you@example.com', { height: 1100 });
  const spy = (s) => s.page.evaluate((sel) => {
    const d = Alpine.$data(document.querySelector(sel)), orig = d.throwDice;
    window.__throws = [];
    d.throwDice = function (o) { window.__throws.push({ count: o.count, values: o.values && [...o.values], groups: o.groups }); return orig.call(this, o); };
  }, DEMO);
  const resultText = (s) => s.page.$eval(`${DEMO} [role=status]`, (e) => e.innerText.replace(/\s+/g, ' ').trim());
  const roll = async (s) => { await s.click(`${DEMO} button`, '=Roll'); await waitStopped(s, DEMO); };

  await h.step('dice: the style guide tray shows values decided beforehand exactly (1, 2 and 4 dice, two lanes) and can be rolled again', async () => {
    await demo.goto('/styleguide');
    await demo.page.waitForSelector(DEMO);
    await spy(demo);
    await demo.page.$eval('#sg-dice', (e) => e.scrollIntoView({ block: 'center' }));
    await demo.page.$eval(`${DEMO} input[type=checkbox][x-model="serverValues"]`, (e) => e.click());
    for (const [n, lanes] of [[1, false], [2, false], [4, true], [4, false]]) {
      await demo.select('#sg-dice', n);
      const box = `${DEMO} input[type=checkbox][x-model="lanes"]`;
      if (await demo.page.$eval(box, (e) => e.checked) !== lanes) await demo.page.$eval(box, (e) => e.click());
      await roll(demo);
      const t = await demo.page.evaluate(() => window.__throws.pop());
      const st = await state(demo, DEMO);
      assert.equal(st.tray, '3d');
      assert.equal(t.values.length, n);
      assert.deepEqual(st.dice, t.values, `the tray stopped on the ${n} given dice${lanes ? ' in two lanes' : ''}`);
      assert.equal(await resultText(demo), `Result: ${t.values.join(' + ')}`, 'the page shows the same numbers');
      assert.equal(await demo.page.$eval(`${DEMO} button.btn-primary`, (b) => b.disabled), false, 'the Roll button is usable again');
    }
  });

  await h.step('dice: a roll the browser decides can be shaken once with the space bar, and then stops with a legal result', async () => {
    await demo.page.$eval(`${DEMO} input[type=checkbox][x-model="serverValues"]`, (e) => { if (e.checked) e.click(); });
    await demo.page.$eval(`${DEMO} input[type=checkbox][x-model="lanes"]`, (e) => { if (e.checked) e.click(); });
    await demo.select('#sg-dice', 2);
    await demo.click(`${DEMO} button`, '=Roll'); // not waited for: shake while it rolls
    await demo.page.waitForFunction((sel) => Alpine.$data(document.querySelector(sel)).rolling === true, {}, DEMO);
    await demo.page.keyboard.press('Space');
    assert.equal(await demo.page.evaluate((sel) => Alpine.$data(document.querySelector(sel)).shaken, DEMO), true, 'the space bar shook the table');
    await demo.page.keyboard.press('Space'); // a second one is ignored
    await waitStopped(demo, DEMO);
    const st = await state(demo, DEMO);
    assert.equal(st.dice.length, 2);
    assert.ok(st.dice.every((d) => d >= 1 && d <= 6), `legal dice (${st.dice})`);
    assert.equal(await resultText(demo), `Result: ${st.dice.join(' + ')}`);
  });

  await h.step('dice: with reduced motion the result comes at once, without animation', async () => {
    await demo.page.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: 'reduce' }]);
    await demo.page.$eval(`${DEMO} input[type=checkbox][x-model="serverValues"]`, (e) => { if (!e.checked) e.click(); });
    await demo.select('#sg-dice', 3);
    await roll(demo); // a first roll absorbs the cold start (the 3D script and its WebGL context load lazily, slowly on a CI runner); the timed one is the warm path
    await demo.page.evaluate(() => window.__throws.pop());
    const t0 = Date.now();
    await roll(demo);
    const took = Date.now() - t0;
    const t = await demo.page.evaluate(() => window.__throws.pop());
    const st = await state(demo, DEMO);
    assert.deepEqual(st.dice, t.values, 'the given dice are shown');
    assert.equal(await resultText(demo), `Result: ${t.values.join(' + ')}`);
    assert.ok(took < 3500, `no rolling to wait for (${took} ms)`);
    await demo.page.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: 'no-preference' }]);
  });

  await h.step('dice: a 3D script that cannot be loaded falls back to the flat dice (nothing blocks the page)', async () => {
    const lost = await h.session('pippin@example.com', { login: true, height: 1100 });
    lost.allow.push(/dice3d/, /Failed to load resource/);
    await lost.page.setRequestInterception(true);
    lost.page.on('request', (r) => { if (r.url().includes('/dice3d.js')) r.abort('failed'); else r.continue(); });
    await lost.goto('/styleguide');
    await lost.page.waitForSelector(DEMO);
    await spy(lost);
    await lost.page.$eval(`${DEMO} input[type=checkbox][x-model="serverValues"]`, (e) => e.click());
    await roll(lost);
    const t = await lost.page.evaluate(() => window.__throws.pop());
    const st = await state(lost, DEMO);
    assert.equal(st.tray, '2d');
    assert.deepEqual(st.dice, t.values);
    assert.equal(await resultText(lost), `Result: ${t.values.join(' + ')}`);
    await roll(lost); // and again: the failed load is remembered, not retried into a hang
    await lost.page.setRequestInterception(false);
  });

  await h.step('dice: the tray fits 360, 768 and 1280 px in light and dark', async () => {
    for (const scheme of ['light', 'dark']) for (const w of [360, 768, 1280]) {
      await layout(demo, w, scheme, 1100);
      await demo.page.$eval(DEMO, (e) => e.scrollIntoView());
      assert.ok(await demo.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      const r = await demo.page.$eval(`${DEMO} canvas`, (c) => { const b = c.getBoundingClientRect(); return [b.left, b.right, b.height]; });
      assert.ok(r[0] >= 0 && r[1] <= w + 1 && r[2] > 40, `the tray is inside the viewport at ${w}px`);
    }
    await layout(demo, 1280, 'light', 1100);
  });
}
