// The glossary article of a bundle unit shows the unit cards of the models it is made of, and those models have articles of their own
// (docs/frontend-glossary.md, "Unit cards"): search the campaign Glossary for the Vault Warden Team, see the Iron Shield Bearer and the Foe Spear Wielder
// as cards with their own stats and points, follow a member's link to its own article, find the members by search, and hold 360 px in light and dark
// with no horizontal scroll. The campaign of "Matches: every scenario" uses the shipped ruleset.
import { assert, campaignId, layout } from '../lib/util.mjs';

const TEAM = '#u-vault-warden-team';

export default async function (h) {
  const admin = await h.session('admin@example.com', { height: 1000 });
  let gl;

  await h.step('glossary bundle: the Vault Warden Team article shows a card for each model, with its own stats and points', async () => {
    gl = `/campaigns/${await campaignId(admin, 'Matches: every scenario')}/glossary`;
    await admin.goto(gl);
    await admin.fill('#glossary-search', 'Vault Warden Team');
    await admin.page.waitForSelector(TEAM, { timeout: 8000 });
    await admin.settle(300);
    const members = await admin.page.$$eval(`${TEAM} [data-member]`, (ms) => ms.map((m) => ({ id: m.dataset.member, name: m.querySelector('h3').innerText, pts: m.querySelector('.unit-sub').innerText, stats: m.querySelectorAll('dl.statline dd').length, chips: m.querySelectorAll('[data-concept]').length })));
    assert.deepEqual(members.map((m) => m.name), ['Iron Shield Bearer', 'Foe Spear Wielder']);
    for (const m of members) {
      assert.ok(/\d+\s*pts/.test(m.pts), `${m.name} has its own points: ${m.pts}`);
      assert.ok(m.stats >= 6, `${m.name} has its stat line`);
      assert.ok(m.chips > 0, `${m.name} has linked wargear or rules`);
    }
    assert.ok(!(await admin.text()).includes('Made of Iron Shield Bearer, Foe Spear Wielder'), 'no longer a text line');
  });

  await h.step('glossary bundle: a member link opens the member\'s own article, and search finds the members', async () => {
    await admin.page.click(`${TEAM} [data-member="u-foe-spear-wielder"] h3 a`);
    await admin.page.waitForFunction(() => document.activeElement?.id === 'u-foe-spear-wielder', { timeout: 8000 });
    const own = await admin.page.$eval('#u-foe-spear-wielder', (a) => a.innerText);
    assert.ok(own.includes('Foe Spear Wielder') && own.includes('Named by'), 'the member has its own article');
    for (const name of ['Iron Shield Bearer', 'Foe Spear Wielder']) {
      await admin.fill('#glossary-search', name);
      await admin.settle(300);
      const shown = await admin.page.evaluate(() => [...document.querySelectorAll('article[data-unit]')].filter((a) => a.offsetParent !== null).map((a) => a.dataset.name));
      assert.ok(shown.includes(name), `search finds ${name}: ${shown}`);
    }
  });

  await h.step('glossary bundle: the bundle article holds at 360 px in light and dark', async () => {
    await admin.fill('#glossary-search', 'Vault Warden Team');
    await admin.page.waitForSelector(TEAM, { timeout: 8000 });
    for (const scheme of ['light', 'dark']) {
      await layout(admin, 360, scheme);
      await admin.page.$eval(TEAM, (e) => e.scrollIntoView({ block: 'start' }));
      const over = await admin.page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(over <= 0, `no horizontal scroll at 360 px ${scheme} (${over})`);
      const wide = await admin.page.$$eval(`${TEAM} *`, (els) => els.filter((e) => e.scrollWidth > e.clientWidth + 1 && getComputedStyle(e).overflowX !== 'auto').map((e) => `${e.tagName}.${e.className} ${e.scrollWidth}>${e.clientWidth}`).slice(0, 3));
      assert.deepEqual(wide, [], `nothing in the bundle article is wider than its box at 360 px ${scheme}`);
    }
    await layout(admin, 1280, 'light');
  });

  await h.step('glossary bundle: the Vault Warden Team preview of Durin\'s Folk on "Choose your company" holds the card of each member', async () => {
    await admin.goto(`${gl.replace('/glossary', '')}/company/new?c=${encodeURIComponent("Durin's Folk")}`);
    await admin.page.waitForSelector('#company-info article', { timeout: 8000 });
    const cards = await admin.page.evaluate(() => {
      const ref = [...document.querySelectorAll('#company-info .unit-ref')].find((e) => e.textContent.trim() === 'Vault Warden Team');
      const box = ref && document.getElementById(ref.dataset.preview);
      return box ? [...box.querySelectorAll('.rounded-lg.border.bg-surface')].map((c) => ({ name: c.querySelector('p.font-heading').innerText, stats: c.querySelectorAll('dl dd').length })) : null;
    });
    assert.ok(cards, 'the Vault Warden Team is named in Durin\'s Folk');
    assert.deepEqual(cards.map((c) => c.name), ['Iron Shield Bearer', 'Foe Spear Wielder']);
    const refs = await admin.page.$$('#company-info .unit-ref');
    for (const r of refs) { if ((await r.evaluate((e) => e.textContent.trim())) === 'Vault Warden Team') { await r.evaluate((e) => e.scrollIntoView({ block: 'center' })); await r.hover(); } }
    await admin.settle(500);
    const tip = await admin.page.evaluate(() => { const t = [...document.querySelectorAll('[role=tooltip]')].find((x) => x.getBoundingClientRect().height && getComputedStyle(x).display !== 'none'); return t ? { h: t.getBoundingClientRect().height, sh: t.scrollHeight, vh: innerHeight } : null; });
    assert.ok(tip && tip.sh <= tip.h + 1, `the tooltip shows both cards whole: ${JSON.stringify(tip)}`);
    assert.ok(cards.every((c) => c.stats >= 6), `each member has its stat line: ${JSON.stringify(cards)}`);
  });
}
