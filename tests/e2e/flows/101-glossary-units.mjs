// The campaign Glossary tab holds a card for every base unit the companies of the ruleset name (docs/frontend.md, "Glossary and item
// definitions"). The cards are not in the page (they are half a megabyte): the page says how many there are and fetches them the first
// time they are wanted (the button, the Units chip, a search, a #u- address). Checked here: the count equals the cards, each card has a stable
// anchor (#u-warrior-of-minas-tirith) that a full load and a link inside the page both reach (the card is fetched, shown even when a search hid
// it, scrolled to and focused), the Units chip and the search filter them, keyboard reachable, and the page holds at 360/768/1280 px in both
// themes (no horizontal scroll, axe, screenshots out/glossary-units-*.png).
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
// axe over the cards and the filter row only: on the whole tab (1,400 terms, 1.5 MB) it takes 20 s locally and passed the 30 s protocol
// timeout on a slow CI runner; 60-pages already runs it over the whole page.
const AXE_SCOPE = ['#glossary-units', '[aria-label="Filter by category"]'];
const runAxe = async (s, scope = AXE_SCOPE) => {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((include) => axe.run({ include: include.map((x) => [x]) }, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id} (${v.impact}, ${v.nodes.length}) ${v.nodes.slice(0, 2).map((n) => n.target.join(' ')).join(' | ')}`)), scope);
};
const cards = (s) => s.page.evaluate(() => [...document.querySelectorAll('article[data-unit]')].length);
const shownUnits = (s) => s.page.evaluate(() => [...document.querySelectorAll('article[data-unit]')].filter((a) => a.offsetParent !== null).map((a) => a.dataset.name));

export default async function (h) {
  const admin = await h.session('admin@example.com', { height: 1000 });
  let gl, count;

  await h.step('glossary units: the page counts the units and fetches the cards when the button is pressed', async () => {
    gl = `/campaigns/${await campaignId(admin, 'Matches: every scenario')}/glossary`;
    await admin.goto(gl);
    assert.equal(await cards(admin), 0, 'the cards are not in the page');
    const btn = await admin.page.$eval('#glossary-units button', (b) => b.innerText);
    count = Number(/Show the (\d+) unit cards/.exec(btn)?.[1]);
    assert.ok(count >= 100, `the button counts the units: ${btn}`);
    assert.ok((await admin.text()).includes(`${count} units`), 'the header counts them too');
    const reqs = admin.reqCount;
    await admin.page.click('#glossary-units button');
    await admin.page.waitForSelector('article[data-unit]', { timeout: 8000 });
    await admin.settle(300);
    assert.equal(admin.reqCount - reqs, 1, 'one request fetched them');
    const info = await admin.page.evaluate(() => {
      const cs = [...document.querySelectorAll('article[data-unit]')];
      return { n: cs.length, ids: cs.map((c) => c.id), pending: !!document.querySelector('[data-units-pending]'), bad: cs.filter((c) => !/^u-[a-z0-9]+(-[a-z0-9]+)*$/.test(c.id) || !c.querySelector('h3') || !(c.querySelector('dl') || c.innerText.includes('Made of'))).map((c) => c.id) };
    });
    assert.equal(info.n, count, 'a card for every unit the page counted');
    assert.ok(!info.pending, 'the placeholder is gone');
    assert.equal(new Set(info.ids).size, info.n, 'anchors are unique');
    assert.deepEqual(info.bad, [], 'every card has a plain anchor, a name and a profile');
    assert.ok(info.ids.includes('u-warrior-of-minas-tirith'), 'the Warrior of Minas Tirith has its card');
    const card = await admin.page.$eval('#u-warrior-of-minas-tirith', (c) => c.innerText);
    for (const t of ['Warrior of Minas Tirith', 'pts', 'Wargear', 'Named by']) assert.ok(card.includes(t), `the card says "${t}": ${card}`);
  });

  await h.step('glossary units: the anchor in the address fetches and reaches the card, shown and focused, even after a search', async () => {
    await admin.goto(`${gl}#u-warrior-of-minas-tirith`);
    await admin.page.waitForSelector('#u-warrior-of-minas-tirith', { timeout: 8000 });
    await admin.settle(600);
    let at = await admin.page.evaluate(() => ({ top: document.getElementById('u-warrior-of-minas-tirith').getBoundingClientRect().top, focus: document.activeElement.id }));
    assert.ok(at.top >= -2 && at.top < 400, `scrolled into view (top ${at.top})`);
    assert.equal(at.focus, 'u-warrior-of-minas-tirith', 'the card takes the focus');
    await admin.fill('#glossary-search', 'zzzz-nothing');
    await admin.settle(150);
    assert.ok(!(await admin.page.$eval('#u-warrior-of-minas-tirith', (c) => c.offsetParent !== null)), 'the search hid the card');
    await admin.page.evaluate(() => { location.hash = '#u-cave-troll'; });
    await admin.settle(500);
    at = await admin.page.evaluate(() => ({ shown: document.getElementById('u-cave-troll').offsetParent !== null, q: document.getElementById('glossary-search').value }));
    assert.ok(at.shown && at.q === '', 'the card is shown and the search cleared');
  });

  await h.step('glossary units: a search or the Units chip fetches them; a company finds its units; ?c=units opens on them', async () => {
    await admin.goto(gl);
    assert.equal(await cards(admin), 0);
    await admin.fill('#glossary-search', 'Cave Troll');
    await admin.page.waitForSelector('article[data-unit]', { timeout: 8000 });
    await admin.settle(300);
    assert.ok((await shownUnits(admin)).includes('Cave Troll'), 'a search finds a unit by name');
    await admin.goto(gl);
    await admin.page.click('[data-chip="units"]');
    await admin.page.waitForSelector('article[data-unit]', { timeout: 8000 });
    await admin.settle(300);
    const v = await admin.page.evaluate(() => ({ terms: [...document.querySelectorAll('article.term:not([data-unit])')].filter((a) => a.offsetParent !== null).length, url: location.search, empty: [...document.querySelectorAll('[role=status]')].some((e) => e.offsetParent !== null && e.innerText.includes('No term matches')) }));
    assert.equal(v.terms, 0, 'only units show');
    assert.equal(v.url, '?c=units');
    assert.ok(!v.empty, 'no "no term matches" notice');
    assert.equal((await shownUnits(admin)).length, count);
    await admin.fill('#glossary-search', 'Minas Tirith');
    await admin.settle(200);
    const names = await shownUnits(admin);
    assert.ok(names.length > 0 && names.length < 20 && names.includes('Warrior of Minas Tirith'), names.join(', '));
    await admin.goto(`${gl}?c=units`);
    await admin.page.waitForSelector('article[data-unit]', { timeout: 8000 });
    assert.equal(await admin.page.evaluate(() => document.querySelector('[data-chip="units"]').getAttribute('aria-pressed')), 'true');
  });

  await h.step('glossary units: keyboard (the button, the chip and the search are reachable; a link to a card is followed with Enter)', async () => {
    await admin.goto(gl);
    await admin.page.focus('#glossary-search');
    await admin.page.keyboard.press('Tab');
    assert.ok(await admin.page.evaluate(() => document.activeElement.getAttribute('aria-pressed') !== null), 'Tab reaches the category chips');
    await admin.page.focus('#glossary-units button');
    await admin.page.keyboard.press('Enter');
    await admin.page.waitForSelector('article[data-unit]', { timeout: 8000 });
    await admin.settle(300);
    // an in-page link to a unit card (what the next step of the roadmap adds to unit references) is handled like a term link
    await admin.page.evaluate(() => {
      const a = document.createElement('a'); a.href = '#u-cave-troll'; a.id = 'e2e-unit-link'; a.textContent = 'Cave Troll';
      document.querySelector('[x-data="glossary"]').prepend(a);
    });
    await admin.page.focus('#e2e-unit-link');
    await admin.page.keyboard.press('Enter');
    await admin.settle(500);
    const at = await admin.page.evaluate(() => ({ focus: document.activeElement.id, hash: location.hash }));
    assert.deepEqual(at, { focus: 'u-cave-troll', hash: '#u-cave-troll' });
  });

  for (const [scheme, contrast] of [['light', 'normal'], ['dark', 'normal'], ['light', 'high'], ['dark', 'high']]) {
    for (const w of SIZES) {
      await h.step(`glossary units at ${w} px, ${scheme}${contrast === 'high' ? ', high contrast' : ''}: no horizontal scroll, cards fit${w !== 768 && (contrast === 'normal' || scheme === 'dark') ? ', axe clean' : ''}`, async () => {
        const ck = { name: 'bcm-contrast', value: contrast, url: admin.h.base };
        if (admin.page.setCookie) await admin.page.setCookie(ck); else await admin.ctx.setCookie(ck);
        await layout(admin, w, scheme, w === 360 ? 780 : 900);
        await admin.goto(`${gl}?c=units#u-warrior-of-minas-tirith`);
        await admin.page.waitForSelector('#u-warrior-of-minas-tirith', { timeout: 8000 });
        await admin.settle(500);
        assert.ok(await admin.noHScroll(), 'no horizontal scroll');
        const over = await admin.page.evaluate(() => [...document.querySelectorAll('article[data-unit]')].filter((a) => a.offsetParent !== null && a.scrollWidth > a.clientWidth + 1).map((a) => a.id));
        assert.deepEqual(over, [], 'no card overflows');
        await admin.page.screenshot({ path: `out/glossary-units-${w}-${scheme}${contrast === 'high' ? '-high' : ''}.png` });
        // layout checks in every look; axe at 360 and 1280 in light, dark and the dark high-contrast look
        if (w !== 768 && (contrast === 'normal' || scheme === 'dark')) assert.deepEqual(await runAxe(admin), []);
        assert.equal(await admin.page.evaluate(() => document.documentElement.dataset.contrast || ''), contrast === 'high' ? 'high' : '', 'the contrast applied');
        admin.issues();
      });
    }
  }
  await admin.page.deleteCookie({ name: 'bcm-contrast', url: admin.h.base }).catch(() => {});

  await h.step('glossary units: the placeholder at 360 px (the first screen of the tab), light and dark, axe clean', async () => {
    for (const scheme of ['light', 'dark']) {
      await layout(admin, 360, scheme, 780);
      await admin.goto(gl);
      await admin.page.$eval('#glossary-units', (e) => e.scrollIntoView());
      await admin.settle(200);
      assert.ok(await admin.noHScroll(), 'no horizontal scroll');
      await admin.page.screenshot({ path: `out/glossary-units-placeholder-360-${scheme}.png` });
      const res = await runAxe(admin);
      assert.deepEqual(res, []);
    }
    admin.issues();
  });
}
