// A match from creation to its aftermath, with two signed-in players and the organiser watching.
import { chooseScenario, endMatch, confirmEnd, assert, campaignId, MODAL, confirmDialog, sleep } from '../lib/util.mjs';

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
        let el = document.querySelector(`section[aria-label="Side ${side}"]`);
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

  await h.step('match: the match has the rolled scenario; Change scenario picks another before the game', async () => {
    await chooseScenario(you, 'Chance Encounter');
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
    const label = (v) => `[aria-label="Might of Rider of Rohan: ${v} of 1"]`;
    await you.page.waitForSelector(label(1));
    await eowyn.click('main button[aria-label="Might of Rider of Rohan down by one"]', '');
    await you.page.waitForSelector(label(0), { timeout: 8000 });
    await eowyn.click('[role=status] button', 'Undo');
    await you.page.waitForSelector(label(1), { timeout: 8000 });
  });

  await h.step('live: the mount action of a mounted hero', async () => {
    await eowyn.liveMount(false); // no button on the card for now: the action it sent
    await eowyn.waitText('On foot');
    await you.page.waitForFunction(() => document.body.innerText.includes('On foot'), { timeout: 8000 });
  });

  await h.step('live: a button clicked the instant it is swapped in still sends its request', async () => {
    // htmx wires a swapped fragment when it settles (20 ms later): without htmx_ready.js a click inside that gap did nothing. The
    // objective switch of the second unit swaps the zones in; its Wounded milestone is clicked in that gap.
    const objective = () => eowyn.page.evaluateHandle(() => [...document.querySelectorAll('main button')].filter((b) => b.getAttribute('aria-label')?.startsWith('Got the objective ('))[1]);
    await eowyn.page.evaluate(() => {
      window.__sent = [];
      document.body.addEventListener('htmx:beforeRequest', (e) => window.__sent.push(e.detail.requestConfig.path));
      document.body.addEventListener('htmx:afterSwap', function once(e) {
        if (e.detail.target?.id !== 'live-zones') return;
        const entry = [...document.querySelectorAll('#live-zones button')].filter((b) => b.getAttribute('aria-label')?.startsWith('Wounded an enemy ('))[1];
        if (!entry) return;
        document.body.removeEventListener('htmx:afterSwap', once);
        setTimeout(() => entry.click(), 0); // after this swap, before the 20 ms settle timer of htmx
      });
    });
    await (await objective()).asElement().click();
    await eowyn.page.waitForFunction(() => window.__sent.some((p) => /sheet\/wound$/.test(p)), { timeout: 8000 })
      .catch(() => { throw new Error('the button clicked at the moment of the swap sent no request'); });
    await eowyn.waitText('Pick who was wounded');
    await eowyn.click(MODAL + ' button', '=Close');
    await eowyn.settle(400);
    await (await objective()).asElement().click(); // the objective back off
    await eowyn.settle(400);
  });

  await h.step('live: a click that straddles a live refresh still lands (the swap waits for the pointer)', async () => {
    // Press on Wounded, make the zones refresh while the button is held, release: without live.js holding the swap the pressed
    // button is gone by mouseup and no click is fired. The refresh is forced through htmx on the same URL the SSE refresh uses.
    const p = eowyn.page;
    await p.evaluate(() => {
      const zones = document.getElementById('live-zones');
      zones.dataset.probe = 'old';
      window.__swaps = 0; window.__clicked = 0;
      document.body.addEventListener('htmx:beforeSwap', (e) => { if (e.detail.target?.id === 'live-zones') window.__swaps++; });
      const b = [...zones.querySelectorAll('button')].find((x) => x.getAttribute('aria-label')?.startsWith('Wounded an enemy ('));
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
      .catch(() => { throw new Error('the click that straddled the refresh did not open the window'); });
    await eowyn.waitText('Pick who was wounded');
    await eowyn.click(MODAL + ' button', '=Close');
    await p.waitForFunction((m) => ![...document.querySelectorAll(m)].some((d) => d.getBoundingClientRect().height), { timeout: 4000 }, MODAL)
      .catch(() => { throw new Error('the window did not close'); });
    await eowyn.settle(400);
  });

  await h.step('live: wound an enemy unit from the other company (preview, confirm)', async () => {
    const actions = await eowyn.page.$$('main button');
    const idx = await eowyn.page.evaluate(() => [...document.querySelectorAll('main button')].map((b, i) => [b, i]).filter(([b]) => b.getAttribute('aria-label')?.startsWith('Wounded an enemy (')).map(([, i]) => i)[1]);
    await actions[idx].click();
    await eowyn.click(MODAL + ' label', 'Dwarf Warrior');
    await eowyn.click(MODAL + ' button', '=Confirm');
    await sauron.page.waitForFunction(() => /Field casualties|Wounded by/i.test(document.body.innerText), { timeout: 8000 });
    await sauron.waitText('Wounded by');
  });

  await h.step('live: a unit retreats (failed courage) and returns to the battle', async () => {
    await sauron.liveSheet('retreat'); // no Fled button on the card: its window
    await sauron.click(MODAL + ' button', '=Retreated');
    await sauron.waitText('Failed courage');
    await eowyn.goto(`/campaigns/${cid}/companies/${sCo}`);
    await eowyn.waitText('Failed courage');
    await sauron.click('main button', 'Return to the battle');
    await sauron.waitGone('Failed courage');
  });

  await h.step('live: the company page has no stats and no log; a chip changes company and Go to match opens the log', async () => {
    await eowyn.goto(`/campaigns/${cid}/companies/${eCo}`);
    assert.ok(!(await eowyn.has('Match log')) && !(await eowyn.page.$('#company-influence')), 'no log and no resources in play');
    assert.ok(await eowyn.page.$('#live-banner [data-match-companies] [aria-current="page"]'), 'the company on the page is marked');
    await eowyn.click(`#live-banner [data-match-companies] a[href$="/companies/${sCo}"]`, '');
    await eowyn.page.waitForFunction((co) => document.getElementById('live-zones')?.dataset.company === String(co), { timeout: 8000 }, sCo);
    await eowyn.click('#live-banner [data-go-match]', '');
    await eowyn.page.waitForSelector('#log[open] #live-log');
    await eowyn.waitText('wounded');
    assert.ok(await eowyn.page.$('#scenario-folded[open]'), 'the scenario is open while the match is played');
    await eowyn.goto(`/campaigns/${cid}/companies/${eCo}`);
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
  const might = (v) => `[aria-label="Might of Rider of Rohan: ${v} of 1"]`;

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

  await h.step('match: End match (winner, MVP), the other player confirms with their own vote', async () => {
    const path = `/campaigns/${cid}/matches/${mid}`;
    await endMatch(eowyn, path, 'Draw');
    await eowyn.waitText('proposes to end');
    await confirmEnd(sauron, path);
    await sauron.waitText('Ended');
  });

  await h.step('aftermath: no casualties, Start aftermath on the match page finishes it at once', async () => {
    await eowyn.goto(`/campaigns/${cid}/matches/${mid}`);
    await eowyn.click('#aftermath button', 'Start aftermath');
    await eowyn.settle(600);
    await eowyn.page.waitForSelector('[data-change-summary]');
    await eowyn.goto(`/campaigns/${cid}/my-company`);
    assert.ok(!(await eowyn.has('The company is locked until the match ends')), 'the company is unlocked');
  });

  await h.step('aftermath: Start aftermath on the match page skips the sheet; the other player can go back to it, rolls for an injury and the aftermath finishes on its own', async () => {
    await sauron.goto(`/campaigns/${cid}/matches/${mid}`);
    await sauron.click('#aftermath button', 'Start aftermath');
    await sauron.waitText('Edit the sheet again');
    assert.ok(new URL(sauron.page.url()).pathname.endsWith(`/aftermath/${sCo}`), 'the aftermath page opens');
    assert.equal(await sauron.page.$$eval('main fieldset.unit', (f) => f.length), 0, 'the sheet is not the step in front of the player');
    await sauron.click('main button', 'Edit the sheet again');
    await sauron.page.waitForSelector('main fieldset.unit');
    await sauron.click('main button', 'Start aftermath');
    await sauron.settle(500);
    const dice = await sauron.page.$$('main select');
    await dice[0].select('3'); await dice[1].select('4');
    await sauron.click('main button', 'Roll 2D6');
    await sauron.waitGone('Roll 2D6', 15000);
    await sauron.page.waitForSelector('[data-change-summary]', { timeout: 15000 }); // the last roll finishes it
    await sauron.goto(`/campaigns/${cid}/my-company`);
    assert.ok(!(await sauron.has('The company is locked until the match ends')), 'the company is unlocked');
  });

}
