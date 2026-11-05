// The "Choose a path" window is a carousel (GitHub issue 63; docs/frontend-level-up.md, "Path carousel"): every path on one strip (a radio group, in rows by colour), exactly one selected
// and shown below with its details, the floating previous/next arrows, the arrow keys, the first magical power of a Sorcerer between the rows and the button, no horizontal scroll
// in the 360 x 640 window, axe clean in every look. Eowyn's Rohan company in "Companies being built" has officers without a path.
import { assert, campaignId, MODAL } from '../lib/util.mjs';
import { axeScoped, eachLook, resetLook, setLook } from '../lib/axe.mjs';

const STRIP = MODAL + ' [role=radiogroup]';
const picked = (s) => s.page.$eval(MODAL + ' input[name=path]:checked', (r) => r.value);
const shown = (s) => s.page.$$eval(MODAL + ' .path-card[data-rel="0"]', (e) => e.map((x) => x.querySelector('h3').innerText.trim()));
const short = (n) => n.replace(/^Path of the /, '');

export default async function (h, { only }) {
  const s = await h.session('eowyn@example.com', { height: 900 });
  s.allow.push(/422/);
  let cid;
  const open = async () => {
    cid ||= await campaignId(s, 'Companies being built');
    await s.goto(`/campaigns/${cid}/my-company`);
    await s.click('main button.promo', 'Choose a path');
    await s.page.waitForSelector(STRIP, { visible: true });
    await s.settle(400);
  };
  const names = () => s.page.$$eval(STRIP + ' input', (r) => r.map((x) => x.value));

  const mosaic = async () => {
    // the mosaic: every path button is visible at once, no scrolling, the icon and the name on one line
    const mo = await s.page.$eval(STRIP, (e) => { const b = e.getBoundingClientRect(); const tabs = [...e.querySelectorAll('label')].map((l) => { const r = l.getBoundingClientRect(), i = l.querySelector('.path-icon').getBoundingClientRect(), t = l.querySelector('.path-tab-name').getBoundingClientRect(); return { l: r.left, r: r.right, t: r.top, b: r.bottom, sameLine: Math.abs((i.top + i.bottom) / 2 - (t.top + t.bottom) / 2) <= Math.max(i.height, t.height) / 2 }; }); return { scroll: e.scrollWidth > e.clientWidth + 1, right: b.right, tabs, vw: window.innerWidth, cols: new Set(tabs.map((x) => Math.round(x.l))).size }; });
    assert.ok(!mo.scroll && mo.tabs.every((x) => x.r <= mo.vw + 1 && x.l >= -1 && x.r <= mo.right + 1), 'all the path buttons are visible without scrolling');
    assert.ok(mo.cols >= 2, 'a grid of several columns');
    assert.ok(mo.tabs.every((x) => x.sameLine), 'the icon and the name are on the same line');
    assert.ok(mo.tabs.some((x, i) => i && x.t > mo.tabs[0].b - 1), 'the buttons wrap into rows');
  };

  await h.step('path carousel: the paths are in rows by colour, each opened by its icon, with the button right under them', async () => {
    await open();
    const rows = await s.page.$$eval(STRIP + ' > [role=group]', (e) => e.map((r) => { const i = r.querySelector('.path-row-icon'), u = i.querySelector('use'); return { label: r.getAttribute('aria-label'), icon: u.getAttribute('href').split('#').pop(), hidden: i.getAttribute('aria-hidden'), interactive: !!i.closest('label, button, a') || getComputedStyle(i).cursor === 'pointer', radius: parseFloat(getComputedStyle(i).borderTopLeftRadius), w: i.getBoundingClientRect().width, paths: [...r.querySelectorAll('input')].map((x) => x.value.replace(/^Path of the /, '')), kinds: [...r.querySelectorAll('label')].map((l) => [...l.classList].find((c) => c.startsWith('path-k-'))) }; }));
    assert.deepEqual(rows.map((r) => r.paths), [['Warlord', 'Shieldbearer', 'Tactician', 'Sorcerer'], ['Soldier', 'Claimant', 'Berserker', 'Duelist'], ['Sharpshooter', 'Marksman', 'Ranger']], 'the exact rows');
    assert.deepEqual(rows.map((r) => r.icon), ['horn', 'sword-brandish', 'high-shot'], 'the icons of the rows');
    assert.deepEqual(rows.map((r) => r.label), ['Command paths', 'Melee paths', 'Ranged paths'], 'each row is a labelled group');
    assert.ok(rows.every((r) => r.hidden === 'true' && !r.interactive), 'the row icon is hidden from assistive tech and not interactive');
    assert.ok(rows.every((r) => r.w >= 34 && r.radius < r.w / 2), 'a rounded square, not a round badge');
    assert.deepEqual(rows[0].kinds, ['path-k-support', 'path-k-support', 'path-k-support', 'path-k-will'], 'the box and the tab take the main category: the Tactician stays amber, the Sorcerer blue');
    assert.deepEqual(await s.page.$$eval(MODAL + ' .path-card-body', (e) => e.map((b) => b.dataset.path)), await s.page.$$eval(MODAL + ' input[name=path]', (r) => r.map((x) => x.value)), 'the stack has the order of the radios');
    assert.equal(await picked(s), 'Path of the Warlord', 'the first selection is the first of the new order');
    const hybrid = await s.page.$$eval(STRIP + ' label', (l) => l.filter((x) => x.querySelector('.path-icon-hybrid')).map((x) => x.querySelector('input').value));
    assert.deepEqual(hybrid, ['Path of the Tactician', 'Path of the Duelist', 'Path of the Ranger'], 'only the paths with two categories have a split badge');
    assert.match(await s.page.$eval(STRIP + ' .path-icon-hybrid', (e) => getComputedStyle(e).backgroundImage), /linear-gradient/, 'the split shows');
    // the Tactician's badge: the amber of the Warlord top-left, the blue of the Sorcerer bottom-right
    const badge = (path) => s.page.$eval(`${STRIP} label:has(input[value="${path}"]) .path-icon`, (e) => { const cs = getComputedStyle(e); return { bg: cs.backgroundColor, img: cs.backgroundImage }; });
    const stops = (await badge('Path of the Tactician')).img.match(/rgba?\([^)]*\)|color\([^)]*\)/g);
    assert.equal(stops[0], (await badge('Path of the Warlord')).bg, 'the Tactician starts amber');
    assert.equal(stops[stops.length - 1], (await badge('Path of the Sorcerer')).bg, 'and ends blue');
    // the order of the window: the rows, the power block for a Sorcerer, the button, the stack
    const under = () => s.page.evaluate((m) => { const top = (q) => { const e = document.querySelector(m + ' ' + q); return e && e.getClientRects().length ? e.getBoundingClientRect().top : null; }; const strip = document.querySelector(m + ' [role=radiogroup]').getBoundingClientRect().bottom; return { strip, power: top('#power-open'), button: top('form > button.btn-primary'), deck: top('.path-deck') }; }, MODAL);
    let at = await under();
    assert.equal(at.power, null, 'no power block for another path');
    assert.ok(at.strip <= at.button && at.button < at.deck, 'the button is right under the rows, above the stack');
    await s.click(STRIP + ' label', 'Sorcerer');
    await s.page.waitForSelector('#power-open', { visible: true });
    at = await under();
    assert.ok(at.strip <= at.power && at.power < at.button && at.button < at.deck, 'the power block is between the rows and the button');
    // the arrow keys cross rows
    await s.page.focus(STRIP + ' input:checked');
    await s.page.keyboard.press('ArrowRight');
    await s.settle(600);
    assert.equal(await picked(s), 'Path of the Soldier', 'the arrow key goes from the end of a row to the next one');
    assert.deepEqual(await shown(s), ['Path of the Soldier'], 'and the front card follows');
    assert.equal(await s.page.$$eval(MODAL + ' #power-open', (e) => e.filter((x) => x.getClientRects().length).length), 0, 'another path hides the power block');
  }, { only });

  await h.step('path carousel: every path is on the strip, one is selected and its details show', async () => {
    await open();
    assert.ok((await names()).length >= 4, 'several paths on the strip');
    assert.equal(await s.page.$$eval(MODAL + ' input[name=path]:checked', (r) => r.length), 1, 'exactly one selected');
    assert.deepEqual(await shown(s), [await picked(s)], 'only the details of the selection');
    assert.equal(await s.page.$eval(STRIP, (e) => e.getAttribute('aria-label')), 'Paths', 'a labelled radio group');
    // the front card has the whole path: limits, level-up rows with their dice, Might, Will and Fate on their own row
    const front = await s.page.$eval(MODAL + ' .path-card[data-rel="0"]', (e) => ({ limits: e.querySelectorAll('.path-limits dl:not(.statline-hero) > div').length, hero: e.querySelectorAll('.path-limits dl.statline-hero > div').length, alts: e.querySelectorAll('.path-or').length, rows: e.querySelectorAll('.path-rows li').length, text: e.innerText }));
    assert.ok(front.limits >= 3, 'the limits of the characteristics');
    const rows = await s.page.$eval(MODAL + ' .path-card[data-rel="0"] .path-limits', (e) => [...e.children].map((c) => { const r = c.getBoundingClientRect(); return { cls: c.className, top: r.top, bottom: r.bottom }; }));
    await mosaic();
    assert.equal(rows.length, 2, 'exactly two statlines');
    assert.ok(rows[1].top >= rows[0].bottom - 1 && /statline-hero/.test(rows[1].cls), 'Might, Will and Fate stack under the profile limits');
    assert.ok(await s.page.$eval(MODAL + ' .path-card[data-rel="0"] .path-rows li', (e) => parseFloat(getComputedStyle(e).borderTopWidth) >= 1), 'every roll of the chart has a border');
    const fab = await s.page.$$eval(MODAL + ' button[data-float]', (b) => b.map((x) => ({ label: x.getAttribute('aria-label'), dis: x.disabled, w: x.getBoundingClientRect().width, r: x.getBoundingClientRect() })));
    assert.deepEqual(fab.map((b) => b.label), ['Previous path', 'Next path'], 'floating arrows');
    assert.ok(fab.every((b) => b.w >= 44), '44 px targets');
    assert.ok(fab[0].dis, 'previous is off on the first path, as at the end of the list');
    assert.ok(await s.page.$eval(MODAL + ' .path-card[data-rel="0"] .path-card-body', (e) => { const f = [...document.querySelectorAll('button[data-float]')].map((b) => b.getBoundingClientRect()), c = e.getBoundingClientRect(); return f[0].right <= c.left + 12 && f[1].left >= c.right - 12; }), 'the arrows overlap only the margin of the card, not its text');
    assert.ok(await s.page.$eval(MODAL + ' .path-card[data-rel="1"]', (e) => /blur/.test(getComputedStyle(e).filter)), 'the cards behind are blurred');
    assert.ok(front.hero >= 1, 'Might, Will and Fate on their own row, as on a hero card');
    assert.ok(!/\bMastery\b\s*\n.*Rule/i.test(front.text) && !(await s.page.$(MODAL + ' .path-card[data-rel="0"] .path-facts dt:not(:first-child)')), 'no separate Mastery or rule sections');
    assert.ok(front.rows >= 6, 'the level-up chart');
    assert.match(front.text, /Level-up: roll/i, 'with its dice');
    // the cards behind are hidden from assistive tech and from the keyboard
    const behind = await s.page.$$eval(MODAL + ' .path-card:not([data-rel="0"])', (e) => e.map((x) => [x.getAttribute('aria-hidden'), x.querySelector('.path-card-body').inert]));
    assert.ok(behind.length && behind.every(([a, i]) => a === 'true' && i === true), 'the others are aria-hidden and inert');
    assert.equal(await s.page.$$eval(MODAL + ' .path-card[data-rel="1"]', (e) => e.length), 1, 'the next card sits behind the front one');
  }, { only });

  await h.step('path carousel: clicking a path selects it, the side buttons and the arrow keys move', async () => {
    await open();
    const n = await names();
    await s.click(STRIP + ' label', short(n[2]));
    assert.equal(await picked(s), n[2]);
    await s.click(MODAL + ' button[data-float]', '=Previous path');
    assert.equal(await picked(s), n[1], 'previous');
    await s.click(MODAL + ' button[data-float]', '=Next path');
    await s.click(MODAL + ' button[data-float]', '=Next path');
    assert.equal(await picked(s), n[3], 'next twice');
    await s.page.focus(STRIP + ' input:checked');
    await s.page.keyboard.press('ArrowLeft');
    await s.settle(200);
    assert.equal(await picked(s), n[2], 'arrow left');
    await s.page.keyboard.press('ArrowRight');
    await s.settle(600);
    assert.equal(await picked(s), n[3], 'arrow right');
    assert.deepEqual(await shown(s), [n[3]], 'the front card follows the selection');
    // the floating arrows stay in the window when the card is scrolled and move the selection like the top ones
    await s.page.evaluate(() => { const m = document.querySelector('[role=dialog]').parentElement; m.scrollTop = 200; });
    await s.settle(200);
    assert.ok(await s.page.$eval(MODAL + ' button[data-float]', (b) => { const r = b.getBoundingClientRect(); return r.top >= 0 && r.bottom <= window.innerHeight; }), 'a floating arrow is in the window');
    await s.click(MODAL + ' button[data-float]', '=Previous path');
    assert.equal(await picked(s), n[2], 'the floating previous button');
    await s.click(MODAL + ' button[data-float]', '=Next path');
    assert.equal(await picked(s), n[3], 'the floating next button');
    // a click on the card behind selects it
    const peek = await s.page.$eval(MODAL + ' .path-card[data-rel="1"]', (e) => { const r = e.closest('.path-stage').getBoundingClientRect(); return [r.right - 5, r.top + 40]; });
    await s.page.mouse.click(peek[0], peek[1]);
    await s.settle(500);
    assert.equal(await picked(s), n[4], 'a card behind selects when clicked');
    assert.ok(await s.page.$eval(STRIP, (e) => e.querySelector('input:checked').closest('label').classList.contains('is-on')), 'the selected button is marked');
  }, { only });

  await h.step('path carousel: a jump of five paths plays one proportional, capped slide and reduced motion plays none', async () => {
    await open();
    const n = await names();
    await s.click(STRIP + ' label', short(n[5]));
    assert.equal(await picked(s), n[5], 'five steps away');
    assert.equal(await s.page.$eval(MODAL + ' .path-stage', (e) => getComputedStyle(e).getPropertyValue('--dur').trim()), '600ms', 'the slide is proportional to the distance, at most 0.6 s');
    await s.settle(900);
    assert.deepEqual(await shown(s), [n[5]], 'the selected card is in front');
    assert.equal(await s.page.$$eval(MODAL + ' .path-stage.is-moving', (e) => e.length), 0, 'the others stop sweeping');
    await s.click(STRIP + ' label', short(n[4]));
    assert.equal(await s.page.$eval(MODAL + ' .path-stage', (e) => getComputedStyle(e).getPropertyValue('--dur').trim()), '280ms', 'a neighbour is a short slide');
    await s.page.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: 'reduce' }]);
    await s.click(STRIP + ' label', short(n[0]));
    assert.equal(await s.page.$eval(MODAL + ' .path-stage', (e) => getComputedStyle(e).getPropertyValue('--dur').trim()), '0ms', 'no animation under reduced motion');
    assert.ok(parseFloat(await s.page.$eval(MODAL + ' .path-card', (e) => getComputedStyle(e).transitionDuration)) < 0.001, 'no transition');
    await s.page.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: 'no-preference' }]);
  }, { only });

  const POWERS = MODAL + ' [aria-labelledby="power-title"]';
  const openPowers = async () => {
    await s.click('#power-open', '');
    await s.page.waitForSelector(POWERS, { visible: true });
    await s.settle(200);
  };

  await h.step('path carousel: a Sorcerer opens the list of powers, every one in full, and the choice fills the panel', async () => {
    await open();
    const sorc = 'Path of the Sorcerer';
    await s.click(STRIP + ' label', 'Sorcerer');
    assert.equal(await picked(s), sorc);
    await s.page.waitForSelector('#power-open', { visible: true });
    assert.match(await s.page.$eval('#power-open', (e) => e.innerText), /Select magical power/);
    assert.ok(await s.page.$$eval(MODAL + ' form > button.btn-primary', (b) => b.every((x) => x.disabled)), 'no path without a power for a Sorcerer');
    await openPowers();
    const list = await s.page.$$eval(POWERS + ' label', (l) => l.map((x) => { const t = x.querySelector('.power-text'); return { text: t.innerText.trim(), links: t.querySelectorAll('a.term-link').length, clamp: getComputedStyle(t).webkitLineClamp, cut: t.scrollHeight > t.clientHeight + 1, name: x.querySelector('.font-medium').innerText.trim() }; }));
    assert.ok(list.length >= 3, 'every power of the Sorcerer is listed');
    assert.ok(list.every((p) => p.text.length > 40 && p.clamp === 'none' && !p.cut && !/(\.\.\.|…)$/.test(p.text)), 'each description is whole, never cut');
    assert.ok(list.some((p) => p.links > 0), 'terms are linked');
    assert.equal(await s.page.$$eval(POWERS + ' input[type=radio][name=power]', (r) => r.length), list.length, 'real radio inputs');
    assert.equal(await s.page.$eval(POWERS + ' [role=radiogroup]', (e) => !!e.getAttribute('aria-labelledby')), true);
    // the focus is trapped in the list; Escape closes only the list
    for (let i = 0; i < 40; i++) await s.page.keyboard.press('Tab');
    assert.ok(await s.page.evaluate((c) => !!document.activeElement.closest(c), '[aria-labelledby="power-title"]'), 'the focus stays in the list');
    await s.page.keyboard.press('Escape');
    await s.settle(300);
    assert.ok(await s.page.$(MODAL + ' [name=path]'), 'the path window is still open');
    assert.ok(!(await s.page.$eval(POWERS, (e) => e.getClientRects().length > 0)), 'the list is closed');
    assert.equal(await s.page.evaluate(() => document.activeElement.id), 'power-open', 'the focus is back on the button');
    // a click on a power chooses it and closes the list
    await openPowers();
    await s.page.click(POWERS + ' label:nth-of-type(2) .font-medium');
    await s.settle(300);
    assert.ok(!(await s.page.$eval(POWERS, (e) => e.getClientRects().length > 0)), 'a click closes the list');
    assert.equal(await s.page.$eval('#power-chosen', (e) => e.innerText.trim()), list[1].name, 'the panel shows the chosen power');
    assert.match(await s.page.$eval('#power-open', (e) => e.innerText), /Change/);
    // the arrow keys move through the list without closing it
    await openPowers();
    await s.page.focus(POWERS + ' input:checked');
    await s.page.keyboard.press('ArrowDown');
    await s.settle(200);
    assert.ok(await s.page.$eval(POWERS, (e) => e.getClientRects().length > 0), 'an arrow key does not close the list');
    await s.page.keyboard.press('Enter');
    await s.settle(300);
    assert.ok(await s.page.$$eval(MODAL + ' form > button.btn-primary', (b) => b.every((x) => !x.disabled)), 'the path can be taken now');
    const other = (await names()).find((v) => v !== sorc);
    await s.click(STRIP + ' label', short(other));
    await s.settle(200);
    assert.ok(!(await s.page.$eval('#power-open', (e) => e.getClientRects().length > 0)), 'the power is only for the Sorcerer');
  }, { only });

  await h.step('path carousel: taking the Sorcerer path with a chosen power assigns both', async () => {
    await open();
    await s.click(STRIP + ' label', 'Sorcerer');
    await openPowers();
    const name = await s.page.$eval(POWERS + ' label:nth-of-type(3) .font-medium', (e) => e.innerText.trim());
    await s.page.click(POWERS + ' label:nth-of-type(3) .font-medium'); // its name: the description has glossary links, and a click on one opens the term instead
    await s.settle(300);
    await s.click(MODAL + ' form > button.btn-primary', 'Take this path');
    await s.waitText('The path is chosen');
    assert.ok(await s.has('Sorcerer'), 'the officer is a Sorcerer');
    assert.ok(await s.has(name.split(/[ ·(]/)[0]), 'with its first power');
  }, { only });

  await eachLook(h, 'path carousel', only, async (w, scheme, contrast, axeHere) => {
    await setLook(s, w, scheme, contrast);
    if (w === 360) await s.page.setViewport({ width: 360, height: 640 });
    await open();
    assert.ok(await s.noHScroll(), 'no horizontal scroll');
    await mosaic();
    await s.page.screenshot({ path: `out/path-window-${w}-${scheme}${contrast === 'high' ? '-hc' : ''}.png` });
    // the row icons reach 3:1 against their tile, in every look; the rows are 4 aligned columns where they fit and 2 at 360 px
    const tiles = await s.page.$$eval(STRIP + ' .path-row-icon', (e) => { const rgb = (c) => { const x = document.createElement('canvas').getContext('2d', { willReadFrequently: true }); x.fillStyle = c; x.fillRect(0, 0, 1, 1); return [...x.getImageData(0, 0, 1, 1).data].slice(0, 3); }; const lum = (c) => { const [r, g, b] = rgb(c).map((v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; }; return e.map((i) => { const st = getComputedStyle(i), a = lum(st.color), b = lum(st.backgroundColor), hi = Math.max(a, b), lo = Math.min(a, b); return (hi + 0.05) / (lo + 0.05); }); });
    assert.ok(tiles.every((c) => c >= 3), `the row icons reach 3:1: ${tiles}`);
    const cols = await s.page.$$eval(STRIP + ' .path-row-cells', (e) => e.map((c) => getComputedStyle(c).gridTemplateColumns.split(' ').length));
    if (w === 360) assert.deepEqual(cols, [2, 2, 2], '2 columns in a narrow window');
    if (w === 1280) assert.deepEqual(cols, [4, 4, 4], '4 columns where they fit');
    assert.ok(await s.page.$eval(MODAL, (e) => e.scrollWidth <= e.clientWidth + 1), 'no horizontal scroll in the window');
    assert.ok(await s.page.evaluate(() => !!document.activeElement && document.activeElement.closest('[role=dialog]') !== null), 'focus is inside the window');
    if (axeHere) assert.deepEqual(await axeScoped(s, [MODAL]), [], 'axe on the window');
    if (axeHere && await s.page.$('.path-card')) { // the list of powers of a Sorcerer, in this look
      await s.click(STRIP + ' label', 'Sorcerer');
      await openPowers();
      assert.ok(await s.noHScroll(), 'no horizontal scroll with the list open');
      assert.deepEqual(await axeScoped(s, [POWERS]), [], 'axe on the list of powers');
      await s.page.keyboard.press('Escape');
    }
  });

  await h.step('path carousel: Enter takes the selected path, which is final', async () => {
    await resetLook(s);
    await open();
    await s.click(MODAL + ' button[data-float]', '=Next path');
    const want = await picked(s);
    await s.page.focus(STRIP + ' input:checked');
    await s.page.keyboard.press('Enter');
    await s.waitText('The path is chosen');
    assert.ok(await s.has(short(want)), 'the officer shows its path');
  }, { only });
}
