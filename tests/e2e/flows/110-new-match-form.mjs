// The new match form (docs/matches.md, "New match"), in the demo world's "Two bands" campaign: the
// scenario card (empty until the player rolls it on the 3D dice, then Roll again, Choose another and One-off), the player's own companies marked and ordered, no side names or
// roles, and the page at 360/768/1280 px in light and dark. Nothing is created: the form writes nothing until Create match.
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';

export default async function (h) {
  const eowyn = h.sessions.find((s) => s.email === 'eowyn@example.com') || await h.session('eowyn@example.com', { height: 1800 });
  let bands;
  const scenarioName = (s) => s.page.$eval('#new-scenario input[name=scenario-name]', (i) => i.value).catch(() => '');

  await h.step('new match: two sides with the company search only, her own company already on side 1, no scenario until it is rolled', async () => {
    bands = await campaignId(eowyn, 'Two bands');
    await eowyn.goto(`/campaigns/${bands}/matches/new`);
    await eowyn.page.waitForSelector('section[aria-label="Side 2"]');
    assert.equal(await eowyn.page.$$eval('main form section[aria-label^="Side "]', (s) => s.length), 2, 'two side blocks');
    assert.ok(!(await eowyn.page.$('[name^=side-name], [name^=role-]')), 'no side names, no roles');
    assert.equal(await eowyn.page.$$eval('section[aria-label="Side 1"] ul[aria-label="Companies on this side"] li', (l) => l.length), 1, 'her first company is on side 1');
    assert.ok(await eowyn.page.$('[data-practice-box]') && await eowyn.page.$('[data-ffa-box]') && await eowyn.page.$('[data-add-side]'), 'Practice, All vs all and + Add side');
    assert.equal(await scenarioName(eowyn), '', 'nothing is rolled on page load');
    assert.match(await eowyn.page.$eval('#new-scenario', (e) => e.innerText), /No scenario yet/, 'the card is empty');
    assert.ok(await eowyn.page.$('#new-scenario button.btn-primary[data-roll-again]'), 'a primary roll button');
    assert.equal(await eowyn.page.$eval('#new-scenario button.btn-primary[data-roll-again]', (b) => b.innerText.trim()), 'Roll the scenario');
    assert.ok(await eowyn.page.$eval('#new-scenario', (s) => !!(s.compareDocumentPosition(document.querySelector('section[aria-label="Side 1"]')) & Node.DOCUMENT_POSITION_PRECEDING)), 'the scenario comes after the sides');
  });

  await h.step('new match: Roll the scenario throws the 3D dice, then the card shows the scenario with its dice', async () => {
    await eowyn.click('#new-scenario button', 'Roll the scenario');
    await eowyn.page.waitForSelector('#new-scenario canvas.dice-tray');
    assert.equal(await scenarioName(eowyn), '', 'the scenario is not decided while the dice roll');
    await eowyn.page.waitForSelector('#new-scenario input[name=scenario-name]', { timeout: 15000 });
    await eowyn.page.waitForFunction(() => /Rolled \d \(table \d\) and \d/.test(document.querySelector('#new-scenario').innerText), { timeout: 15000 });
    assert.ok(await scenarioName(eowyn), 'a scenario once the dice stop');
    assert.ok(await eowyn.page.$('#new-scenario [data-scenario]'), 'the card shows it');
    assert.ok(await eowyn.page.$$eval('#new-scenario button', (bs) => ['Roll again', 'Choose another', 'One-off scenario'].every((t) => bs.some((b) => b.innerText.trim() === t))), 'Roll again, Choose another, One-off');
  });

  await h.step('new match: her own companies are marked "Yours", first on side 1 and last on side 2', async () => {
    const own = (side) => eowyn.page.$$eval(`section[aria-label="Side ${side}"] ul[role=listbox] button`, (bs) => bs.map((b) => b.getAttribute('data-own') === 'true'));
    const one = await own(1), two = await own(2);
    assert.ok(two.includes(true) && two.includes(false), `side 2 lists hers and others: ${two}`);
    assert.ok(two.lastIndexOf(false) < two.indexOf(true), `side 2: hers last once one is placed: ${two}`);
    assert.ok(!one.includes(true) || one.indexOf(false) === -1 || one.lastIndexOf(true) < one.indexOf(false), `side 1: hers first: ${one}`);
    assert.ok(await eowyn.page.$$eval('ul[role=listbox] button[data-own=true]', (bs) => bs.every((b) => /Yours/.test(b.innerText))), 'the Yours badge');
    assert.ok(!(await eowyn.page.$$eval('main button', (bs) => bs.some((b) => b.innerText.trim() === 'Mine'))), 'no Mine chip');
  });

  await h.step('new match: a company in another open match is listed struck through and disabled, with the reason', async () => {
    const busy = await eowyn.page.$$eval('section[aria-label="Side 2"] ul[role=listbox] button[data-unavailable=true]', (bs) => bs.map((b) => ({ disabled: b.disabled, text: b.textContent, struck: getComputedStyle(b.querySelector('span span')).textDecorationLine })));
    assert.ok(busy.length > 0, 'the demo world has busy companies');
    assert.ok(busy.every((b) => b.disabled && b.struck === 'line-through'), `disabled and struck through: ${JSON.stringify(busy)}`);
    assert.ok(busy.some((b) => /Already in Match \d+\./.test(b.text)), 'the reason is on the row');
  });

  await h.step('new match: Roll again rolls both dice without writing anything; Choose another and One-off replace the scenario', async () => {
    for (let i = 0; i < 4; i++) {
      await eowyn.click('#new-scenario button', 'Roll again');
      await eowyn.page.waitForFunction(() => /Rolled \d \(table \d\) and \d/.test(document.querySelector('#new-scenario').innerText), { timeout: 15000 });
      assert.ok(await scenarioName(eowyn), 'a scenario after every roll');
    }
    assert.ok(eowyn.reqs.every((r) => !/> POST \/campaigns\/\d+\/matches$/.test(r)), 'no match was created');
    await eowyn.click('#new-scenario button', 'Choose another');
    await eowyn.page.waitForSelector('#new-scenario input[type=search]');
    await eowyn.click('#new-scenario button', 'Rescue');
    await eowyn.settle(300);
    assert.equal(await scenarioName(eowyn), 'Rescue', 'one press sets it');
    assert.match(await eowyn.page.$eval('#new-scenario', (e) => e.innerText), /Chosen/);
    await eowyn.click('#new-scenario button', 'One-off scenario');
    await eowyn.page.waitForSelector('#new-scenario-name');
    assert.equal(await eowyn.page.$eval('#new-scenario input[name=scenario-kind]', (i) => i.value), 'custom');
    await eowyn.click('#new-scenario button', 'Roll instead');
    await eowyn.page.waitForFunction(() => /Rolled \d \(table \d\) and \d/.test(document.querySelector('#new-scenario').innerText), { timeout: 15000 });
    assert.ok(await scenarioName(eowyn), 'rolled again from the one-off form');
  });

  for (const scheme of ['light', 'dark']) for (const w of SIZES) {
    await h.step(`new match: the form at ${w} px, ${scheme}: no horizontal scroll`, async () => {
      await layout(eowyn, w, scheme);
      await eowyn.goto(`/campaigns/${bands}/matches/new`);
      await eowyn.page.waitForSelector('#new-scenario');
      assert.equal(await scenarioName(eowyn), '', 'empty card');
      assert.ok(await eowyn.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      await eowyn.page.screenshot({ path: `out/new-match-${w}-${scheme}.png`, fullPage: true });
      await eowyn.click('#new-scenario button', 'Roll the scenario');
      await eowyn.page.waitForSelector('#new-scenario input[name=scenario-name]', { timeout: 15000 });
      assert.ok(await eowyn.noHScroll(), `no horizontal scroll with the rolled card at ${w}px ${scheme}`);
      await eowyn.page.screenshot({ path: `out/new-match-rolled-${w}-${scheme}.png`, fullPage: true });
    });
  }
  await layout(eowyn, 1280, 'light');
}
