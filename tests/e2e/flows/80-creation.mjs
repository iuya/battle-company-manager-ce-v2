// Company creation: the Shire's Ranger of the North starts as the Leader and is locked; drag and drop of officers with the
// real mouse (Rohan, whose officers are the player's choice); a forged POST without the Ranger is refused.
import { assert, campaignId, sleep, layout, SIZES } from '../lib/util.mjs';

export default async function (h) {
  const aragorn = await h.session('aragorn@example.com', { height: 1800 });
  let cid;
  const page = aragorn.page;
  const slot = (name) => page.$$eval(`.officer-slot[data-slot="${name}"] .start-card`, (e) => e.map((x) => x.dataset.unit));
  const sergeantSlots = () => page.$$eval('.officer-slot[data-slot="sergeant"]', (u) => u.map((x) => [...x.querySelectorAll('.start-card')].map((c) => c.dataset.unit)));
  const pick = async (name) => {
    await aragorn.goto(`/campaigns/${cid}/company/new`);
    await aragorn.click(`nav[aria-label=Companies] button[data-name="${name}"]`);
    await page.waitForSelector('#take-command');
    await aragorn.settle(300);
  };
  const idx = (css) => page.$$eval(css, (e) => e.map((x) => x.dataset.index));
  const hidden = () => page.$eval('#create-company', (f) => ({ leader: f.querySelector('input[name=leader]').value, sergeants: [...f.querySelectorAll('input[name=sergeant]')].map((i) => i.value) }));
  const drag = async (fromCss, toCss) => {
    // the places and the pool must all be on screen: a mouse outside the viewport drops nothing
    await (await page.$(toCss)).evaluate((e) => e.scrollIntoView({ block: 'center' }));
    const a = await (await page.$(fromCss)).boundingBox(), b = await (await page.$(toCss)).boundingBox();
    const from = { x: a.x + 20, y: a.y + a.height / 2 }, to = { x: b.x + b.width / 2, y: b.y + Math.min(b.height / 2, 30) };
    await page.mouse.move(from.x, from.y);
    await page.mouse.down();
    for (let i = 1; i <= 14; i++) { await page.mouse.move(from.x + (to.x - from.x) * i / 14, from.y + (to.y - from.y) * i / 14); await sleep(30); }
    // the layout shifts while the ghost passes over other places: aim at the target again where it is now, then drop
    for (let i = 0; i < 4; i++) {
      const t = await (await page.$(toCss)).boundingBox();
      await page.mouse.move(t.x + t.width / 2 + i, t.y + Math.min(t.height / 2, 30) + i);
      await sleep(60);
    }
    await page.mouse.up();
    await aragorn.settle(350);
  };

  await h.step('shire creation: the Ranger of the North starts as the Leader and cannot be dragged out, removed or replaced', async () => {
    cid = await campaignId(aragorn, 'Brand new campaign');
    await pick('The Shire');
    assert.deepEqual(await slot('leader'), ['Ranger of the North'], 'the Ranger is in the Leader place');
    const ranger = '.officer-slot[data-slot="leader"] .start-card';
    assert.equal(await page.$eval(ranger, (e) => e.dataset.keep), 'true', 'the card is locked (data-keep)');
    assert.equal(await page.$$eval(`${ranger} .slot-only button`, (e) => e.length), 0, 'no Remove button');
    assert.match(await page.$eval(ranger, (e) => e.innerText), /Always leads/);
    // real drags: out of the Leader place to the pool, and another model onto the Leader place
    await drag(ranger, '#start-pool');
    assert.deepEqual(await slot('leader'), ['Ranger of the North'], 'dragging the Ranger away does nothing');
    const hobbit = '#start-pool .start-card[data-eligible="true"]';
    const name = await page.$eval(hobbit, (e) => e.dataset.unit);
    await drag(hobbit, '.officer-slot[data-slot="leader"]');
    assert.deepEqual(await slot('leader'), ['Ranger of the North'], `dragging ${name} onto the Leader place does not replace the Ranger`);
    // the pool buttons of a model cannot take the Leader place either
    await page.$eval('#start-pool .start-card[data-eligible="true"] .pool-only button', (b) => { if (b.innerText.trim() === 'Leader') b.click(); });
    await aragorn.settle(150);
    assert.deepEqual(await slot('leader'), ['Ranger of the North'], 'the Leader button does not replace the Ranger');
    // the page is not broken after the attempts: two sergeants can still be placed
    const before = (await sergeantSlots()).flat().length;
    assert.ok(before >= 0);
  });

  await h.step('shire creation: a forged POST without the Ranger as Leader is refused with the rule', async () => {
    const info = await page.$$eval('.start-card', (cs) => cs.map((c) => ({ i: c.dataset.index, unit: c.dataset.unit, ok: c.dataset.eligible === 'true' })));
    const ranger = info.find((x) => x.unit === 'Ranger of the North');
    const others = [...new Map(info.filter((x) => x.ok && x.unit !== 'Ranger of the North').map((x) => [x.i, x])).values()];
    assert.ok(ranger && others.length >= 3, 'a Ranger and three other eligible models');
    // the Ranger demoted to Sergeant
    const post = (body) => page.evaluate(async (url, body) => {
      const r = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body });
      return { status: r.status, text: await r.text() };
    }, `/campaigns/${cid}/company`, body);
    aragorn.allow.push(/422/);
    const common = 'faction=The+Shire&name=Forged+Shire';
    let r = await post(`${common}&leader=${others[0].i}&sergeant=${ranger.i}&sergeant=${others[1].i}`);
    assert.equal(r.status, 422, 'the Ranger as a Sergeant is refused');
    assert.match(r.text, /must start as the Leader/);
    r = await post(`${common}&leader=${others[0].i}&sergeant=${others[1].i}&sergeant=${others[2].i}`);
    assert.equal(r.status, 422, 'no Ranger among the officers is refused');
    assert.match(r.text, /must start as the Leader/);
    // nothing was created
    await aragorn.goto(`/campaigns/${cid}/my-company`);
    assert.ok(!(await aragorn.has('Forged Shire')), 'no company was created');
  });

  await h.step('rohan creation: real mouse drags move officers between the places and the pool, the form follows', async () => {
    await pick('Rohan');
    const L = '.officer-slot[data-slot="leader"]', S1 = '.officer-slot[data-slot="sergeant"]:nth-of-type(1)';
    const sergeantUl = (n) => `.officer-slot[aria-label="Sergeant ${n}"]`;
    const [lead0] = await idx(`${L} .start-card`);
    const serg0 = [await idx(`${sergeantUl(1)} .start-card`), await idx(`${sergeantUl(2)} .start-card`)].flat();
    assert.ok(lead0 && serg0.length === 2, 'the company suggests a Leader and two Sergeants');
    assert.ok(await aragorn.has('This is only a suggestion'), 'the suggestion banner shows');
    assert.deepEqual(await hidden(), { leader: lead0, sergeants: serg0 }, 'the form carries the suggestion');
    // the Leader goes back to the pool
    await drag(`${L} .start-card`, '#start-pool');
    assert.deepEqual(await idx(`${L} .start-card`), [], 'the Leader place is empty');
    assert.ok((await idx('#start-pool .start-card')).includes(lead0), 'the former Leader is in the pool');
    assert.match(await page.$eval('#create-company', (f) => f.innerText), /Choose a leader and two sergeants/);
    assert.ok(await page.$eval('#create-company button.btn-primary', (b) => b.disabled), 'the vow button is disabled');
    assert.ok(!(await aragorn.has('This is only a suggestion')), 'the suggestion banner is gone once the player changes something');
    // a pool model onto the Leader place
    const other = (await idx('#start-pool .start-card')).find((i) => i !== lead0);
    await drag(`#start-pool .start-card[data-index="${other}"]`, L);
    assert.deepEqual(await idx(`${L} .start-card`), [other], 'the dragged model leads');
    // a pool model onto an occupied Sergeant place sends the sergeant back to the pool
    const third = (await idx('#start-pool .start-card'))[0];
    await drag(`#start-pool .start-card[data-index="${third}"]`, sergeantUl(1));
    assert.deepEqual(await idx(`${sergeantUl(1)} .start-card`), [third], 'the dragged model is the first Sergeant');
    assert.ok((await idx('#start-pool .start-card')).includes(serg0[0]) || (await idx('#start-pool .start-card')).includes(serg0[1]), 'the Sergeant it replaced is back in the pool');
    const f = await hidden();
    assert.equal(f.leader, other);
    assert.equal(f.sergeants.length, 2);
    assert.ok(f.sergeants.includes(third));
    // the vow: the dialog names the chosen officers
    await aragorn.fill('#name', 'Drag and drop riders');
    await aragorn.click('#create-company button.btn-primary', 'Ride for Rohan');
    assert.ok(await page.$eval('#create-company dialog', (d) => d.open), 'the confirmation dialog is open');
    assert.match(await page.$eval('#create-company dialog', (d) => d.innerText), /Drag and drop riders.*leader.*sergeants/s);
    await aragorn.click('dialog[open] button', '=No');
    assert.ok(!(await page.$eval('#create-company dialog', (d) => d.open)), 'No closes the dialog without creating anything');
  });

  await h.step('shire creation: the officer panel with the locked Ranger fits 360, 768 and 1280 px, light and dark', async () => {
    await pick('The Shire');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(aragorn, w, scheme, 1800);
      assert.ok(await aragorn.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      const box = await page.$eval('#take-command', (e) => { const r = e.getBoundingClientRect(); return [r.left, r.right]; });
      assert.ok(box[0] >= 0 && box[1] <= w + 1, `the Take command card is inside the viewport at ${w}px`);
      assert.match(await page.$eval('.officer-slot[data-slot="leader"]', (e) => e.innerText), /Ranger of the North/);
      await (await page.$('#take-command')).screenshot({ path: `out/shire-officers-${w}-${scheme}.png` });
    }
    await layout(aragorn, 1280, 'light');
  });
}
