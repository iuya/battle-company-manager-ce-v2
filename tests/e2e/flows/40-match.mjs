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

  await h.step('live: a menu entry clicked the instant it is swapped in still sends its request', async () => {
    // htmx wires a swapped fragment when it settles (20 ms later): without htmx_ready.js a click inside that gap did nothing.
    await eowyn.page.evaluate(() => {
      window.__sent = [];
      document.body.addEventListener('htmx:beforeRequest', (e) => window.__sent.push(e.detail.requestConfig.path));
      document.body.addEventListener('htmx:afterSwap', function once() {
        const entry = [...document.querySelectorAll('#modal-body [role=menuitem]')].find((b) => b.innerText.includes('Wounded a unit'));
        if (!entry) return;
        document.body.removeEventListener('htmx:afterSwap', once);
        setTimeout(() => entry.click(), 0); // after this swap, before the 20 ms settle timer of htmx
      });
    });
    const actions = await eowyn.page.evaluateHandle(() => [...document.querySelectorAll('main button')].filter((b) => b.innerText.trim() === 'Actions')[1]);
    await actions.asElement().click();
    await eowyn.page.waitForFunction(() => window.__sent.some((p) => /sheet\/wound$/.test(p)), { timeout: 8000 })
      .catch(() => { throw new Error('the entry clicked at the moment of the swap sent no request'); });
    await eowyn.waitText('Pick who was wounded');
    await eowyn.click(MODAL + ' button', '=Close');
    await eowyn.settle(400);
  });

  await h.step('live: a click that straddles a live refresh still lands (the swap waits for the pointer)', async () => {
    // Press on Actions, make the zones refresh while the button is held, release: without live.js holding the swap the pressed
    // button is gone by mouseup and no click is fired. The refresh is forced through htmx on the same URL the SSE refresh uses.
    const p = eowyn.page;
    await p.evaluate(() => {
      const zones = document.getElementById('live-zones');
      zones.dataset.probe = 'old';
      window.__swaps = 0; window.__clicked = 0;
      document.body.addEventListener('htmx:beforeSwap', (e) => { if (e.detail.target?.id === 'live-zones') window.__swaps++; });
      const b = [...zones.querySelectorAll('button')].find((x) => x.innerText.trim() === 'Actions');
      b.dataset.probe = 'pressed';
      b.addEventListener('click', () => window.__clicked++);
      b.scrollIntoView({ block: 'center' });
    });
    const box = await p.evaluate(() => { const r = document.querySelector('[data-probe=pressed]').getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 }; });
    await p.mouse.move(box.x, box.y);
    await p.mouse.down();
    let released = false;
    try {
    await p.evaluate(() => {
      const d = document.getElementById('live-zones').dataset;
      htmx.ajax('GET', `/campaigns/${d.campaign}/companies/${d.company}/live`, { target: '#live-zones', swap: 'outerHTML' });
    });
    await p.waitForFunction(() => window.__swaps >= 1, { timeout: 8000 });
    await sleep(150); // the answer has arrived and been held back
    const kept = await p.evaluate(() => !!document.querySelector('[data-probe=pressed]')?.isConnected);
    assert.ok(kept, 'the pressed button is still in the page while the pointer is down');
    await p.mouse.up(); released = true;
    } finally { if (!released) await p.mouse.up().catch(() => {}); }
    await p.waitForFunction(() => window.__clicked === 1, { timeout: 4000 }).catch(() => { throw new Error('the click that straddled the refresh was lost'); });
    // The held refresh is applied right after the release: the zones are the new ones.
    await p.waitForFunction(() => document.getElementById('live-zones')?.dataset.probe !== 'old', { timeout: 8000 })
      .catch(() => { throw new Error('the held refresh was never applied after the release'); });
    await p.waitForFunction((m) => [...document.querySelectorAll(m)].some((d) => d.getBoundingClientRect().height), { timeout: 8000 }, MODAL)
      .catch(() => { throw new Error('the click that straddled the refresh did not open the menu'); });
    // Leave a sheet, not the menu, as the last content of the window: the next step looks for the menu entry and must not find a stale one.
    await eowyn.click(MODAL + ' button', 'Wounded a unit');
    await eowyn.waitText('Pick who was wounded');
    await eowyn.click(MODAL + ' button', '=Close');
    await p.waitForFunction((m) => ![...document.querySelectorAll(m)].some((d) => d.getBoundingClientRect().height), { timeout: 4000 }, MODAL)
      .catch(() => { throw new Error('the menu did not close'); });
    await eowyn.settle(400);
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
    await eowyn.click('main button[aria-label="Might of Rider of Rohan up by one"]', ''); // (the Undo toast is gone by then)
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
