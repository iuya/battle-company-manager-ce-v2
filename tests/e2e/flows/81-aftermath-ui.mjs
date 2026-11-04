// The aftermath sheet's look: the milestones of My Company inside the card, one press each, small outcome toggles, and the outcome badges of
// the locked sheet (Survived / Casualty / Fled) in three distinct colours with readable contrast, light and dark; then every unit with its results.
import { assert, campaignId, layout, SIZES, contrast } from '../lib/util.mjs';

export default async function (h) {
  const sauron = await h.session('sauron@example.com');
  const page = sauron.page;
  let url;

  await h.step('aftermath sheet: the milestones and outcome toggles stay inside the unit card, with no profile but the hero stats, at 360 / 768 / 1280 px, light and dark', async () => {
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
      const cards = await page.$$eval('fieldset.unit', (fs) => fs.map((f) => {
        const b = f.getBoundingClientRect();
        const marks = [...f.querySelectorAll('.milestone, .outcome-toggle')];
        return { facts: !!f.querySelector('[data-unit-facts]'), profile: !!f.querySelector('dl.statline'), bar: !!f.querySelector('.xp-bar') && f.querySelector('.xp-bar').getBoundingClientRect().bottom >= [...f.querySelectorAll('.milestone')].at(-1).getBoundingClientRect().bottom, medalInFacts: !!f.querySelector('[data-unit-facts] .level-medal'), marks: f.querySelectorAll('.milestone').length, toggles: f.querySelectorAll('.outcome-toggle').length,
          over: marks.filter((m) => { const r = m.getBoundingClientRect(); return r.right > b.right + 1 || r.left < b.left - 1 || r.width < 40; }).length,
          toggleH: Math.max(...[...f.querySelectorAll('.outcome-toggle')].map((t) => t.getBoundingClientRect().height)) };
      }));
      assert.ok(cards.length >= 3 && cards.every((c) => c.facts && !c.profile && c.bar && !c.medalInFacts && c.marks >= 4 && c.toggles === 3 && !c.over), `each unit is told apart and its marks fit its card at ${w}px ${scheme}: ${JSON.stringify(cards.slice(0, 3))}`);
      assert.ok(cards.every((c) => c.toggleH <= 44.5), `the outcome toggles are small at ${w}px: ${cards[0].toggleH}`);
      await (await page.$('fieldset.unit')).screenshot({ path: `out/aftermath-sheet-${w}-${scheme}.png` });
    }
    await layout(sauron, 1280, 'light');
  });

  await h.step('aftermath sheet: one press lights a milestone and the sheet saves itself', async () => {
    await sauron.goto(url);
    const box = 'fieldset.unit:nth-of-type(1) label.milestone-check:has(input[name$="-objective"])';
    const was = await page.$eval(`${box} input`, (i) => i.checked);
    const earned = () => page.$eval('fieldset.unit:nth-of-type(1) [id^=sheet-xp-]', (f) => Number(f.querySelector('.xp-match-text')?.innerText.replace('+', '') || 0));
    const before = await earned();
    assert.ok(before >= 1, 'the bar shows what the match gives in its own colour');
    await page.click(box);
    await sauron.settle(900);
    assert.equal(await earned(), before + (was ? -1 : 1), 'the bar follows the press without a reload');
    await sauron.goto(url);
    assert.equal(await page.$eval(`${box} input`, (i) => i.checked), !was, 'the press is saved');
    const slain = 'fieldset.unit:nth-of-type(1) button.milestone:has(.milestone-count)';
    await page.click(slain);
    await sauron.settle(900);
    await sauron.goto(url);
    assert.ok(await page.$eval(`${slain} .milestone-count`, (c) => Number(c.innerText) >= 1), 'each press of a repeatable gain counts');
  });

  await h.step('aftermath sheet: after the sheet is locked Survived, Casualty and Fled have distinct colours with contrast of at least 4.5 in light and dark', async () => {
    await sauron.goto(url);
    const units = await page.$$('fieldset.unit');
    assert.ok(units.length >= 3);
    await (await units[1].$('.outcome-toggle[data-tone=danger]')).click();
    await (await units[2].$('.outcome-toggle[data-tone=warn]')).click();
    await sauron.settle(900); // the sheet saves itself (debounced) on every change
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

  await h.step('aftermath: with the casualties on top, the rest of the company follows with its outcome, experience and milestones', async () => {
    const r = await page.evaluate(() => {
      const casualty = document.querySelector('article[id^=casualty-]');
      const cards = [...document.querySelectorAll('[data-unit-result]')];
      return {
        order: casualty.compareDocumentPosition(cards[0]) === Node.DOCUMENT_POSITION_FOLLOWING,
        casualtyMarks: !!casualty.querySelector('[data-unit-outcome]'),
        sameWidth: casualty.offsetWidth === cards[0].offsetWidth,
        xp: cards.every((c) => c.querySelector('.xp-bar')),
        allMilestones: cards.every((c) => c.querySelectorAll('.milestone').length >= 4) && !document.querySelector('[data-unit-result] button.milestone'),
        texts: cards.map((c) => c.querySelector('[data-unit-outcome]').innerText),
        lit: document.querySelectorAll('[data-unit-result] .milestone[data-on=true]').length,
        profile: document.querySelectorAll('#aftermath-page dl.statline').length,
        heroInline: document.querySelectorAll('[data-unit-result] [data-hero-inline], article[id^=casualty-] [data-hero-inline]').length,
      };
    });
    assert.ok(r.order && r.casualtyMarks && r.sameWidth && r.xp && r.allMilestones, `results: ${JSON.stringify(r)}`);
    assert.ok(r.texts.some((t) => /Fled/.test(t)) && r.texts.some((t) => /Survived/.test(t)), `who fled and who survived: ${JSON.stringify(r.texts)}`);
    assert.ok(r.lit >= 1, 'the milestones earned are lit');
    assert.equal(r.profile, 0, 'no profile on the aftermath');
    assert.ok(r.heroInline >= 1, 'the heroes show Might, Will and Fate in one line');
    for (const w of SIZES) { await layout(sauron, w, 'light'); assert.ok(await sauron.noHScroll(), `no horizontal scroll at ${w}px`); }
    await layout(sauron, 1280, 'light');
  });

  await h.step('aftermath: who did not play comes last, recovered and able to play the next match', async () => {
    const eowyn = await h.session('eowyn@example.com');
    const cid = await campaignId(eowyn, 'Aftermath ward 4');
    await eowyn.goto(`/campaigns/${cid}/matches`);
    const matches = await eowyn.page.$$eval('main a[href]', (a) => [...new Set(a.map((x) => x.getAttribute('href')).filter((x) => /\/matches\/\d+$/.test(x)))]);
    let done = false;
    for (const m of matches) {
      await eowyn.goto(m);
      if (!/Match 2/.test(await eowyn.page.$eval('main', (e) => e.innerText))) continue;
      const u = await eowyn.page.evaluate(() => [...document.querySelectorAll('main a[href]')].find((x) => /\/aftermath\/\d+$/.test(x.getAttribute('href')) && /nursing/.test(x.innerText))?.getAttribute('href'));
      await eowyn.goto(u);
      const notes = await eowyn.page.$$eval('[data-unit-result]', (cs) => cs.map((c) => c.querySelector('[data-unit-next]')?.innerText || ''));
      assert.match(notes.at(-1), /Recovered: can play the next match/, `the Warrior who missed it is last: ${JSON.stringify(notes)}`);
      assert.match(notes.at(-2), /Can play the next match/, `the Sergeant who stayed out before it: ${JSON.stringify(notes)}`);
      assert.ok(notes.slice(0, -2).every((n) => !n), 'the others play on');
      done = true;
      break;
    }
    assert.ok(done, 'the second match of the ward');
  });
}
