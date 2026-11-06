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
      // a unit without a line of place, path and injuries reserves it only beside a card that has one, in the same row of the grid
      const rows = await page.$$eval('fieldset.unit', (fs) => {
        const by = new Map();
        for (const f of fs) { const top = Math.round(f.getBoundingClientRect().top); by.set(top, [...(by.get(top) || []), f.querySelector('[data-unit-facts]')]); }
        return [...by.values()].map((r) => ({ full: r.map((x) => x.children.length > 0), h: r.map((x) => x.getBoundingClientRect().height), top: r.map((x) => Math.round(x.getBoundingClientRect().top)) }));
      });
      for (const r of rows) {
        const any = r.full.some(Boolean);
        r.h.forEach((h, i) => { if (!r.full[i]) assert.equal(h > 0, any && r.h.length > 1, `an empty line is reserved only beside a full one at ${w}px: ${JSON.stringify(r)}`); });
        // within a pixel: an officer's card has a thicker border than a Warrior's
        if (any && r.h.length > 1) assert.ok(Math.abs(r.top[0] - r.top[1]) <= 1, `the two cards of a row line up at ${w}px: ${JSON.stringify(r)}`);
      }
      if (w >= 1024) assert.ok(rows.some((r) => r.h.length === 2 && !r.full[0] && !r.full[1]), `the sheet has a row of two Warriors at ${w}px`);
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

  await h.step('Start aftermath on the match page: a blank sheet opens with nothing started; a match played live skips it, into the roll or the summary', async () => {
    const admin = await h.session('admin@example.com');
    const cid = await campaignId(admin, 'Matches: every scenario and every state');
    await admin.goto(`/campaigns/${cid}/matches`);
    const matches = await admin.page.$$eval('main a[href]', (a) => [...new Set(a.map((x) => x.getAttribute('href')).filter((x) => /\/matches\/\d+$/.test(x)))]);
    const pending = {};
    for (const m of matches) {
      await admin.goto(m);
      const n = await admin.page.$$eval('#aftermath form[action$="/begin"]', (f) => f.length);
      if (n) pending[/Chance Encounter/.test(await admin.text()) ? 'blank' : 'live'] = m;
    }
    assert.ok(pending.blank && pending.live, `both demo matches wait: ${JSON.stringify(pending)}`);
    const start = async (m) => {
      await admin.goto(m);
      await admin.page.click('#aftermath form[action$="/begin"] button');
      await admin.page.waitForFunction(() => /\/aftermath\/\d+$/.test(location.pathname), { timeout: 8000 });
      await admin.settle(300);
      return admin.page.evaluate(() => ({ sheet: document.querySelectorAll('main fieldset.unit').length > 0, edit: document.body.innerText.includes('Edit the sheet again'), done: !!document.querySelector('[data-change-summary]') }));
    };
    assert.deepEqual(await start(pending.blank), { sheet: true, edit: false, done: false }, 'nothing was tracked: the sheet opens');
    await admin.goto(pending.blank);
    assert.equal(await admin.page.$$eval('#aftermath form[action$="/begin"]', (f) => f.length), 2, 'and nothing started');
    assert.deepEqual(await start(pending.live), { sheet: false, edit: true, done: false }, 'a casualty: on to its roll, with the way back to the sheet');
    assert.deepEqual(await start(pending.live), { sheet: false, edit: false, done: true }, 'no casualty: on to the summary');
    await admin.close?.();
  });

  // The aftermath page of a company of ward 1, found through the links of its match.
  const aftermathOf = async (s, name) => {
    const cid = await campaignId(s, 'Aftermath ward 1');
    await s.goto(`/campaigns/${cid}/matches`);
    const matches = await s.page.$$eval('main a[href]', (a) => [...new Set(a.map((x) => x.getAttribute('href')).filter((x) => /\/matches\/\d+$/.test(x)))]);
    for (const m of matches) {
      await s.goto(m);
      const found = await s.page.evaluate((n) => [...document.querySelectorAll('main a[href]')].find((x) => /\/aftermath\/\d+$/.test(x.getAttribute('href')) && x.innerText.includes(n))?.getAttribute('href'), name);
      if (found) return found;
    }
    throw new Error(`no aftermath page of ${name}`);
  };
  const finished = (p) => p.evaluate(() => {
    const seen = (el) => !!el && el.getBoundingClientRect().height > 0;
    const links = [...document.querySelectorAll('[data-done-links] a')];
    const y = (sel) => document.querySelector(sel)?.getBoundingClientRect().top ?? -1;
    return {
      stepper: !!document.querySelector('ol[aria-label=Steps]'), sheetCard: !!document.querySelector('#aftermath-sheet'),
      summary: document.querySelector('[data-change-summary]')?.innerText || '', counts: document.querySelector('[data-sheet-counts]')?.innerText || '',
      details: document.querySelectorAll('#aftermath-done details').length, open: document.querySelectorAll('#aftermath-done details[open]').length,
      links: links.filter(seen).map((a) => a.innerText.trim()),
      levelUp: [...document.querySelectorAll('#aftermath-step-2 button')].filter(seen).length,
      order: [y('#aftermath-done'), y('#aftermath-step-2'), y('#aftermath-step-1'), y('#aftermath-company')],
    };
  });

  await h.step('finished aftermath: no stepper or step cards, the summary and counts on top, two collapsed details, then Level-ups, Injuries and the company, at 360 / 768 / 1280 px, light and dark', async () => {
    const bilbo = await h.session('bilbo@example.com');
    const path = await aftermathOf(bilbo, 'Hobbits who finished');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(bilbo, w, scheme);
      await bilbo.goto(path);
      assert.ok(await bilbo.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      const r = await finished(bilbo.page);
      assert.ok(!r.stepper && !r.sheetCard, 'no stepper and no Sheet card');
      assert.match(r.summary, /level-up/, 'the summary says a roll is waiting');
      assert.match(r.counts, /\d+ took part · 1 casualty/);
      assert.equal(r.details, 2);
      assert.equal(r.open, 0, 'both details start collapsed');
      assert.deepEqual(r.links, ['My Company', 'Back to the match'], 'the way out is in view');
      assert.ok(r.levelUp > 0, 'the level-up is one press away');
      assert.ok(r.order.every((v, i) => v >= 0 && (i === 0 || v > r.order[i - 1])), `summary, Level-ups, Injuries, company: ${r.order}`);
      await bilbo.page.screenshot({ path: `out/aftermath-finished-${w}-${scheme}.png`, fullPage: true });
    }
    await layout(bilbo, 1280, 'light');
    await bilbo.goto(path);
    await bilbo.click('#aftermath-done summary', 'The sheet');
    assert.ok(await bilbo.page.$('#aftermath-done details[open] li .badge'), 'the sheet opens with every unit');
  });

  await h.step('finished aftermath: somebody else reads the same page, with nothing to press', async () => {
    const path = await aftermathOf(sauron, 'Hobbits who finished');
    await sauron.goto(path);
    const r = await finished(page);
    assert.ok(!r.stepper && !r.sheetCard && r.details === 2 && r.summary, 'the same layout');
    assert.deepEqual(r.links, ['Back to the match']);
    assert.equal(r.levelUp, 0, 'only the owner rolls');
    assert.ok(await sauron.has('You can read this aftermath but not change it.'));
  });

  await h.step('waived aftermath: it says so with its note, with no stepper and no summary of changes', async () => {
    const path = await aftermathOf(sauron, 'Wild Wargs that fled');
    await sauron.goto(path);
    const r = await finished(page);
    assert.ok(!r.stepper && !r.sheetCard && !r.summary, 'nothing was applied');
    assert.ok(await sauron.has('Waived: nothing was applied.'));
    assert.ok(await sauron.has('The Wargs fled; the organiser waives the rest'), 'the note is in view');
    assert.deepEqual(r.links, ['Back to the match']);
  });
}
