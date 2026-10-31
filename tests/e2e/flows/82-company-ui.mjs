// My Company and the armoury: tooltips of profile gear, the buy window's search, the armoury's filters.
import { assert, campaignId, MODAL, layout, SIZES } from '../lib/util.mjs';

// The visible tooltip (role=tooltip) of the page, or null.
const visibleTip = (s) => s.page.evaluate(() => {
  for (const t of document.querySelectorAll('[role=tooltip]')) { const r = t.getBoundingClientRect(); if (r.width && r.height && getComputedStyle(t).display !== 'none') return t.innerText; }
  return null;
});

export default async function (h) {
  const galadriel = await h.session('galadriel@example.com');

  await h.step('my company: the Elven Cloak of a Galadhrim unit has a tooltip with Stalk Unseen and the -1 To Hit', async () => {
    const cid = await campaignId(galadriel, 'Aftermath ward 1');
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    const chip = await galadriel.page.evaluateHandle(() => [...document.querySelectorAll('main li.eq-chip, main ul[aria-label="Unit upgrades"] li')].find((e) => /^Elven Cloak$/i.test(e.innerText.trim())) || null);
    assert.ok(chip.asElement(), 'a unit of this company wears an Elven Cloak');
    const attrs = await chip.evaluate((e) => ({ cls: e.className, title: e.dataset.tipTitle, text: e.dataset.tipText }));
    assert.match(attrs.cls, /has-tip/, 'the chip is marked as having a tooltip');
    assert.equal(attrs.title, 'Elven Cloak');
    assert.match(attrs.text, /Stalk Unseen/);
    assert.match(attrs.text, /To Hit/);
    await chip.asElement().evaluate((e) => e.scrollIntoView({ block: 'center' }));
    await chip.asElement().hover();
    await galadriel.settle(300);
    const tip = await visibleTip(galadriel);
    assert.ok(tip, 'the tooltip is shown on hover');
    assert.match(tip, /Elven Cloak/);
    assert.match(tip, /Stalk Unseen/);
    assert.match(tip, /To Hit/);
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(galadriel, w, scheme);
      const cloak = (await galadriel.page.evaluateHandle(() => [...document.querySelectorAll('main li.eq-chip, main ul[aria-label="Unit upgrades"] li')].find((e) => /^Elven Cloak$/i.test(e.innerText.trim())))).asElement();
      // The viewport changed under a mouse that may already rest on the chip's old place: leave, scroll to it, enter again (a fresh mouseover).
      await galadriel.page.mouse.move(0, 0);
      await cloak.evaluate((e) => e.scrollIntoView({ block: 'center' }));
      await cloak.hover();
      await galadriel.settle(400);
      const box = await galadriel.page.evaluate(() => { const t = [...document.querySelectorAll('[role=tooltip]')].find((x) => x.getBoundingClientRect().width); if (!t) return null; const r = t.getBoundingClientRect(); return [r.left, r.right]; });
      assert.ok(box && box[0] >= -1 && box[1] <= w + 1, `the tooltip fits the viewport at ${w}px ${scheme}: ${box}`);
      await galadriel.page.screenshot({ path: `out/elven-cloak-tip-${w}-${scheme}.png` });
    }
    await layout(galadriel, 1280, 'light');
  });

  await h.step('unit rename: a click on the name opens a focused field across the plate; Enter saves, Escape gives up, a click elsewhere saves, empty is the type', async () => {
    const cid = await campaignId(galadriel, 'Aftermath ward 1');
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    const p = galadriel.page;
    const id = await p.$eval('main article.unit', (e) => e.id.replace('unit-', ''));
    const card = `#unit-${id}`, field = `#rename-${id}`;
    const { original, type } = await p.$eval(field, (e) => ({ original: e.defaultValue, type: e.placeholder }));
    const title = () => p.$eval(`${card} h3`, (e) => e.innerText.trim());
    const open = async () => { await p.click(`${card} .unit-name`); await galadriel.settle(100); };
    const focused = () => p.evaluate((f) => document.activeElement === document.querySelector(f), field);

    const icon = await p.$eval(`${card} .unit-head > .kind-icon`, (e) => { const r = e.getBoundingClientRect(); return [r.width, r.height]; });
    assert.deepEqual(icon, [48, 48], 'the kind icon of the title plate is a 3 rem tile');

    await open();
    assert.ok(await focused(), 'the click puts the cursor in the field');
    const widths = await p.$eval(card, (c, f) => [document.querySelector(f).getBoundingClientRect().width, c.getBoundingClientRect().width], field);
    assert.ok(widths[0] >= widths[1] * 0.6, `the field takes most of the card: ${widths}`);
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(galadriel, w, scheme);
      assert.ok(await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `no horizontal scroll while renaming at ${w}px ${scheme}`);
      await p.$eval(card, (e) => e.scrollIntoView({ block: 'center' }));
      await p.screenshot({ path: `out/unit-rename-${w}-${scheme}.png` });
    }
    await layout(galadriel, 1280, 'light');
    await p.keyboard.type('E2E Hirgon');
    await p.keyboard.press('Enter');
    await galadriel.settle();
    assert.match(await title(), /^E2E Hirgon/, 'Enter saves the name');

    await open();
    await p.keyboard.type('given up');
    await p.keyboard.press('Escape');
    await galadriel.settle(100);
    assert.match(await title(), /^E2E Hirgon/, 'Escape keeps the saved name');
    await open();
    assert.equal(await p.$eval(field, (e) => e.value), 'E2E Hirgon', 'the field opens on the saved name, not on what was given up');

    // Cleared and left with a click elsewhere: saved, and the unit is called by its type (then the seeded name comes back).
    await p.keyboard.press('Backspace');
    const away = await p.$eval('main h1', (e) => { e.scrollIntoView({ block: 'center' }); const r = e.getBoundingClientRect(); return [r.left + 4, r.top + r.height / 2]; });
    await p.mouse.click(away[0], away[1]);
    await galadriel.settle();
    assert.ok((await title()).startsWith(type), 'an empty name, saved by a click elsewhere, is the unit type');
    if (original) { await open(); await p.keyboard.type(original); await p.keyboard.press('Enter'); await galadriel.settle(); }
  });

  await h.step('gear style: "Icons only" in Appearance draws the gear as icon tiles, kept by cookie before the first paint; the name stays for screen readers', async () => {
    const cid = await campaignId(galadriel, 'Aftermath ward 1');
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    const p = galadriel.page;
    await p.evaluate(() => { document.cookie = 'bcm-gear=; Path=/; Max-Age=0'; });
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    const chip = () => p.$eval('main article.unit ul[aria-label=Wargear] li.eq-chip', (c) => { const r = c.getBoundingClientRect(), i = c.querySelector('.eq-ic').getBoundingClientRect(); return { w: r.width, h: r.height, icon: i.width, text: c.innerText.trim() }; });
    const before = await chip();
    assert.equal(before.icon, 22, 'icon and name: a 1.375 rem icon');
    assert.ok(before.w > 60, `icon and name: the chip has the name (${before.w} px)`);
    await p.click('button[aria-label="Appearance settings"]');
    await galadriel.settle(200);
    assert.equal(await p.$eval('[data-gear-option=chips]', (b) => b.getAttribute('aria-pressed')), 'true', 'Icon and name by default');
    await p.click('[data-gear-option=tiles]');
    await galadriel.settle(200);
    assert.equal(await p.evaluate(() => document.documentElement.dataset.gear), 'tiles');
    const tile = await chip();
    assert.deepEqual([tile.w, tile.h, tile.icon], [36, 36, 28], 'icons only: a 2.25 rem tile with a 1.75 rem icon');
    assert.equal(tile.text, before.text, 'the name is still in the chip for screen readers');
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    assert.equal(await p.evaluate(() => document.documentElement.dataset.gear), 'tiles', 'remembered after a reload, set before the first paint');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(galadriel, w, scheme);
      assert.ok(await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `no horizontal scroll with icon tiles at ${w}px ${scheme}`);
      await p.$eval('main article.unit', (e) => e.scrollIntoView({ block: 'start' }));
      await p.screenshot({ path: `out/gear-tiles-${w}-${scheme}.png` });
    }
    await layout(galadriel, 1280, 'light');
    await p.click('button[aria-label="Appearance settings"]');
    await galadriel.settle(200);
    await p.click('[data-gear-option=chips]');
    await galadriel.settle(200);
    assert.equal(await p.evaluate(() => document.documentElement.dataset.gear || ''), '', 'back to icon and name');
    assert.equal((await chip()).icon, 22);
  });

  await h.step('buy window: Find an item filters by name and by alias and says "No item matches your search."', async () => {
    const eowyn = await h.session('eowyn@example.com');
    const cid = await campaignId(eowyn, 'Limits, creatures');
    await eowyn.goto(`/campaigns/${cid}/my-company`);
    await eowyn.click('main button[aria-label^="Buy equipment for"]');
    await eowyn.page.waitForSelector(MODAL + ' #armoury-find', { visible: true });
    const rows = () => eowyn.page.$$eval(MODAL + ' li[data-find]', (l) => l.map((e) => ({ find: e.dataset.find, shown: !e.hidden && e.getBoundingClientRect().height > 0, also: (e.innerText.match(/Also: (.*)/) || [])[1] || '' })));
    const none = () => eowyn.page.evaluate((m) => { const p = [...document.querySelectorAll(m + ' p[role=status]')].find((x) => /No item matches/.test(x.textContent)); return !!p && p.getBoundingClientRect().height > 0 && getComputedStyle(p).display !== 'none'; }, MODAL);
    const all = await rows();
    assert.ok(all.length >= 3 && all.every((r) => r.shown), 'every item is listed before searching');
    assert.ok(!(await none()), 'no empty message before searching');
    // by name (case-insensitive): the rows shown are exactly those whose name or alias contains it
    const word = all[0].find.split(' ')[0].slice(0, 4).toUpperCase();
    await eowyn.fill(MODAL + ' #armoury-find', word);
    await eowyn.settle(200);
    let r = await rows();
    assert.deepEqual(r.map((x) => x.shown), all.map((x) => x.find.toLowerCase().includes(word.toLowerCase())), `search "${word}" shows exactly the matching rows`);
    // by alias: wargear sold under several names (the armour stages) is found by any of them
    await eowyn.page.keyboard.press('Escape');
    await eowyn.settle(300);
    await eowyn.click('main button[aria-label^="Buy wargear for"]');
    await eowyn.page.waitForSelector(MODAL + ' #armoury-find', { visible: true });
    const wrows = await rows();
    const aliased = wrows.find((x) => x.also);
    assert.ok(aliased, 'an item with an alias exists in the wargear list');
    const alias = aliased.also.split(', ')[0];
    await eowyn.fill(MODAL + ' #armoury-find', alias);
    await eowyn.settle(200);
    r = await rows();
    assert.ok(r.find((x) => x.find === aliased.find).shown, `the item is found by its alias "${alias}"`);
    // nothing matches
    await eowyn.fill(MODAL + ' #armoury-find', 'zzzz-nothing');
    await eowyn.settle(300);
    r = await rows();
    assert.ok(r.every((x) => !x.shown), 'no row is shown');
    assert.ok(await none(), 'the "No item matches your search." message shows');
    await eowyn.page.screenshot({ path: 'out/buy-window-no-match.png' });
    // clearing brings everything back and hides the message
    await eowyn.fill(MODAL + ' #armoury-find', '');
    await eowyn.settle(300);
    r = await rows();
    assert.ok(r.every((x) => x.shown), 'clearing the search lists everything again');
    assert.ok(!(await none()), 'the empty message is gone');
  });

  await h.step('armoury: the switch hiding what the company cannot take is on by default and remembered; main categories disable the item chips that do not apply', async () => {
    const pippin = await h.session('pippin@example.com');
    const cid = await campaignId(pippin, 'Limits, creatures');
    const page = pippin.page;
    const sw = () => page.$eval('button[role=switch]', (b) => b.getAttribute('aria-checked'));
    const visible = (sel) => page.$$eval(sel, (l) => l.map((e) => ({ avail: e.dataset.avail, shown: e.getBoundingClientRect().height > 0 })));
    await pippin.goto(`/campaigns/${cid}/armory`);
    await page.evaluate(() => { try { localStorage.clear(); } catch (e) {} });
    await pippin.goto(`/campaigns/${cid}/armory`);
    assert.equal(await sw(), 'true', 'the switch is on by default');
    let items = await visible('li[data-avail]');
    assert.ok(items.some((i) => i.avail === '0') && items.some((i) => i.avail === '1'), 'the list has items the company can and cannot take');
    assert.ok(items.filter((i) => i.avail === '0').every((i) => !i.shown), 'items nobody can take are hidden while the switch is on');
    assert.ok(items.filter((i) => i.avail === '1').every((i) => i.shown), 'items someone can take are shown');
    assert.match(await pippin.text(), /hidden because no one in the company can take them/);
    // off: they show, and the choice survives a reload
    await pippin.click('button[role=switch]');
    assert.equal(await sw(), 'false');
    items = await visible('li[data-avail]');
    assert.ok(items.every((i) => i.shown), 'every item shows with the switch off');
    await pippin.goto(`/campaigns/${cid}/armory`);
    assert.equal(await sw(), 'false', 'off is remembered after a reload');
    await pippin.click('button[role=switch]');
    await pippin.goto(`/campaigns/${cid}/armory`);
    assert.equal(await sw(), 'true', 'on is remembered too');
    // main category chips and the item chips that apply
    const applies = JSON.parse(await page.$eval('[x-data=armory]', (e) => e.dataset.applies));
    const subs = await page.$$eval('[data-sub-chip]', (c) => c.map((x) => x.dataset.subChip));
    const enabled = () => page.$$eval('[data-sub-chip]', (c) => c.filter((x) => !x.disabled).map((x) => x.dataset.subChip));
    assert.deepEqual(await enabled(), subs, 'all item chips are enabled with no main category');
    for (const main of ['wargear', 'equipment']) {
      await pippin.click(`[data-main-chip="${main}"]`);
      assert.deepEqual(await enabled(), subs.filter((k) => (applies[main] || []).includes(k)), `only the chips of ${main} are enabled`);
      assert.ok((await enabled()).length > 0 && (await enabled()).length < subs.length, `${main} enables some, not all`);
    }
    for (const main of ['creatures', 'wanderers']) {
      await pippin.click(`[data-main-chip="${main}"]`);
      assert.deepEqual(await enabled(), [], `${main} have no item categories: every chip is disabled`);
    }
    // a chosen item chip is dropped when a main category it does not belong to is picked
    await pippin.click('[data-main-chip="wargear"]');
    const pick = (await enabled())[0];
    await pippin.click(`[data-sub-chip="${pick}"]`);
    assert.equal(await page.$eval(`[data-sub-chip="${pick}"]`, (c) => c.getAttribute('aria-pressed')), 'true');
    await pippin.click('[data-main-chip="creatures"]');
    assert.equal(await page.$$eval('[data-sub-chip][aria-pressed=true]', (c) => c.length), 0, 'the item chip is cleared');
    await pippin.click('[aria-label="Filter by kind"] button', '=All');
    assert.deepEqual(await enabled(), subs, 'All enables every chip again');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(pippin, w, scheme);
      assert.ok(await pippin.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      await page.screenshot({ path: `out/armoury-filters-${w}-${scheme}.png` });
    }
    await layout(pippin, 1280, 'light');
  });

  await h.step('armoury: creature and wanderer cards carry tooltips on their rule, wargear and action chips', async () => {
    const pippin = h.sessions.find((x) => x.email === 'pippin@example.com');
    const cid = await campaignId(pippin, 'Limits, creatures');
    await pippin.goto(`/campaigns/${cid}/armory`);
    for (const [section, label] of [['Creatures', 'creature'], ['Wanderers', 'wanderer']]) {
      const chips = await pippin.page.$$eval(`section[aria-label=${section}] .has-tip`, (c) => c.map((x) => ({ title: x.dataset.tipTitle, text: x.dataset.tipText, tab: x.getAttribute('tabindex') })));
      assert.ok(chips.length >= 1, `${label} cards have chips with a tooltip`);
      assert.ok(chips.every((c) => c.title && c.text && c.tab === '0'), `${label} tooltips have a title, a text and are focusable`);
      const el = await pippin.page.$(`section[aria-label=${section}] .has-tip`);
      await el.evaluate((e) => e.scrollIntoView({ block: 'center' }));
      await el.hover();
      await pippin.settle(400);
      const tip = await visibleTip(pippin);
      assert.ok(tip && tip.includes(chips[0].title), `hovering a ${label} chip shows its tooltip: ${tip}`);
      await pippin.page.mouse.move(2, 2);
    }
  });
}
