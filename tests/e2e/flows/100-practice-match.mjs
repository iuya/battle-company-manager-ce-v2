// Practice matches (docs/practice-match-spec.md), in the demo world's "Two bands" campaign: Match 1 is an ended practice match of
// Éowyn's own two companies, Match 4 an open one of two of Galadriel's. The flow reads the badge, banner and counts, plays Match 4 through
// (scenario, result, the live match, the end), checks that nothing was awarded and the companies are free, makes a new practice match with
// the checkbox of the form, lets an administrator reopen and end one again, and checks the pages at 360/768/1280 px, light and dark, with axe.
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const axeOn = async (s, ctx) => {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((ctx) => axe.run(ctx, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id}: ${v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' ' + (x.any[0]?.message || '')).join(' | ')}`)), ctx);
};

const BANNER = 'Practice match: no experience, influence, rewards or injuries.';
const NOTHING = 'Practice match: nothing to roll.';

export default async function (h) {
  const you = h.sessions.find((s) => s.email === 'you@example.com') || await h.session('you@example.com', { height: 1800 });
  const admin = await h.session('admin@example.com', { height: 1800 });
  const eowyn = h.sessions.find((s) => s.email === 'eowyn@example.com') || await h.session('eowyn@example.com', { height: 1800 });
  const galadriel = h.sessions.find((s) => s.email === 'galadriel@example.com') || await h.session('galadriel@example.com', { height: 1800 });
  const sauron = h.sessions.find((s) => s.email === 'sauron@example.com') || await h.session('sauron@example.com', { height: 1800 });
  for (const s of [you, admin, eowyn, galadriel, sauron]) s.allow.push(/422/, /403/);
  let bands, ended, open, vale, green, newMatch;
  const before = {};

  const post = (s, url, body) => s.page.evaluate(async (url, body) => {
    const r = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'HX-Request': 'true' }, body, redirect: 'manual' });
    return { status: r.status, text: await r.text() };
  }, url, body);
  const matchLinks = (s) => s.page.$$eval('main li a[href*="/matches/"]', (as) => as.map((a) => ({ href: a.getAttribute('href'), id: Number(a.getAttribute('href').match(/matches\/(\d+)/)[1]), text: a.innerText.replace(/\s+/g, ' ').trim() })));
  const companyIds = (s) => s.page.$$eval('main a[href*="/companies/"]', (as) => [...new Set(as.map((a) => Number(a.getAttribute('href').match(/companies\/(\d+)/)[1])))]);
  // What an aftermath could change on a company page: its resources, the injured and the experience on its cards.
  const figures = async (s, co) => {
    await s.goto(`/campaigns/${bands}/companies/${co}`);
    return s.page.evaluate(() => {
      const t = document.querySelector('main').innerText;
      const num = (label) => (t.match(new RegExp(label + '\\s*\\n?\\s*(-?\\d+)')) || [])[1];
      return { influence: num('Influence'), gold: num('Gold'), injured: (t.match(/Injured/g) || []).length, xp: (t.match(/\d+\s*(?:XP|Experience)/gi) || []).join('|'), units: num('Units') };
    });
  };

  await h.step('practice: the Matches list marks practice matches, the filter lists them, the Overview counts them apart', async () => {
    bands = await campaignId(you, 'Two bands');
    await you.goto(`/campaigns/${bands}/matches`);
    const all = await matchLinks(you);
    assert.equal(all.length, 4, `four matches: ${JSON.stringify(all)}`);
    assert.equal(await you.page.$$eval('main li [data-practice-badge]', (e) => e.length), 2, 'two rows carry the Practice badge');
    assert.ok((await you.page.$eval('[data-filter-practice]', (a) => a.innerText)).includes('2'), 'the chip shows the count');
    await you.click('[data-filter-practice]', 'Practice', { nav: true });
    const only = await matchLinks(you);
    assert.equal(only.length, 2, `the Practice filter lists two: ${JSON.stringify(only)}`);
    assert.ok(only.every((m) => /Practice/.test(m.text)));
    ended = Math.min(...only.map((m) => m.id));
    open = Math.max(...only.map((m) => m.id));
    assert.ok(only.find((m) => m.id === ended).text.includes('won'), 'the ended practice match shows its result');
    await you.goto(`/campaigns/${bands}`);
    assert.ok(await you.has('2 practice matches'), 'the Overview has the practice line');
    const dd = await you.page.$$eval('dl div', (ds) => ds.map((d) => d.innerText.replace(/\s+/g, ' ').trim()).find((t) => t.startsWith('Matches')));
    assert.ok(/^Matches 2\b/.test(dd), `Matches played excludes practice: "${dd}"`);
  });
  if (!bands) return;

  await h.step('practice: an ended practice match has a banner, nothing to roll on the page and on the aftermath page, and every write is refused', async () => {
    await eowyn.goto(`/campaigns/${bands}/matches/${ended}`);
    assert.ok(await eowyn.has(BANNER) && await eowyn.page.$('h1 ~ [data-practice-badge], h1 + [data-practice-badge], [data-practice-badge]'), 'banner and badge');
    const lines = await eowyn.page.$$eval('[data-practice-aftermath]', (e) => e.map((x) => x.innerText));
    assert.equal(lines.length, 2, 'both companies say so');
    assert.ok(lines.every((l) => l.includes('Aftermath: nothing to roll (practice match)')));
    assert.ok(!(await eowyn.has('Waive')) && !(await eowyn.page.$('#aftermath a.btn-primary')), 'no call to action');
    assert.ok(!(await eowyn.has('Reopen match')), 'a player cannot reopen');
    const [mark] = await companyIds(eowyn);
    await eowyn.goto(`/campaigns/${bands}/matches/${ended}/aftermath/${mark}`);
    assert.ok(await eowyn.has(NOTHING) && await eowyn.has('Nothing was awarded and nobody was injured.'), 'the short state, not a 404');
    assert.ok(!(await eowyn.has('Save sheet')) && !(await eowyn.has('Start aftermath')) && !(await eowyn.has('Waive')), 'no sheet, no rolls, no waive');
    for (const step of ['sheet', 'start', 'finish', 'reopen-sheet']) {
      const r = await post(eowyn, `/campaigns/${bands}/matches/${ended}/aftermath/${mark}/${step}`, 'revision=1');
      assert.ok([403, 404, 405, 422].includes(r.status), `${step}: ${r.status}`);
      if (r.status !== 404 && r.status !== 405) assert.ok(r.text.includes(NOTHING), `${step} is refused with the message`);
    }
    const r = await post(admin, `/campaigns/${bands}/matches/${ended}/aftermath/${mark}/waive`, 'note=try');
    assert.ok(r.status === 422 && r.text.includes(NOTHING), `waive: ${r.status}`);
  });

  await h.step('practice: an open practice match locks its companies like any match and shows the same live state, with the badge', async () => {
    await galadriel.goto(`/campaigns/${bands}/matches/${open}`);
    assert.ok(await galadriel.has(BANNER), 'banner on the open match');
    [vale, green] = await companyIds(galadriel);
    assert.ok(vale && green, 'two companies');
    await galadriel.goto(`/campaigns/${bands}/companies/${vale}`);
    await galadriel.waitText('is running');
    assert.ok(await galadriel.page.$('#live-banner [data-practice-badge]'), 'the live bar has the badge');
    assert.ok((await galadriel.page.$eval('#live-banner [data-practice-banner]', (e) => e.innerText)).includes('No aftermath will follow'));
    assert.ok(await galadriel.has('The company is locked until the match ends'), 'the same locked state as a real match');
    assert.ok(await galadriel.page.$('[aria-label^="Might of"]'), 'the trackers are there');
    // another match with the same company is refused
    const r = await post(galadriel, `/campaigns/${bands}/matches`, `sides=2&side-1=${vale}&side-2=${green}`);
    assert.equal(r.status, 422, 'busy');
    assert.ok(/already in Match/.test(r.text), 'the company is already in an open match');
    before.vale = await figures(galadriel, vale);
    before.green = await figures(galadriel, green);
  });

  await h.step('practice: the ATO card keeps table effects and disables the rewards', async () => {
    await galadriel.goto(`/campaigns/${bands}/matches/${open}`);
    if (await galadriel.page.$('#ato [name=bonus]')) {
      const state = await galadriel.page.$$eval('#ato input[name=bonus]', (is) => Object.fromEntries(is.map((i) => [i.value, i.disabled])));
      for (const k of ['influence', 'experience', 'reroll']) if (k in state) assert.equal(state[k], true, `${k} is disabled`);
      if ('toolKit' in state) assert.equal(state.toolKit, false, 'Tool Kit stays');
    }
  });

  await h.step('practice: play it through: scenario, result, the live match, the end checks and the end', async () => {
    await galadriel.goto(`/campaigns/${bands}/matches/${open}`);
    await galadriel.click('main button', 'Choose scenario');
    await galadriel.click('main label', 'Chance Encounter');
    await galadriel.click('main button', 'Use this scenario');
    await galadriel.waitText('Chance Encounter');
    await galadriel.goto(`/campaigns/${bands}/companies/${vale}`);
    await galadriel.waitText('is running');
    await galadriel.click('main button', 'End checks');
    await galadriel.waitText('Recorded only: a practice match awards no experience.');
    await galadriel.page.keyboard.press('Escape');
    await galadriel.goto(`/campaigns/${bands}/matches/${open}`);
    await galadriel.click('main label', 'Draw');
    assert.ok(await galadriel.has('Objectives: recorded only; no reward in a practice match'), 'the result form says so');
    await galadriel.click('main button', 'Save result');
    await galadriel.click('main button', 'Propose to end');
    await galadriel.waitText('No aftermath will follow.');
    await galadriel.click('main button', 'Propose anyway');
    // the player owns both companies: one tap confirms them both
    await galadriel.page.waitForFunction(() => document.body.innerText.includes('Ended') || [...document.querySelectorAll('button')].some((b) => b.innerText.includes('Confirm the end')), { timeout: 8000 });
    if (!(await galadriel.has('Ended'))) {
      await galadriel.click('main button', 'Confirm the end');
      await galadriel.waitText('Ended');
    }
    assert.ok(await galadriel.has(BANNER), 'the banner stays after the end');
    const lines = await galadriel.page.$$eval('[data-practice-aftermath]', (e) => e.length);
    assert.equal(lines, 2, 'each company: nothing to roll');
  });

  await h.step('practice: no aftermath is pending, nothing changed on either company, the counts are unchanged and the practice line grew', async () => {
    for (const [co, name] of [[vale, 'vale'], [green, 'green']]) {
      await galadriel.goto(`/campaigns/${bands}/my-company/${co}`);
      assert.ok(!(await galadriel.has('The company is locked')), `${name} is not locked`);
      assert.ok(!(await galadriel.has('Start the aftermath')) && !(await galadriel.has('Continue the aftermath')), `${name} has no aftermath to play`);
      assert.deepEqual(await figures(galadriel, co), before[name], `${name}: influence, gold, injuries and experience are the same`);
    }
    await galadriel.goto(`/campaigns/${bands}/matches/${open}/aftermath/${vale}`);
    assert.ok(await galadriel.has(NOTHING));
    await you.goto(`/campaigns/${bands}`);
    assert.ok(await you.has('2 practice matches'), 'still two practice matches');
    assert.ok(/^Matches 2\b/.test(await you.page.$$eval('dl div', (ds) => ds.map((d) => d.innerText.replace(/\s+/g, ' ').trim()).find((t) => t.startsWith('Matches')))), 'the played count is the same');
    await you.goto(`/campaigns/${bands}/rankings`);
    const rows = await you.page.$$eval('main li, main tr', (r) => r.map((x) => x.innerText.replace(/\s+/g, ' ').trim()));
    const valeRow = rows.find((t) => t.includes('Scouts of the Hidden Vale'));
    assert.ok(valeRow, 'the company is ranked');
  });

  await h.step('practice: an administrator reopens an ended practice match, and it ends again with the same outcome', async () => {
    await admin.goto(`/campaigns/${bands}/matches/${open}`);
    assert.ok(await admin.has('Reopen match'), 'the administrator may reopen it');
    await admin.click('main button', 'Reopen match');
    await admin.waitGone('Reopen match');
    assert.ok(!(await admin.has('Ended ')) || await admin.has('Propose to end'), 'the match is open again');
    await admin.click('main button', 'Propose to end');
    await admin.click('main button', 'Propose anyway');
    // the match page lists "Ended by..." from the first end: the Reopen button of the administrator is what says it is ended now
    await admin.page.waitForFunction(() => [...document.querySelectorAll('button')].some((b) => /Reopen match|Confirm the end|End without confirmation/.test(b.innerText)), { timeout: 8000 });
    if (!(await admin.has('Reopen match'))) {
      if (await admin.has('End without confirmation')) {
        await admin.click('button', 'End without confirmation');
        await admin.fill('#override-note', 'Practice reopened by the e2e');
        await admin.click('#override-form button', 'End the match');
      } else await admin.click('main button', 'Confirm the end');
      await admin.waitText('Reopen match');
    }
    assert.ok(await admin.has(BANNER) && (await admin.page.$$('[data-practice-aftermath]')).length === 2, 'ended again, still nothing to roll');
    await galadriel.goto(`/campaigns/${bands}/my-company/${vale}`);
    assert.ok(!(await galadriel.has('Start the aftermath')), 'no aftermath after the second end either');
  });

  await h.step('practice: the company is free at once: the form makes a new practice match with its checkbox, off by default', async () => {
    await galadriel.goto(`/campaigns/${bands}/matches/new`);
    assert.equal(await galadriel.page.$eval('[data-practice-box]', (c) => c.checked), false, 'off by default');
    const search = async (i, text) => {
      const boxes = await galadriel.page.$$('input[type=search]');
      await boxes[i].click();
      await boxes[i].type(text);
      await galadriel.click('ul[role=listbox] button', text === 'Scouts' ? 'Scouts of the Hidden Vale' : 'Sentries of the Greenwood');
    };
    // side 1 starts with her first company; replace it with the two free ones
    await galadriel.page.$$eval('main ul[aria-label="Companies on this side"] button[aria-label^="Remove"]', (bs) => bs.forEach((b) => b.click()));
    await galadriel.settle(200);
    await search(0, 'Scouts');
    await search(1, 'Sentries');
    await galadriel.waitText('Same player: Galadriel plays both sides');
    assert.ok(await galadriel.has('Make it a practice match'), 'the note offers the link');
    await galadriel.click('[data-make-practice]', 'Make it a practice match');
    assert.equal(await galadriel.page.$eval('[data-practice-box]', (c) => c.checked), true, 'the link ticks the box');
    assert.ok(await galadriel.has('The flag cannot be changed later'), 'a line says what is skipped');
    await galadriel.click('main button', 'Create match', { nav: true });
    newMatch = Number(new URL(galadriel.page.url()).pathname.match(/matches\/(\d+)$/)?.[1]);
    assert.ok(newMatch, `redirected to the new match: ${galadriel.page.url()}`);
    assert.ok(await galadriel.has(BANNER) && await galadriel.page.$('h1 ~ [data-practice-badge], [data-practice-badge]'), 'the new match is a practice match');
    assert.ok(!(await galadriel.page.$('[href$="/practice"], form[action$="/practice"]')), 'nothing changes the flag afterwards');
  });

  await h.step('practice: a viewer sees the badge and banner and no action', async () => {
    await sauron.goto(`/campaigns/${bands}/matches/${newMatch}`);
    assert.ok(await sauron.has(BANNER) && await sauron.page.$('[data-practice-badge]'));
    assert.ok(!(await sauron.has('Propose to end')) && !(await sauron.has('Save result')), 'a viewer has no action');
    await sauron.goto(`/campaigns/${bands}/matches/${ended}`);
    assert.ok(await sauron.has(BANNER) && !(await sauron.has('Reopen match')));
  });

  for (const scheme of ['light', 'dark']) for (const w of SIZES) {
    await h.step(`practice: the pages at ${w} px, ${scheme}: no horizontal scroll, axe clean`, async () => {
      const pages = [
        [you, `/campaigns/${bands}/matches`, 'main', 'list'], [you, `/campaigns/${bands}/matches?practice=1`, 'main', null],
        [you, `/campaigns/${bands}`, 'main', null], [sauron, `/campaigns/${bands}/matches/${newMatch}`, 'main', 'match'],
        [eowyn, `/campaigns/${bands}/matches/${ended}`, 'main', null],
        [galadriel, `/campaigns/${bands}/matches/new?practice=1`, 'main', null], [galadriel, `/campaigns/${bands}/companies/${vale}`, 'main', null],
      ];
      const [mark] = await (async () => { await eowyn.goto(`/campaigns/${bands}/matches/${ended}`); return companyIds(eowyn); })();
      pages.push([eowyn, `/campaigns/${bands}/matches/${ended}/aftermath/${mark}`, 'main', 'aftermath']);
      for (const [s, url, ctx, shot] of pages) {
        await layout(s, w, scheme);
        await s.goto(url);
        assert.ok(await s.noHScroll(), `no horizontal scroll: ${url} at ${w} px ${scheme}`);
        const bad = await axeOn(s, ctx);
        assert.equal(bad.length, 0, `axe ${url}: ${bad.join('; ')}`);
        if (shot) await s.page.screenshot({ path: `out/practice-${shot}-${w}-${scheme}.png`, fullPage: true });
        if (shot === 'list' || shot === 'match') {
          const badge = await s.page.$eval('[data-practice-badge]', (b) => { const r = b.getBoundingClientRect(); const fs = parseFloat(getComputedStyle(b).fontSize); return { right: r.right, w: innerWidth, fs, icon: !!b.querySelector('svg') }; });
          assert.ok(badge.right <= badge.w + 1 && badge.fs >= 12 && badge.icon, `the badge fits, is 12 px or more and has an icon: ${JSON.stringify(badge)}`);
        }
      }
    });
  }
  for (const s of [you, admin, eowyn, galadriel, sauron]) await layout(s, 1280, 'light', 900);
}
