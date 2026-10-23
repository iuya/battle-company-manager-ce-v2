// The aftermath sheet's look: compact XP chips grouped inside the card, and the outcome badges of the locked sheet
// (Survived / Casualty / Fled) in three distinct colours with readable contrast, light and dark.
import { assert, campaignId, layout, SIZES, contrast } from '../lib/util.mjs';

export default async function (h) {
  const sauron = await h.session('sauron@example.com');
  const page = sauron.page;
  let url;

  await h.step('aftermath sheet: XP toggle chips are small, wrap inside the unit card and do not overflow, at 360 / 768 / 1280 px, light and dark', async () => {
    const cid = await campaignId(sauron, 'Aftermath ward 1');
    await sauron.goto(`/campaigns/${cid}/matches`);
    const matches = await page.$$eval('main a[href]', (a) => [...new Set(a.map((x) => x.getAttribute('href')).filter((x) => /\/matches\/\d+$/.test(x)))]);
    for (const m of matches) {
      await sauron.goto(m);
      url = await page.evaluate(() => { const a = [...document.querySelectorAll('main a[href]')].find((x) => /\/aftermath\/\d+$/.test(x.getAttribute('href')) && /half-filled sheet/.test(x.innerText)); return a && a.getAttribute('href'); });
      if (url) break;
    }
    assert.ok(url, 'the Orcs with a half-filled sheet have an aftermath page');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(sauron, w, scheme);
      await sauron.goto(url);
      assert.ok(await sauron.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      const r = await page.evaluate(() => {
        const out = [];
        for (const fs of document.querySelectorAll('fieldset')) {
          const chips = [...fs.querySelectorAll('label .chip')];
          if (!chips.length) continue;
          const f = fs.getBoundingClientRect();
          for (const c of chips) { const b = c.getBoundingClientRect(), cs = getComputedStyle(c), lh = parseFloat(cs.lineHeight) || parseFloat(cs.fontSize) * 1.4; out.push({ text: c.innerText.slice(0, 30), len: c.innerText.length, h: b.height, lines: Math.round((b.height - 2 * parseFloat(cs.paddingTop) - 2) / lh), pad: b.height - Math.round((b.height - 2 * parseFloat(cs.paddingTop) - 2) / lh) * lh, over: b.right - f.right, left: b.left - f.left, overflowX: c.scrollWidth - c.clientWidth }); }
        }
        return out;
      });
      assert.ok(r.length >= 20, `XP chips found (${r.length})`);
      // a chip is one line high plus its padding (about 32 px); a long label wraps and then is taller only by whole lines
      assert.ok(r.every((c) => c.len > 40 || c.h <= 36.5), `short chips no taller than 36px at ${w}px: ${JSON.stringify(r.filter((c) => c.len <= 40 && c.h > 36.5).slice(0, 3))}`);
      assert.ok(r.every((c) => c.pad <= 22), `the padding of a chip stays small (it is not a big button) at ${w}px: ${JSON.stringify(r.filter((c) => c.pad > 22).slice(0, 3))}`);
      if (w >= 1024) assert.ok(r.every((c) => c.h <= 36.5), 'single-line chips on desktop');
      assert.ok(r.every((c) => c.over <= 1 && c.left >= -1), `chips stay inside their card at ${w}px ${scheme}: ${JSON.stringify(r.filter((c) => c.over > 1).slice(0, 3))}`);
      await (await page.$('fieldset:has(label .chip)')).screenshot({ path: `out/aftermath-chips-${w}-${scheme}.png` });
    }
    await layout(sauron, 1280, 'light');
  });

  await h.step('aftermath sheet: after the sheet is locked Survived, Casualty and Fled have distinct colours with contrast of at least 4.5 in light and dark', async () => {
    await sauron.goto(url);
    const names = await page.$$eval('fieldset input[type=radio]', (r) => [...new Set(r.map((x) => x.name))]);
    assert.ok(names.length >= 3);
    const set = { 0: 'survived', 1: 'casualty', 2: 'fled' };
    for (const [i, v] of Object.entries(set)) await page.evaluate((n, v) => document.querySelector(`input[name="${n}"][value="${v}"]`).click(), names[i], v);
    await sauron.click('button', 'Save sheet', { nav: false });
    await sauron.settle(400);
    await sauron.click('button', '=Start aftermath');
    await sauron.settle(500);
    await page.evaluate(() => document.querySelectorAll('details').forEach((d) => { if (d.innerText.includes('The sheet')) d.open = true; }));
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(sauron, w, scheme);
      const badges = await page.evaluate(() => {
        const effBg = (el) => { // the first opaque background up the tree, with the element's own composited over it
          const own = getComputedStyle(el).backgroundColor;
          let under = [255, 255, 255];
          for (let p = el.parentElement; p; p = p.parentElement) { const c = getComputedStyle(p).backgroundColor.match(/[\d.]+/g).map(Number); if ((c[3] ?? 1) === 1 && c[0] + c[1] + c[2] + (c[3] === 0 ? 1 : 0) >= 0 && getComputedStyle(p).backgroundColor !== 'rgba(0, 0, 0, 0)') { under = c.slice(0, 3); break; } }
          return { own, under };
        };
        const cv = document.createElement('canvas'); cv.width = cv.height = 1;
        const cx = cv.getContext('2d', { willReadFrequently: true });
        const norm = (c) => { cx.clearRect(0, 0, 1, 1); cx.fillStyle = '#000'; cx.fillStyle = c; cx.fillRect(0, 0, 1, 1); const d = cx.getImageData(0, 0, 1, 1).data; return `rgb(${d[0]}, ${d[1]}, ${d[2]})`; }; // color-mix() comes back as color(srgb ...)
        return [...document.querySelectorAll('details li .badge')].filter((b) => /^(Survived|Casualty|Fled)$/.test(b.innerText.trim())).map((b) => ({ text: b.innerText.trim(), color: norm(getComputedStyle(b).color), ...(({ own, under }) => ({ own: norm(own), under }))(effBg(b)) }));
      });
      const by = Object.fromEntries(badges.map((b) => [b.text, b]));
      assert.ok(by.Survived && by.Casualty && by.Fled, `three outcome badges at ${scheme}: ${JSON.stringify(badges.map((b) => b.text))}`);
      const pair = (b) => b.color + '/' + b.own;
      assert.equal(new Set([pair(by.Survived), pair(by.Casualty), pair(by.Fled)]).size, 3, `three different colour pairs in ${scheme}: ${JSON.stringify(by)}`);
      assert.equal(new Set([by.Survived.own, by.Casualty.own, by.Fled.own]).size, 3, 'three different backgrounds');
      for (const b of Object.values(by)) {
        const c = contrast(b.color, b.own, b.under);
        assert.ok(c >= 4.5, `${b.text} contrast ${c.toFixed(2)} in ${scheme} at ${w}px (${b.color} on ${b.own} over ${b.under})`);
      }
      assert.ok(await sauron.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      await (await page.$('details li')).evaluate((e) => e.scrollIntoView({ block: 'center' }));
      await page.screenshot({ path: `out/aftermath-outcomes-${w}-${scheme}.png` });
    }
    await layout(sauron, 1280, 'light');
  });
}
