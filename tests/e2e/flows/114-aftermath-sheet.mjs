// The Sheet step of the aftermath as a player and as the organiser: the Against the Odds block, the match result and why the sheet opened.
// Companies of "Aftermath ward 6".
import { assert, campaignId } from '../lib/util.mjs';

const WARD = 'Aftermath ward 6';
const BLANK = 'No casualties during the match, is this correct?';
const TIED = 'The MVP vote tied: choose the MVP.';
const RELIC = 'Choose who holds the Relic.';

export default async function (h) {
  const you = await h.session('you@example.com');
  const owners = {
    tookKit: await h.session('eowyn@example.com'),
    tookNothing: await h.session('aragorn@example.com'),
    relic: await h.session('gimli@example.com'),
    tied: await h.session('radagast@example.com'),
  };

  // The aftermath page of a company of the ward, found through the organiser's match pages (Start aftermath would start it).
  const urls = {};
  async function pageOf(name) {
    if (!urls[name]) {
      const cid = await campaignId(you, WARD);
      await you.goto(`/campaigns/${cid}/matches`);
      const matches = await you.page.$$eval('main a[href]', (a) => [...new Set(a.map((x) => x.getAttribute('href')).filter((x) => /\/matches\/\d+$/.test(x)))]);
      for (const m of matches) {
        await you.goto(m);
        const company = await you.page.evaluate((n) => {
          const link = [...document.querySelectorAll('#aftermath li a[href*="/companies/"]')].find((a) => a.innerText.includes(n));
          return link ? link.getAttribute('href').match(/\/companies\/(\d+)/)[1] : null;
        }, name);
        if (company) { urls[name] = `${m}/aftermath/${company}`; break; }
      }
      assert.ok(urls[name], `an aftermath page for ${name}`);
    }
    return urls[name];
  }
  const block = (s) => s.page.evaluate(() => {
    const b = document.querySelector('#aftermath-ato');
    return b && { text: b.innerText, control: b.querySelectorAll('input[name=odds]').length };
  });
  const reasons = (s) => s.page.$$eval('#aftermath-reasons li', (l) => l.map((x) => x.innerText.trim()));

  await h.step('a player reads the bonuses taken, also those with no effect here, and cannot change them', async () => {
    await owners.tookKit.goto(await pageOf('Riders who took a Tool Kit'));
    const b = await block(owners.tookKit);
    assert.ok(b, 'the block is there');
    assert.equal(b.control, 0, 'nothing to tick');
    assert.match(b.text, /Influence/);
    assert.match(b.text, /Tool Kit/);
    assert.doesNotMatch(b.text, /No effect on the aftermath/);
    const text = await owners.tookKit.text();
    assert.match(text, /Review the match results/);
    assert.doesNotMatch(text, /Match result:/);
    assert.doesNotMatch(text, /From the live match|From the match/);
  });

  await h.step('the organiser changes the bonuses and is told how to change the result', async () => {
    await you.goto(await pageOf('Riders who took a Tool Kit'));
    const b = await block(you);
    assert.ok(b.control > 0, 'the bonuses can be ticked');
    assert.match(await you.text(), /Match result:.*Change it on the match page \(administrators only\)\./);
  });

  await h.step('a player whose bonuses the organiser removed reads that; the rival has no block', async () => {
    await owners.tookNothing.goto(await pageOf('Rangers who lost their bonuses'));
    const b = await block(owners.tookNothing);
    assert.match(b.text, /No bonus taken\./);
    assert.equal(b.control, 0);
    await you.goto(await pageOf('Elves who hired a Wanderer'));
    assert.equal(await block(you), null, 'the higher company has no bonus to take, not even for the organiser');
  });

  await h.step('the sheet says why it opened', async () => {
    await owners.relic.goto(await pageOf('Dwarves who forgot the Relic'));
    assert.equal(await block(owners.relic), null, 'equal ratings, no bonus: no block');
    assert.deepEqual(await reasons(owners.relic), [BLANK, RELIC]);
    assert.match(await owners.relic.text(), /The company holds the Relic\./);
    await owners.tied.goto(await pageOf('Wargs with a tied vote'));
    assert.deepEqual(await reasons(owners.tied), [TIED]);
  });
}
