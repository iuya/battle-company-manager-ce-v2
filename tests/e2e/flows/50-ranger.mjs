// The Ranger of the North swap in the "Cemetery" demo campaign.
import { assert, campaignId, MODAL } from '../lib/util.mjs';

export default async function (h) {
  const bilbo = await h.session('bilbo@example.com');
  await h.step('ranger: a successful recruit result can be swapped for a Ranger of the North', async () => {
    const id = await campaignId(bilbo, 'Cemetery');
    await bilbo.goto(`/campaigns/${id}/my-company`);
    await bilbo.waitText('Cemetery');
    await bilbo.click('main button', 'Recruit');
    await bilbo.select(MODAL + ' select', '3');
    await bilbo.click(MODAL + ' button', 'Roll');
    await bilbo.waitText('You rolled a');
    await bilbo.click(MODAL + ' label', 'Swap it for the Ranger of the North');
    await bilbo.click(MODAL + ' button', 'Confirm');
    await bilbo.waitText('takes command');
    // the officer panel: the Ranger is the Leader, has no remove button and fits 360 / 768 / 1280 px in both themes
    for (const [w, theme] of [[360, 'light'], [768, 'dark'], [1280, 'light'], [360, 'dark']]) {
      await bilbo.page.setViewport({ width: w, height: 900 });
      await bilbo.setTheme(theme);
      await bilbo.settle(200);
      assert.ok(await bilbo.noHScroll(), `no horizontal scroll at ${w}px ${theme}`);
      await bilbo.page.screenshot({ path: `out/officers-${w}-${theme}.png` });
    }
    await bilbo.page.setViewport({ width: 1280, height: 900 });
    assert.equal(await bilbo.page.$$eval(MODAL + ' [data-keep] .slot-only', (e) => e.length), 0, 'the Ranger has no remove button');
    // keyboard path: move the Ranger to a Sergeant place; the Leader is the one who was in that place or the old Leader (in the demo world the promoted Hobbit)
    await bilbo.page.click(MODAL + ' [data-keep] button:nth-of-type(2)');
    await bilbo.settle(200);
    const sum = await bilbo.page.$eval(MODAL + ' form', (f) => f.innerText);
    assert.match(sum, /Appoint/);
    assert.match(sum, /Hobbit Militia \(Leader\) leads; Ranger of the North and Hobbit Militia \(Sergeant\) 1 are Sergeants/, 'the Ranger moved to a Sergeant place; the old Leader (a promoted Hobbit) leads and the other Sergeant stays');
    await bilbo.page.screenshot({ path: 'out/officers-moved.png' });
    await bilbo.click(MODAL + ' button', 'Appoint');
    await bilbo.settle(800);
    await bilbo.goto(`/campaigns/${id}/my-company`);
    const t = await bilbo.text();
    assert.match(t, /Units\s*25 \/ 25/, 'the company is full again');
    assert.match(t, /Ranger of the North/, 'a Ranger of the North is in the company');
    assert.ok(!(await bilbo.has('waiting for its new Leader')), 'the banner is gone');
  });
}
