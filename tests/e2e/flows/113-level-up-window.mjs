// The level-up window (docs/frontend-level-up.md): options are buttons that apply at once, with the marks of the results that take the other one too,
// a stat's progress dots, the (i) that shows an option's text without choosing it, a warrior's no change and its Hero in the Making.
// Galadriel's companies in "Level-up options" (one hero per case) and "Level-ups waiting".
import { assert, campaignId, layout, MODAL, SIZES, sleep } from '../lib/util.mjs';
import { axeScoped } from '../lib/axe.mjs';

const OPT = MODAL + ' .level-opt';

export default async function (h) {
  const s = await h.session('galadriel@example.com');
  s.allow.push(/404/); // the (i) of a stat asks for a term that is not in the glossary and shows the option's own text
  let options, waiting;
  const windowText = () => s.page.$eval('#modal-body', (e) => e.innerText);
  const legends = () => s.page.$$eval(MODAL + ' fieldset legend', (l) => l.map((x) => x.innerText.trim()));
  const pressed = () => s.page.$$eval(OPT + '[aria-pressed=true]', (b) => b.map((x) => x.querySelector('.level-opt-name').innerText.trim()));
  const off = () => s.page.$$eval(OPT + '[aria-disabled=true]', (b) => b.map((x) => x.querySelector('.level-opt-name').innerText.trim()));
  const closeWindow = async () => { await s.page.$eval('#modal-body', (e) => e.parentElement.querySelector('button[aria-label=Close]').click()); await s.settle(150); };

  // Opens the level-ups of the page in turn until one fits: every roll waiting stays waiting.
  const openWhere = async (fits, what) => {
    const n = await s.page.$$eval('main button.promo', (b) => b.length);
    const seen = [];
    for (let i = 0; i < n; i++) {
      const b = (await s.page.$$('main button.promo'))[i];
      if (!b || !/Level Up/.test(await b.evaluate((e) => e.innerText))) continue;
      await b.evaluate((e) => e.click());
      await s.page.waitForFunction(() => { const b = document.querySelector('#modal-body'); return b && !/Loading/.test(b.innerText) && b.innerText.trim().length > 0; });
      await s.settle(250);
      if (await fits()) return;
      seen.push((await windowText()).slice(0, 140));
      await closeWindow();
    }
    assert.fail(`no level-up window is ${what}: ${JSON.stringify(seen)}`);
  };
  const rolled = (...sums) => async () => { const l = await legends(); return sums.every((x, i) => (l[i] || '').endsWith('· ' + x)) && l.filter((x) => /roll ·/.test(x)).length === sums.length; };
  const onOptions = async () => { options ||= await campaignId(s, 'Level-up options'); await s.goto(`/campaigns/${options}/my-company`); };
  const onWaiting = async () => { waiting ||= await campaignId(s, 'Level-ups waiting'); await s.goto(`/campaigns/${waiting}/my-company`); };
  const noRadios = async () => assert.equal(await s.page.$$eval(MODAL + ' input[type=radio]', (r) => r.length), 0, 'no radios');
  const hasCourage = async () => (await s.page.$$eval(OPT, (b) => b.map((x) => x.value))).includes('stat:courage:1');

  await h.step('level-up window: a 5 beside another result marks Courage, moves the mark, and the other result applies both', async () => {
    await onOptions();
    await openWhere(async () => await rolled(5, 8)() && await hasCourage() && (await pressed()).includes('Courage +1'), 'a 5 and an 8 with Courage');
    await noRadios();
    assert.ok(!(await s.has('Apply both')), 'only two marks have a button');
    assert.ok(await s.has('+ other roll'), 'the options that take the other result say so');
    await s.click(OPT, 'Courage +1'); // the marked one does nothing
    await s.settle(150);
    assert.deepEqual(await pressed(), ['Courage +1'], 'the mark cannot be removed');
    await s.click(OPT, 'Intelligence +1');
    await s.settle(150);
    assert.deepEqual(await pressed(), ['Intelligence +1'], 'the other stat moves the mark, applying nothing');
    assert.ok(await s.has('Second roll'), 'nothing was applied');
    await s.click(OPT, 'Fight +1');
    await s.waitText('Done');
    const t = await windowText();
    assert.match(t, /Intelligence/);
    assert.match(t, /Fight/);
    await s.click(MODAL + ' button', '=Done');
  });

  await h.step('level-up window: Courage at its limit marks Intelligence', async () => {
    await onOptions();
    await openWhere(async () => await rolled(5, 8)() && (await pressed()).includes('Intelligence +1') && !(await hasCourage()), 'a 5 and an 8 with Courage at its limit');
    await closeWindow();
  });

  await h.step('level-up window: with nothing in the other result the options apply at once', async () => {
    await onOptions();
    await openWhere(rolled(5, 2), 'a 5 and a 2 that gives nothing');
    assert.deepEqual(await pressed(), [], 'nothing is marked');
    assert.ok(!(await s.has('+ other roll')), 'nothing else to take');
    await s.click(OPT, 'Courage +1');
    await s.waitText('Done');
    await s.click(MODAL + ' button', '=Done');
  });

  await h.step('level-up window: the normal option beside a mark applies alone', async () => {
    await onOptions();
    await openWhere(rolled(7, 8), 'a 7 and an 8');
    assert.deepEqual(await pressed(), ['Courage +1']);
    await s.click(OPT, 'Heroic Resolve Mastery');
    await s.waitText('Done');
    const t = await windowText();
    assert.match(t, /Heroic Resolve Mastery/);
    assert.ok(!/Fight|Courage/.test(t), `only that option: ${t}`);
    await s.click(MODAL + ' button', '=Done');
  });

  await h.step('level-up window: both results take the other: a mark in each and Apply both, at every size', async () => {
    await onOptions();
    await openWhere(rolled(5, 5), 'a 5 and a 5');
    assert.deepEqual(await pressed(), ['Courage +1', 'Intelligence +1'], 'Courage in the first, Intelligence in the second');
    assert.deepEqual(await off(), [], 'everything fits');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(s, w, scheme);
      assert.ok(await s.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      await s.page.screenshot({ path: `out/level-both-${w}-${scheme}.png` });
    }
    await layout(s, 1280, 'light');
    for (const scheme of ['dark', 'light']) {
      await layout(s, 1280, scheme);
      assert.deepEqual(await axeScoped(s, ['#modal-body']), [], `axe on the options, ${scheme}`);
    }
    await s.click(MODAL + ' button', '=Apply both');
    await s.waitText('Done');
    const t = await windowText();
    assert.match(t, /Courage/);
    assert.match(t, /Intelligence/);
    await s.click(MODAL + ' button', '=Done');
  });

  await h.step("level-up window: the Warlord's 5 and 7 can take the 7's rule with the 5's stat", async () => {
    await onOptions();
    await openWhere(rolled(5, 7), 'a 5 and a 7');
    await s.click(OPT, 'Heroic Resolve Mastery');
    await s.settle(150);
    assert.ok((await pressed()).includes('Heroic Resolve Mastery'), 'the normal option takes the mark of its result');
    await s.click(MODAL + ' button', '=Apply both');
    await s.waitText('Done');
    const t = await windowText();
    assert.match(t, /Heroic Resolve Mastery/);
    assert.match(t, /Courage/);
    await s.click(MODAL + ' button', '=Done');
  });

  await h.step('level-up window: a stat part of the way has its dots, and the (i) shows its text without choosing it', async () => {
    await onOptions();
    await openWhere(rolled(8, 8), 'an 8 and an 8');
    const dots = await s.page.$eval(OPT, (b) => [...b.querySelectorAll('.level-dots i')].map((d) => d.className));
    assert.ok(dots.includes('level-dot-taken') && dots.includes('level-dot-gain') && dots.includes('level-dot-free'), `taken, gain and free: ${dots}`);
    const colour = (c) => s.page.$eval(`${OPT} .${c}`, (d) => getComputedStyle(d).backgroundColor);
    assert.notEqual(await colour('level-dot-gain'), await colour('level-dot-taken'), 'the gain has its own colour');
    await s.page.click(MODAL + ' .level-info');
    await s.waitText('path limit');
    await s.page.keyboard.press('Escape');
    await s.settle(200);
    assert.ok(await s.has('First roll'), 'nothing was chosen');
    await closeWindow();
  });

  await h.step('level-up window: no change with no roll left closes by itself, 2 seconds after the reveal', async () => {
    await onWaiting();
    await openWhere(async () => /A warrior rolls/.test(await windowText()) && / 1 roll waiting/.test(await windowText()), 'a warrior with one roll');
    await s.select(MODAL + ' select[name=roll]', '2');
    await s.click(MODAL + ' button', '=Roll');
    await s.waitText('No change');
    assert.ok(!(await s.has('Next level up')), 'no roll is left');
    const shown = await s.page.$eval('#modal-body .level-fail', (e) => e.innerText);
    assert.match(shown, /2/);
    await sleep(1200);
    assert.ok(await s.has('No change'), 'it stays a moment');
    await s.waitGone('No change');
    assert.match(s.page.url(), /my-company/);
    assert.ok(await s.has('Galadhrim with experience'), 'back on the company, refreshed');
  });

  await h.step('level-up window: no change with rolls left stays open with Next level up', async () => {
    await onWaiting();
    await s.click('main button.promo', '×3');
    await s.select(MODAL + ' select[name=roll]', '2');
    await s.click(MODAL + ' button', '=Roll');
    await s.waitText('No change');
    assert.ok(await s.has('Next level up'));
    await sleep(2500);
    assert.ok(await s.has('No change'), 'no timer with rolls left');
    await s.click(MODAL + ' button', '=Done');
  });

  await h.step('level-up window: a stored Hero in the Making shows again without confetti; closing decides nothing', async () => {
    await onWaiting();
    await openWhere(async () => s.has('Hero in the Making!'), 'a Hero in the Making');
    assert.equal(await s.page.$$eval(MODAL + ' .confetti', (c) => c.length), 0, 'no confetti when reopened');
    assert.ok(!(await s.page.$(MODAL + ' .path-card')), 'no paths on this screen');
    await s.page.keyboard.press('Escape');
    await s.settle(200);
    await onWaiting();
    await openWhere(async () => s.has('Hero in the Making!'), 'still a Hero in the Making');
    await s.click(MODAL + ' button', '=Accept and close');
    await s.waitGone('Hero in the Making!');
    assert.ok(await s.has('Choose a path'), 'the card asks for the path');
  });

  await h.step('level-up window: a new Hero in the Making has confetti (none under reduced motion) and goes on to the path window', async () => {
    await onWaiting();
    await openWhere(async () => /A warrior rolls/.test(await windowText()) && !!(await s.page.$(MODAL + ' select[name=roll]')), 'a warrior to roll');
    await s.select(MODAL + ' select[name=roll]', '6');
    await s.click(MODAL + ' button', '=Roll');
    await s.waitText('Hero in the Making!');
    assert.equal(await s.page.$$eval(MODAL + ' .confetti i', (c) => c.length), 18, 'confetti the first time');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(s, w, scheme);
      assert.ok(await s.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      await s.page.screenshot({ path: `out/level-hero-${w}-${scheme}.png` });
    }
    await layout(s, 1280, 'light');
    await s.page.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: 'reduce' }]);
    assert.equal(await s.page.$eval(MODAL + ' .confetti', (c) => getComputedStyle(c).display), 'none', 'no confetti under reduced motion');
    await s.page.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: 'no-preference' }]);
    for (const scheme of ['dark', 'light']) {
      await layout(s, 1280, scheme);
      assert.deepEqual(await axeScoped(s, ['#modal-body']), [], `axe on the celebration, ${scheme}`);
    }
    await s.click(MODAL + ' button', '=Choose path now');
    await s.page.waitForSelector(MODAL + ' .path-card', { visible: true });
    await s.waitText('Choose a path');
    await s.click(MODAL + ' .path-strip label', 'Soldier');
    await s.click(MODAL + ' button', 'Take this path');
    await s.waitText('The path is chosen.');
  });

  await h.step('level-up window: opened from a finished aftermath, no change comes back to the aftermath', async () => {
    const bilbo = await h.session('bilbo@example.com');
    const cid = await campaignId(bilbo, 'Aftermath ward 1');
    await bilbo.goto(`/campaigns/${cid}/matches`);
    const matches = await bilbo.page.$$eval('main a[href]', (a) => [...new Set(a.map((x) => x.getAttribute('href')).filter((x) => /\/matches\/\d+$/.test(x)))]);
    let path;
    for (const m of matches) {
      await bilbo.goto(m);
      path = await bilbo.page.evaluate(() => [...document.querySelectorAll('main a[href]')].find((x) => /\/aftermath\/\d+$/.test(x.getAttribute('href')) && x.innerText.includes('Hobbits who finished'))?.getAttribute('href'));
      if (path) break;
    }
    assert.ok(path, 'the finished aftermath of the Hobbits');
    await bilbo.goto(path);
    await bilbo.click('#aftermath-step-2 button', 'Hobbit'); // the warrior with a roll, not the Ranger
    await bilbo.page.waitForSelector(MODAL + ' select[name=roll]', { visible: true });
    await bilbo.select(MODAL + ' select[name=roll]', '2');
    await bilbo.click(MODAL + ' button', '=Roll');
    await bilbo.waitText('No change');
    await bilbo.waitGone('No change');
    assert.match(bilbo.page.url(), /\/aftermath\/\d+$/);
    assert.ok(await bilbo.page.$('#aftermath-done'), 'the aftermath is on the page, not My Company');
    assert.ok(!/Level up\s*Hobbit/.test(await bilbo.page.$eval('#aftermath-step-2', (e) => e.textContent)), 'refreshed: its roll is gone');
  });
}
