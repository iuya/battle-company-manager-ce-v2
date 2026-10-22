// A recruit, an armoury purchase and a level-up on the demo world's companies.
import { assert, campaignId, MODAL, numberAfter, sleep } from '../lib/util.mjs';

export default async function (h) {
  const eowyn = await h.session('eowyn@example.com');
  const galadriel = await h.session('galadriel@example.com');

  let c2;
  await h.step('recruit: roll a 2, confirm, the company grows by one unit', async () => {
    c2 = await campaignId(eowyn, 'Companies being built');
    await eowyn.goto(`/campaigns/${c2}/my-company`);
    const before = await eowyn.text();
    const units = numberAfter(before, 'Units');
    const influence = numberAfter(before, 'Influence');
    await eowyn.click('main button', 'Recruit');
    await eowyn.select(MODAL + ' select', '2');
    await eowyn.click(MODAL + ' button', 'Roll');
    await eowyn.waitText('You rolled a');
    await eowyn.click(MODAL + ' button', '=Confirm');
    await eowyn.goto(`/campaigns/${c2}/my-company`);
    const after = await eowyn.text();
    assert.equal(numberAfter(after, 'Units'), units + 1, 'one more unit');
    assert.equal(numberAfter(after, 'Influence'), influence - 3, 'the roll cost 3 influence');
  });

  await h.step('armoury: buy an item for a unit with gold', async () => {
    await eowyn.goto(`/campaigns/${c2}/armory`);
    const before = numberAfter(await eowyn.text(), 'Gold');
    await eowyn.click('main button', 'Buy for');
    await eowyn.click('main button', '=Buy · 1 gold');
    await eowyn.settle(500);
    await eowyn.goto(`/campaigns/${c2}/armory`);
    assert.equal(numberAfter(await eowyn.text(), 'Gold'), before - 1, 'one gold was spent');
  });

  await h.step('level-up: a hero chooses one of its two results', async () => {
    const c5 = await campaignId(galadriel, 'Level-ups waiting');
    await galadriel.goto(`/campaigns/${c5}/my-company`);
    await galadriel.click('main button', 'Level Up');
    await galadriel.click(MODAL + ' label', 'Will +1');
    await galadriel.click(MODAL + ' button', '=Choose');
    await galadriel.waitText('Will +1.');
    await galadriel.click(MODAL + ' button', '=Done');
    await galadriel.waitGone('Will +1.');
  });

  await h.step('formation: drag a card to another place with the mouse, save, and the order is kept after a reload', async () => {
    const ids = () => eowyn.page.$$eval('main .unit-slot', (e) => e.map((x) => x.dataset.unitId));
    await eowyn.goto(`/campaigns/${c2}/my-company`);
    const before = await ids();
    assert.ok(before.length >= 3, 'at least three units');
    await eowyn.click('main button', 'Change formation');
    const handle = (id) => `main .unit-slot[data-unit-id="${id}"] .unit-handle`;
    const centre = async (css) => { const b = await (await eowyn.page.$(css)).boundingBox(); return { x: b.x + b.width / 2, y: b.y + b.height / 2 }; };
    await (await eowyn.page.$(handle(before[2]))).evaluate((e) => e.scrollIntoView({ block: 'center' }));
    const from = await centre(handle(before[2])), to = await centre(handle(before[0]));
    await eowyn.page.mouse.move(from.x, from.y);
    await eowyn.page.mouse.down();
    for (let i = 1; i <= 12; i++) { await eowyn.page.mouse.move(from.x + (to.x - from.x) * i / 12, from.y + (to.y - from.y) * i / 12 - 4); await sleep(30); }
    await eowyn.page.mouse.up();
    await eowyn.settle(300);
    const moved = await ids();
    assert.equal(moved[0], before[2], 'the dragged card is now first');
    await eowyn.click('main button', 'Save formation');
    await eowyn.settle(500);
    await eowyn.goto(`/campaigns/${c2}/my-company`);
    assert.deepEqual(await ids(), moved, 'the order survives a reload');
  });
}
