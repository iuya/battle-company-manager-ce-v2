// The copy button and its dialog (docs/tabletop-simulator.md): the company as a Tabletop Simulator army list.
// 1. My company of a Rohan cavalry company: the button, its tooltip, the dialog (list, counts, notes, how-to), Escape and focus, the
//    Tab trap, Copy to the clipboard, the switches (a Warg company), the remembered choice, 360/768/1280 px in both themes with axe.
// 2. Another player's read-only company page, the wanderers and creatures of a Shire company (stand-ins, "Shown as").
// 3. The live variant in a match: a retreated unit goes to a second warband after a blank line, with a note.
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const axeOn = async (s, ctx) => {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((ctx) => axe.run(ctx, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id}: ${v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' ' + (x.any[0]?.message || '')).join(' | ')}`)), ctx);
};

const LINE = /^( {4})?\((\d+x )?[^()\n]+: [^()\n]*\)$/; // one bracket pair per line, "Nx" for followers, ": )" for no options
const open = async (s) => { await s.click('#tts-open'); await s.page.waitForSelector('#tts-result', { visible: true }); await s.settle(200); };
const text = (s) => s.page.$eval('#tts-text', (e) => e.value);
const lines = (t) => t.split('\n').filter((l) => l.trim());
const close = async (s) => { await s.page.keyboard.press('Escape'); await s.settle(300); };
const flip = async (s, name) => { await s.page.evaluate((n) => document.querySelector(`#tts-body input[name=${n}]`).closest('label').click(), name); await s.settle(400); await s.page.waitForSelector('#tts-text'); };
const checked = (s, name) => s.page.$eval(`#tts-body input[name=${name}]`, (e) => e.checked);

export default async function (h) {
  const eowyn = await h.session('eowyn@example.com');
  const pippin = await h.session('pippin@example.com');
  const you = h.sessions.find((s) => s.email === 'you@example.com') || await h.session('you@example.com');
  let limits, gallery;

  await h.step('tts: the button is next to the company name and has an accessible name and a tooltip on keyboard focus', async () => {
    limits = await campaignId(eowyn, 'Limits, creatures');
    await eowyn.goto(`/campaigns/${limits}/my-company`);
    const b = await eowyn.page.$eval('#tts-open', (e) => ({ label: e.getAttribute('aria-label'), desc: e.getAttribute('aria-describedby'), w: e.getBoundingClientRect().width, h: e.getBoundingClientRect().height, near: e.closest('.relative').querySelector('h2')?.innerText }));
    assert.match(b.label, /army list for Tabletop Simulator/i);
    assert.equal(b.near, 'Riders on every horse', 'the button sits in the row of the company name');
    assert.ok(b.w >= 32 && b.h >= 32, `a usable touch target (${b.w}x${b.h})`);
    assert.ok(!(await eowyn.page.$eval('#tts-tip', (e) => e.getBoundingClientRect().height)), 'the tooltip is hidden until hover or focus');
    await eowyn.page.focus('#tts-open');
    await eowyn.settle(100);
    const tip = await eowyn.page.$eval('#tts-tip', (e) => ({ h: e.getBoundingClientRect().height, t: e.innerText }));
    assert.ok(tip.h > 0, 'keyboard focus shows the tooltip');
    assert.match(tip.t, /Paste it into a Notebook tab named ArmyList and press Spawn Army/);
    assert.match(tip.t, /closest standard MESBG model/);
    assert.match(tip.t, /Names, paths, injuries and special rules are not carried over/);
    await eowyn.page.mouse.move(5, 5);
    await eowyn.page.hover('#tts-open');
    assert.ok(await eowyn.page.$eval('#tts-tip', (e) => e.getBoundingClientRect().height > 0), 'hover shows it too');
  });

  await h.step('tts: the dialog of a Rohan cavalry company: the list, the counts, the notes and the how-to', async () => {
    await eowyn.goto(`/campaigns/${limits}/my-company`);
    await open(eowyn);
    const t = await text(eowyn);
    for (const l of lines(t)) assert.match(l, LINE, `a line the mod can parse: ${l}`);
    assert.match(t.split('\n')[0], /^\(Rider of Rohan: \)$/, 'the first unit is the hero line, without a count');
    assert.ok(/\n {4}\(\d+x Warrior of Rohan: Shield\)/.test(t), 'followers carry their count');
    assert.ok(/\(1x Rohan Royal Guard: Horse\)/.test(t), 'a Royal Guard with the horse option');
    assert.ok(!/Warg/.test(t), 'no Warg on a Rohan company');
    const body = await eowyn.page.$eval('#tts-body', (e) => e.innerText.replace(/\s+/g, ' '));
    assert.match(body, /15 models\. Break Point: 7\.5 \(50% of 15\)/);
    assert.match(body, /it will report 15 models/);
    assert.match(body, /Dismounting/);
    assert.match(body, /Rider of Rohan: change the figure's state to dismount it/);
    assert.match(body, /ArmyList/); assert.match(body, /Spawn Army/);
    assert.equal(await eowyn.page.$$eval('#tts-body > ol li', (l) => l.length), 3, 'three numbered steps');
    assert.equal(await eowyn.page.$eval('#tts-text', (e) => e.readOnly), true, 'the area is read only');
    assert.match(await eowyn.page.$eval('#tts-text', (e) => getComputedStyle(e).fontFamily), /mono/i);
    assert.ok(await eowyn.page.$eval('#tts-status', (e) => e.getAttribute('aria-live') === 'polite'), 'a live region for the confirmation');
  });

  await h.step('tts: Escape closes the dialog and focus returns to the button; Tab stays inside', async () => {
    const inside = () => eowyn.page.evaluate(() => !!document.activeElement.closest('[role=dialog]'));
    assert.ok(await inside(), 'focus moved into the dialog when it opened');
    for (let i = 0; i < 12; i++) { await eowyn.page.keyboard.press('Tab'); assert.ok(await inside(), `Tab ${i + 1} stays in the dialog`); }
    await eowyn.page.keyboard.down('Shift'); await eowyn.page.keyboard.press('Tab'); await eowyn.page.keyboard.up('Shift');
    assert.ok(await inside(), 'Shift+Tab stays in the dialog');
    await close(eowyn);
    assert.ok(!(await eowyn.page.$eval('#tts-text', (e) => e.getBoundingClientRect().height).catch(() => 0)), 'the dialog is closed');
    assert.equal(await eowyn.page.evaluate(() => document.activeElement.id), 'tts-open', 'focus is back on the button');
  });

  await h.step('tts: Copy puts the list into the clipboard and says so in a live region', async () => {
    const origin = new URL(eowyn.page.url()).origin;
    await eowyn.ctx.overridePermissions(origin, ['clipboard-read', 'clipboard-write']);
    await open(eowyn);
    const want = await text(eowyn);
    await eowyn.click('#tts-body button', 'Copy to clipboard');
    const got = await eowyn.page.evaluate(() => navigator.clipboard.readText());
    assert.equal(got, want, 'the clipboard holds the list');
    assert.match(await eowyn.page.$eval('#tts-status', (e) => e.innerText), /Copied: 15 models/);
    // without the clipboard API the text is selected for Ctrl+C
    await eowyn.page.evaluate(() => { Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true }); document.execCommand = () => false; });
    await eowyn.click('#tts-body button', 'Copy to clipboard');
    assert.match(await eowyn.page.$eval('#tts-status', (e) => e.innerText), /press Ctrl\+C/i);
    assert.equal(await eowyn.page.evaluate(() => { const a = document.getElementById('tts-text'); return a.value.slice(a.selectionStart, a.selectionEnd).length; }), want.length, 'all the text is selected');
    await close(eowyn);
  });

  await h.step('tts: the switches (a Warg company) change the list without a page load and are remembered', async () => {
    gallery = await campaignId(you, 'Gallery');
    await you.goto(`/campaigns/${gallery}/rankings`);
    const href = await you.page.evaluate(() => [...document.querySelectorAll('main tbody tr')].find((r) => /Angmar/.test(r.innerText))?.querySelector('a[href*="/companies/"]')?.getAttribute('href'));
    assert.ok(href, 'the Angmar company is in the gallery');
    await you.goto(href);
    await you.page.evaluate(() => localStorage.removeItem('bcm-tts'));
    await open(you);
    assert.ok(await checked(you, 'foot') && await checked(you, 'mounts'), 'both switches default on');
    const marker = await you.page.evaluate(() => { window.__mark = 1; return 1; });
    const both = await text(you);
    assert.match(both, /\(1x Fell Warg: \)/, 'the Warg is a figure of its own');
    const bothModels = await you.page.$eval('#tts-count', (e) => e.innerText);
    const reqs = you.reqCount;
    await flip(you, 'mounts');
    const noMounts = await text(you);
    assert.ok(!/Fell Warg/.test(noMounts), 'without the mount figures there is no Fell Warg');
    assert.ok(you.reqCount > reqs && await you.page.evaluate((m) => window.__mark === m, marker), 'an htmx request, no page load');
    await flip(you, 'foot');
    const noFoot = await text(you);
    assert.ok(lines(noFoot).length < lines(both).length, 'fewer lines with both off');
    assert.match(await you.page.$eval('#tts-count', (e) => e.innerText), /it will report (\d+) models/);
    assert.ok(await you.page.$eval('#tts-count', (e) => !/extra figure/.test(e.innerText)), 'no extra figures with both off');
    assert.match(bothModels, /extra figure/, 'with both on the count names the extra figures');
    const mem = await you.page.evaluate(() => JSON.parse(localStorage.getItem('bcm-tts')));
    assert.deepEqual({ foot: mem.foot, mounts: mem.mounts, rules: mem.rules }, { foot: '0', mounts: '0', rules: 'none' });
    await close(you);
    await you.goto(href); // a new page: the choice is remembered
    await open(you);
    assert.ok(!(await checked(you, 'foot')) && !(await checked(you, 'mounts')), 'both switches stay off');
    assert.equal(await text(you), noFoot);
    await flip(you, 'foot'); await flip(you, 'mounts');
    assert.equal(await text(you), both, 'on again, the first list');
    await close(you);
  });

  await h.step('tts: another player\'s company page has the button; the dialog works there', async () => {
    const pippinId = await eowyn.page.evaluate(async (c) => {
      const html = await (await fetch(`/campaigns/${c}/rankings`)).text();
      const doc = new DOMParser().parseFromString(html, 'text/html');
      return [...doc.querySelectorAll('main tbody tr')].find((r) => /Pony Express/.test(r.textContent))?.querySelector('a[href*="/companies/"]')?.getAttribute('href');
    }, limits);
    assert.ok(pippinId, 'Pippin\'s company is listed');
    await eowyn.goto(pippinId);
    assert.match(await eowyn.text(), /Viewing only|Read only/);
    await open(eowyn);
    const body = await eowyn.page.$eval('#tts-body', (e) => e.innerText.replace(/\s+/g, ' '));
    const t = await text(eowyn);
    assert.match(t, /\(1x Bill the Pony: \)/, 'the Baggage Pony is Bill the Pony');
    assert.match(t, /Crebain/); assert.match(t, /Malbeth the Seer/);
    assert.match(body, /Shown as/);
    assert.match(body, /Baggage Pony is shown as Bill the Pony/);
    assert.match(body, /Wandering Sage is shown as Malbeth the Seer/);
    assert.match(body, /16 models/);
    await close(eowyn);
  });

  await h.step('tts: a stranger to the campaign cannot read a company\'s list', async () => {
    const frodo = await h.session('frodo@example.com');
    frodo.allow.push(/404/);
    const res = await frodo.page.evaluate(async (u) => (await fetch(u)).status, `/campaigns/${limits}/companies/1/tts`);
    assert.equal(res, 404, 'not a member: the 404 of the company page');
  });

  let liveCompany, angmar;
  await h.step('tts: live, a retreated unit goes to a second warband with a note', async () => {
    // Companies of the gallery campaign (the other flows' matches leave the pool companies busy): Angmar against Rohan, started by the organiser
    angmar = await h.session('angmar-carn-dum@example.com');
    await angmar.goto(`/campaigns/${gallery}/my-company`);
    if (!(await angmar.page.$('#live-zones'))) {
      await you.goto(`/campaigns/${gallery}/matches/new`);
      const pick = async (side, t) => {
        await you.page.evaluate((side, t) => {
          let el = document.querySelector(`[name=side-name-${side}]`);
          while (el && ![...el.querySelectorAll('button')].some((b) => b.innerText.includes(t))) el = el.parentElement;
          [...el.querySelectorAll('button')].find((b) => b.innerText.includes(t)).click();
        }, side, t);
        await you.settle();
      };
      await pick(1, 'Angmar');
      await pick(2, 'Rohan Company');
      await you.click('main button', 'Create match', { nav: true }).catch(() => {});
      await you.settle(400);
      await angmar.goto(`/campaigns/${gallery}/my-company`);
    }
    await angmar.page.waitForSelector('#live-zones');
    liveCompany = await angmar.page.$eval('#live-zones', (e) => e.dataset.company);
    assert.match(await angmar.page.$eval('#tts-open', (e) => e.getAttribute('hx-get')), /\/tts\?live=1$/, 'in a match the button asks for the live list');
    await open(angmar);
    const before = await text(angmar);
    assert.ok(!/\n\n/.test(before.trim()), 'one warband while everybody is in the field');
    assert.ok(!(await angmar.page.$eval('#tts-body', (e) => /Second warband/.test(e.innerText))), 'no second warband yet');
    await close(angmar);
    // retreat the first unit of the field
    await angmar.liveSheet('retreat'); // no Fled button on the card: its window
    await angmar.click('[role=dialog] button', '=Retreated');
    await angmar.waitText('Retreated');
    await angmar.settle(500);
    await open(angmar);
    const after = await text(angmar);
    const bands = after.trim().split(/\n\s*\n/);
    assert.equal(bands.length, 2, `two warbands apart by a blank line:\n${after}`);
    assert.match(bands[1].split('\n')[0], /^\([^()]+: [^()]*\)$/, 'the second warband starts with its hero line');
    for (const l of lines(after)) assert.match(l, LINE);
    const body = await angmar.page.$eval('#tts-body', (e) => e.innerText.replace(/\s+/g, ' '));
    assert.match(body, /Second warband/);
    assert.match(body, /wounded or out of the fight \(.+\) are in a second warband after the main one/);
    assert.match(body, /as it is in the match now/);
    // level B: the names of the live list mark the fallen figure
    const p = JSON.parse(await angmar.page.$eval('#tts-names-text', (e) => e.value));
    assert.ok(p.figures.some((f) => f.d.includes('[b]Fallen[/b]')), 'a fallen figure is marked in its description');
    assert.ok(p.figures.some((f) => !f.d.includes('Fallen')), 'the others are not');
    await close(angmar);
  });

  await h.step('tts: the live list is also on the page of the same company seen by another player', async () => {
    await you.goto(`/campaigns/${gallery}/companies/${liveCompany}`);
    assert.match(await you.page.$eval('#tts-open', (e) => e.getAttribute('hx-get')), /\?live=1$/);
    await open(you);
    assert.equal((await text(you)).trim().split(/\n\s*\n/).length, 2, 'the same second warband');
    await close(you);
  });

  for (const scheme of ['light', 'dark']) for (const w of SIZES) {
    await h.step(`tts: the dialog at ${w} px, ${scheme}: no horizontal scroll, axe, screenshot`, async () => {
      await layout(eowyn, w, scheme, 900);
      await eowyn.goto(`/campaigns/${limits}/my-company`);
      assert.ok(await eowyn.noHScroll(), 'the page, with the button, has no horizontal scroll');
      await eowyn.page.screenshot({ path: `out/tts-button-${w}-${scheme}.png` });
      await eowyn.page.hover('#tts-open');
      await eowyn.page.screenshot({ path: `out/tts-tooltip-${w}-${scheme}.png` });
      const tipBox = await eowyn.page.$eval('#tts-tip', (e) => { const r = e.getBoundingClientRect(); return [r.left, r.right]; });
      assert.ok(tipBox[0] >= 0 && tipBox[1] <= w, `the tooltip is inside the screen: ${tipBox}`);
      await open(eowyn);
      assert.ok(await eowyn.noHScroll(), 'no horizontal scroll with the dialog open');
      const box = await eowyn.page.$eval('[role=dialog]', (e) => { const r = e.getBoundingClientRect(); return [r.left, r.right]; });
      assert.ok(box[0] >= 0 && box[1] <= w, `the dialog is inside the screen: ${box}`);
      assert.deepEqual(await axeOn(eowyn, '[role=dialog]'), [], 'axe: the dialog');
      await eowyn.page.screenshot({ path: `out/tts-dialog-${w}-${scheme}.png` });
      await close(eowyn);
    });
  }
  await layout(eowyn, 1280, 'light');
}
