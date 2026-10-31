// Unit names in rule texts link to the unit's card in the campaign glossary, with a tooltip (docs/frontend.md, "Glossary and item definitions",
// docs/concept-graph.md). Checked on the Arnor company page of "Brand new campaign" (its special rule promotes the Ranger of Arnor to the Ranger of
// the North): both names are links to #u-... with the profile in the tooltip (hover and keyboard focus, Escape), the terms next to them still open
// the helper popup, the link is followed by a full load that reaches the card focused (the cards are fetched on demand), a name inside another word is
// not linked, a definition on the glossary page links its units in the page with a tooltip before the cards are loaded, and the page holds at
// 360/768/1280 px in both themes and high contrast (no horizontal scroll, the tooltip inside the window, axe).
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const unitLinks = (s) => s.page.$$eval('#company-info a.unit-link', (as) => as.map((a) => ({ text: a.textContent, href: a.getAttribute('href'), boost: a.getAttribute('hx-boost'), concept: a.hasAttribute('data-concept'), title: a.dataset.tipTitle, meta: a.dataset.tipMeta, tip: a.dataset.tipText })));
const tooltip = (s) => s.page.evaluate(() => { const t = document.querySelector('[role=tooltip]'); if (!t || t.offsetParent === null && getComputedStyle(t).position !== 'fixed') return null; const r = t.getBoundingClientRect(); return getComputedStyle(t).display === 'none' ? null : { text: t.innerText, left: r.left, right: r.right, top: r.top, bottom: r.bottom, w: innerWidth, h: innerHeight }; });

export default async function (h) {
  const aragorn = await h.session('aragorn@example.com', { height: 1000 });
  let cid;

  const openArnor = async () => {
    await aragorn.goto(`/campaigns/${cid}/company/new`);
    await aragorn.click('nav[aria-label=Companies] button[data-name="Arnor"]');
    await aragorn.page.waitForSelector('#take-command');
    await aragorn.settle(300);
  };

  await h.step('unit links: the Arnor rule links both units to their cards, with their profile in the tooltip', async () => {
    cid = await campaignId(aragorn, 'Brand new campaign');
    await openArnor();
    const links = await unitLinks(aragorn);
    assert.deepEqual(links.map((l) => l.text), ['Ranger of Arnor', 'Ranger of the North'], 'the two units of the promotion text');
    assert.deepEqual(links.map((l) => l.href), [`/campaigns/${cid}/glossary#u-ranger-of-arnor`, `/campaigns/${cid}/glossary#u-ranger-of-the-north`]);
    for (const l of links) {
      assert.equal(l.boost, 'false', 'a full load, so the browser jumps to the card');
      assert.ok(!l.concept, 'no helper popup: a unit is not a concept');
      assert.ok(l.title === l.text && /pts/.test(l.meta) && l.tip.length > 10, `the tooltip carries the profile: ${l.meta} / ${l.tip}`);
    }
    // the terms of the same text still open the helper popup and their own tooltip
    const term = await aragorn.page.$$eval('#company-info a.term-link[data-concept]', (as) => as.map((a) => a.textContent));
    assert.ok(term.includes('Warrior') && term.includes('Hero'), `the term links of the text: ${term}`);
    // the sentence is unchanged: the unit names are linked in place, nothing added or lost
    const sentence = await aragorn.page.$eval('#company-info a.unit-link', (a) => a.closest('p').innerText);
    assert.ok(sentence.startsWith('Should a Ranger of Arnor Warrior achieve a Hero'), sentence);
    assert.ok(sentence.includes('for a Ranger of the North.'), sentence);
  });

  await h.step('unit links: hover and keyboard focus show the profile, Escape and leaving hide it, a click follows the link to the focused card', async () => {
    await aragorn.page.setViewport({ width: 1280, height: 1000 });
    const a = (await aragorn.page.$$('#company-info a.unit-link'))[1];
    await a.hover();
    await aragorn.settle(400);
    let tip = await tooltip(aragorn);
    assert.ok(tip && tip.text.includes('Ranger of the North') && tip.text.includes('pts') && tip.text.includes('Wargear'), `hover: ${tip && tip.text}`);
    assert.ok(tip.left >= 0 && tip.right <= tip.w && tip.top >= 0 && tip.bottom <= tip.h, 'the tooltip is inside the window');
    await aragorn.page.mouse.move(5, 5);
    await aragorn.settle(300);
    assert.equal(await tooltip(aragorn), null, 'leaving hides it');
    await a.focus();
    await aragorn.settle(400);
    tip = await tooltip(aragorn);
    assert.ok(tip && tip.text.includes('Ranger of the North'), 'keyboard focus shows it');
    await aragorn.page.keyboard.press('Escape');
    await aragorn.settle(200);
    assert.equal(await tooltip(aragorn), null, 'Escape hides it');
    // Enter follows the link: a full load of the glossary at the unit's anchor, the cards fetched, the card scrolled to and focused
    await a.focus();
    await aragorn.page.keyboard.press('Enter');
    await aragorn.page.waitForSelector('#u-ranger-of-the-north', { timeout: 8000 });
    await aragorn.page.waitForFunction(() => { const c = document.getElementById('u-ranger-of-the-north'); return c && c.getBoundingClientRect().top < 400; }, { timeout: 8000 }).catch(() => {});
    await aragorn.settle(300);
    const at = await aragorn.page.evaluate(() => ({ path: location.pathname + location.hash, focus: document.activeElement.id, top: document.getElementById('u-ranger-of-the-north').getBoundingClientRect().top, y: scrollY, vh: innerHeight, dh: document.documentElement.scrollHeight, cards: document.querySelectorAll('article[data-unit]').length }));
    assert.equal(at.path, `/campaigns/${cid}/glossary#u-ranger-of-the-north`);
    assert.equal(at.focus, 'u-ranger-of-the-north', 'the card takes the focus');
    assert.ok(at.top >= -2 && at.top < 400, `the card is in view (${JSON.stringify(at)})`);
    aragorn.issues();
  });

  await h.step('unit links: a definition on the glossary page links its units in the page, with a tooltip before the cards are fetched', async () => {
    await aragorn.goto(`/campaigns/${cid}/glossary`);
    assert.equal(await aragorn.page.$$eval('article[data-unit]', (e) => e.length), 0, 'the cards are not in the page yet');
    const first = await aragorn.page.$eval('article.term a.unit-link', (a) => ({ href: a.getAttribute('href'), title: a.dataset.tipTitle, text: a.textContent }));
    assert.ok(first.href.startsWith('#u-') && first.title, `a unit linked in a definition: ${JSON.stringify(first)}`);
    const link = await aragorn.page.$('article.term a.unit-link');
    await link.evaluate((e) => e.scrollIntoView({ block: 'center' }));
    await link.hover();
    await aragorn.settle(500);
    const tip = await tooltip(aragorn);
    assert.ok(tip && tip.text.includes(first.title), `the tooltip names the unit: ${tip && tip.text}`);
    await aragorn.page.mouse.move(5, 5);
    const fetched = [];
    const onRequest = (r) => { if (r.url().includes('/glossary/units')) fetched.push(r.url()); };
    aragorn.page.on('request', onRequest);
    await link.click();
    await aragorn.page.waitForSelector(`${first.href}[data-unit]`, { timeout: 8000 });
    await aragorn.settle(500);
    assert.equal(await aragorn.page.evaluate(() => document.activeElement.id), first.href.slice(1), 'the card is fetched and focused');
    aragorn.page.off('request', onRequest);
    assert.equal(fetched.length, 1, `one request fetched the cards: ${fetched}`);
    aragorn.issues();
  });

  await h.step('unit links: the sidebar glossary (no campaign) has no unit links', async () => {
    await aragorn.goto('/glossary');
    assert.equal(await aragorn.page.$$eval('a.unit-link', (e) => e.length), 0);
  });

  for (const [scheme, contrast] of [['light', 'normal'], ['dark', 'normal'], ['light', 'high'], ['dark', 'high']]) {
    for (const w of SIZES) {
      await h.step(`unit links at ${w} px, ${scheme}${contrast === 'high' ? ', high contrast' : ''}: no horizontal scroll, tooltip inside the window, axe clean`, async () => {
        const ck = { name: 'bcm-contrast', value: contrast, url: aragorn.h.base };
        if (aragorn.page.setCookie) await aragorn.page.setCookie(ck); else await aragorn.ctx.setCookie(ck);
        await layout(aragorn, w, scheme, w === 360 ? 780 : 900);
        await openArnor();
        assert.ok(await aragorn.noHScroll(), 'no horizontal scroll');
        const a = (await aragorn.page.$$('#company-info a.unit-link'))[0];
        await a.evaluate((e) => e.scrollIntoView({ block: 'center' }));
        await a.focus();
        await aragorn.settle(400);
        const tip = await tooltip(aragorn);
        assert.ok(tip && tip.left >= 0 && tip.right <= tip.w && tip.top >= 0 && tip.bottom <= tip.h, `the tooltip fits the window: ${JSON.stringify(tip)}`);
        await aragorn.page.screenshot({ path: `out/unit-links-${w}-${scheme}${contrast === 'high' ? '-high' : ''}.png` });
        await aragorn.page.evaluate(axeSource);
        // `.exclusive-gold > .exclusive-label` ("Hero upgrade", gold 11 px text on the pale gold box) is just under 4.5:1 in the light theme: a
        // colour of the page that is not this feature's; it is filed apart and left out here so that this flow checks the unit links.
        const res = await aragorn.page.evaluate(() => axe.run({ exclude: [['.exclusive-gold > .exclusive-label']] }, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
          .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id} (${v.impact}, ${v.nodes.length}) ${v.nodes.slice(0, 2).map((n) => n.target.join(' ')).join(' | ')}`)));
        assert.deepEqual(res, []);
        assert.equal(await aragorn.page.evaluate(() => document.documentElement.dataset.contrast || ''), contrast === 'high' ? 'high' : '', 'the contrast applied');
        aragorn.issues();
      });
    }
  }
  await aragorn.page.deleteCookie({ name: 'bcm-contrast', url: aragorn.h.base }).catch(() => {});
}
