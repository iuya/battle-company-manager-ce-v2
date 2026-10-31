// Units On Their Own Path, and officers keeping their profile (the hero promotion is a level-up result).
// 1. The demo campaign with everything on: the settings text, a Wood Elf Sentinel that is a plain warrior with XP and a Level up
//    button, a typed 6 that makes it a hero on the Path of Their Own Making at once (no path window, no forgo offer).
// 2. The Take command preview names the setting's effect on the badge, and an Arnor officer stays a Ranger of Arnor.
import { assert, campaignId, MODAL, SIZES, layout, numberAfter } from '../lib/util.mjs';

export default async function (h) {
  const galadriel = await h.session('galadriel@example.com');
  const organiser = h.sessions.find((s) => s.email === 'you@example.com') || await h.session('you@example.com', { height: 1800 });
  let cid;
  const cardOf = (s, title) => s.page.$$eval('main .unit-slot', (slots, t) => slots.filter((u) => u.querySelector('.unit-handle .truncate')?.textContent.includes(t) || u.innerText.includes(t)).map((u) => u.innerText.replace(/\s+/g, ' ')), title);

  await h.step('own path: the campaign overview says those units are warriors that become heroes on a 6', async () => {
    cid = await campaignId(galadriel, 'Settings: everything on');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(galadriel, w, scheme);
      await galadriel.goto(`/campaigns/${cid}`);
      const text = await galadriel.text();
      assert.match(text, /warriors that earn experience and level up; on a roll of 6 they become heroes/);
      assert.ok(!/start as heroes/.test(text), 'no more "start as heroes"');
      assert.ok(await galadriel.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
    }
    await layout(galadriel, 1280, 'light');
  });
  if (!cid) return;

  let sentinel;
  await h.step('own path: a Sentinel is a plain warrior with XP and a level-up waiting, until it rolls a 6', async () => {
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    const cards = (await cardOf(galadriel, 'Wood Elf Sentinel')).filter((t) => /Level Up ready/.test(t));
    assert.ok(cards.length >= 1, 'a Sentinel has its level-up waiting');
    sentinel = cards[0];
    assert.ok(/XP/.test(sentinel), 'the card shows experience');
    assert.ok(!/no experience/.test(sentinel), 'it is not "on its own path: no experience"');
  });

  await h.step('own path: a typed 6 makes it a hero on the Path of Their Own Making at once, with no path window or forgo offer', async () => {
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    await galadriel.click('main button', 'Level Up');
    await galadriel.select(MODAL + ' select[name=roll]', '6');
    await galadriel.click(MODAL + ' button', 'Roll');
    await galadriel.waitText('Path of Their Own Making');
    const dialog = await galadriel.page.$eval('#modal-body', (d) => d.innerText);
    assert.match(dialog, /You rolled a\s*6/);
    assert.match(dialog, /becomes a hero on the Path of Their Own Making/);
    assert.ok(!/Re-roll|Forgo|Choose its path/i.test(dialog), 'no re-roll, forgo or path choice');
    assert.equal(await galadriel.page.$$eval('#modal-body form[action*="levelup/choose"]', (f) => f.length), 0, 'no choose form');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(galadriel, w, scheme);
      assert.ok(await galadriel.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
    }
    await layout(galadriel, 1280, 'light');
    await galadriel.click(MODAL + ' button', '=Done');
    await galadriel.goto(`/campaigns/${cid}/my-company`);
    const heroes = (await cardOf(galadriel, 'Wood Elf Sentinel')).filter((t) => /their own making/i.test(t) && /fate/i.test(t));
    assert.ok(heroes.length >= 2, `the Sentinel that rolled the 6 and the one the demo made are heroes on the path (${heroes.length})`);
  });

  await h.step('own path: Take command wording follows the campaign setting', async () => {
    await organiser.goto(`/campaigns/${cid}/company/new`);
    if (new URL(organiser.page.url()).pathname.endsWith('/company/new')) {
      await organiser.click('nav[aria-label=Companies] button[data-name="Lothlorien"]');
      await organiser.page.waitForSelector('#take-command');
      const titles = await organiser.page.$$eval('#company-info .badge[title^="On Their Own Path"]', (b) => b.map((x) => x.title));
      assert.ok(titles.length > 0 && titles.every((t) => /earns experience/.test(t)), `badges say they earn experience with the setting (${titles.length})`);
      assert.match(await organiser.page.$eval('#company-info', (e) => e.innerText), /warriors that earn experience in this campaign/);
      assert.ok(await organiser.page.$('#company-info a[href$="/glossary#t-path-of-their-own-making"]'), 'the note links the Path of Their Own Making');
    }
    const plain = await campaignId(organiser, 'Aftermath ward 1').catch(() => null);
    if (plain) {
      await organiser.goto(`/campaigns/${plain}/company/new`);
      if (new URL(organiser.page.url()).pathname.endsWith('/company/new')) {
        await organiser.click('nav[aria-label=Companies] button[data-name="Lothlorien"]');
        await organiser.page.waitForSelector('#take-command');
        const titles = await organiser.page.$$eval('#company-info .badge[title^="On Their Own Path"]', (b) => b.map((x) => x.title));
        assert.ok(titles.every((t) => /never becomes a hero and earns no experience/.test(t)), 'without the setting they earn none');
      }
    }
  });

  // Arnor, in a throwaway campaign of its own.
  const arnor = await h.session('aragorn@example.com', { height: 1800 });
  const name = `E2E Arnor ${Date.now() % 100000}`;
  let tid;
  await h.step('arnor creation: a throwaway campaign and a player joining by the link', async () => {
    await organiser.goto('/campaigns/new');
    await organiser.fill('input[name=name]', name);
    await organiser.fill('textarea[name=description]', 'Throwaway campaign of the Arnor officer preview.');
    await organiser.click('button', 'Create campaign', { nav: true });
    tid = await campaignId(organiser, name);
    await organiser.goto(`/campaigns/${tid}`);
    await organiser.click('button', 'Invite players');
    await organiser.waitText('Invitation link');
    const link = await organiser.page.evaluate(() => (document.body.innerHTML.match(/\/join\/[A-Za-z0-9_-]{20,}/) || [])[0]);
    assert.ok(link, 'the invitation link is on the page');
    await arnor.goto(link);
    await arnor.waitText(name);
    await arnor.click('button', 'Join', { nav: true });
  });
  if (!tid) return;

  await h.step('arnor creation: an officer stays a Ranger of Arnor in the preview (no promotion at creation)', async () => {
    await layout(arnor, 1280, 'light', 1800);
    await arnor.goto(`/campaigns/${tid}/company/new`);
    await arnor.click('nav[aria-label=Companies] button[data-name="Arnor"]');
    await arnor.page.waitForSelector('#take-command');
    // the suggestion is three Warriors of Arnor: send them back and put three Rangers of Arnor in the places with the card buttons
    await arnor.page.evaluate(() => document.querySelectorAll('.officer-slot .start-card .slot-only.btn').forEach((b) => b.click()));
    await arnor.settle(300);
    for (const role of ['Leader', 'Sergeant', 'Sergeant']) {
      await arnor.page.evaluate((r) => {
        const card = [...document.querySelectorAll('#start-pool .start-card')].find((c) => c.dataset.unit === 'Ranger of Arnor');
        [...card.querySelectorAll('button')].find((b) => b.innerText.trim() === r).click();
      }, role);
      await arnor.settle(250);
    }
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(arnor, w, scheme, 1800);
      await arnor.settle(300);
      const slots = await arnor.page.$$eval('.officer-slot .start-card', (cards) => cards.map((c) => ({
        unit: c.dataset.unit, becomes: c.dataset.becomes,
        shown: [...c.querySelectorAll('p')].filter((p) => p.offsetParent !== null).map((p) => p.innerText.trim()).join(' | '),
      })));
      assert.equal(slots.length, 3, 'three officers are in the places');
      for (const s of slots) {
        assert.equal(s.unit, 'Ranger of Arnor');
        assert.equal(s.becomes, undefined, 'no promotion is previewed');
        assert.match(s.shown, /Ranger of Arnor/);
        assert.ok(!/Ranger of the North|as an officer/.test(s.shown), `an officer is not shown as a Ranger of the North (${s.shown})`);
      }
      assert.ok(await arnor.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
    }
    await layout(arnor, 1280, 'light', 1800);
  });

  await h.step('arnor creation: the confirmation and the company page name the Ranger of Arnor, and 101 points and 4 gold are 105', async () => {
    await arnor.goto(`/campaigns/${tid}/company/new`);
    await arnor.click('nav[aria-label=Companies] button[data-name="Arnor"]');
    await arnor.page.waitForSelector('#take-command');
    await arnor.page.evaluate(() => document.querySelectorAll('.officer-slot .start-card .slot-only.btn').forEach((b) => b.click()));
    await arnor.settle(300);
    for (const role of ['Leader', 'Sergeant', 'Sergeant']) {
      await arnor.page.evaluate((r) => {
        const card = [...document.querySelectorAll('#start-pool .start-card')].find((c) => c.dataset.unit === 'Ranger of Arnor');
        [...card.querySelectorAll('button')].find((b) => b.innerText.trim() === r).click();
      }, role);
      await arnor.settle(250);
    }
    await arnor.fill('#name', 'Rangers of the E2E');
    await arnor.click('#create-company button.btn-primary', '');
    const text = await arnor.page.$eval('#create-company dialog', (d) => d.innerText);
    assert.match(text, /Ranger of Arnor as its leader/);
    assert.ok(!/Ranger of the North/.test(text), 'the dialog does not promote an officer');
    await arnor.click('dialog[open] button', '=Yes', { nav: true });
    await arnor.waitText('Rangers of the E2E');
    const titles = await arnor.page.$$eval('main .unit-slot', (slots) => slots.map((u) => `${(u.querySelector('article.unit').className.match(/unit-(leader|sergeant|hero|warrior)/) || [])[1]}:${u.querySelector('.unit-handle .truncate').textContent.trim()}`));
    const officers = titles.filter((t) => /^(leader|sergeant):/.test(t));
    assert.equal(officers.length, 3);
    assert.ok(officers.every((t) => /Ranger of Arnor/.test(t) && !/Ranger of the North/.test(t)), `the officers are Rangers of Arnor (${officers.join(', ')})`);
    const points = await arnor.page.$$eval('main .unit-slot', (slots) => slots.map((u) => Number((u.querySelector('article.unit').innerText.match(/(\d+)\s*pts/) || [])[1])));
    const total = points.reduce((a, n) => a + n, 0);
    assert.equal(total, 101, `three Rangers of Arnor officers (8 + 15 each) and four more models are 101 points (${points.join('+')})`);
    assert.equal(total + numberAfter(await arnor.text(), 'Gold'), 105, 'points plus starting gold are 105');
  });
}
