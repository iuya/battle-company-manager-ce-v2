// A match from creation to its aftermath, with two signed-in players and the organiser watching.
import { assert, campaignId, MODAL, confirmDialog, sleep } from '../lib/util.mjs';

export default async function (h) {
  const you = await h.session('you@example.com');
  const eowyn = await h.session('eowyn@example.com');
  const sauron = await h.session('sauron@example.com');
  let cid, mid, eCo, sCo;

  await h.step('match: the organiser creates a match between two companies', async () => {
    cid = await campaignId(you, 'Limits, creatures');
    await you.goto(`/campaigns/${cid}/matches/new`);
    const pick = async (side, text) => {
      await you.page.evaluate((side, text) => {
        let el = document.querySelector(`[name=side-name-${side}]`);
        while (el && ![...el.querySelectorAll('button')].some((b) => b.innerText.includes(text))) el = el.parentElement;
        [...el.querySelectorAll('button')].find((b) => b.innerText.includes(text)).click();
      }, side, text);
      await you.settle();
    };
    await pick(1, 'Riders on every horse');
    await pick(2, 'Garrison of Helm');
    await you.click('main button', 'Create match', { nav: true }).catch(() => {});
    await you.settle(400);
    mid = (new URL(you.page.url()).pathname.match(/matches\/(\d+)/) || [])[1];
    assert.ok(mid, 'redirected to the new match: ' + you.page.url());
    const ids = await you.page.evaluate(() => [...document.querySelectorAll('main a[href*="/companies/"]')].map((a) => a.getAttribute('href').match(/companies\/(\d+)/)[1]));
    [eCo, sCo] = [...new Set(ids)];
    assert.ok(eCo && sCo, 'two company links');
  });
  if (!mid) return;

  await h.step('match: pick a scenario', async () => {
    await you.click('main button', 'Choose scenario');
    await you.click('main label', 'Chance Encounter');
    await you.click('main button', 'Use this scenario');
    await you.waitText('Chance Encounter');
  });

  await h.step('live: both players open their company and see the live banner', async () => {
    await eowyn.goto(`/campaigns/${cid}/companies/${eCo}`);
    await sauron.goto(`/campaigns/${cid}/companies/${sCo}`);
    await eowyn.waitText('is running');
    await sauron.waitText('is running');
  });

  await h.step('live: a tracker change reaches the viewer through the event stream, and Undo reverts it', async () => {
    await you.goto(`/campaigns/${cid}/companies/${eCo}`);
    await you.waitText('is running');
    const label = (v) => `[aria-label="Might of Rider of Rohan: ${v} of 1, tap to set"]`;
    await you.page.waitForSelector(label(1));
    await eowyn.click('main button[aria-label="Might of Rider of Rohan down by one"]', '');
    await you.page.waitForSelector(label(0), { timeout: 8000 });
    await eowyn.click('[role=status] button', 'Undo');
    await you.page.waitForSelector(label(1), { timeout: 8000 });
  });

  await h.step('live: the actions menu of a mounted hero', async () => {
    await eowyn.click('main button', '=Actions');
    await eowyn.click(MODAL + ' button', 'Mount removed');
    await eowyn.click(MODAL + ' button', '=Confirm');
    await eowyn.waitText('On foot');
    await you.page.waitForFunction(() => document.body.innerText.includes('On foot'), { timeout: 8000 });
  });

  await h.step('live: wound an enemy unit from the other company (preview, confirm)', async () => {
    const actions = await eowyn.page.$$('main button');
    const idx = await eowyn.page.evaluate(() => [...document.querySelectorAll('main button')].map((b, i) => [b, i]).filter(([b]) => b.innerText.trim() === 'Actions').map(([, i]) => i)[1]);
    await actions[idx].click();
    await eowyn.click(MODAL + ' button', 'Wounded a unit');
    await eowyn.click(MODAL + ' label', 'Dwarf Warrior');
    await eowyn.click(MODAL + ' button', '=Confirm');
    await sauron.page.waitForFunction(() => /Field casualties|Wounded by/i.test(document.body.innerText), { timeout: 8000 });
    await sauron.waitText('Wounded by');
  });

  await h.step('live: a unit retreats (failed courage) and returns to the battle', async () => {
    const idx = await sauron.page.evaluate(() => [...document.querySelectorAll('main button')].map((b, i) => [b, i]).filter(([b]) => b.innerText.trim() === 'Actions').map(([, i]) => i)[0]);
    await (await sauron.page.$$('main button'))[idx].click();
    await sauron.click(MODAL + ' button', 'Retreated');
    await sauron.click(MODAL + ' button', '=Retreated');
    await sauron.waitText('Failed courage');
    await eowyn.goto(`/campaigns/${cid}/companies/${sCo}`);
    await eowyn.waitText('Failed courage');
    await sauron.click('main button', 'Return to the battle');
    await sauron.waitGone('Failed courage');
  });

  await h.step('live: end checks (got the objective) and the match log', async () => {
    await eowyn.goto(`/campaigns/${cid}/companies/${eCo}`);
    await eowyn.click('main button', 'End checks');
    await eowyn.click(MODAL + ' button', 'Rider of Rohan');
    await eowyn.page.waitForFunction((m) => [...document.querySelectorAll(m + ' button')].some((b) => /^Rider of Rohan Has the objective$/.test(b.innerText.replace(/\s+/g, ' ').trim())), {}, MODAL);
    await eowyn.click(MODAL + ' button[aria-label]', 'Close').catch(() => eowyn.page.keyboard.press('Escape'));
    await eowyn.click('main button', 'Match log');
    await eowyn.waitText('wounded');
    await eowyn.page.keyboard.press('Escape');
  });

  // The stream is cut with request interception: aborted = a network error (the browser keeps retrying), refused = HTTP 429 (the
  // caps: the page polls instead). The viewer is `you`; Eowyn changes the Might of her Rider meanwhile.
  let streamMode = 'open';
  await you.page.setRequestInterception(true);
  you.page.on('request', (req) => {
    if (!req.url().includes('/events')) return req.continue();
    if (streamMode === 'abort') return req.abort('connectionfailed');
    if (streamMode === 'refuse') return req.respond({ status: 429, contentType: 'text/plain', headers: { 'Retry-After': '10' }, body: 'busy' });
    return req.continue();
  });
  const might = (v) => `[aria-label="Might of Rider of Rohan: ${v} of 1, tap to set"]`;

  await h.step('live: a cut stream shows Reconnecting and the page catches up when it is back', async () => {
    you.allow.push(/connectionfailed|events/i);
    streamMode = 'abort';
    await you.goto(`/campaigns/${cid}/companies/${eCo}`);
    await you.page.waitForFunction(() => document.getElementById('live-dot-text')?.textContent.includes('Reconnecting'), { timeout: 8000 });
    await eowyn.click('main button[aria-label="Might of Rider of Rohan down by one"]', '');
    await sleep(1500);
    assert.ok(await you.page.$(might(1)), 'the viewer has not seen the change while the stream is cut');
    streamMode = 'open';
    await you.page.waitForSelector(might(0), { timeout: 12000 }); // the browser retries after 3 s, then the page refetches
    await you.page.waitForFunction(() => document.getElementById('live-dot-text')?.textContent === 'Live', { timeout: 5000 });
    await eowyn.click('[role=status] button', 'Undo');
    await you.page.waitForSelector(might(1), { timeout: 8000 });
  });

  await h.step('live: a refused stream falls back to polling every few seconds', async () => {
    streamMode = 'refuse';
    await you.goto(`/campaigns/${cid}/companies/${eCo}`);
    await you.page.waitForFunction(() => document.getElementById('live-dot-text')?.textContent.includes('Updating every few seconds'), { timeout: 8000 });
    await eowyn.click('main button[aria-label="Might of Rider of Rohan down by one"]', '');
    await you.page.waitForSelector(might(0), { timeout: 15000 }); // the poll runs every 10 s
    await eowyn.click('main button[aria-label="Might of Rider of Rohan up by one"]', ''); // (the Undo toast is gone by now)
    await you.page.waitForSelector(might(1), { timeout: 15000 });
    streamMode = 'open';
    await you.goto(`/campaigns/${cid}/companies/${eCo}`);
    await you.page.waitForFunction(() => document.getElementById('live-dot-text')?.textContent === 'Live', { timeout: 8000 });
  });

  await h.step('match: record the result, propose to end, the other player confirms', async () => {
    await eowyn.goto(`/campaigns/${cid}/matches/${mid}`);
    await eowyn.click('main label', 'Draw');
    await eowyn.click('main button', 'Save result');
    await eowyn.click('main button', 'Propose to end');
    await eowyn.click('main button', 'Propose anyway');
    await eowyn.waitText('proposed');
    await sauron.goto(`/campaigns/${cid}/matches/${mid}`);
    await sauron.click('main button', 'Confirm the end');
    await sauron.waitText('Ended');
  });

  await h.step('aftermath: start, finish (no casualties)', async () => {
    await eowyn.goto(`/campaigns/${cid}/matches/${mid}/aftermath/${eCo}`);
    await eowyn.click('main button', 'Start aftermath');
    await eowyn.settle(500);
    await eowyn.waitText('Finish aftermath');
    await eowyn.click('main button', '=Finish aftermath');
    await eowyn.settle(600);
    await eowyn.goto(`/campaigns/${cid}/my-company`);
    assert.ok(!(await eowyn.has('The company is locked until the match ends')), 'the company is unlocked');
  });

  await h.step('aftermath: the other player rolls for an injury and finishes', async () => {
    await sauron.goto(`/campaigns/${cid}/matches/${mid}/aftermath/${sCo}`);
    await sauron.click('main button', 'Start aftermath');
    await sauron.settle(500);
    const dice = await sauron.page.$$('main select');
    await dice[0].select('3'); await dice[1].select('4');
    await sauron.click('main button', 'Roll 2D6');
    await sauron.waitGone('Roll 2D6', 15000);
    await sauron.waitText('Finish aftermath');
    await sauron.click('main button', '=Finish aftermath');
    await sauron.settle(600);
    await sauron.goto(`/campaigns/${cid}/my-company`);
    assert.ok(!(await sauron.has('The company is locked until the match ends')), 'the company is unlocked');
  });

}
