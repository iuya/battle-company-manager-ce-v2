// Who a rule works against (docs/unit-keywords.md "Rule targets"): the rule's tooltip and popup say "Works against: ...", the live match shows the
// opponent's keyword chips on every card (also the compact ones), and an enemy unit that a rule of YOUR units names gets a "<rule> applies" marker.
// The campaign "Keyword targets" has Gimli's Dwarves (Ancient Enemies (Orc)) and Radagast's Mirkwood company (Orcs, Goblins, Spiders), no match yet.
import { assert, campaignId, layout, MODAL, SIZES } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const axeOn = async (s, ctx) => {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((ctx) => axe.run(ctx, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id}: ${v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' ' + (x.any[0]?.message || '')).join(' | ')}`)), ctx);
};
const tipText = (s) => s.page.evaluate(() => [...document.querySelectorAll('[role=tooltip]')].filter((t) => { const r = t.getBoundingClientRect(); return r.width && r.height && getComputedStyle(t).display !== 'none'; }).map((t) => t.innerText).join('\n'));
const waitPop = (s, t) => s.page.waitForFunction((t) => {
  const p = document.querySelector('.concept-pop'), h = p && p.querySelector('#concept-title');
  return h && p.getAttribute('aria-busy') !== 'true' && h.textContent.trim().toLowerCase().includes(t.toLowerCase());
}, { timeout: 8000 }, t).catch(() => { throw new Error(`the popup did not show "${t}"`); });
const closePop = async (s) => {
  await s.page.keyboard.press('Escape');
  await s.page.waitForFunction(() => !document.querySelector('.concept-layer'), { timeout: 4000 });
};

// The Field cards of a company page: title, every keyword it has (row and full list), and the markers.
const cards = (s) => s.page.$$eval('#live-zones .unit-slot', (slots) => slots.map((u) => ({
  title: (u.querySelector('h3')?.innerText || '').trim(),
  keywords: [...u.querySelectorAll('[data-keywords] li.kw-chip')].map((c) => c.textContent.trim()),
  rowShown: [...u.querySelectorAll('[data-keywords] ul[aria-label=Keywords] > li.kw-chip')].map((c) => c.textContent.trim()),
  marks: [...u.querySelectorAll('[data-rule-match]')].map((m) => ({ rule: m.dataset.ruleMatch, text: m.textContent.trim(), tab: m.getAttribute('tabindex'), tip: m.dataset.tipText, meta: m.dataset.tipMeta })),
})));

export default async function (h) {
  const you = await h.session('you@example.com');
  const gimli = await h.session('gimli@example.com');
  const radagast = await h.session('radagast@example.com');
  let cid, dCo, mCo;

  await h.step('rule targets: Erebor\'s Dwarves against Mirkwood\'s company, a match is opened', async () => {
    cid = await campaignId(you, 'Keyword targets');
    await you.goto(`/campaigns/${cid}/matches/new`);
    const pick = async (side, text) => {
      await you.page.evaluate((side, text) => {
        let el = document.querySelector(`[name=side-name-${side}]`);
        while (el && ![...el.querySelectorAll('button')].some((b) => b.innerText.includes(text))) el = el.parentElement;
        [...el.querySelectorAll('button')].find((b) => b.innerText.includes(text)).click();
      }, side, text);
      await you.settle();
    };
    await pick(1, 'Dwarves who hate Orcs');
    await pick(2, 'Mirkwood');
    await you.click('main button', 'Create match', { nav: true }).catch(() => {});
    await you.settle(400);
    const mid = (new URL(you.page.url()).pathname.match(/matches\/(\d+)/) || [])[1];
    assert.ok(mid, 'redirected to the new match: ' + you.page.url());
    const ids = await you.page.evaluate(() => [...new Set([...document.querySelectorAll('main a[href*="/companies/"]')].map((a) => a.getAttribute('href').match(/companies\/(\d+)/)[1]))]);
    assert.equal(ids.length, 2, 'two companies in the match');
    [dCo, mCo] = ids;
  });
  if (!mCo) return;

  await h.step('rule targets: a rule\'s tooltip and helper popup say who it works against; the unit\'s own card is not marked', async () => {
    await gimli.goto(`/campaigns/${cid}/companies/${dCo}`);
    await gimli.waitText('is running');
    const rule = await gimli.page.evaluateHandle(() => [...document.querySelectorAll('#live-zones ul[aria-label="Special rules"] li')].find((l) => l.textContent.trim().startsWith('Ancient Enemies (Orc)')));
    const chip = rule.asElement();
    assert.ok(chip, 'a Dwarf has Ancient Enemies (Orc)');
    const data = await chip.evaluate((c) => ({ tip: c.dataset.tipText, works: c.dataset.tipWorks }));
    assert.match(data.tip, /Works against: enemy models with the Orc keyword\./, `the tooltip text: ${data.tip}`);
    assert.equal(data.works, 'Works against: enemy models with the Orc keyword.');
    await chip.evaluate((x) => x.scrollIntoView({ block: 'center' }));
    await chip.hover();
    await gimli.settle(350);
    assert.match(await tipText(gimli), /Works against: enemy models with the Orc keyword/, 'hovering shows the line in the tooltip');
    await gimli.page.mouse.move(2, 2);
    await chip.focus();
    await gimli.page.keyboard.press('Enter');
    await waitPop(gimli, 'Ancient Enemies');
    const works = await gimli.page.$eval('.concept-pop .concept-works', (e) => e.textContent.trim());
    assert.equal(works, 'Works against: enemy models with the Orc keyword.', 'the popup carries the line');
    await closePop(gimli);
    assert.equal(await gimli.page.$$eval('#live-zones [data-rule-match]', (e) => e.length), 0, 'nothing on the unit\'s own side is marked');
    assert.equal(await gimli.page.$$eval('#live-zones .unit-slot .ring-2, #live-zones .unit-slot [aria-current]', (e) => e.length), 0, 'no highlight on the own card for an enemy rule');
  });

  let marked = 0;
  await h.step('rule targets: on the opponent\'s page every card shows its keywords and the Orcs carry "Ancient Enemies (Orc) applies" (and only the Orcs)', async () => {
    await gimli.goto(`/campaigns/${cid}/companies/${mCo}`);
    await gimli.waitText('is running');
    const cs = await cards(gimli);
    assert.ok(cs.length >= 4, `the Mirkwood company has units (${cs.length})`);
    for (const c of cs) {
      assert.ok(c.rowShown.length > 0 || c.keywords.length > 0, `${c.title}: the opponent's unit shows its keywords`);
      const orc = c.keywords.includes('Orc');
      assert.equal(c.marks.length, orc ? 1 : 0, `${c.title} (${c.keywords}): the marker is on the Orcs only, got ${JSON.stringify(c.marks)}`);
    }
    const withMark = cs.filter((c) => c.marks.length);
    assert.ok(withMark.length >= 1 && withMark.length < cs.length, `some, not all, are marked (${withMark.length} of ${cs.length})`);
    marked = withMark.length;
    const m = withMark[0].marks[0];
    assert.equal(m.rule, 'Ancient Enemies (Orc)');
    assert.equal(m.text, 'Ancient Enemies (Orc) applies');
    assert.equal(m.tab, '0', 'keyboard reachable');
    assert.match(m.tip, /^Your .+ Ancient Enemies \(Orc\)\. This unit has the Orc keyword, so the rule works against it\.$/, m.tip);
    assert.match(m.meta, /^Your /);
    // the marker has a tooltip and opens the helper popup for the rule; Escape returns the focus to it
    const badge = await gimli.page.$('#live-zones [data-rule-match]');
    await badge.evaluate((x) => x.scrollIntoView({ block: 'center' }));
    await badge.hover();
    await gimli.settle(350);
    assert.match(await tipText(gimli), /works against it/, 'hovering the marker shows its tooltip');
    await gimli.page.mouse.move(2, 2);
    await badge.focus();
    await gimli.page.keyboard.press('Enter');
    await waitPop(gimli, 'Ancient Enemies');
    await closePop(gimli);
    assert.ok(await gimli.page.evaluate(() => document.activeElement?.hasAttribute('data-rule-match')), 'Escape returns the focus to the marker');
  });

  await h.step('rule targets: the reverse is not marked, and the opponent sees nothing on his own units', async () => {
    await radagast.goto(`/campaigns/${cid}/companies/${dCo}`);
    await radagast.waitText('is running');
    assert.equal(await radagast.page.$$eval('#live-zones [data-rule-match]', (e) => e.length), 0, 'Mirkwood has no rule that names a Dwarf: no marker on the Dwarves');
    assert.ok((await cards(radagast)).every((c) => c.keywords.length > 0), 'the Dwarves show their keywords to the opponent');
    await radagast.goto(`/campaigns/${cid}/companies/${mCo}`);
    await radagast.waitText('is running');
    assert.equal(await radagast.page.$$eval('#live-zones [data-rule-match]', (e) => e.length), 0, 'a player\'s own company is not marked');
    await you.goto(`/campaigns/${cid}/companies/${mCo}`);
    await you.waitText('is running');
    assert.equal(await you.page.$$eval('#live-zones [data-rule-match]', (e) => e.length), 0, 'an onlooker with no company in the match sees no markers');
  });

  await h.step('rule targets: "Wounded a unit" lists the enemy units with their keywords and the markers', async () => {
    await gimli.goto(`/campaigns/${cid}/companies/${dCo}`);
    await gimli.waitText('is running');
    const idx = await gimli.page.evaluate(() => [...document.querySelectorAll('main button')].map((b, i) => [b, i]).filter(([b]) => b.innerText.trim() === 'Actions').map(([, i]) => i)[0]);
    await (await gimli.page.$$('main button'))[idx].click();
    await gimli.click(MODAL + ' button', 'Wounded a unit');
    await gimli.waitText('Pick who was wounded');
    const rows = await gimli.page.$$eval(MODAL + ' [role=radiogroup] li', (l) => l.map((li) => ({
      name: li.querySelector('.font-medium')?.textContent.trim(), kws: [...li.querySelectorAll('[data-keywords-flat] .kw-chip')].map((c) => c.textContent.trim()),
      marks: [...li.querySelectorAll('[data-rule-match]')].map((m) => m.textContent.trim()),
    })));
    assert.ok(rows.length >= 4, 'the enemy units are listed');
    for (const r of rows) {
      assert.ok(r.kws.length > 0, `${r.name} shows its keywords`);
      assert.deepEqual(r.marks, r.kws.includes('Orc') ? ['Ancient Enemies (Orc) applies'] : [], `${r.name} (${r.kws}): the marker is on the Orcs`);
    }
    assert.equal(rows.filter((r) => r.marks.length).length, marked, 'the same units are marked as on the page');
    if (await gimli.page.evaluate(() => innerWidth) <= 1280) await gimli.page.screenshot({ path: 'out/keyword-targets-wound-sheet.png' });
    await gimli.click(MODAL + ' button', '=Close');
    await gimli.settle(300);
  });

  await h.step('rule targets: an enemy that leaves the field loses its marker through the live refresh, and its compact card keeps the chips', async () => {
    await gimli.goto(`/campaigns/${cid}/companies/${mCo}`);
    await gimli.waitText('is running');
    const before = (await cards(gimli)).filter((c) => c.marks.length);
    assert.equal(before.length, marked);
    const victim = before[0].title;
    await radagast.goto(`/campaigns/${cid}/companies/${mCo}`);
    await radagast.waitText('is running');
    const idx = await radagast.page.evaluate((title) => {
      const slots = [...document.querySelectorAll('#live-zones .unit-slot')];
      const slot = slots.find((u) => (u.querySelector('h3')?.innerText || '').trim() === title && u.querySelector('[data-keywords] li.kw-chip')?.textContent.includes('Orc'));
      const btn = [...slot.querySelectorAll('button')].find((b) => b.innerText.trim() === 'Actions');
      return [...document.querySelectorAll('main button')].indexOf(btn);
    }, victim);
    assert.ok(idx >= 0, 'the Actions button of the Orc');
    await (await radagast.page.$$('main button'))[idx].click();
    await radagast.click(MODAL + ' button', 'Retreated');
    await radagast.click(MODAL + ' button', '=Retreated');
    await radagast.waitText('Failed courage');
    // Gimli's page is refreshed by the event stream: one marker fewer, the rest are still there
    await gimli.page.waitForFunction((n) => document.querySelectorAll('#live-zones section[aria-label=Retreated] li, #live-zones [aria-label=Retreated] li').length > 0 && document.querySelectorAll('#live-zones .unit-slot [data-rule-match]').length === n,
      { timeout: 10000 }, marked - 1).catch(async () => { throw new Error(`the refresh did not leave ${marked - 1} markers (${await gimli.page.$$eval('#live-zones [data-rule-match]', (e) => e.length)})`); });
    if (marked > 1) assert.equal(await gimli.page.$$eval('#live-zones .unit-slot [data-rule-match]', (e) => e.length), marked - 1, 'the other markers survive the refresh');
    const compact = await gimli.page.$$eval('#live-zones [aria-label=Retreated] li.card', (l) => l.map((li) => ({ chips: [...li.querySelectorAll('[data-keywords] li.kw-chip')].map((c) => c.textContent.trim()), marks: li.querySelectorAll('[data-rule-match]').length })));
    assert.equal(compact.length, 1, 'one retreated unit');
    assert.ok(compact[0].chips.includes('Orc'), `the compact card shows its keywords: ${compact[0].chips}`);
    assert.equal(compact[0].marks, 0, 'a unit outside the field is not marked');
    // bring it back: the marker returns
    await radagast.click('main button', 'Return to the battle');
    await gimli.page.waitForFunction((n) => document.querySelectorAll('#live-zones .unit-slot [data-rule-match]').length === n, { timeout: 10000 }, marked)
      .catch(() => { throw new Error('the marker did not come back with the unit'); });
  });

  for (const scheme of ['light', 'dark']) for (const w of SIZES) {
    await h.step(`rule targets: the opponent's cards at ${w} px, ${scheme}: no horizontal scroll, the markers wrap inside the card, axe clean`, async () => {
      await layout(gimli, w, scheme);
      await gimli.goto(`/campaigns/${cid}/companies/${mCo}`);
      await gimli.waitText('is running');
      assert.ok(await gimli.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      const outside = await gimli.page.$$eval('#live-zones [data-rule-match]', (m) => m.filter((e) => { const c = e.closest('.unit-slot').getBoundingClientRect(), r = e.getBoundingClientRect(); return r.right > c.right + 1 || r.left < c.left - 1; }).length);
      assert.equal(outside, 0, 'the markers stay inside their card');
      const small = await gimli.page.$$eval('#live-zones [data-rule-match]', (m) => m.map((e) => { const r = e.getBoundingClientRect(); return r.height; }));
      assert.ok(small.every((hgt) => hgt >= 16), 'the markers are drawn');
      const bad = await axeOn(gimli, '#live-zones');
      assert.equal(bad.length, 0, `axe: ${bad.join('; ')}`);
      const card = await gimli.page.evaluateHandle(() => document.querySelector('#live-zones [data-rule-match]').closest('.unit-slot'));
      await card.asElement().screenshot({ path: `out/keyword-targets-${w}-${scheme}.png` });
      // the Dwarves' side: the rule tooltip chip fits too
      await gimli.goto(`/campaigns/${cid}/companies/${dCo}`);
      assert.ok(await gimli.noHScroll(), `no horizontal scroll on the own page at ${w}px ${scheme}`);
    });
  }
  await layout(gimli, 1280, 'light');
}
