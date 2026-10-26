// The To Wound chart (docs/matches.md, "To Wound chart"): a window opened from the match page and from the live company page, with a find
// control, the rulebook's values, and a window that no live refresh closes. Axe, contrast and screenshots at 3 widths x 2 themes.
import { assert, campaignId, layout, SIZES, contrast } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const axeOn = async (s, ctx) => {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((ctx) => axe.run(ctx, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id}: ${v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' ' + (x.any[0]?.message || '')).join(' | ')}`)), ctx);
};

const DLG = '#to-wound-dialog';
const open = async (s) => { await s.click('[data-to-wound-open]'); await s.page.waitForSelector(`${DLG} [data-to-wound] td[data-roll]`, { visible: true }); await s.settle(200); };
const close = async (s) => { await s.page.keyboard.press('Escape'); await s.settle(300); };
const shown = (s) => s.page.$eval(DLG, (e) => e.offsetParent !== null).catch(() => false);
const cell = (s, S, D) => s.page.$eval(`${DLG} td[data-s="${S}"][data-d="${D}"]`, (e) => e.dataset.roll);
const find = async (s, S, D) => {
  await s.fill('[data-tw-input=s]', String(S));
  await s.fill('[data-tw-input=d]', String(D));
  await s.settle(150);
};
const result = (s) => s.page.$eval('[data-tw-result]', (e) => e.innerText.replace(/\s+/g, ' ').trim());

export default async function (h) {
  const you = await h.session('you@example.com');
  const angmar = await h.session('angmar-carn-dum@example.com');
  let gallery, mid;

  await h.step('to-wound: a gallery match is running (the live company and the match page share it)', async () => {
    gallery = await campaignId(you, 'Gallery');
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
    mid = await angmar.page.$eval('#live-banner a[href*="/matches/"]', (a) => a.getAttribute('href').match(/matches\/(\d+)/)[1]);
    assert.ok(mid);
  });
  if (!mid) return;

  await h.step('to-wound: the match page opens the chart, which holds the rulebook values, and Escape closes it and returns the focus', async () => {
    await you.goto(`/campaigns/${gallery}/matches/${mid}`);
    assert.equal(await shown(you), false, 'closed at first');
    await you.page.waitForSelector(`${DLG} td[data-roll]`, { hidden: false }).catch(() => {});
    await open(you);
    assert.equal(await shown(you), true);
    assert.equal(await you.page.$$eval(`${DLG} td[data-roll]`, (c) => c.length), 100, '10 x 10 cells');
    // well-known cells of the printed chart, written here independently of the data file
    for (const [S, D, want] of [[3, 3, '4+'], [4, 6, '5+'], [5, 3, '3+'], [1, 1, '4+'], [2, 7, '6+/4+'], [1, 9, '-'], [9, 10, '5+']]) assert.equal(await cell(you, S, D), want, `S${S} vs D${D}`);
    assert.ok(await you.page.$eval(DLG, (d) => d.contains(document.activeElement)), 'focus moved into the window');
    const label = await you.page.$eval(DLG, (d) => d.getAttribute('aria-labelledby'));
    assert.match(await you.page.$eval(`#${label}`, (e) => e.innerText), /To Wound/);
    await close(you);
    assert.equal(await shown(you), false, 'Escape closes');
    assert.ok(await you.page.evaluate(() => document.activeElement?.hasAttribute('data-to-wound-open')), 'focus is back on the button');
    assert.equal(await you.page.$eval(DLG, (e) => !!e.closest('#match')), false, 'outside #match, which every write swaps');
  });

  await h.step('to-wound: the find control shows the roll large, highlights the row and the column, steps and clamps', async () => {
    await open(you);
    assert.match(await result(you), /Pick the attacker/, 'nothing found before a pick');
    await find(you, 3, 3);
    assert.match(await result(you), /^4\+ Strength 3 against Defence 3/);
    assert.equal(await you.page.$eval(`${DLG} td.tw-sel`, (e) => e.dataset.s + '/' + e.dataset.d), '3/3', 'the cell is marked');
    assert.equal(await you.page.$$eval(`${DLG} td.tw-hl`, (c) => c.length), 19, 'the row and the column (19 cells)');
    assert.deepEqual(await you.page.$$eval(`${DLG} th.tw-on`, (c) => c.map((e) => e.innerText.trim())), ['3', '3'], 'both headers');
    await find(you, 4, 6);
    assert.match(await result(you), /^5\+ Strength 4 against Defence 6/);
    await find(you, 2, 7);
    assert.match(await result(you), /^6\+ then 4\+ Strength 2 against Defence 7/);
    assert.match(await result(you), /further dice/i, 'the notation text is shown');
    await find(you, 1, 9);
    assert.match(await result(you), /^Cannot wound /);
    // the steppers
    await find(you, 3, 3);
    await you.page.click('[aria-label="Attacker\'s Strength, one higher"]');
    await you.page.click('[aria-label="Target\'s Defence, one lower"]');
    await you.settle(100);
    assert.match(await result(you), /^3\+ Strength 4 against Defence 2/);
    await find(you, 99, 0);
    assert.equal(await you.page.$eval('[data-tw-input=s]', (e) => e.value), '10', 'clamped to the chart');
    assert.equal(await you.page.$eval('[data-tw-input=d]', (e) => e.value), '', 'an impossible Defence is empty');
    // a tap on a cell
    await you.page.click(`${DLG} td[data-s="6"][data-d="2"]`);
    await you.settle(100);
    assert.match(await result(you), /^3\+ Strength 6 against Defence 2/);
    assert.equal(await you.page.$eval('[data-tw-input=s]', (e) => e.value), '6');
    // the key and the notes
    const txt = await you.page.$eval(DLG, (e) => e.innerText);
    assert.match(txt, /cannot wound/i);
    assert.match(txt, /Rend/);
    await close(you);
  });

  await h.step('to-wound: the live company page has the button in the banner and the window outside #live-zones; a live refresh does not close it or lose the pick', async () => {
    await angmar.goto(`/campaigns/${gallery}/my-company`);
    assert.equal(await angmar.page.$eval('#live-banner [data-to-wound-open]', (b) => b.innerText.trim()), 'To wound');
    assert.equal(await angmar.page.$eval(DLG, (e) => !!e.closest('#live-zones, #live-banner')), false, 'outside what is swapped or sticky');
    await open(angmar);
    await find(angmar, 4, 6);
    await angmar.page.evaluate(() => { document.querySelector('#to-wound-dialog [data-to-wound]').__keep = 1; window.__swaps = 0; document.body.addEventListener('htmx:afterSwap', (e) => { if (e.detail.target?.id === 'live-zones') window.__swaps++; }); });
    await angmar.page.evaluate(() => { const d = document.getElementById('live-zones').dataset; htmx.ajax('GET', `/campaigns/${d.campaign}/companies/${d.company}/live`, { target: '#live-zones', swap: 'outerHTML' }); });
    await angmar.page.waitForFunction(() => window.__swaps > 0, { timeout: 8000 });
    await angmar.settle(300);
    assert.equal(await shown(angmar), true, 'still open after the zones were swapped');
    assert.equal(await angmar.page.evaluate(() => document.querySelector('#to-wound-dialog [data-to-wound]').__keep), 1, 'the body is the same element');
    assert.match(await result(angmar), /^5\+ Strength 4 against Defence 6/, 'the pick survived');
    // and with the event stream down the chart still works (nothing is fetched)
    const before = angmar.reqCount;
    await find(angmar, 3, 3);
    assert.match(await result(angmar), /^4\+/);
    assert.equal(angmar.reqCount, before, 'no request to find a roll');
    await close(angmar);
    assert.ok(await angmar.page.evaluate(() => document.activeElement?.hasAttribute('data-to-wound-open')), 'focus back on the banner button');
  });

  await h.step('to-wound: Tab stays inside the window; the backdrop and Close both close it', async () => {
    await open(angmar);
    for (let i = 0; i < 12; i++) {
      await angmar.page.keyboard.press('Tab');
      assert.ok(await angmar.page.evaluate(() => document.getElementById('to-wound-dialog').contains(document.activeElement)), `Tab ${i + 1} stays in the window`);
    }
    await angmar.page.keyboard.down('Shift'); await angmar.page.keyboard.press('Tab'); await angmar.page.keyboard.up('Shift');
    assert.ok(await angmar.page.evaluate(() => document.getElementById('to-wound-dialog').contains(document.activeElement)));
    await angmar.page.click(`${DLG} button[aria-label=Close]`);
    await angmar.settle(300);
    assert.equal(await shown(angmar), false, 'Close');
    await open(angmar);
    await angmar.page.mouse.click(4, 400); // on the backdrop
    await angmar.settle(300);
    assert.equal(await shown(angmar), false, 'the backdrop');
  });

  for (const [who, page] of [['match', () => you], ['live', () => angmar]]) {
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await h.step(`to-wound: the ${who} page's chart at ${w} px, ${scheme}: no page scroll, sticky headers, band colours readable and distinct, axe, screenshot`, async () => {
        const s = page();
        await s.goto(who === 'match' ? `/campaigns/${gallery}/matches/${mid}` : `/campaigns/${gallery}/my-company`);
        await layout(s, w, scheme, 800);
        assert.ok(await s.noHScroll(), `no horizontal scroll with the button at ${w}px`);
        await open(s);
        await find(s, 4, 6);
        assert.ok(await s.noHScroll(), `no horizontal scroll with the window open at ${w}px`);
        const g = await s.page.evaluate(() => {
          const dlg = document.getElementById('to-wound-dialog').getBoundingClientRect();
          const sc = document.querySelector('#to-wound-dialog .tw-scroll');
          const t = sc.querySelector('table').getBoundingClientRect();
          const cv = document.createElement('canvas'); cv.width = cv.height = 1;
          const cx = cv.getContext('2d', { willReadFrequently: true });
          const norm = (c) => { cx.clearRect(0, 0, 1, 1); cx.fillStyle = '#000'; cx.fillStyle = c; cx.fillRect(0, 0, 1, 1); const d = cx.getImageData(0, 0, 1, 1).data; return `rgb(${d[0]}, ${d[1]}, ${d[2]})`; };
          const tiers = {};
          for (const td of sc.querySelectorAll('td[data-tier]')) {
            if (tiers[td.dataset.tier]) continue;
            const cs = getComputedStyle(td);
            tiers[td.dataset.tier] = { fg: norm(cs.color), bg: norm(cs.backgroundColor), text: td.innerText.trim() };
          }
          const head = sc.querySelector('thead th:nth-child(2)'), corner = sc.querySelector('thead th'), rowh = sc.querySelector('tbody th');
          return {
            fits: dlg.left >= 0 && dlg.right <= innerWidth + 0.5, tableW: Math.round(t.width), scrollW: sc.scrollWidth, clientW: sc.clientWidth, scrollH: sc.scrollHeight, clientH: sc.clientHeight,
            sticky: [corner, head, rowh].map((e) => getComputedStyle(e).position), tiers,
            cellH: Math.round(sc.querySelector('td').getBoundingClientRect().height), cellW: Math.round(sc.querySelector('td').getBoundingClientRect().width),
            btn: Math.round(document.querySelector('#to-wound-dialog [aria-label$="one higher"]').getBoundingClientRect().height),
          };
        });
        assert.ok(g.fits, 'the window fits the viewport');
        assert.deepEqual(g.sticky, ['sticky', 'sticky', 'sticky'], 'corner, header row and header column stick');
        assert.deepEqual(Object.keys(g.tiers).sort(), ['3', '4', '5', '6', 'none', 'split'], 'six bands');
        assert.equal(new Set(Object.values(g.tiers).map((t) => t.bg)).size, 6, `six different band colours: ${JSON.stringify(g.tiers)}`);
        for (const [tier, t] of Object.entries(g.tiers)) assert.ok(contrast(t.fg, t.bg) >= 4.5, `band ${tier} (${t.text}) contrast ${contrast(t.fg, t.bg).toFixed(2)} in ${scheme}: ${t.fg} on ${t.bg}`);
        assert.ok(g.btn >= 44, `stepper buttons are ${g.btn}px tall`);
        console.log(`    (${who} ${w}px ${scheme}: table ${g.tableW}px in a ${g.clientW}px box${g.scrollW > g.clientW ? ', scrolls sideways' : ''}${g.scrollH > g.clientH ? ', scrolls down' : ''}; cell ${g.cellW}x${g.cellH})`);
        assert.deepEqual(await axeOn(s, DLG), [], 'axe: the window');
        await s.page.screenshot({ path: `out/to-wound-${who}-${w}-${scheme}.png` });
        await close(s);
        await s.page.evaluate(() => document.querySelector('[data-to-wound-open]').scrollIntoView({ block: 'center' }));
        if (w === 360 && scheme === 'light') await s.page.screenshot({ path: `out/to-wound-${who}-button-360.png` });
      });
    }
  }
  await layout(you, 1280, 'light');
  await layout(angmar, 1280, 'light');
}
