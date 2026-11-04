// An ended campaign is a read-only archive (docs/archive.md). The demo world has one that was ended with the archiver on ("Finished campaign: the
// Siege of Osgiliath") and one ended before archives existed ("Ended before archives"): the archive says what it is, has no Armory tab, no
// Manage buttons and no way back; its overview, rankings, matches, a match and a company page work; the pages it does not have answer so; the
// one not archived yet still has its Armory. The ending dialog in 20-campaign says the campaign becomes an archive; here its text is read.
import { assert, campaignId, layout } from '../lib/util.mjs';

export default async function (h) {
  const you = await h.session('you@example.com', { height: 1000 });
  let id, legacy;

  await h.step('archive: the ended campaign says it is a read-only archive, with no Armory tab and nothing to manage', async () => {
    await you.page.setCookie({ name: 'bcm-finished', value: '1', url: you.h.base });
    id = await campaignId(you, 'Finished campaign');
    legacy = await campaignId(you, 'Ended before archives');
    await you.goto(`/campaigns/${id}`);
    await you.waitText('read-only archive');
    const tabs = await you.page.$$eval('nav[aria-label=Campaign] a', (a) => a.map((x) => x.innerText.trim()));
    assert.deepEqual(tabs, ['Overview', 'My Company', 'Rankings', 'Matches', 'Glossary'], 'an archive has no Armory tab');
    const buttons = await you.page.$$eval('#manage button', (b) => b.map((x) => x.innerText.trim()));
    assert.deepEqual(buttons, [], 'the Manage card of an archive has no button: nobody reopens it');
    assert.ok(!(await you.has('Reactivate')), 'no reactivation');
    assert.ok(await you.has('Rules'), 'the Rules box still names the files it was played with');
  });

  await h.step('archive: the sidebar leaves the Armory out of the archived campaign and keeps it for the one not archived yet', async () => {
    const entries = async (cid) => you.page.$$eval(`#sidebar a[href^="/campaigns/${cid}/"]`, (a) => a.map((x) => x.getAttribute('href').split('/').pop()));
    await you.goto(`/campaigns/${id}`);
    await you.page.waitForSelector(`#sidebar a[href="/campaigns/${id}/rankings"]`, { timeout: 8000 });
    assert.ok(!(await entries(id)).includes('armory'), 'the archive has no Armory entry');
    await you.goto(`/campaigns/${legacy}`);
    await you.page.waitForSelector(`#sidebar a[href="/campaigns/${legacy}/rankings"]`, { timeout: 8000 });
    assert.ok((await entries(legacy)).includes('armory'), 'a finished campaign that is not archived keeps its Armory');
    await you.goto(`/campaigns/${legacy}`);
    assert.ok(await you.has('Everything is frozen'), 'and says it is frozen, not archived');
  });

  await h.step('archive: rankings, matches, a match, a company and the glossary are read from the archive', async () => {
    await you.goto(`/campaigns/${id}/rankings`);
    assert.ok(await you.has('Rangers of Ithilien') && await you.has('Orc raiders of the river'), 'the ranking has both companies');
    await you.goto(`/campaigns/${id}/matches`);
    const links = await you.page.$$eval(`a[href^="/campaigns/${id}/matches/"]`, (a) => [...new Set(a.map((x) => x.getAttribute('href')))]);
    assert.ok(links.length >= 2, `the matches are listed: ${links}`);
    await you.goto(links[0]);
    assert.ok(await you.has('This match has ended') || await you.has('Result'), 'a match shows its result');
    assert.ok(!(await you.page.$('#ato-slot')) && !(await you.page.$('#live-log-slot')), 'no Against the Odds or live log on an archived match');
    const company = await you.page.$$eval(`a[href^="/campaigns/${id}/companies/"]`, (a) => a.map((x) => x.getAttribute('href'))[0]);
    await you.goto(company);
    assert.ok(await you.has('Break Point'), 'a company page shows its roster and rules');
    assert.ok(!(await you.page.$('[data-tts-open]')), 'and no Tabletop Simulator button');
    await you.goto(`/campaigns/${id}/glossary`);
    const terms = await you.page.evaluate(() => Number((document.body.innerText.match(/(\d+) terms/) || [])[1]));
    assert.ok(terms > 0 && terms < 600, `the glossary of an archive holds the terms in use, not the whole book: ${terms}`);
  });

  await h.step('archive: the pages it does not have, and every change, are refused with the reason', async () => {
    you.allow.push(/404|403/);
    for (const path of ['armory', 'matches/new', 'company/new']) {
      await you.goto(`/campaigns/${id}/${path}`);
      assert.ok(await you.has('part of it') || await you.has('archive'), `${path} says the campaign is an archive`);
    }
    const status = await you.page.evaluate(async (id) => {
      const r = await fetch(`/campaigns/${id}/status`, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body: new URLSearchParams({ to: 'active' }), credentials: 'same-origin' });
      return r.status;
    }, id);
    assert.equal(status, 403, 'a status change is refused');
  });

  await h.step('archive: the ending steps tell what is kept and that it cannot be undone', async () => {
    const open = await campaignId(you, 'Matches: every scenario');
    await you.goto(`/campaigns/${open}`);
    await you.click('main button', 'End campaign');
    await you.waitText('Ending is final');
    assert.ok(await you.has('read-only archive'), 'the first step says the campaign becomes an archive');
    assert.ok(!(await you.has('administrator can reactivate')), 'and does not offer an administrator a way back');
  });

  await h.step('archive: an archived overview holds at 360 px in light and dark', async () => {
    for (const scheme of ['light', 'dark']) {
      await layout(you, 360, scheme);
      await you.goto(`/campaigns/${id}`);
      const over = await you.page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(over <= 0, `no horizontal scroll at 360 px ${scheme} (${over})`);
    }
    await layout(you, 1280, 'light');
  });
}
