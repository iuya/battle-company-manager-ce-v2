// Against the Odds is chosen on the match page and the game waits for it (docs/matches.md, "Against the Odds"), in the demo world's match that
// the player above created: the rival cannot play or end it, the player below is asked under Your turn and confirms, the rival's page follows;
// Change and a new scenario reopen the decision, and Ambush! fixes the scenario to the card.
import { assert, campaignId } from '../lib/util.mjs';

const slug = (s) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
const account = (company) => `${slug(company.replace(/ Company$/, ''))}@example.com`;

export default async function (h) {
  const you = await h.session('you@example.com', { height: 1800 });
  let cid, path, lower, rival;
  const ticked = (s, key) => s.page.$eval(`#ato input[name=bonus][value=${key}]`, (i) => i.checked);
  const tick = async (s, css) => { await s.page.click(css); await s.settle(300); };

  await h.step('ato card: the demo world has a match that waits for the player below, created by the player above', async () => {
    cid = await campaignId(you, 'Matches: every scenario');
    await you.goto(`/campaigns/${cid}/matches`);
    const links = await you.page.$$eval('main a[href*="/matches/"]', (as) => [...new Set(as.map((a) => a.getAttribute('href')))].filter((u) => /\/matches\/\d+$/.test(u)));
    let names;
    for (const link of links) {
      const html = await you.page.evaluate((u) => fetch(u + '/ato').then((r) => r.text()), link);
      const m = html.match(/([^<>]+?) is <strong>(\d+)<\/strong> below ([^<>]+?) \(\d+ vs \d+\)/);
      if (m && html.includes('data-ato-confirm') && Number(m[2]) >= 60) { path = link; names = [m[1].trim(), m[3].trim()]; break; }
    }
    assert.ok(path, 'a match with a gap of 60 waits for the decision');
    lower = await h.session(account(names[0]), { height: 1800 });
    rival = await h.session(account(names[1]), { height: 1800 });
    await you.goto(path);
    assert.ok(await you.page.$eval('#ato-slot', (s) => s.getAttribute('hx-trigger') === 'live:match from:body'), 'the card is drawn with the page and follows the match, never on a click');
  });
  if (!path) return;
  const matchId = path.match(/(\d+)$/)[1];

  await h.step('ato card: the new match form has no Against the Odds', async () => {
    await you.goto(`/campaigns/${cid}/matches/new`);
    assert.ok(!(await you.has('Against the Odds')) && !(await you.page.$('#new-ato')), 'chosen on the match page only');
  });

  await h.step('ato card: the rival waits: no form, End match disabled with the reason, no line under Your turn, and a note is refused', async () => {
    await rival.goto(path);
    await rival.page.waitForSelector('[data-ato-waiting]');
    assert.ok(!(await rival.page.$('#ato [data-ato-confirm]')), 'no form for the rival');
    assert.ok(await rival.page.$eval('main [data-end-match]', (b) => b.disabled), 'End match is disabled');
    assert.match(await rival.page.$eval('#end-reason', (e) => e.innerText), /Waiting for .* to choose Against the Odds\./);
    await rival.goto(`/campaigns/${cid}/matches`);
    assert.ok(!(await rival.has('Choose Against the Odds')), 'nothing to choose for the rival');
    await rival.goto(`/campaigns/${cid}/my-company`);
    await rival.page.waitForSelector('[data-ato-banner]');
    const company = await rival.page.$eval('#live-zones', (z) => z.dataset.company);
    const refused = await rival.page.evaluate((u) => fetch(u, { method: 'POST', headers: { 'content-type': 'application/x-www-form-urlencoded', 'HX-Request': 'true' }, body: 'kind=broken' }).then(async (r) => ({ status: r.status, text: await r.text() })),
      `/campaigns/${cid}/matches/${matchId}/live/${company}/actions`);
    assert.equal(refused.status, 422, 'the service refuses');
    assert.ok(refused.text.includes('to choose Against the Odds.'), 'and says why');
  });

  await h.step('ato card: the player below is asked under Your turn and sees every bonus; a tick stays and nothing fetches the card again', async () => {
    for (const page of [`/campaigns/${cid}/matches`, `/campaigns/${cid}`]) {
      await lower.goto(page);
      assert.ok(await lower.has('Choose Against the Odds for Match'), `${page} asks`);
    }
    await lower.goto(path);
    await lower.page.waitForSelector('#ato [data-ato-confirm]');
    for (const key of ['influence', 'experience', 'reroll', 'toolKit', 'wanderer', 'ambush']) {
      assert.ok(await lower.page.$(`#ato input[name=bonus][value=${key}]`), `${key} is on the card`);
    }
    assert.equal(await lower.page.$eval('#ato [data-ato-confirm]', (b) => b.innerText.trim()), 'Play without bonuses');
    await lower.page.$eval('#ato form', (f) => { f.dataset.first = '1'; });
    await tick(lower, '#ato input[name=bonus][value=toolKit]');
    assert.ok(await ticked(lower, 'toolKit'), 'the Tool Kit stays ticked');
    assert.ok(await lower.page.$('#ato form[data-first]'), 'the form was not replaced');
    await tick(lower, '#ato input[name=kit]');
    assert.ok(await lower.page.$eval('#ato input[name=kit]', (i) => i.checked), 'the kit stays ticked');
    assert.equal(await lower.page.$eval('#ato [data-ato-confirm]', (b) => b.innerText.trim()), 'Confirm bonuses');
    await tick(lower, '#ato input[name=bonus][value=toolKit]');
    await tick(lower, '#ato input[name=bonus][value=wanderer]');
    await tick(lower, '#ato input[name=wanderer]');
  });

  await h.step('ato card: confirming frees the game, and the rival\'s open page follows', async () => {
    await rival.goto(path);
    await rival.page.waitForSelector('[data-ato-waiting]');
    await lower.click('#ato [data-ato-confirm]', '');
    await lower.waitText('Against the Odds confirmed.');
    await rival.page.waitForSelector('[data-ato-summary]', { timeout: 8000 });
    await rival.page.waitForFunction(() => !document.querySelector('main [data-end-match]')?.disabled, { timeout: 8000 });
    assert.ok(!(await rival.page.$('[data-ato-change]')), 'the rival cannot change it');
    await lower.goto(path);
    await lower.page.waitForSelector('[data-ato-change]');
  });

  await h.step('ato card: Change reopens with the selection; a new scenario takes the decision back and says so', async () => {
    await lower.click('#ato [data-ato-change]', '');
    await lower.page.waitForSelector('#ato [data-ato-confirm]');
    assert.ok(await ticked(lower, 'wanderer'), 'the Wanderer is still chosen');
    await lower.click('#ato [data-ato-confirm]', '');
    await lower.waitText('Against the Odds confirmed.');
    await you.goto(path);
    await you.click('main [data-change-scenario]', '');
    await you.click('main [data-choose-another]', '');
    await you.click('main #scenario button', 'Hold the Line');
    await you.waitText('Against the Odds has to be confirmed again.');
  });

  await h.step('ato card: Ambush! fixes the scenario to the card', async () => {
    await lower.goto(path);
    await lower.page.waitForSelector('#ato [data-ato-confirm]');
    await tick(lower, '#ato input[name=bonus][value=wanderer]');
    await tick(lower, '#ato input[name=bonus][value=ambush]');
    await lower.page.select('#ato select[name=ambush]', 'Seize Ground');
    await lower.click('#ato [data-ato-confirm]', '');
    await lower.waitText('Against the Odds confirmed.');
    await you.goto(path);
    await you.page.waitForSelector('[data-scenario-ambush]');
    assert.ok(!(await you.page.$('main [data-change-scenario]')), 'no Change scenario');
  });
}
