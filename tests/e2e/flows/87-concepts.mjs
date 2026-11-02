// The helper popup (docs/concept-graph.md, static/js/concepts.js): a term on My Company opens a sheet with its text and linked
// concepts, a chip navigates, Back returns, Escape closes and gives the focus back; the shield's cancels are grouped; an unknown id
// says so; a name the graph does not know falls back to the tooltip's text; the sheet fits 360/768/1280 px in both themes and passes axe.
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const POP = '.concept-pop';

const title = (s) => s.page.evaluate(() => document.querySelector('.concept-pop #concept-title')?.textContent.trim() || null);
const waitTitle = (s, t) => s.page.waitForFunction((t) => {
  const p = document.querySelector('.concept-pop'), h = p && p.querySelector('#concept-title');
  return h && p.getAttribute('aria-busy') !== 'true' && (!t || h.textContent.trim() === t);
}, { timeout: 8000 }, t).catch(async () => { throw new Error(`the popup did not show "${t}" (shows ${await title(s)})`); });
const popGone = (s) => s.page.waitForFunction(() => !document.querySelector('.concept-layer'), { timeout: 4000 });
const tipVisible = (s) => s.page.evaluate(() => [...document.querySelectorAll('[role=tooltip]')].some((t) => { const r = t.getBoundingClientRect(); return r.width && r.height && getComputedStyle(t).display !== 'none'; }));
const chipOf = (s, name) => s.page.$(`main [data-concept][data-tip-title="${name}"]`);

// A stand-in term in the page for a concept of the rules (the same markup the unit cards use), so a check does not depend on who owns what.
async function plant(s, name, text = 'tooltip text') {
  await s.page.evaluate((name, text) => {
    document.querySelector('#e2e-term')?.remove();
    const e = document.createElement('span');
    e.id = 'e2e-term'; e.tabIndex = 0; e.className = 'rule-chip';
    e.setAttribute('data-concept', ''); e.dataset.tipTitle = name; e.dataset.tipMeta = 'Special rule'; e.dataset.tipText = text; e.textContent = name;
    document.querySelector('main').prepend(e);
  }, name, text);
}

async function axeOnPopup(s) {
  await s.page.evaluate(axeSource);
  return s.page.evaluate(() => axe.run('.concept-layer', {
    runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'],
  }).then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id}: ${v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' ' + (x.any[0]?.message || '')).join(' | ')}`)));
}

export default async function (h) {
  const galadriel = await h.session('galadriel@example.com');
  let cid;

  await h.step('concepts: a chip opens the popup with its text, groups and the glossary link; the tooltip steps aside', async () => {
    cid = await campaignId(galadriel, 'Aftermath ward 1');
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    const chip = await chipOf(galadriel, 'Elven Cloak');
    assert.ok(chip, 'a unit wears an Elven Cloak and its chip is a popup trigger');
    await chip.evaluate((e) => e.scrollIntoView({ block: 'center' }));
    await chip.hover();
    await galadriel.settle(350);
    assert.ok(await tipVisible(galadriel), 'hovering still shows the tooltip');
    await chip.click();
    await waitTitle(galadriel, 'Elven Cloak');
    assert.ok(!(await tipVisible(galadriel)), 'the tooltip is gone once the popup is open');
    const info = await galadriel.page.evaluate(() => {
      const p = document.querySelector('.concept-pop');
      return {
        role: p.getAttribute('role'), modal: p.getAttribute('aria-modal'), label: p.getAttribute('aria-label'),
        groups: [...p.querySelectorAll('.concept-group')].map((g) => g.getAttribute('aria-label')),
        grants: [...p.querySelectorAll('[aria-label="Grants"] .concept-chip')].map((c) => c.textContent),
        glossary: p.querySelector('a[href*="/glossary#t-"]')?.getAttribute('href') || null,
        focus: document.activeElement?.id, back: p.querySelector('[data-concept-back]').hidden,
      };
    });
    assert.equal(info.role, 'dialog'); assert.equal(info.modal, 'true'); assert.equal(info.label, 'Elven Cloak');
    assert.ok(info.grants.some((g) => g.includes('Stalk Unseen')), `Elven Cloak grants Stalk Unseen: ${info.grants}`);
    assert.match(info.glossary || '', new RegExp(`^/campaigns/${cid}/glossary#t-`), 'Open in glossary');
    assert.equal(info.focus, 'concept-title', 'the focus moves into the popup');
    assert.ok(info.back, 'nothing to go back to yet');
    assert.match(new URL(galadriel.page.url()).pathname, new RegExp(`^/campaigns/${cid}/my-company/\\d+$`), 'the page did not navigate');
  });

  await h.step('concepts: a chip swaps the content, Back returns, Escape closes and the focus goes back to the term', async () => {
    await galadriel.page.click(`${POP} [aria-label="Grants"] .concept-chip`);
    await waitTitle(galadriel, 'Stalk Unseen');
    assert.equal(await galadriel.page.$eval(`${POP} [data-concept-back]`, (b) => b.hidden), false, 'Back is offered after a step');
    await galadriel.page.click(`${POP} [data-concept-back]`);
    await waitTitle(galadriel, 'Elven Cloak');
    assert.equal(await galadriel.page.$eval(`${POP} [data-concept-back]`, (b) => b.hidden), true, 'back at the start');
    await galadriel.page.keyboard.press('Escape');
    await popGone(galadriel);
    assert.ok(await galadriel.page.evaluate(() => document.activeElement?.dataset?.tipTitle === 'Elven Cloak'), 'the focus is back on the chip');
    assert.equal(await galadriel.page.evaluate(() => document.documentElement.classList.contains('concept-open')), false, 'the page scrolls again');
  });

  await h.step('concepts: the backdrop and the Close button close it; a second open starts fresh', async () => {
    const chip = await chipOf(galadriel, 'Elven Cloak');
    await chip.click();
    await waitTitle(galadriel, 'Elven Cloak');
    await galadriel.page.mouse.click(5, 450); // the backdrop, away from the card
    await popGone(galadriel);
    await chip.click();
    await waitTitle(galadriel, 'Elven Cloak');
    await galadriel.page.click(`${POP} [data-concept-close]`);
    await popGone(galadriel);
  });

  await h.step('concepts: the explicit shield cancels are grouped into closed lines with a count', async () => {
    await plant(galadriel, 'Shields');
    await galadriel.page.click('#e2e-term');
    await waitTitle(galadriel, 'Shields');
    const g = await galadriel.page.evaluate(() => {
      const sec = document.querySelector('.concept-pop [aria-label="Cancelled by"]');
      if (!sec) return null;
      return {
        folds: [...sec.querySelectorAll('details.concept-fold')].map((d) => ({ label: d.querySelector('summary').textContent.replace(/\s+/g, ' ').trim(), open: d.open, n: d.querySelectorAll('.concept-chip').length })),
        loose: sec.querySelectorAll(':scope > ul .concept-chip').length,
      };
    });
    assert.ok(g, 'Shields has a "Cancelled by" group');
    const bow = g.folds.find((f) => /bow weapon/.test(f.label));
    assert.ok(bow && /\(\d+\)$/.test(bow.label) && bow.n >= 5 && !bow.open, `the bows fold into one closed line: ${JSON.stringify(g)}`);
    assert.ok(g.folds.length >= 2, 'two-handed weapons have their own line');
    await galadriel.page.click(`${POP} details.concept-fold summary`);
    await galadriel.settle(100);
    assert.ok(await galadriel.page.evaluate(() => document.querySelector('.concept-pop details.concept-fold').open), 'the line opens');
    await galadriel.page.screenshot({ path: 'out/concept-cancels-grouped.png' });
    await galadriel.page.keyboard.press('Escape');
    await popGone(galadriel);
  });

  await h.step('concepts: a concept that is gone says it is no longer in these rules; a name the graph lacks falls back to the tooltip text', async () => {
    await plant(galadriel, 'Elven Cloak');
    await galadriel.page.click('#e2e-term');
    await waitTitle(galadriel, 'Elven Cloak');
    await galadriel.page.evaluate(() => {
      const b = document.createElement('button');
      b.type = 'button'; b.id = 'ghost'; b.className = 'concept-chip'; b.dataset.conceptGo = 'item:no-such-thing'; b.textContent = 'ghost';
      document.querySelector('.concept-body').append(b);
    });
    await galadriel.page.click('#ghost');
    await waitTitle(galadriel, 'Not found');
    assert.match(await galadriel.page.$eval(`${POP} .concept-body`, (b) => b.innerText), /no longer in these rules/);
    await galadriel.page.click(`${POP} [data-concept-back]`);
    await waitTitle(galadriel, 'Elven Cloak');
    await galadriel.page.keyboard.press('Escape');
    await popGone(galadriel);
    await plant(galadriel, 'Nothing like this in any rules book', 'The tooltip says this.');
    await galadriel.page.click('#e2e-term');
    await galadriel.page.waitForFunction(() => document.querySelector('.concept-pop .concept-text')?.textContent.includes('The tooltip says this.'), { timeout: 8000 });
    assert.equal(await title(galadriel), 'Nothing like this in any rules book');
    await galadriel.page.keyboard.press('Escape');
    await popGone(galadriel);
    galadriel.issues(); // the 404 of the lookup is the expected answer
  });

  await h.step('concepts: the keyboard opens it with Enter, Tab stays inside, Escape closes and returns the focus', async () => {
    await plant(galadriel, 'Stalk Unseen');
    await galadriel.page.focus('#e2e-term');
    await galadriel.page.keyboard.press('Enter');
    await waitTitle(galadriel, 'Stalk Unseen');
    for (let i = 0; i < 14; i++) {
      await galadriel.page.keyboard.press('Tab');
      assert.ok(await galadriel.page.evaluate(() => !!document.activeElement.closest('.concept-pop')), `Tab ${i + 1} stays in the popup`);
    }
    for (let i = 0; i < 14; i++) {
      await galadriel.page.keyboard.down('Shift'); await galadriel.page.keyboard.press('Tab'); await galadriel.page.keyboard.up('Shift');
      assert.ok(await galadriel.page.evaluate(() => !!document.activeElement.closest('.concept-pop')), `Shift+Tab ${i + 1} stays in the popup`);
    }
    // a chip is reached and pressed with the keyboard too
    const reached = await galadriel.page.evaluate(() => { const c = document.querySelector('.concept-pop .concept-chip, .concept-pop .concept-term'); if (!c) return false; c.focus(); return true; });
    if (reached) {
      const before = await title(galadriel);
      await galadriel.page.keyboard.press('Enter');
      await galadriel.page.waitForFunction((t) => { const p = document.querySelector('.concept-pop'), h = p.querySelector('#concept-title'); return h && p.getAttribute('aria-busy') !== 'true' && h.textContent.trim() !== t; }, { timeout: 8000 }, before);
      assert.equal(await galadriel.page.evaluate(() => document.activeElement?.id), 'concept-title', 'the focus follows to the new heading');
    }
    await galadriel.page.keyboard.press('Escape');
    await popGone(galadriel);
    assert.equal(await galadriel.page.evaluate(() => document.activeElement?.id), 'e2e-term', 'the focus is back on the term');
  });

  for (const scheme of ['light', 'dark']) for (const w of SIZES) {
    await h.step(`concepts: the popup at ${w} px, ${scheme}: sheet or card, inside the window, no horizontal scroll, axe clean`, async () => {
      await layout(galadriel, w, scheme, 780);
      await plant(galadriel, 'Shields');
      await galadriel.page.$eval('#e2e-term', (e) => e.scrollIntoView({ block: 'center' }));
      await galadriel.page.click('#e2e-term');
      await waitTitle(galadriel, 'Shields');
      await galadriel.settle(250);
      const m = await galadriel.page.evaluate(() => {
        const r = document.querySelector('.concept-pop').getBoundingClientRect();
        const bs = getComputedStyle(document.querySelector('.concept-pop')).backgroundColor.match(/\d+/g).map(Number);
        return { l: r.left, r: r.right, t: r.top, b: r.bottom, w: r.width, h: r.height, iw: innerWidth, ih: innerHeight, bg: (bs[0] + bs[1] + bs[2]) / 3, scroll: document.querySelector('.concept-body').scrollWidth <= document.querySelector('.concept-body').clientWidth + 1 };
      });
      assert.ok(m.l >= -1 && m.r <= m.iw + 1 && m.t >= -1 && m.b <= m.ih + 1, `inside the window: ${JSON.stringify(m)}`);
      assert.ok(m.scroll, 'no sideways scroll inside the popup');
      assert.ok(await galadriel.noHScroll(), 'the page has no horizontal scroll');
      assert.equal(m.bg < 128, scheme === 'dark', `the popup is ${scheme}: ${m.bg}`);
      if (w < 768) assert.ok(Math.abs(m.w - m.iw) <= 1 && Math.abs(m.b - m.ih) <= 1 && m.h <= m.ih * 0.7 + 1, `a bottom sheet at ${w}: ${JSON.stringify(m)}`);
      else assert.ok(m.w < 420 && m.w > 300, `a card at ${w}: ${JSON.stringify(m)}`);
      const bad = await axeOnPopup(galadriel);
      assert.equal(bad.length, 0, 'axe: ' + bad.join('; '));
      await galadriel.page.screenshot({ path: `out/concept-popup-${w}-${scheme}.png` });
      await galadriel.page.keyboard.press('Escape');
      await popGone(galadriel);
    });
  }
  await layout(galadriel, 1280, 'light');

  await h.step('concepts: the armoury and a campaign page without a popup keep working (terms there open it too, links still go to the glossary)', async () => {
    await galadriel.goto(`/campaigns/${cid}/armory`);
    const th = await galadriel.page.evaluateHandle(() => [...document.querySelectorAll('main [data-concept][data-tip-title]')].find((a) => { const r = a.getBoundingClientRect(); return r.width && r.height && getComputedStyle(a).visibility !== 'hidden'; }) || null);
    const t = th.asElement();
    assert.ok(t, 'the armoury has terms that open the popup');
    await t.evaluate((e) => e.scrollIntoView({ block: 'center' }));
    await t.click();
    await galadriel.page.waitForSelector('.concept-pop #concept-title');
    await waitTitle(galadriel);
    await galadriel.page.keyboard.press('Escape');
    await popGone(galadriel);
    // the popup closes when the page changes under it
    await t.click();
    await galadriel.page.waitForSelector('.concept-pop #concept-title');
    await galadriel.page.evaluate(() => history.pushState({}, '', location.href));
    await galadriel.page.goBack();
    await popGone(galadriel);
  });

  await h.step('concepts: a glossary link in an expanded card opens the popup over it; Escape closes only the popup', async () => {
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    await galadriel.click('button[aria-label="Expand unit"]');
    await galadriel.page.waitForSelector('.unit-expanded', { visible: true });
    const link = await galadriel.page.evaluateHandle(() => [...document.querySelectorAll('.unit-expanded a.term-link[data-concept]')].find((a) => { const r = a.getBoundingClientRect(); return r.width && r.height; }) || null);
    assert.ok(link.asElement(), 'the expanded card has a visible glossary link');
    const name = await link.evaluate((a) => a.dataset.tipTitle);
    await link.asElement().evaluate((e) => e.scrollIntoView({ block: 'center' }));
    await link.asElement().click();
    await galadriel.page.waitForSelector('.concept-pop #concept-title');
    await waitTitle(galadriel);
    assert.match(new URL(galadriel.page.url()).pathname, new RegExp(`^/campaigns/${cid}/my-company/\\d+$`), `clicking "${name}" did not leave the page`);
    assert.ok(await galadriel.page.evaluate(() => !!document.querySelector('.unit-expanded')), 'the card stays open under the popup');
    await galadriel.page.keyboard.press('Escape');
    await popGone(galadriel);
    assert.ok(await galadriel.page.evaluate(() => !!document.querySelector('.unit-expanded')), 'Escape closed the popup and not the card');
    galadriel.issues();
  });

  await h.step('concepts: the company page of a match (the live card) opens the popup too', async () => {
    const admin = await h.session('admin@example.com');
    const m = await campaignId(admin, 'Matches: every scenario');
    const href = (re) => admin.page.evaluate((src) => { const a = [...document.querySelectorAll('main a[href]')].find((x) => new RegExp(src).test(x.getAttribute('href'))); return a && a.getAttribute('href'); }, re.source);
    await admin.goto(`/campaigns/${m}/matches`);
    const match = await href(/\/matches\/\d+$/);
    await admin.goto(match);
    const live = await href(/\/companies\/\d+$/);
    assert.ok(live, 'a company page of the match');
    await admin.goto(live);
    const t = await admin.page.$('main [data-concept][data-tip-title]');
    assert.ok(t, 'the company page has terms that open the popup');
    const visible = await admin.page.evaluateHandle(() => [...document.querySelectorAll('main [data-concept][data-tip-title]')].find((e) => { const r = e.getBoundingClientRect(); return r.width && r.height; }) || null);
    assert.ok(visible.asElement(), 'one of them is on screen');
    await visible.asElement().evaluate((e) => e.scrollIntoView({ block: 'center' }));
    await visible.asElement().click();
    await admin.page.waitForSelector('.concept-pop #concept-title');
    await waitTitle(admin);
    await admin.page.keyboard.press('Escape');
    await popGone(admin);
    admin.issues();
  });
}
