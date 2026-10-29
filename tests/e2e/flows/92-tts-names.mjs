// The names block of the Tabletop Simulator dialog (level B, docs/tabletop-simulator.md): the ArmyNames JSON next to the list, the rule-text
// switch, Copy names, the helper download, the Lua fallback, 360/768/1280 px in both themes with axe and screenshots.
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const axeOn = async (s, ctx) => {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((ctx) => axe.run(ctx, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id}: ${v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' ' + (x.any[0]?.message || '')).join(' | ')}`)), ctx);
};

const open = async (s) => { await s.click('#tts-open'); await s.page.waitForSelector('#tts-names-text', { visible: true }); await s.settle(200); };
const close = async (s) => { await s.page.keyboard.press('Escape'); await s.settle(300); };
const val = (s, id) => s.page.$eval(`#${id}`, (e) => e.value);
const names = async (s) => JSON.parse(await val(s, 'tts-names-text'));
const pickRules = async (s, v) => { await s.page.evaluate((v) => document.querySelector(`#tts-body input[name=rules][value=${v}]`).click(), v); await s.settle(500); await s.page.waitForSelector('#tts-names-text'); };

// the figures a list spawns, in order: the line without its brackets and count, once per figure
const listFigures = (t) => t.split('\n').filter((l) => l.trim()).flatMap((l) => {
  const m = l.trim().match(/^\((?:(\d+)x )?(.*)\)$/);
  return Array(m[1] ? Number(m[1]) : 1).fill(m[2]);
});

export default async function (h) {
  const eowyn = await h.session('eowyn@example.com');
  let limits;

  await h.step('tts-names: the block shows valid ArmyNames JSON, one entry per figure, in the order of the list', async () => {
    limits = await campaignId(eowyn, 'Limits, creatures');
    await eowyn.goto(`/campaigns/${limits}/my-company`);
    await eowyn.page.evaluate(() => localStorage.removeItem('bcm-tts'));
    await open(eowyn);
    const p = await names(eowyn);
    assert.ok(p.v === 1 || p.v === 2, 'version 1, or 2 when the company has aura rings (the Auras switch is on by default; flow 96)');
    assert.ok(p.company);
    const want = listFigures(await val(eowyn, 'tts-text'));
    assert.equal(p.figures.length, want.length, 'one entry per figure of the list');
    assert.deepEqual(p.figures.map((f) => f.l), want, 'the nicknames are the list lines, in order');
    for (const f of p.figures) {
      assert.ok(f.k && f.n.startsWith('[b]') && f.d.length > 0 && f.d.length <= 900, `a name and a description: ${JSON.stringify(f).slice(0, 120)}`);
    }
    assert.equal(await eowyn.page.$eval('#tts-names-text', (e) => e.readOnly), true, 'read only');
    assert.equal(await eowyn.page.$eval('#tts-names-status', (e) => e.getAttribute('aria-live')), 'polite');
    const size = await eowyn.page.$eval('#tts-names-size', (e) => e.innerText);
    assert.match(size, new RegExp(`${want.length} figures`));
    assert.match(size, /KB of 24 KB/);
    assert.ok(await eowyn.page.$eval('#tts-names-text', (e) => !!document.querySelector(`label[for=${e.id}]`)), 'the area has a label');
    assert.equal(await eowyn.page.$eval('#tts-body input[name=rules]:checked', (e) => e.value), 'none', 'none (names only) is the default');
    const body = await eowyn.page.$eval('#tts-body', (e) => e.innerText.replace(/\s+/g, ' '));
    assert.match(body, /Name and describe the figures/);
    for (const t of [/Saved Objects/, /Library\/Tabletop Simulator\/Saves\/Saved Objects/, /Documents\\My Games/, /Linux \(not verified\)/, /ArmyNames/, /Apply names/, /Re-apply/, /drag/i]) assert.match(body, t);
  });

  await h.step('tts-names: the rule-text switch changes the size, without a page load, and is remembered', async () => {
    const mark = await eowyn.page.evaluate(() => { window.__mark = 7; return 7; });
    const none = (await val(eowyn, 'tts-names-text')).length;
    const reqs = eowyn.reqCount;
    await pickRules(eowyn, 'short');
    const short = (await val(eowyn, 'tts-names-text')).length;
    assert.ok(none < short, `no rule texts is smaller (${none} < ${short})`);
    assert.ok(eowyn.reqCount > reqs && await eowyn.page.evaluate((m) => window.__mark === m, mark), 'an htmx request, no page load');
    assert.equal(await eowyn.page.$eval('#tts-body input[name=rules]:checked', (e) => e.value), 'short');
    await pickRules(eowyn, 'full');
    const full = (await val(eowyn, 'tts-names-text')).length;
    assert.ok(full >= short && full > none, `full is the largest (${full})`);
    assert.match(await eowyn.page.$eval('#tts-names-size', (e) => e.innerText), /rule texts: full|rule texts: short|rule texts: none/);
    assert.equal((await eowyn.page.evaluate(() => JSON.parse(localStorage.getItem('bcm-tts')))).rules, 'full');
    // the list switches keep the rule choice and refresh both blocks
    await eowyn.page.evaluate(() => document.querySelector('#tts-body input[name=foot]').closest('label').click());
    await eowyn.settle(500);
    assert.equal(await eowyn.page.$eval('#tts-body input[name=rules]:checked', (e) => e.value), 'full', 'the rule choice survives a list switch');
    await eowyn.page.evaluate(() => document.querySelector('#tts-body input[name=foot]').closest('label').click());
    await eowyn.settle(500);
    await close(eowyn);
    await eowyn.goto(`/campaigns/${limits}/my-company`);
    await open(eowyn);
    assert.equal(await eowyn.page.$eval('#tts-body input[name=rules]:checked', (e) => e.value), 'full', 'remembered on a new page');
    await pickRules(eowyn, 'none');
  });

  await h.step('tts-names: Copy names puts the JSON into the clipboard; the fallback selects it', async () => {
    const origin = new URL(eowyn.page.url()).origin;
    await eowyn.ctx.overridePermissions(origin, ['clipboard-read', 'clipboard-write']);
    const want = await val(eowyn, 'tts-names-text');
    await eowyn.click('#tts-body button', '=Copy names');
    assert.equal(await eowyn.page.evaluate(() => navigator.clipboard.readText()), want);
    assert.match(await eowyn.page.$eval('#tts-names-status', (e) => e.innerText), /Copied names/);
    assert.equal(await eowyn.page.$eval('#tts-status', (e) => e.innerText), '', 'the other confirmation is untouched');
    await eowyn.page.evaluate(() => { Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true }); document.execCommand = () => false; });
    await eowyn.click('#tts-body button', '=Copy names');
    assert.match(await eowyn.page.$eval('#tts-names-status', (e) => e.innerText), /press Ctrl\+C/i);
    assert.equal(await eowyn.page.evaluate(() => { const a = document.getElementById('tts-names-text'); return a.value.slice(a.selectionStart, a.selectionEnd).length; }), want.length);
  });

  await h.step('tts-names: the helper link downloads a zip with the Saved Object and its icon', async () => {
    const href = await eowyn.page.$eval('#tts-helper-link', (e) => e.getAttribute('href'));
    assert.equal(href, '/tts/helper.zip');
    // the page is hx-boosted: without hx-boost="false" the click is turned into an htmx request and the zip's bytes are swapped into #main
    // instead of being downloaded: clicking must leave the dialog and the page as they are
    assert.equal(await eowyn.page.$eval('#tts-helper-link', (e) => e.getAttribute('hx-boost')), 'false');
    await eowyn.page.click('#tts-helper-link');
    await new Promise((r) => setTimeout(r, 800));
    const after = await eowyn.page.evaluate(() => ({ title: !!document.querySelector('#tts-names-title'), pk: document.body.innerText.startsWith('PK') }));
    assert.equal(after.title, true, 'the dialog is still there after the click');
    assert.equal(after.pk, false, 'the zip bytes were not swapped into the page');
    const r = await eowyn.page.evaluate(async (u) => {
      const res = await fetch(u);
      const b = new Uint8Array(await res.arrayBuffer());
      const files = [];
      const dv = new DataView(b.buffer);
      for (let i = 0; i + 46 < b.length; i++) {
        if (dv.getUint32(i, true) === 0x02014b50) { // central directory entry
          const n = dv.getUint16(i + 28, true);
          files.push({ name: new TextDecoder().decode(b.slice(i + 46, i + 46 + n)), size: dv.getUint32(i + 24, true) });
        }
      }
      return { status: res.status, type: res.headers.get('content-type'), disp: res.headers.get('content-disposition'), etag: res.headers.get('etag'), len: b.length, magic: [...b.slice(0, 4)], files };
    }, href);
    assert.equal(r.status, 200);
    assert.equal(r.type, 'application/zip');
    assert.match(r.disp, /^attachment/);
    assert.ok(r.etag, 'an ETag');
    assert.deepEqual(r.magic, [0x50, 0x4b, 0x03, 0x04], 'a zip');
    assert.equal(r.files.length, 2);
    const json = r.files.find((f) => f.name.endsWith('.json'));
    const png = r.files.find((f) => f.name.endsWith('.png'));
    assert.ok(json && json.size > 500, 'the Saved Object');
    assert.ok(png && png.size > 100, 'the icon');
  });

  await h.step('tts-names: the Lua fallback is collapsed, shows the script and has its own copy button', async () => {
    assert.equal(await eowyn.page.$eval('#tts-body details', (e) => e.open), false, 'collapsed');
    assert.match(await eowyn.page.$eval('#tts-body summary', (e) => e.innerText), /Show the Lua \(fallback, unverified\)/);
    await eowyn.page.$eval('#tts-body summary', (e) => e.click());
    await eowyn.settle(200);
    const lua = await val(eowyn, 'tts-lua-text');
    assert.match(lua, /function onLoad/);
    assert.match(lua, /ArmyNames/);
    assert.ok(await eowyn.page.$eval('#tts-lua-text', (e) => e.getBoundingClientRect().height > 0), 'visible once opened');
    await eowyn.click('#tts-body button', '=Copy the Lua');
    assert.match(await eowyn.page.$eval('#tts-lua-status', (e) => e.innerText), /Copied the Lua|press Ctrl\+C/);
  });

  for (const scheme of ['light', 'dark']) for (const w of SIZES) {
    await h.step(`tts-names: the names block at ${w} px, ${scheme}: no horizontal scroll, axe, screenshot`, async () => {
      await layout(eowyn, w, scheme, 900);
      await eowyn.goto(`/campaigns/${limits}/my-company`);
      await open(eowyn);
      await eowyn.page.$eval('#tts-body summary', (e) => { e.closest('details').open = true; });
      assert.ok(await eowyn.noHScroll(), 'no horizontal scroll');
      const box = await eowyn.page.$eval('[role=dialog]', (e) => { const r = e.getBoundingClientRect(); return [r.left, r.right]; });
      assert.ok(box[0] >= 0 && box[1] <= w, `the dialog is inside the screen: ${box}`);
      const overflow = await eowyn.page.$$eval('#tts-names-title ~ *, #tts-names-title ~ * *', (els) => els.filter((e) => e.getBoundingClientRect().right > innerWidth + 1).map((e) => e.tagName + '#' + e.id));
      assert.deepEqual(overflow, [], 'nothing of the block is wider than the screen');
      assert.deepEqual(await axeOn(eowyn, '[role=dialog]'), [], 'axe: the dialog');
      await eowyn.page.$eval('#tts-names-title', (e) => e.scrollIntoView());
      await eowyn.page.screenshot({ path: `out/tts-names-${w}-${scheme}.png` });
      await close(eowyn);
    });
  }
  await layout(eowyn, 1280, 'light');
}
