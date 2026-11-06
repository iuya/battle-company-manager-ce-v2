// The dev-only dice script (docs/testing.md, "Scripting the server's dice"): a flow queues the numbers the server rolls next through
// /_e2e/dice and the pages show exactly those. Radagast's campaign has the server roll every die (Shake the table off).
// Nothing here depends on another flow: it reads, scripts and clears only its own queue.
import { assert, campaignId } from '../lib/util.mjs';
import { E2E_DICE_TOKEN } from '../lib/env.mjs';

export default async function (h) {
  const dice = async (method, next) => {
    const res = await fetch(h.base + '/_e2e/dice', {
      method, headers: { 'X-E2E-Dice-Token': E2E_DICE_TOKEN, 'Content-Type': 'application/json' },
      body: next ? JSON.stringify({ next }) : undefined,
    });
    return { status: res.status, body: res.ok ? await res.json() : await res.text() };
  };
  const rada = await h.session('radagast@example.com', { height: 1100 });

  await h.step('server dice: the endpoint needs the token, queues, reads and clears', async () => {
    const noToken = await fetch(h.base + '/_e2e/dice', { method: 'POST', body: '{"next":[1]}' });
    assert.equal(noToken.status, 403, 'no token, no script');
    assert.equal((await dice('DELETE')).body.queued, 0);
    assert.equal((await dice('POST', [3, 5])).body.queued, 2);
    assert.equal((await dice('GET')).body.queued, 2);
    assert.equal((await dice('DELETE')).body.queued, 0);
  });

  await h.step('server dice: the scenario roll on the new match form shows the scripted pair', async () => {
    const cid = await campaignId(rada, 'Settings: the server rolls every die');
    await rada.goto(`/campaigns/${cid}/matches/new`);
    assert.equal((await dice('POST', [2, 5])).body.queued, 2);
    await rada.click('#new-scenario button', 'Roll the scenario');
    await rada.page.waitForFunction(() => /Rolled \d \(table \d\) and \d|found no scenario/.test(document.querySelector('#new-scenario').innerText), { timeout: 30000 });
    const text = await rada.page.$eval('#new-scenario', (e) => e.innerText);
    assert.match(text, /Rolled 2 \(table \d\) and 5/, `the card shows the scripted dice: ${text}`);
    assert.equal((await dice('GET')).body.queued, 0, 'both numbers were used');
  });

  // Sauron's company can recruit: Radagast's is in a running match. The recruit is finished so no other flow finds it pending.
  const sauron = await h.session('sauron@example.com', { height: 1100 });
  await h.step('server dice: a recruit roll shows the scripted number', async () => {
    const cid = await campaignId(sauron, 'Settings: the server rolls every die');
    await sauron.goto(`/campaigns/${cid}/my-company`);
    assert.equal((await dice('POST', [4])).body.queued, 1);
    await sauron.click('main button', 'Recruit');
    await sauron.page.waitForSelector('#modal-body [x-data="rollPanel"]');
    await sauron.click('#modal-body button', 'Roll ·');
    await sauron.page.waitForFunction(() => /You rolled a\s*\d/.test(document.querySelector('#modal-body').innerText), { timeout: 30000 });
    assert.equal(await sauron.page.$eval('#modal-body [x-data="rollPanel"]', (e) => e.dataset.roll), '4', 'the server sent the scripted die');
    await sauron.page.waitForFunction(() => /You rolled a\s*4/.test(document.querySelector('#modal-body').innerText), { timeout: 15000 });
    assert.equal((await dice('GET')).body.queued, 0);
    for (let i = 0; i < 5; i++) { // finish: confirm, lower, choose... whatever the 4 offers
      const kind = await sauron.page.evaluate(() => ['confirm', 'lower', 'choose', 'officers'].find((k) => document.querySelector(`#modal-body form[action$="/recruit/${k}"]`)) || '');
      if (!kind || kind === 'officers') break;
      await sauron.page.$$eval(`#modal-body form[action$="/recruit/${kind}"] input[type=radio]:not(:disabled)`, (r) => { const seen = new Set(); r.forEach((x) => { if (!seen.has(x.name) && !r.some((y) => y.name === x.name && y.checked)) x.click(); seen.add(x.name); }); });
      await sauron.click(`#modal-body form[action$="/recruit/${kind}"] button.btn-primary`, '');
    }
  });
}
