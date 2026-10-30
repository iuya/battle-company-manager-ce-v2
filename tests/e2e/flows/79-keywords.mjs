// Unit keywords on the cards (docs/unit-keywords.md): the chip row of My Company (a Rohan cavalry company, Elves, Dwarves) without the keywords
// the icon, the insignia and the gear already say, the +N (on the style guide: real units rarely have more than five) that opens the full list in place, the expanded card, a chip's tooltip and helper popup, the armoury's wanderers and creatures, a fallen
// unit, another player's read-only page, 360/768/1280 px in both themes, axe. (A lost mount changes no chip: 84-match-extras.)
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const axeOn = async (s, ctx) => {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((ctx) => axe.run(ctx, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id}: ${v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' ' + (x.any[0]?.message || '')).join(' | ')}`)), ctx);
};

// Every card of the page: its title, the chips of the compact row (name, group class, concept) and the groups of the full list.
const cards = (s, sel = 'main .unit-slot') => s.page.$$eval(sel, (slots) => slots.map((u) => {
  const row = u.querySelector('[data-keywords] ul[aria-label=Keywords]');
  const full = u.querySelector('[data-keywords] .kw-full');
  const chip = (c) => ({ name: c.textContent.trim(), cls: c.className, concept: c.dataset.concept, meta: c.dataset.tipMeta, tip: c.dataset.tipText, label: c.getAttribute('aria-label'), tab: c.getAttribute('tabindex') });
  return {
    title: (u.querySelector('h3')?.innerText || '').trim(),
    has: !!u.querySelector('[data-keywords]'),
    row: row ? [...row.querySelectorAll(':scope > li.kw-chip')].map(chip) : [],
    more: row?.querySelector('button.kw-more')?.innerText.replace(/\s+/g, ' ').trim() || null,
    groups: full ? [...full.querySelectorAll('dt')].map((d) => d.textContent.trim()) : [],
    all: full ? [...full.querySelectorAll('li.kw-chip')].map(chip) : [],
  };
}));
const names = (l) => l.map((c) => c.name);

const popTitle = (s) => s.page.evaluate(() => document.querySelector('.concept-pop #concept-title')?.textContent.trim() || null);
const waitPop = (s, t) => s.page.waitForFunction((t) => {
  const p = document.querySelector('.concept-pop'), h = p && p.querySelector('#concept-title');
  return h && p.getAttribute('aria-busy') !== 'true' && h.textContent.trim().toLowerCase().includes(t.toLowerCase());
}, { timeout: 8000 }, t).catch(async () => { throw new Error(`the popup did not show "${t}" (shows ${await popTitle(s)})`); });
const tipVisible = (s) => s.page.evaluate(() => [...document.querySelectorAll('[role=tooltip]')].some((t) => { const r = t.getBoundingClientRect(); return r.width && r.height && getComputedStyle(t).display !== 'none'; }));

export default async function (h) {
  const eowyn = await h.session('eowyn@example.com');
  const galadriel = await h.session('galadriel@example.com');
  const gimli = await h.session('gimli@example.com');
  const pippin = await h.session('pippin@example.com');
  const bilbo = await h.session('bilbo@example.com');
  let cid;

  const OBVIOUS = ['Hero', 'Warrior', 'Infantry', 'Cavalry', 'Beast', 'War Beast', 'Monster', 'Mount'];
  await h.step('keywords: a Rohan cavalry unit shows Man and Rohan (race first) and none of what the icon, insignia and gear say', async () => {
    cid = await campaignId(eowyn, 'Limits, creatures');
    await eowyn.goto(`/campaigns/${cid}/my-company`);
    const cs = await cards(eowyn);
    assert.ok(cs.length >= 3, 'the company has units');
    const rider = cs.find((c) => c.title.startsWith('Rider of Rohan')) || cs[0];
    assert.ok(rider.has, 'the Rider has a keyword row');
    assert.equal(rider.row[0].name, 'Man', `race first: ${names(rider.row)}`);
    assert.ok(rider.row[0].cls.includes('kw-race'));
    assert.ok(names(rider.row).includes('Rohan') && rider.row.find((c) => c.name === 'Rohan').cls.includes('kw-faction'), `a faction chip: ${names(rider.row)}`);
    for (const c of cs) {
      assert.ok(!c.all.some((k) => OBVIOUS.includes(k.name)), `${c.title}: no obvious keyword anywhere (row or full list): ${names(c.all)}`);
      assert.ok(!c.groups.some((g) => g.startsWith('Mount')), `${c.title}: no mount group`);
      assert.ok(c.row.length <= 5, 'at most 5 chips');
    }
    assert.ok(rider.row.every((c) => c.tab === '0' && c.tip && c.meta && c.label), 'every chip is focusable and has a tooltip, a group and a label');
    assert.ok(rider.row[0].concept, 'a race chip opens a term (its own, else Keywords)');
    assert.ok(rider.groups.includes('Race') && rider.groups.includes('Faction'), `groups: ${rider.groups}`);
    assert.ok(await eowyn.page.$eval('main .unit-slot', (u) => !!u.querySelector('[role=img][aria-label]')), 'the kind icon stays');
    const order = await eowyn.page.$eval('main .unit-slot', (u) => {
      const t = u.querySelector('[data-keywords]'), a = u.querySelector('[aria-label=Wargear]'), b = u.querySelector('.level-medal');
      return [a.compareDocumentPosition(t) & Node.DOCUMENT_POSITION_FOLLOWING, b ? b.compareDocumentPosition(t) & Node.DOCUMENT_POSITION_FOLLOWING : 1];
    });
    assert.ok(order[0] && order[1], 'the row is at the foot of the card: under the wargear and the experience');
  });

  await h.step('keywords: Elf and Dwarf companies show their race first; the row wraps and holds at most five chips then +N', async () => {
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    const el = await cards(galadriel);
    assert.ok(el.length >= 3 && el.every((c) => c.has && c.row[0].cls.includes('kw-race')), `every card starts with its race: ${el.map((c) => names(c.row))}`);
    assert.ok(el.filter((c) => c.row[0].name === 'Elf').length >= 3, 'the Elves start with Elf');
    assert.ok(el.some((c) => c.row.some((x) => x.cls.includes('kw-faction'))), 'factions are shown');
    assert.ok(el.every((c) => c.row.length <= 5));
    await gimli.goto(`/campaigns/${cid}/my-company`);
    const dw = await cards(gimli);
    assert.ok(dw.length >= 3 && dw.every((c) => c.has && c.row[0].cls.includes('kw-race')), `every card starts with its race: ${dw.map((c) => names(c.row))}`);
    assert.ok(dw.filter((c) => c.row[0].name === 'Dwarf').length >= 3 && dw.some((c) => names(c.row).includes('Erebor')), 'the Dwarves start with Dwarf and name Erebor');
  });

  await h.step('keywords: +N (style guide, the same component) opens the full list in place (button, aria-expanded), Fewer closes it', async () => {
    await eowyn.goto('/styleguide');
    const el = (await eowyn.page.$('#sg-keywords [data-keywords]'));
    await el.evaluate((e) => e.scrollIntoView({ block: 'center' }));
    const state = () => el.evaluate((u) => ({ btn: u.querySelector('button.kw-more').getAttribute('aria-expanded'), rowShown: !!u.querySelector('ul[aria-label=Keywords]').offsetParent, fullShown: !!u.querySelector('.kw-full').offsetParent, groups: [...u.querySelectorAll('.kw-full dt')].map((d) => d.textContent.trim()) }));
    let st = await state();
    assert.deepEqual([st.btn, st.rowShown, st.fullShown], ['false', true, false], 'closed at first');
    const url0 = eowyn.page.url(), reqs0 = eowyn.reqCount;
    await el.$eval('button.kw-more', (b) => b.focus());
    await eowyn.page.keyboard.press('Enter');
    await eowyn.settle(200);
    st = await state();
    assert.equal(st.fullShown, true, '+N opens the full list');
    assert.equal(st.rowShown, false, 'in place of the compact row');
    assert.deepEqual(st.groups, ['Race', 'Faction', 'Unit type']);
    assert.equal(eowyn.page.url(), url0, 'no navigation');
    assert.equal(eowyn.reqCount, reqs0, 'and no request');
    await el.$eval('.kw-full button', (b) => b.click());
    await eowyn.settle(150);
    st = await state();
    assert.equal(st.fullShown, false, 'Fewer keywords closes it');
  });

  await h.step('keywords: the expanded card always shows the full list and hides the row', async () => {
    await gimli.goto(`/campaigns/${cid}/my-company`);
    const el = (await gimli.page.$('main .unit-slot'));
    await el.evaluate((e) => e.scrollIntoView({ block: 'center' }));
    await el.$eval('.unit-corner', (b) => b.click());
    await gimli.settle(300);
    const ex = await el.evaluate((u) => ({ fullShown: !!u.querySelector('.kw-full').offsetParent, rowShown: !!u.querySelector('[data-keywords] ul[aria-label=Keywords]').offsetParent, fewer: !!u.querySelector('.kw-full button').offsetParent, groups: [...u.querySelectorAll('.kw-full dt')].map((d) => d.textContent.trim()) }));
    assert.deepEqual([ex.fullShown, ex.rowShown, ex.fewer], [true, false, false], 'the expanded card always shows the full list');
    assert.ok(ex.groups.includes('Race') && ex.groups.includes('Faction'), `groups: ${ex.groups}`);
    await gimli.page.screenshot({ path: 'out/keywords-expanded.png' });
    await el.$eval('.unit-corner', (b) => b.click());
    await gimli.settle(200);
  });

  await h.step('keywords: a chip shows its tooltip on hover and opens the helper popup (its term, or Keywords); Escape closes it and returns the focus', async () => {
    await eowyn.goto(`/campaigns/${cid}/my-company`);
    const chipAt = async (name) => {
      const hs = await eowyn.page.evaluateHandle((name) => [...document.querySelectorAll('main .unit-slot ul[aria-label=Keywords] > li.kw-chip')].find((c) => c.textContent.trim() === name && c.offsetParent), name);
      const e = hs.asElement();
      assert.ok(e, `a ${name} chip`);
      await e.evaluate((x) => x.scrollIntoView({ block: 'center' }));
      return e;
    };
    for (const name of ['Man', 'Rohan']) {
      const term = await (await chipAt(name)).evaluate((c) => c.dataset.concept);
      const chip = await chipAt(name);
      await chip.hover();
      await eowyn.settle(350);
      assert.ok(await tipVisible(eowyn), `hovering ${name} shows the tooltip`);
      await eowyn.page.mouse.move(2, 2);
      await chip.focus();
      await eowyn.page.keyboard.press('Enter');
      await waitPop(eowyn, term);
      assert.ok(await eowyn.page.evaluate(() => !!document.querySelector('.concept-pop')), 'the popup is open');
      await eowyn.page.keyboard.press('Escape');
      await eowyn.page.waitForFunction(() => !document.querySelector('.concept-layer'), { timeout: 4000 });
      const back = await eowyn.page.evaluate((name) => document.activeElement?.textContent.trim() === name && document.activeElement.classList.contains('kw-chip'), name);
      assert.ok(back, `Escape returns the focus to the ${name} chip`);
    }
    // a click opens it too
    const chip = await chipAt('Man');
    await chip.click();
    await waitPop(eowyn, await chip.evaluate((c) => c.dataset.concept));
    await eowyn.page.keyboard.press('Escape');
    await eowyn.page.waitForFunction(() => !document.querySelector('.concept-layer'), { timeout: 4000 });
  });

  await h.step('keywords: another player\'s read-only company page shows the same row and no edit control', async () => {
    await gimli.goto(`/campaigns/${cid}/rankings`);
    const href = await gimli.page.evaluate(() => [...document.querySelectorAll('main tbody tr')].find((r) => r.innerText.includes('Riders on every horse'))?.querySelector('a[href*="/companies/"]')?.getAttribute('href'));
    assert.ok(href, 'a link to the Rohan company');
    await gimli.goto(href);
    const cs = await cards(gimli);
    assert.ok(cs.length >= 3 && cs.every((c) => c.has && c.row[0].name === 'Man'), `Rohan units show Man first: ${cs.map((c) => names(c.row))}`);
        assert.equal(await gimli.page.$$eval('main .unit-slot [data-lock]', (e) => e.length), 0, 'a read-only page has no edit control');
    assert.ok(await gimli.page.$eval('main .unit-slot .kw-chip', (c) => c.dataset.tipText.length > 10), 'the chips carry their tooltip');
  });

  await h.step('keywords: the armoury shows a row on a wanderer (Man) and no obvious keyword on a creature', async () => {
    await pippin.goto(`/campaigns/${cid}/armory`);
    const data = await pippin.page.evaluate(() => {
      const of = (sec) => [...document.querySelectorAll(`section[aria-label=${sec}] > ul > li`)].map((li) => ({ name: li.dataset.name, has: !!li.querySelector('[data-keywords]'), row: [...li.querySelectorAll('[data-keywords] ul[aria-label=Keywords] > li.kw-chip')].map((c) => ({ n: c.textContent.trim(), concept: c.dataset.concept, tip: c.dataset.tipText, tab: c.getAttribute('tabindex') })) }));
      return { wanderers: of('Wanderers'), creatures: of('Creatures') };
    });
    assert.ok(data.wanderers.length >= 1 && data.wanderers.every((w) => w.has), `every wanderer has a row: ${JSON.stringify(data.wanderers.map((w) => w.name))}`);
    assert.ok(data.wanderers.every((w) => w.row[0].n === 'Man' || w.row.length > 0), 'wanderers have chips');
    assert.ok(data.wanderers.some((w) => w.row.some((c) => c.n === 'Man')), 'a wanderer is a Man (race group)');
    assert.ok(data.creatures.length >= 1, 'the armoury lists creatures');
    const types = data.creatures.flatMap((c) => c.row);
    assert.ok(!types.some((c) => /^(Beast|Infantry|Warrior|Monster|Cavalry|War Beast|Mount|Hero|Horse|Man|Elf)$/.test(c.n)), `creatures show nothing the icon or insignia says: ${types.map((c) => c.n)}`);
    // a chip there opens the popup
    const chip = await pippin.page.$('section[aria-label=Wanderers] [data-keywords] ul[aria-label=Keywords] > li.kw-chip');
    await chip.evaluate((e) => e.scrollIntoView({ block: 'center' }));
    await chip.hover();
    await pippin.settle(350);
    assert.ok(await tipVisible(pippin), 'a wanderer chip has the tooltip');
    await pippin.page.mouse.move(2, 2);
    await chip.click();
    await waitPop(pippin, await chip.evaluate((c) => c.dataset.concept));
    await pippin.page.keyboard.press('Escape');
    await pippin.page.waitForFunction(() => !document.querySelector('.concept-layer'), { timeout: 4000 });
  });

  await h.step('keywords: a fallen unit in the cemetery shows its keywords and no mount group', async () => {
    const bid = await campaignId(bilbo, 'Cemetery');
    await bilbo.goto(`/campaigns/${bid}/my-company`);
    const fallen = await cards(bilbo, 'main .unit-fallen');
    assert.ok(fallen.length >= 1, 'the cemetery has fallen units');
    assert.ok(fallen.every((c) => c.has && c.row.length >= 1), `fallen cards have a row: ${fallen.map((c) => names(c.row))}`);
    assert.ok(fallen.every((c) => !c.groups.some((g) => g.startsWith('Mount'))), 'no mount group');
  });

  await h.step('keywords: the style guide shows the chip row in its states, axe clean at 360/768/1280 in both themes', async () => {
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(eowyn, w, scheme);
      await eowyn.goto('/styleguide');
      assert.ok(await eowyn.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      const sg = await eowyn.page.$eval('#sg-keywords', (s) => ({ rows: s.querySelectorAll('[data-keywords]').length, off: s.querySelectorAll('.kw-off').length, more: s.querySelectorAll('button.kw-more').length, race: s.querySelectorAll('.kw-race').length, faction: s.querySelectorAll('.kw-faction').length, type: s.querySelectorAll('.kw-type').length }));
      const first = await eowyn.page.$eval('#sg-keywords [data-keywords]', (k) => ({ n: k.querySelectorAll('ul[aria-label=Keywords] > li.kw-chip').length, more: k.querySelector('button.kw-more').innerText.trim() }));
      assert.equal(first.n, 5, 'five chips then +N');
      assert.match(first.more, /^\+2/, 'the rest is counted');
      assert.ok(sg.rows === 4 && sg.more >= 1 && sg.race && sg.faction && sg.type, JSON.stringify(sg));
      const bad = await axeOn(eowyn, '#sg-keywords');
      assert.equal(bad.length, 0, `axe: ${bad.join('; ')}`);
      await (await eowyn.page.$('#sg-keywords')).screenshot({ path: `out/keywords-styleguide-${w}-${scheme}.png` });
    }
    await layout(eowyn, 1280, 'light');
  });

  for (const scheme of ['light', 'dark']) for (const w of SIZES) {
    await h.step(`keywords: the cards at ${w} px, ${scheme}: no horizontal scroll, axe clean, screenshot`, async () => {
      for (const [who, s, name] of [['rohan', eowyn, 'Limits, creatures'], ['dwarf', gimli, 'Limits, creatures'], ['elf', galadriel, 'Limits, creatures']]) {
        await layout(s, w, scheme);
        await s.goto(`/campaigns/${cid}/my-company`);
        assert.ok(await s.noHScroll(), `no horizontal scroll: ${who} at ${w}px ${scheme}`);
        const bad = await axeOn(s, 'main .unit-slot');
        assert.equal(bad.length, 0, `axe (${who}): ${bad.join('; ')}`);
        // chips wrap inside the card at every width
        const overflow = await s.page.$$eval('main .unit-slot [data-keywords]', (k) => k.filter((e) => e.scrollWidth > e.clientWidth + 1 || e.getBoundingClientRect().right > innerWidth).length);
        assert.equal(overflow, 0, 'the chips wrap inside the card');
        const card = await s.page.$('main .unit-slot');
        await card.screenshot({ path: `out/keywords-${who}-${w}-${scheme}.png` });
      }
      await layout(pippin, w, scheme);
      await pippin.goto(`/campaigns/${cid}/armory`);
      assert.ok(await pippin.noHScroll(), `no horizontal scroll: armoury at ${w}px ${scheme}`);
      const bad = await axeOn(pippin, 'section[aria-label=Wanderers]');
      assert.equal(bad.length, 0, `axe (armoury): ${bad.join('; ')}`);
      await (await pippin.page.$('section[aria-label=Wanderers] > ul > li')).screenshot({ path: `out/keywords-wanderer-${w}-${scheme}.png` });
    });
  }
  for (const s of [eowyn, galadriel, gimli, pippin]) await layout(s, 1280, 'light');
}
