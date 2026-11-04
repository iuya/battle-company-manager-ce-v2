// Moving a campaign to the app's current rules files (docs/admin.md, "Rules files of a campaign"): the organiser opens the Rules card of a
// campaign pinned to the previous edition of the Battle Companies file, checks the update (every company works, what changes is listed) and
// moves it; the card then has nothing to offer. A campaign whose company carries something the current file dropped shows what blocks and
// offers no move. A campaign with a file of its own asks for the explicit switch. A player has no card. The holds at 360 px.
import { assert, campaignId, layout } from '../lib/util.mjs';

const CARD = '[data-rules-update]';

export default async function (h) {
  const you = await h.session('you@example.com', { height: 1000 });
  let ok, blocked, forked;

  await h.step('rules update: the organiser checks a campaign on the previous edition and sees every company work', async () => {
    ok = await campaignId(you, 'Older edition: move to the current files');
    blocked = await campaignId(you, 'Older edition: the update is blocked');
    forked = await campaignId(you, 'Pinned to another edition');
    await you.goto(`/campaigns/${ok}`);
    await you.page.waitForSelector(`${CARD} button`, { timeout: 8000 });
    assert.ok(await you.has('Check the update'), 'the Rules card offers the check');
    await you.page.click(`${CARD} button`);
    await you.page.waitForSelector(`${CARD} [data-clean]`, { timeout: 15000 });
    const text = await you.page.$eval(CARD, (e) => e.textContent);
    assert.match(text, /Every company works with the current files/);
    assert.match(text, /Heralds of the older edition/);
    assert.match(text, /Company points 108 → 109/, 'the report names what the files change (the Healing Herbs are a point dearer)');
    assert.ok(await you.page.$(`${CARD} form button.btn-primary`), 'the move button is there');
  });

  await h.step('rules update: moving the campaign leaves the Rules card with nothing to offer', async () => {
    await you.page.click(`${CARD} form button.btn-primary`);
    await you.page.waitForFunction(() => !document.querySelector('[data-rules-update]'), { timeout: 15000 });
    assert.ok(!(await you.page.$(CARD)), 'every file is current now');
    const badges = await you.page.$$eval('section:has(h2) .badge', (b) => b.filter((x) => x.innerText.trim() === 'Default').length);
    assert.equal(badges, 3, 'the three files are the default');
  });

  await h.step('rules update: a company that carries what the current file dropped blocks the move', async () => {
    await you.goto(`/campaigns/${blocked}`);
    await you.page.click(`${CARD} button`);
    await you.page.waitForSelector(`${CARD} [data-problems]`, { timeout: 15000 });
    const text = await you.page.$eval(CARD, (e) => e.innerText);
    assert.match(text, /Demo Relic \(retired\)/, 'the problem names the item');
    assert.match(text, /Relic bearers of the older edition/, 'and the company');
    assert.ok(!(await you.page.$(`${CARD} form button.btn-primary`)), 'no move button while something blocks');
    assert.ok(!(await you.page.$(`${CARD} [data-clean]`)), 'and no green banner');
  });

  await h.step('rules update: a campaign with a file of its own asks for the explicit switch', async () => {
    await you.goto(`/campaigns/${forked}`);
    await you.page.click(`${CARD} button`);
    await you.page.waitForSelector(`${CARD} h3`, { timeout: 15000 });
    const sw = await you.page.$(`${CARD} input[name=switch_custom]`);
    const text = await you.page.$eval(CARD, (e) => e.innerText);
    assert.match(text, /own/, 'the report says the file is the campaign\'s own');
    if (sw) assert.ok(await sw.evaluate((e) => e.required), 'the switch is required before moving');
  });

  await h.step('rules update: a player has no card, and the report holds at 360 px in light and dark', async () => {
    const gimli = await h.session('gimli@example.com');
    await gimli.goto(`/campaigns/${blocked}`);
    assert.ok(!(await gimli.page.$(CARD)), 'a player has no Rules update card');
    for (const scheme of ['light', 'dark']) {
      await layout(you, 360, scheme);
      await you.goto(`/campaigns/${blocked}`);
      await you.page.click(`${CARD} button`);
      await you.page.waitForSelector(`${CARD} [data-problems]`, { timeout: 15000 });
      const over = await you.page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(over <= 0, `no horizontal scroll at 360 px ${scheme} (${over})`);
    }
    await layout(you, 1280, 'light');
  });
}
