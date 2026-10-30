// Company creation, submitted for real, in a throwaway campaign of its own (so the demo world the other flows share is not touched):
// the organiser creates a campaign, two players join by the invitation link, one swears a Shire company (the Ranger of the North
// leads), the other a Rohan company whose officers it arranges with the mouse. Checks the redirect, the company page (officers,
// units, points), the Companies and Rankings pages, the confirmation dialog, and that a second submit is refused.
import { assert, campaignId, sleep, confirmDialog } from '../lib/util.mjs';

export default async function (h) {
  const organiser = h.sessions.find((s) => s.email === 'you@example.com') || await h.session('you@example.com', { height: 1800 });
  const shire = await h.session('frodo@example.com', { height: 1800 });
  const rohan = await h.session('aragorn@example.com', { height: 1800 });
  const name = `E2E creation ${Date.now() % 100000}`;
  let cid, link;

  await h.step('creation submit: the organiser makes a throwaway campaign and two players join by the invitation link', async () => {
    await organiser.goto('/campaigns/new');
    await organiser.fill('input[name=name]', name);
    await organiser.fill('textarea[name=description]', 'Throwaway campaign of the company creation tests.');
    await organiser.click('button', 'Create campaign', { nav: true });
    cid = await campaignId(organiser, name);
    await organiser.goto(`/campaigns/${cid}`);
    await organiser.click('button', 'Invite players');
    await organiser.waitText('Invitation link');
    link = await organiser.page.evaluate(() => (document.body.innerHTML.match(/\/join\/[A-Za-z0-9_-]{20,}/) || [])[0]);
    assert.ok(link, 'the invitation link is on the page');
    for (const s of [shire, rohan]) {
      await s.goto(link);
      await s.waitText(name);
      await s.click('button', 'Join', { nav: true });
      await s.goto(`/campaigns/${cid}`);
      await s.waitText('Choose your company');
    }
  });
  if (!cid) return;

  // What the creation page shows for the officers, by name: [leader, sergeant, sergeant].
  const officers = (s) => s.page.evaluate(() => {
    const one = (css) => [...document.querySelectorAll(`.officer-slot${css} .start-card`)].map((c) => c.dataset.unit);
    return [one('[data-slot="leader"]'), one('[aria-label="Sergeant 1"]'), one('[aria-label="Sergeant 2"]')].map((a) => a[0]);
  });
  const pick = async (s, company) => {
    await s.goto(`/campaigns/${cid}/company/new`);
    await s.click(`nav[aria-label=Companies] button[data-name="${company}"]`);
    await s.page.waitForSelector('#take-command');
    await s.settle(300);
  };
  const swear = async (s, companyName) => {
    await s.fill('#name', companyName);
    await s.click('#create-company button.btn-primary', '');
    assert.ok(await s.page.$eval('#create-company dialog', (d) => d.open), 'the confirmation dialog is open');
    const text = await s.page.$eval('#create-company dialog', (d) => d.innerText);
    assert.match(text, /Are you sure/);
    assert.ok(text.includes(companyName), 'the dialog names the company');
    assert.match(text, /These decisions are final/);
    return text;
  };
  // The roster of the company page: [{role, title, pts}] in order.
  const roster = (s) => s.page.$$eval('main .unit-slot', (slots) => slots.map((u) => ({
    role: (u.querySelector('article.unit').className.match(/unit-(leader|sergeant|hero|warrior)/) || [])[1],
    title: u.querySelector('.unit-handle .truncate').textContent.trim(),
    pts: Number((u.querySelector('article.unit').innerText.match(/(\d+)\s*pts/) || [])[1]),
  })));
  const post = (s, body) => s.page.evaluate(async (url, body) => {
    const r = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body });
    return { status: r.status, url: r.url, text: await r.text() };
  }, `/campaigns/${cid}/company`, body);

  let shireNames, shireTitle = 'Shire of the E2E';
  await h.step('creation submit: a Shire company is created, the Ranger of the North leads, and the company page shows it', async () => {
    await pick(shire, 'The Shire');
    shireNames = await officers(shire);
    assert.equal(shireNames[0], 'Ranger of the North', 'the Ranger starts as the Leader');
    assert.ok(shireNames[1] && shireNames[2], 'two sergeants are suggested');
    const text = await swear(shire, shireTitle);
    assert.ok(text.includes('Ranger of the North'), 'the dialog names the Ranger as leader');
    for (const n of shireNames.slice(1)) assert.ok(text.includes(n), `the dialog names the sergeant ${n}`);
    await confirmDialog(shire, '=Yes', { nav: true });
    assert.match(new URL(shire.page.url()).pathname, new RegExp(`^/campaigns/${cid}/my-company/\\d+$`), 'redirected to My company');
    await shire.waitText(shireTitle);
    const units = await roster(shire);
    assert.ok(units.length >= 5, `a starting company has units (${units.length})`);
    assert.deepEqual(units.filter((u) => u.role === 'leader').map((u) => u.title.replace(/^.*?(Ranger of the North)$/, '$1')), ['Ranger of the North'], 'one Leader, the Ranger');
    assert.equal(units.filter((u) => u.role === 'sergeant').length, 2, 'two Sergeants');
    assert.ok(units.every((u) => u.pts > 0), 'every unit shows its points');
    const sergeants = units.filter((u) => u.role === 'sergeant').map((u) => u.title);
    for (const n of shireNames.slice(1)) assert.ok(sergeants.some((t) => t.includes(n)), `${n} is a Sergeant on the company page`);
    shire.total = units.reduce((a, u) => a + u.pts, 0);
  });

  let rohanNames, rohanTitle = 'Riders of the E2E';
  await h.step('creation submit: a Rohan company with arranged officers (mouse drags) is created exactly as arranged', async () => {
    await pick(rohan, 'Rohan');
    const page = rohan.page;
    const drag = async (fromCss, toCss) => {
      await (await page.$(toCss)).evaluate((e) => e.scrollIntoView({ block: 'center' }));
      const a = await (await page.$(fromCss)).boundingBox(), b = await (await page.$(toCss)).boundingBox();
      const from = { x: a.x + 20, y: a.y + a.height / 2 }, to = { x: b.x + b.width / 2, y: b.y + Math.min(b.height / 2, 30) };
      await page.mouse.move(from.x, from.y);
      await page.mouse.down();
      for (let i = 1; i <= 14; i++) { await page.mouse.move(from.x + (to.x - from.x) * i / 14, from.y + (to.y - from.y) * i / 14); await sleep(30); }
      for (let i = 0; i < 4; i++) { const t = await (await page.$(toCss)).boundingBox(); await page.mouse.move(t.x + t.width / 2 + i, t.y + Math.min(t.height / 2, 30) + i); await sleep(60); }
      await page.mouse.up();
      await rohan.settle(350);
    };
    const suggested = await officers(rohan);
    const L = '.officer-slot[data-slot="leader"]', S1 = '.officer-slot[aria-label="Sergeant 1"]';
    // swap: a pool model leads, another takes the first Sergeant place, whoever was there goes back to the pool
    const pool = await page.$$eval('#start-pool .start-card', (e) => e.filter((x) => x.dataset.eligible === 'true').map((x) => [x.dataset.index, x.dataset.unit]));
    assert.ok(pool.length >= 2, 'two eligible models wait in the pool');
    await drag(`#start-pool .start-card[data-index="${pool[0][0]}"]`, L);
    await drag(`#start-pool .start-card[data-index="${pool[1][0]}"]`, S1);
    rohanNames = await officers(rohan);
    assert.equal(rohanNames[0], pool[0][1], 'the first dragged model leads');
    assert.equal(rohanNames[1], pool[1][1], 'the second dragged model is the first Sergeant');
    assert.notDeepEqual(rohanNames, suggested, 'the arrangement differs from the suggestion');
    const fields = await page.$eval('#create-company', (f) => ({ leader: f.querySelector('input[name=leader]').value, sergeants: [...f.querySelectorAll('input[name=sergeant]')].map((i) => i.value) }));
    assert.equal(fields.sergeants.length, 2);
    const text = await swear(rohan, rohanTitle);
    for (const n of rohanNames) assert.ok(text.includes(n), `the dialog names ${n}`);
    await confirmDialog(rohan, '=Yes', { nav: true });
    assert.match(new URL(page.url()).pathname, new RegExp(`^/campaigns/${cid}/my-company/\\d+$`), 'redirected to My company');
    await rohan.waitText(rohanTitle);
    const units = await roster(rohan);
    const role = (r) => units.filter((u) => u.role === r).map((u) => u.title);
    assert.equal(role('leader').length, 1);
    assert.equal(role('sergeant').length, 2);
    assert.ok(role('leader')[0].includes(rohanNames[0]), `the Leader is ${rohanNames[0]}, as dragged (got ${role('leader')[0]})`);
    for (const n of rohanNames.slice(1)) assert.ok(role('sergeant').some((t) => t.includes(n)), `${n} is a Sergeant, as dragged`);
    rohan.total = units.reduce((a, u) => a + u.pts, 0);
  });

  await h.step('creation submit: the new companies are in the Rankings with their faction and points', async () => {
    for (const [s, title, faction] of [[shire, shireTitle, 'The Shire'], [rohan, rohanTitle, 'Rohan']]) {
      await organiser.goto(`/campaigns/${cid}/rankings`);
      const row = await organiser.page.$$eval('main tbody tr', (rs, t) => rs.filter((r) => r.innerText.includes(t)).map((r) => r.innerText.replace(/\s+/g, ' ')), title);
      assert.equal(row.length, 1, `${title} is in the Rankings`);
      assert.ok(row[0].includes(faction), `${title} shows its faction`);
      assert.ok(row[0].includes(String(s.total)), `the Rankings show ${s.total} points for ${title} (got "${row[0]}")`);
    }
  });

  await h.step('creation submit: a second submit is refused (one company per member), nothing is added', async () => {
    shire.allow.push(/422/);
    const r = await post(shire, `faction=Rohan&name=Second+company&leader=0&sergeant=1&sergeant=2`);
    assert.equal(r.status, 422, 'the second company is refused');
    assert.match(r.text, /already have a company/i);
    // the form page of a member who has a company does not offer another creation either
    await shire.goto(`/campaigns/${cid}/company/new`);
    assert.match(new URL(shire.page.url()).pathname, new RegExp(`^/campaigns/${cid}/my-company/\\d+$`), 'the creation page sends a member with a company to it');
    await organiser.goto(`/campaigns/${cid}/rankings`);
    assert.ok(!(await organiser.has('Second company')), 'no second company was created');
    assert.equal(await organiser.page.$$eval('main tbody tr', (e) => e.length), 2, 'two companies in the campaign');
  });
}
