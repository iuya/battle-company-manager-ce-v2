// Aura rings in the Tabletop Simulator dialog and as chips on the unit cards (docs/aura-ranges.md, section 4.6), on the demo campaign "Aura parade" (Fiefdoms of Gondor,
// account `you`): the Auras section (switch, kinds with counts and radii, groups, legend), the names payload (v2 with `a` entries, v1 when no ring is selected), the
// notice to download the helper again, the choices remembered and refreshed by htmx, the keyboard, another player's company, the live variant, the aura chips of the cards
// (radius, tooltip, +N, glossary popup), 360/768/1280 px in both themes with axe and screenshots.
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const axeOn = async (s, ctx) => {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((ctx) => axe.run(ctx, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id}: ${v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' ' + (x.any[0]?.message || '')).join(' | ')}`)), ctx);
};

const open = async (s) => { await s.click('#tts-open'); await s.page.waitForSelector('#tts-names-text', { visible: true }); await s.page.waitForSelector('#tts-auras', { visible: true }); await s.settle(250); };
const close = async (s) => { await s.page.keyboard.press('Escape'); await s.settle(300); };
const payload = async (s) => JSON.parse(await s.page.$eval('#tts-names-text', (e) => e.value));
const ringsOf = (p) => p.figures.flatMap((f) => f.a || []);
const memory = (s) => s.page.evaluate(() => JSON.parse(localStorage.getItem('bcm-tts') || '{}'));
// a click on the switch, like a finger; then the htmx swap
const flip = async (s, sel) => { await s.page.evaluate((sel) => document.querySelector(sel).click(), sel); await s.settle(700); };
const checked = (s, sel) => s.page.$eval(sel, (e) => e.checked);
const unit = (f) => f.n.replace(/\[\/?b\]/g, '').replace(/ \(\d+\)$/, '');
const KINDS = ['banner', 'command', 'support', 'enemy', 'utility'];

// the kind rows of the section: [{kind, on, rings, radii}]
const kindRows = (s) => s.page.$$eval('#tts-auras [data-tts-kinds] li', (lis) => lis.map((li) => {
  const text = li.querySelector('[data-tts-kind-count]').innerText.trim();
  const m = text.match(/^(\d+) rings?, radii (.*)$/);
  return { kind: li.querySelector('input').value, on: li.querySelector('input').checked, rings: m ? Number(m[1]) : 0, radii: m ? m[2].split(', ').map((r) => Number(r.replace('"', ''))) : [], text };
}));
const total = (s) => s.page.$eval('#tts-auras-total', (e) => e.innerText.trim());
const distinct = (a) => [...new Set(a)].sort((x, y) => x - y);

// chips of the cards of a company page
const chips = (s) => s.page.$$eval('main article.unit', (arts) => arts.flatMap((a) => [...a.querySelectorAll('[data-auras] li.aura-chip')].map((c) => ({
  unit: a.querySelector('h3').innerText.trim(), text: c.querySelector('span:last-child').innerText.trim(), kind: [...c.classList].find((x) => x.startsWith('aura-k-')).slice(7), off: c.classList.contains('aura-off'),
  shown: c.offsetParent !== null, label: c.getAttribute('aria-label'), title: c.dataset.tipTitle, meta: c.dataset.tipMeta, tip: c.dataset.tipText, concept: c.dataset.concept, tab: c.getAttribute('tabindex'), letter: c.querySelector('.aura-ring').innerText.trim(),
}))));

const waitPop = (s, t) => s.page.waitForFunction((t) => {
  const p = document.querySelector('.concept-pop'), h = p && p.querySelector('#concept-title');
  return h && p.getAttribute('aria-busy') !== 'true' && h.textContent.trim().toLowerCase().includes(t.toLowerCase());
}, { timeout: 8000 }, t);
const tipVisible = (s) => s.page.evaluate(() => [...document.querySelectorAll('[role=tooltip]')].some((t) => { const r = t.getBoundingClientRect(); return r.width && r.height && getComputedStyle(t).display !== 'none'; }));

export default async function (h) {
  const you = await h.session('you@example.com');
  const eowyn = await h.session('eowyn@example.com');
  let parade;
  let base; // the default payload of the Fiefdoms company

  await h.step('auras: the section counts the kinds and radii of the company, and the payload is v2 with the rings of known figures', async () => {
    parade = await campaignId(you, 'Aura parade');
    await you.goto(`/campaigns/${parade}/my-company`);
    await you.page.evaluate(() => localStorage.removeItem('bcm-tts'));
    await open(you);
    assert.ok(await checked(you, '#tts-auras-on'), 'the master switch is on by default');
    assert.equal(await you.page.$eval('#tts-auras-on', (e) => e.getAttribute('role')), 'switch');
    const p = base = await payload(you);
    assert.equal(p.v, 2, 'rings in the names make it version 2');
    const all = ringsOf(p);
    assert.ok(all.length > 10, `the company carries many rings: ${all.length}`);
    for (const f of p.figures) assert.ok((f.a || []).length <= 6, `at most 6 rings per figure: ${unit(f)}`);
    assert.ok(all.length <= 150);
    // the rows agree with the payload, kind by kind
    const rows = await kindRows(you);
    assert.deepEqual(rows.map((r) => r.kind), KINDS, 'the five kinds the company has, in the order of the legend');
    for (const r of rows.filter((r) => r.rings)) {
      const mine = all.filter((a) => a.k === r.kind);
      assert.equal(r.rings, mine.length, `${r.kind}: ${r.rings} rings in the row, ${mine.length} in the payload`);
      assert.deepEqual(r.radii, distinct(mine.map((a) => a.r)), `${r.kind}: the radii`);
    }
    const byKind = Object.fromEntries(rows.map((r) => [r.kind, r]));
    assert.ok(byKind.banner.radii.includes(3) && byKind.command.radii.includes(12) && byKind.command.radii.includes(6), 'Standard 3", Drum and Voice of Command 12", Stand Fast 6"');
    assert.match(await total(you), new RegExp(`^${all.length} rings on ${p.figures.filter((f) => f.a).length} figures`), 'the totals line');
    // the figures
    const std = p.figures.find((f) => f.d.includes('Company Standard'));
    assert.deepEqual(std.a.map((a) => [a.r, a.k, a.w]), [[3, 'banner', 'a']], 'the Company Standard: one 3" banner ring');
    const drum = p.figures.find((f) => f.d.includes('Company Drum'));
    assert.deepEqual(drum.a.map((a) => [a.r, a.k, a.w]), [[12, 'command', 'd']], 'the Company Drum: 12" command, when declared');
    const leader = p.figures.find((f) => f.d.includes('Voice of Command'));
    assert.ok(leader.a.some((a) => a.r === 12 && a.k === 'command' && /Voice of Command/.test(a.n)), 'Voice of Command 12"');
    assert.ok(!leader.a.some((a) => a.r === 6 && a.k === 'command'), 'Voice of Command replaces the 6" command ring of the Leader');
    assert.ok(leader.a.length >= 5 && leader.a.some((a) => a.k === 'enemy' && a.r === 6 && /Furious Tirade/.test(a.n)), `the Leader keeps every ring (six per figure), Furious Tirade among them: ${leader.a.length}`);
    for (const f of p.figures.filter((f) => /\[i\](Hero|Sergeant)/.test(f.d) && !f.d.includes('Voice of Command'))) {
      assert.ok(f.a.some((a) => a.r === 6 && a.k === 'command' && /Stand Fast/.test(a.n)), `${unit(f)}: Stand Fast 6" on a Hero`);
    }
    // groups: declared is on, the spells, targeting ranges, large and optional rings are off (the ones with rings are listed)
    assert.ok(await checked(you, '#tts-auras input[name=adeclared]'));
    assert.ok(await you.page.$('#tts-auras input[name=alarge]'), 'the Large rings switch is there (war horn 24"): the counts come before the limits');
    assert.ok(rows.find((r) => r.kind === 'enemy').rings >= 1, 'the enemy row counts the Furious Tirade ring');
    for (const g of ['acast', 'atarget', 'alarge', 'aoptional']) {
      if (await you.page.$(`#tts-auras input[name=${g}]`)) assert.ok(!(await checked(you, `#tts-auras input[name=${g}]`)), `${g} is off by default`);
    }
    assert.ok(all.every((a) => a.w !== 't' && a.w !== 'c'), 'no targeting ranges and no spells in the default payload');
    assert.ok(all.every((a) => a.r <= 12), 'no ring over 12"');
    //     // the legend
    const legend = await you.page.$eval('[data-tts-legend]', (e) => e.innerText.replace(/\s+/g, ' '));
    for (const t of [/Banner, yellow/, /Command, orange/, /Support, sky blue/, /Enemy, vermillion, thick line/, /Utility, reddish purple/]) assert.match(legend, t);
    const widths = await you.page.$$eval('[data-tts-legend] .aura-ring', (els) => els.map((e) => [e.innerText.trim(), parseFloat(getComputedStyle(e).borderTopWidth)]));
    assert.deepEqual(widths.map((w) => w[0]), ['B', 'C', 'S', 'E', 'U'], 'the letters');
    assert.ok(widths[3][1] > widths[0][1] && widths[0][1] === widths[1][1], 'the enemy ring has the thicker line');
    const colours = await you.page.$$eval('[data-tts-legend] .aura-ring', (els) => els.map((e) => getComputedStyle(e).borderTopColor));
    assert.equal(new Set(colours).size, 5, 'five distinct colours');
    assert.match(await you.page.$eval('#tts-auras', (e) => e.innerText), /measured from the edge of the base/);
    await close(you);
  });

  await h.step('auras: the notice to download the helper again, and the steps for the Rings buttons', async () => {
    await open(you);
    const notice = await you.page.$eval('#tts-helper-v2', (e) => e.innerText.replace(/\s+/g, ' '));
    assert.match(notice, /Download the helper again/);
    assert.match(notice, /older than the app/);
    assert.equal(await you.page.$eval('#tts-helper-link', (e) => e.getAttribute('href')), '/tts/helper.zip');
    const steps = await you.page.$eval('ol[aria-label="How to use the names in Tabletop Simulator"]', (e) => e.innerText.replace(/\s+/g, ' '));
    for (const t of [/Rings/, /B C S E U/, /Redraw/, /Banner, Command, Support, Enemy, Utility/]) assert.match(steps, t);
    await close(you);
  });

  await h.step('auras: the kind and group switches change the payload by htmx, without a page load', async () => {
    await open(you);
    const mark = await you.page.evaluate(() => { window.__mark = 9; return 9; });
    const reqs = you.reqCount;
    const had = ringsOf(base).length;
    // a group: declared off removes the Drum's ring and the other declared ones
    await flip(you, '#tts-auras input[name=adeclared]');
    let p = await payload(you);
    assert.ok(you.reqCount > reqs && await you.page.evaluate((m) => window.__mark === m, mark), 'an htmx request, no page load');
    assert.ok(ringsOf(p).length < had, `declared off: fewer rings (${ringsOf(p).length} < ${had})`);
    assert.ok(ringsOf(p).every((a) => a.w !== 'd'), 'no declared ring');
    assert.ok(!p.figures.find((f) => f.d.includes('Company Drum')).a, 'the Drum figure has no ring left');
    assert.match(await total(you), new RegExp(`^${ringsOf(p).length} rings on `), 'the totals follow');
    assert.equal((await memory(you)).adeclared, '0', 'remembered');
    await flip(you, '#tts-auras input[name=adeclared]');
    assert.equal(ringsOf(await payload(you)).length, had, 'on again: the first payload');
    // targeting ranges and the Torch are off by default and add rings when turned on
    for (const [g, check] of [['atarget', (a) => a.w === 't'], ['aoptional', (a) => a.k === 'utility' && a.r === 6]]) {
      const sw = `#tts-auras input[name=${g}]`;
      assert.ok(await you.page.$(sw), `${g} is offered: the company has such a ring`);
      assert.match(await you.page.$eval(sw, (e) => e.closest('label').innerText), /adds \d+ rings?/);
      await flip(you, sw);
      p = await payload(you);
      assert.ok(ringsOf(p).some(check), `${g} on: its ring is in the payload`);
      assert.ok(ringsOf(p).length > had);
      await flip(you, sw);
    }
    // a kind: command off removes all the command rings and the row says so
    await flip(you, '#tts-akind-command');
    p = await payload(you);
    assert.ok(ringsOf(p).length && ringsOf(p).every((a) => a.k !== 'command'), 'command off: no command ring');
    const rows = await kindRows(you);
    assert.equal(rows.find((r) => r.kind === 'command').on, false);
    assert.ok(rows.find((r) => r.kind === 'banner').on);
    assert.equal((await memory(you)).akoff, 'command', 'the kind that is off is remembered');
    await close(you);
    // remembered on a new page
    await you.goto(`/campaigns/${parade}/my-company`);
    await open(you);
    assert.ok(!(await checked(you, '#tts-akind-command')) && await checked(you, '#tts-akind-banner'), 'the kind stays off');
    assert.ok(ringsOf(await payload(you)).every((a) => a.k !== 'command'));
    await flip(you, '#tts-akind-command');
    assert.equal(ringsOf(await payload(you)).length, had, 'command on again');
    assert.equal((await memory(you)).akoff, '', 'nothing off');
    await close(you);
  });

  await h.step('auras: with the switch off, or no ring selected, the payload stays version 1', async () => {
    await open(you);
    const plain = JSON.stringify((await payload(you)).figures.map((f) => ({ ...f, a: undefined })));
    await flip(you, '#tts-auras-on');
    let p = await payload(you);
    assert.equal(p.v, 1, 'switch off: version 1, so an older helper keeps working');
    assert.ok(p.figures.every((f) => !('a' in f) && !('b' in f)), 'no ring fields');
    assert.equal(JSON.stringify(p.figures), plain, 'the names are the same without the rings');
    assert.equal(await you.page.$('#tts-helper-v2'), null, 'no notice to download the helper again');
    assert.equal(await you.page.$('#tts-auras [data-tts-kinds]'), null, 'the kind rows are gone');
    assert.equal((await memory(you)).auras, '0');
    await flip(you, '#tts-auras-on');
    // every kind off: nothing selected, version 1
    for (const k of KINDS) await flip(you, `#tts-akind-${k}`);
    p = await payload(you);
    assert.equal(p.v, 1, 'no kind selected: version 1');
    assert.equal(ringsOf(p).length, 0);
    assert.equal(await you.page.$('#tts-helper-v2'), null);
    for (const k of KINDS) await flip(you, `#tts-akind-${k}`);
    assert.equal((await payload(you)).v, 2, 'back to version 2');
    await close(you);
  });

  await h.step('auras: the switches work from the keyboard and keep the focus after the swap', async () => {
    await open(you);
    await you.page.focus('#tts-auras-on');
    await you.page.keyboard.press('Space');
    await you.settle(700);
    assert.equal(await you.page.evaluate(() => document.activeElement.id), 'tts-auras-on', 'the focus is back on the switch after the swap');
    assert.equal((await payload(you)).v, 1, 'Space turned it off');
    await you.page.keyboard.press('Space');
    await you.settle(700);
    assert.equal(await you.page.evaluate(() => document.activeElement.id), 'tts-auras-on');
    assert.equal((await payload(you)).v, 2);
    // Tab reaches the kind checkboxes and Space flips one
    await you.page.keyboard.press('Tab');
    assert.equal(await you.page.evaluate(() => document.activeElement.id), 'tts-akind-banner');
    await you.page.keyboard.press('Space');
    await you.settle(700);
    assert.equal(await you.page.evaluate(() => document.activeElement.id), 'tts-akind-banner');
    assert.ok(ringsOf(await payload(you)).every((a) => a.k !== 'banner'));
    await you.page.keyboard.press('Space');
    await you.settle(700);
    assert.equal(ringsOf(await payload(you)).length, ringsOf(base).length);
    await close(you);
  });

  await h.step('auras: the chips are hidden by default; the Appearance toggle shows them, remembered by cookie, without a flash', async () => {
    await you.goto(`/campaigns/${parade}/my-company`);
    await you.page.evaluate(() => { document.cookie = 'bcm-auras=; Path=/; Max-Age=0'; });
    await you.goto(`/campaigns/${parade}/my-company`);
    const visible = () => you.page.$$eval('main article.unit [data-auras] li.aura-chip', (cs) => cs.filter((c) => c.offsetParent !== null).length);
    assert.equal(await you.page.evaluate(() => document.documentElement.dataset.showAuras || ''), '', 'off by default');
    assert.equal(await visible(), 0, 'no chip is visible by default');
    assert.ok((await you.page.$$('main article.unit [data-auras]')).length > 5, 'the chips are in the page, hidden by CSS');
    await you.page.click('button[aria-label="Appearance settings"]');
    await you.settle(200);
    assert.ok(!(await checked(you, '[data-auras-toggle]')), 'the toggle is off');
    await flip(you, '[data-auras-toggle]');
    assert.equal(await you.page.evaluate(() => document.documentElement.dataset.showAuras), 'on');
    assert.ok((await visible()) > 5, 'the chips show');
    await you.goto(`/campaigns/${parade}/my-company`);
    assert.equal(await you.page.evaluate(() => document.documentElement.dataset.showAuras), 'on', 'remembered after a reload, set before the first paint');
    assert.ok((await visible()) > 5);
    // the expanded card follows the setting
    assert.ok(await you.page.$$eval('main article.unit [data-auras-full]', (es) => es.length > 0));
    // off again: hidden, the Auras section of a card too
    await you.page.click('button[aria-label="Appearance settings"]');
    await you.settle(200);
    await flip(you, '[data-auras-toggle]');
    assert.equal(await visible(), 0, 'off again');
    assert.equal(await you.page.$$eval('main article.unit [data-auras-full]', (es) => es.filter((e) => e.offsetParent !== null).length), 0, 'no Auras section either');
    await flip(you, '[data-auras-toggle]');
    await you.page.keyboard.press('Escape');
    assert.ok((await visible()) > 5, 'on for the steps that follow');
  });

  await h.step('auras: chips on the unit cards: radii, kinds, tooltips, +N and the glossary popup', async () => {
    await you.goto(`/campaigns/${parade}/my-company`);
    const all = await chips(you);
    const of = (unit) => all.filter((c) => c.unit.startsWith(unit));
    const std = of('Clansman of Lamedon').find((c) => c.kind === 'banner');
    assert.equal(std.text, 'Aura 3"');
    assert.equal(std.letter, 'B');
    assert.equal(std.tab, '0');
    assert.match(std.label, /^Aura, 3 inches: Company Standard; works for friendly models, always/);
    assert.match(std.meta, /Banner · friendly models · always/);
    assert.ok(std.tip.length > 10 && std.title === 'Company Standard');
    const drum = of('Blackroot Vale Archer').find((c) => c.text === 'Aura 12"');
    assert.equal(drum.kind, 'command');
    assert.match(drum.meta, /when declared/);
    const leader = of('Knight of Dol Amroth');
    const command = leader.filter((c) => c.kind === 'command').map((c) => c.text);
    assert.ok(command.includes('Aura 12"') && !command.includes('Aura 6"'), `Voice of Command 12" replaces the 6" command ring: ${command}`);
    assert.equal(leader.find((c) => c.text === 'Aura 12"').title, 'Voice of Command');
    const horn = leader.find((c) => c.text === 'Aura 24"');
    assert.ok(horn.off && /off by default/.test(horn.meta) && /off by default/.test(horn.label), 'the war horn ring is muted and says it is off by default');
    // the Heroes: Stand Fast 6", with the glossary concept
    const sf = all.filter((c) => c.text === 'Aura 6"' && c.kind === 'command');
    assert.ok(sf.length >= 4 && sf.every((c) => /^Stand Fast/.test(c.title) && c.concept === 'Stand Fast'), `Stand Fast 6" on the Heroes: ${sf.length}`);
    assert.ok(sf.every((c) => c.unit !== leader[0].unit));
    // the three chips and +N
    const more = await you.page.$$eval('main article.unit', (arts) => arts.map((a) => ({ u: a.querySelector('h3').innerText.trim(), shown: [...a.querySelectorAll('[data-auras] li.aura-chip')].filter((c) => c.offsetParent).length,
      all: a.querySelectorAll('[data-auras] li.aura-chip').length, btn: a.querySelector('[data-auras] .aura-more')?.innerText.trim() || null })));
    const big = more.find((m) => m.u.startsWith('Knight of Dol Amroth'));
    assert.equal(big.shown, 3);
    assert.ok(big.all > 3 && big.btn.startsWith(`+${big.all - 3}`), `+N: ${JSON.stringify(big)}`);
    assert.ok(more.filter((m) => m.all <= 3).every((m) => m.btn === null), 'no +N for three chips or fewer');
    const art = (await you.page.evaluateHandle(() => [...document.querySelectorAll('main article.unit')].find((a) => a.querySelector('h3').innerText.startsWith('Knight of Dol Amroth')))).asElement();
    const btn = await art.$('[data-auras] .aura-more');
    await btn.evaluate((b) => b.scrollIntoView({ block: 'center' }));
    assert.equal(await btn.evaluate((b) => b.getAttribute('aria-expanded')), 'false');
    await btn.click();
    await you.settle(300);
    const after = await art.$$eval('[data-auras] li.aura-chip', (cs) => cs.filter((c) => c.offsetParent).length);
    assert.equal(after, big.all, '+N shows them all, in place');
    assert.equal(await art.$$eval('[data-auras] .aura-more', (bs) => bs.filter((b) => b.offsetParent).map((b) => b.innerText.trim().startsWith('Fewer') && b.getAttribute('aria-expanded') === 'true')).then((a) => a.join()), 'true', 'only the Fewer button is left');
    // the tooltip on hover, the popup on Enter and the focus back on Escape
    const probe = (await art.evaluateHandle((a) => [...a.querySelectorAll('[data-auras] li.aura-chip')].find((c) => c.dataset.tipTitle === 'Voice of Command'))).asElement();
    await probe.evaluate((x) => x.scrollIntoView({ block: 'center' }));
    await probe.hover();
    await you.settle(350);
    assert.ok(await tipVisible(you), 'hovering a chip shows the tooltip');
    await you.page.mouse.move(2, 2);
    await probe.focus();
    await you.page.keyboard.press('Enter');
    await waitPop(you, 'Voice of Command');
    await you.page.keyboard.press('Escape');
    await you.page.waitForFunction(() => !document.querySelector('.concept-layer'), { timeout: 4000 });
    assert.equal(await you.page.evaluate(() => document.activeElement.dataset.tipTitle), 'Voice of Command', 'Escape returns the focus to the chip');
    // the expanded card prints them in full
    await art.$eval('.unit-corner', (b) => b.click());
    await you.settle(400);
    const full = await art.$eval('[data-auras-full]', (e) => e.innerText.replace(/\s+/g, ' '));
    assert.match(full, /Aura 12"/);
    assert.match(full, /Voice of Command/);
    await art.$eval('.unit-corner', (b) => b.click());
    await you.settle(300);
  });

  await h.step('auras: another player\'s company page has the dialog and the chips, read only', async () => {
    await eowyn.goto(`/campaigns/${parade}/companies`);
    const href = await eowyn.page.evaluate(() => [...document.querySelectorAll('main a[href*="/companies/"]')].find((a) => /Fiefdoms/.test(a.innerText))?.getAttribute('href'));
    assert.ok(href, 'a link to the Fiefdoms company');
    await eowyn.goto(href);
    await eowyn.page.evaluate(() => { document.cookie = 'bcm-auras=1; Path=/'; });
    await eowyn.goto(href);
    await eowyn.page.evaluate(() => localStorage.removeItem('bcm-tts'));
    const cs = await chips(eowyn);
    assert.ok(cs.some((c) => c.text === 'Aura 3"' && c.kind === 'banner') && cs.some((c) => c.text === 'Aura 12"' && c.kind === 'command'), 'the chips of the other company');
    assert.equal(await eowyn.page.$('main article.unit button[hx-get*="/armoury"]'), null, 'read only: no buy buttons');
    await open(eowyn);
    const p = await payload(eowyn);
    assert.equal(p.v, 2);
    assert.equal(ringsOf(p).length, ringsOf(base).length, 'the same rings as the owner sees');
    assert.deepEqual((await kindRows(eowyn)).map((r) => r.kind), KINDS);
    await flip(eowyn, '#tts-auras-on');
    assert.equal((await payload(eowyn)).v, 1);
    await flip(eowyn, '#tts-auras-on');
    await close(eowyn);
    // her own company: its own rings (a Company Standard on a rider)
    await eowyn.goto(`/campaigns/${parade}/my-company`);
    await open(eowyn);
    const mine = await payload(eowyn);
    assert.equal(mine.v, 2);
    assert.ok(ringsOf(mine).some((a) => a.r === 3 && a.k === 'banner'), 'the Company Standard of the Riders');
    assert.ok(ringsOf(mine).some((a) => a.k === 'command' && a.r === 6), 'Stand Fast');
    await close(eowyn);
  });

  await h.step('auras: live, a unit that left the field draws no ring and keeps no chip on its compact card', async () => {
    await you.goto(`/campaigns/${parade}/my-company`);
    if (!(await you.page.$('#live-zones'))) {
      await you.goto(`/campaigns/${parade}/matches/new`);
      await you.page.waitForSelector('[name=side-name-2]');
      const pick = async (side, t) => {
        await you.page.evaluate((side, t) => {
          let el = document.querySelector(`[name=side-name-${side}]`);
          while (el && ![...el.querySelectorAll('button')].some((b) => b.innerText.includes(t))) el = el.parentElement;
          [...el.querySelectorAll('button')].find((b) => b.innerText.includes(t)).click();
        }, side, t);
        await you.settle();
      };
      // the page starts with the player's own company on side 1
      await pick(2, 'Riders of the Mark');
      await you.click('main button', 'Create match', { nav: true }).catch(() => {});
      await you.settle(400);
      await you.goto(`/campaigns/${parade}/my-company`);
    }
    await you.page.waitForSelector('#live-zones');
    assert.match(await you.page.$eval('#tts-open', (e) => e.getAttribute('hx-get')), /\?live=1$/);
    const fieldChips = (await chips(you)).length;
    assert.ok(fieldChips > 10, `the cards of the field have their chips: ${fieldChips}`);
    await open(you);
    const before = await payload(you);
    assert.equal(before.v, 2);
    const nBefore = ringsOf(before).length;
    await close(you);
    // retreat the first unit of the field
    const idx = await you.page.evaluate(() => [...document.querySelectorAll('main button')].map((b, i) => [b, i]).filter(([b]) => b.innerText.trim() === 'Actions').map(([, i]) => i)[0]);
    await (await you.page.$$('main button'))[idx].click();
    await you.click('[role=dialog] button', 'Retreated');
    await you.click('[role=dialog] button', '=Retreated');
    await you.waitText('Retreated');
    await you.settle(500);
    await open(you);
    const after = await payload(you);
    const fallen = after.figures.filter((f) => f.d.includes('[b]Fallen[/b]'));
    assert.ok(fallen.length >= 1, 'the retreated unit is marked');
    assert.ok(fallen.every((f) => !f.a), 'a fallen unit draws no ring');
    assert.ok(ringsOf(after).length < nBefore, `fewer rings than before (${ringsOf(after).length} < ${nBefore})`);
    assert.match(await total(you), new RegExp(`^${ringsOf(after).length} rings on `), 'the dialog counts the live figures');
    await close(you);
    // the compact card of the retreated unit has no chip row, the others keep theirs
    const compact = await you.page.$$eval('main li.unit-slot', (lis) => lis.map((l) => l.querySelectorAll('[data-auras]').length));
    assert.ok(compact.length >= 1 && compact.every((n) => n === 0), 'no chips on the compact cards');
    const now = (await chips(you)).length;
    assert.ok(now < fieldChips && now > 5, `the field cards keep their chips (${now} of ${fieldChips})`);
  });

  for (const scheme of ['light', 'dark']) for (const w of SIZES) {
    await h.step(`auras: the section and the chips at ${w} px, ${scheme}: no horizontal scroll, axe, screenshot`, async () => {
      await layout(you, w, scheme, 900);
      await you.goto(`/campaigns/${parade}/my-company`);
      assert.ok(await you.noHScroll(), 'the page with the chip rows has no horizontal scroll');
      const outside = await you.page.$$eval('main article.unit [data-auras]', (els) => els.filter((e) => { const a = e.closest('article').getBoundingClientRect(); const r = e.getBoundingClientRect(); return r.right > a.right + 1 || r.left < a.left - 1; }).length);
      assert.equal(outside, 0, 'the chips stay inside their cards');
      assert.deepEqual(await axeOn(you, 'main'), [], 'axe: the cards');
      await you.page.$eval('main article.unit [data-auras]', (e) => e.scrollIntoView({ block: 'center' }));
      await you.page.screenshot({ path: `out/tts-auras-chips-${w}-${scheme}.png` });
      await open(you);
      assert.ok(await you.noHScroll(), 'no horizontal scroll with the dialog open');
      const box = await you.page.$eval('[role=dialog]', (e) => { const r = e.getBoundingClientRect(); return [r.left, r.right]; });
      assert.ok(box[0] >= 0 && box[1] <= w, `the dialog is inside the screen: ${box}`);
      const wide = await you.page.$$eval('#tts-auras, #tts-auras *', (els) => els.filter((e) => e.getBoundingClientRect().right > innerWidth + 1).map((e) => e.tagName + '#' + e.id));
      assert.deepEqual(wide, [], 'nothing of the section is wider than the screen');
      assert.deepEqual(await axeOn(you, '[role=dialog]'), [], 'axe: the dialog');
      await you.page.$eval('#tts-auras', (e) => e.scrollIntoView());
      await you.page.screenshot({ path: `out/tts-auras-${w}-${scheme}.png` });
      await close(you);
    });
  }
  await layout(you, 1280, 'light');
}
