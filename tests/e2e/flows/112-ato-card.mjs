// The Against the Odds card of the match page (docs/matches.md), in the demo world's match that took a bonus on the new match form: ticking a
// bonus stays ticked (the card is not fetched again) and the Tool Kit and the Wanderer the form left out are chosen and saved here.
import { assert, campaignId } from '../lib/util.mjs';

export default async function (h) {
  const you = await h.session('you@example.com', { height: 1800 });
  let path;
  const ticked = (key) => you.page.$eval(`#ato input[name=bonus][value=${key}]`, (i) => i.checked);
  const tick = async (css) => { await you.page.click(css); await you.settle(300); };
  const save = async () => {
    await you.click('#ato button', 'Save');
    await you.waitText('Against the Odds saved.');
    await you.goto(path);
  };

  await h.step('ato card: the match that took a bonus at creation shows it on the card', async () => {
    const cid = await campaignId(you, 'Matches: every scenario');
    await you.goto(`/campaigns/${cid}/matches`);
    const links = await you.page.$$eval('main a[href*="/matches/"]', (as) => [...new Set(as.map((a) => a.getAttribute('href')))].filter((u) => /\/matches\/\d+$/.test(u)));
    for (const link of links) {
      const html = await you.page.evaluate((u) => fetch(u + '/ato').then((r) => r.text()), link);
      if (/&#34;bonus&#34;:\[&#34;|"bonus":\["/.test(html) && html.includes('name="bonus"')) { path = link; break; }
    }
    assert.ok(path, 'the demo world has an open match with a bonus taken at creation');
    await you.goto(path);
    assert.ok(await you.page.$eval('#ato-slot', (s) => s.getAttribute('hx-trigger') === 'none'), 'the card is drawn with the page');
  });

  await h.step('ato card: a tick stays and nothing fetches the card again; the Tool Kit is saved with its kit', async () => {
    await you.page.$eval('#ato form', (f) => { f.dataset.first = '1'; });
    await tick('#ato input[name=bonus][value=toolKit]');
    assert.ok(await ticked('toolKit'), 'the Tool Kit stays ticked');
    assert.ok(await you.page.$('#ato form[data-first]'), 'the form was not replaced');
    await tick('#ato input[name=kit]');
    assert.ok(await you.page.$eval('#ato input[name=kit]', (i) => i.checked), 'the kit stays ticked');
    await save();
    assert.ok(await ticked('toolKit'), 'saved');
  });

  await h.step('ato card: the Wanderer replaces the Tool Kit and is saved', async () => {
    await tick('#ato input[name=bonus][value=toolKit]');
    await tick('#ato input[name=bonus][value=wanderer]');
    assert.ok(await ticked('wanderer'), 'the Wanderer stays ticked');
    await tick('#ato input[name=wanderer][value="Wandering Swordsman"]');
    assert.ok(await you.page.$eval('#ato input[name=wanderer][value="Wandering Swordsman"]', (i) => i.checked), 'the Wanderer is chosen');
    await save();
    assert.ok(await ticked('wanderer') && !(await ticked('toolKit')), 'saved');
    assert.ok(await you.page.$eval('#ato input[name=wanderer][value="Wandering Swordsman"]', (i) => i.checked), 'its name too');
  });
}
