// A unit card of the campaign Glossary links what it names like the unit cards of My Company (GitHub issue: glossary unit links): search the glossary for
// the Goblin Mercenary; its special rules (Chittering Hordes) and wargear (Armour) show what they do on hover and open the helper popup on a click, and
// the page holds at 360 px in light and dark with no horizontal scroll. The campaign of "Matches: every scenario" uses the shipped ruleset.
import { assert, campaignId, layout } from '../lib/util.mjs';

const CARD = '#u-goblin-mercenary';
const tipVisible = (s) => s.page.evaluate(() => [...document.querySelectorAll('[role=tooltip]')].find((t) => { const r = t.getBoundingClientRect(); return r.width && r.height && getComputedStyle(t).display !== 'none' && t.innerText.trim(); })?.innerText || '');
const chip = (name) => `${CARD} [data-concept][data-tip-title="${name}"]`;

export default async function (h) {
  const admin = await h.session('admin@example.com', { height: 1000 });
  let gl;

  await h.step('glossary unit links: searching the Goblin Mercenary shows its rules and wargear as chips with a tooltip', async () => {
    gl = `/campaigns/${await campaignId(admin, 'Matches: every scenario')}/glossary`;
    await admin.goto(gl);
    await admin.fill('#glossary-search', 'Goblin Mercenary');
    await admin.page.waitForSelector(CARD, { timeout: 8000 });
    await admin.settle(300);
    for (const name of ['Chittering Hordes', 'Cave Dweller', 'Armour']) {
      const tip = await admin.page.$eval(chip(name), (e) => e.dataset.tipText);
      assert.ok(tip && tip.length > 20, `${name} has its text as the tooltip`);
    }
    assert.ok(await admin.page.$(`${CARD} [data-concept^="Goblin"]`), 'a keyword has its popup too');
  });

  await h.step('glossary: the To Wound Chart article draws the chart, not only how to read it', async () => {
    await admin.fill('#glossary-search', 'To Wound Chart');
    await admin.page.waitForSelector('#t-to-wound-chart', { timeout: 8000 });
    await admin.settle(300);
    const t = await admin.page.$eval('#t-to-wound-chart [data-to-wound-chart]', (e) => ({ rows: e.querySelectorAll('tbody tr').length, cols: e.querySelectorAll('thead th').length, first: e.querySelector('tbody td')?.textContent.trim() }));
    assert.ok(t.rows >= 8 && t.cols >= 8 && t.first, `the chart has its grid: ${JSON.stringify(t)}`);
    await admin.fill('#glossary-search', 'Goblin Mercenary');
    await admin.page.waitForSelector(CARD, { timeout: 8000 });
    await admin.settle(300);
  });

  await h.step('glossary unit links: hover shows what Chittering Hordes does, a click opens the helper popup', async () => {
    const sel = chip('Chittering Hordes');
    await admin.page.$eval(sel, (e) => e.scrollIntoView({ block: 'center' }));
    await admin.page.hover(sel);
    await admin.settle(350);
    const tip = await tipVisible(admin);
    assert.match(tip, /Chittering Hordes/, `the tooltip names the rule: ${tip}`);
    await admin.page.click(sel);
    await admin.page.waitForSelector('.concept-pop #concept-title', { timeout: 8000 });
    assert.match(await admin.page.$eval('.concept-pop #concept-title', (e) => e.textContent), /Chittering Hordes/);
    await admin.page.keyboard.press('Escape');
    await admin.page.waitForFunction(() => !document.querySelector('.concept-layer'), { timeout: 4000 });
  });

  await h.step('glossary unit links: the card holds at 360 px in light and dark', async () => {
    for (const scheme of ['light', 'dark']) {
      await layout(admin, 360, scheme);
      await admin.page.$eval(CARD, (e) => e.scrollIntoView({ block: 'start' }));
      const over = await admin.page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(over <= 0, `no horizontal scroll at 360 px ${scheme} (${over})`);
    }
    await layout(admin, 1280, 'light');
  });
}
