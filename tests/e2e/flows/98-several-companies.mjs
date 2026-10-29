// Several companies per player (docs/multi-company-spec.md). In a throwaway campaign: the organiser raises "Companies per player" from 1,
// a player creates a second company, the chip switcher appears only with two companies and switches between them, the company id is in
// the address (another player's company is 404, the bare address is an alias), lowering the limit keeps the companies. In the demo world's
// "Two bands": the switcher for Éowyn and Galadriel (not for Gimli), the "Same player" notes of a match and of the new match form.
// 360/768/1280 px in both themes, no horizontal scroll, axe.
import { assert, campaignId, confirmDialog, layout, SIZES } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const axeOn = async (s, ctx) => {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((ctx) => axe.run(ctx, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id}: ${v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' ' + (x.any[0]?.message || '')).join(' | ')}`)), ctx);
};

const SWITCHER = 'nav[aria-label="Your companies"]';
// The chips of the switcher: [{text, current, href}] (empty when it is not on the page).
const chips = (s) => s.page.$$eval(`${SWITCHER} a.chip`, (as) => as.map((a) => ({ text: a.innerText.replace(/\s+/g, ' ').trim(), current: a.getAttribute('aria-current') === 'true', href: a.getAttribute('href') })));
const hasSwitcher = (s) => s.page.$(SWITCHER).then((e) => !!e);
const path = (s) => new URL(s.page.url()).pathname + new URL(s.page.url()).search;

export default async function (h) {
  const organiser = h.sessions.find((s) => s.email === 'you@example.com') || await h.session('you@example.com', { height: 1800 });
  const pippin = await h.session('pippin@example.com', { height: 1800 });
  const frodo = await h.session('frodo@example.com', { height: 1800 });
  const eowyn = h.sessions.find((s) => s.email === 'eowyn@example.com') || await h.session('eowyn@example.com', { height: 1800 });
  const galadriel = h.sessions.find((s) => s.email === 'galadriel@example.com') || await h.session('galadriel@example.com', { height: 1800 });
  const gimli = h.sessions.find((s) => s.email === 'gimli@example.com') || await h.session('gimli@example.com', { height: 1800 });
  const name = `E2E bands ${Date.now() % 100000}`;
  let cid, first, second, frodoCo;
  const firstName = 'First band of the E2E', secondName = 'Second band of the E2E';

  const post = (s, url, body) => s.page.evaluate(async (url, body) => {
    const r = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body });
    return { status: r.status, url: r.url, text: await r.text() };
  }, url, body);
  const setCap = async (n) => {
    await organiser.goto(`/campaigns/${cid}`);
    await organiser.fill('#companies-cap-input', n);
    await organiser.click('#companies-cap button', 'Save');
    await organiser.waitText(`${n === 1 ? 'Each player has one company' : `Each player can have up to ${n} companies`} in this campaign.`);
  };
  // The creation page: pick a faction, name the company, confirm. Ends on the new company's page.
  const create = async (s, faction, title) => {
    await s.goto(`/campaigns/${cid}/company/new`);
    await s.click(`nav[aria-label=Companies] button[data-name="${faction}"]`);
    await s.page.waitForSelector('#take-command');
    await s.settle(300);
    await s.fill('#name', title);
    await s.click('#create-company button.btn-primary', '');
    assert.ok(await s.page.$eval('#create-company dialog', (d) => d.open), 'the confirmation dialog is open');
    await confirmDialog(s, '=Yes', { nav: true });
    await s.waitText(title);
    return Number(path(s).match(/\/my-company\/(\d+)$/)?.[1]);
  };

  await h.step('several companies: the organiser makes a campaign (Companies per player defaults to 1) and two players join', async () => {
    await organiser.goto('/campaigns/new');
    assert.equal(await organiser.page.$eval('#companies-per-player', (i) => i.value), '1', 'the creation form defaults to one company per player');
    await organiser.fill('input[name=name]', name);
    await organiser.click('button', 'Create campaign', { nav: true });
    cid = await campaignId(organiser, name);
    await organiser.goto(`/campaigns/${cid}`);
    assert.ok(await organiser.has('Each player has one company in this campaign.'), 'the settings list shows the limit');
    await organiser.click('button', 'Invite players');
    await organiser.waitText('Invitation link');
    const link = await organiser.page.evaluate(() => (document.body.innerHTML.match(/\/join\/[A-Za-z0-9_-]{20,}/) || [])[0]);
    for (const s of [pippin, frodo]) {
      await s.goto(link);
      await s.click('button', 'Join', { nav: true });
    }
    // Frodo's company stands for "another player's company" below; it is made through the form's own address.
    const r = await post(frodo, `/campaigns/${cid}/company`, 'faction=Rohan&name=Frodo+band&leader=0&sergeant=1&sergeant=2');
    frodoCo = Number(r.url.match(/\/my-company\/(\d+)$/)?.[1]);
    assert.ok(frodoCo, `Frodo's company was created and its page is /my-company/ID (${r.url})`);
  });
  if (!cid) return;

  await h.step('several companies: with one company a player sees today\'s pages (no chip, no new wording) and the company id is in the address', async () => {
    first = await create(pippin, 'Rohan', firstName);
    assert.ok(first && first !== frodoCo, 'the company page is /my-company/ID');
    assert.equal(await hasSwitcher(pippin), false, 'one company, no switcher');
    await pippin.goto(`/campaigns/${cid}`);
    assert.ok(await pippin.has('Your company') && !(await pippin.has('Your companies')), 'the Overview keeps "Your company"');
    assert.ok(!(await pippin.has('Create another company')) && !(await pippin.has('allows')), 'at the default limit nothing new is said');
    await pippin.goto(`/campaigns/${cid}/my-company`);
    assert.equal(path(pippin), `/campaigns/${cid}/my-company/${first}`, 'the bare address is an alias of the company page');
    await pippin.goto(`/campaigns/${cid}/armory`);
    assert.equal(path(pippin), `/campaigns/${cid}/armory?company=${first}`, 'the Armory tab names the company too');
    assert.equal(await hasSwitcher(pippin), false, 'one company, no switcher in the armory either');
  });

  await h.step('several companies: another player\'s company is 404 on every owner address, and a second company is refused at the limit of one', async () => {
    pippin.allow.push(/404/, /422/);
    organiser.allow.push(/404/);
    for (const [s, url] of [[pippin, `/campaigns/${cid}/my-company/${frodoCo}`], [pippin, `/campaigns/${cid}/armory?company=${frodoCo}`], [organiser, `/campaigns/${cid}/my-company/${frodoCo}`], [pippin, `/campaigns/${cid}/my-company/999999`]]) {
      const res = await s.page.goto(s.url(url), { waitUntil: 'load' });
      assert.equal(res.status(), 404, `${s.email} gets 404 for ${url}`);
    }
    await pippin.goto(`/campaigns/${cid}/my-company/${first}`);
    for (const url of [`/campaigns/${cid}/my-company/${frodoCo}/rename`, `/campaigns/${cid}/my-company/${frodoCo}/recruit/roll`]) {
      const r = await post(pippin, url, 'name=Hijacked&roll=4');
      assert.equal(r.status, 404, `POST ${url} is 404`);
    }
    const r = await post(pippin, `/campaigns/${cid}/company`, 'faction=Rohan&name=Too+many&leader=0&sergeant=1&sergeant=2');
    assert.equal(r.status, 422);
    assert.match(r.text, /already have a company/i);
    await pippin.goto(`/campaigns/${cid}/company/new`);
    assert.equal(path(pippin), `/campaigns/${cid}/my-company/${first}`, 'the creation page sends a player at the limit to the company');
  });

  await h.step('several companies: the organiser raises the limit; the player creates a second company and the switcher appears', async () => {
    await setCap(2);
    await pippin.goto(`/campaigns/${cid}`);
    assert.ok(await pippin.has('Create another company'), 'below the limit the Overview offers another company');
    await pippin.click('a', 'Create another company', { nav: true });
    second = await create(pippin, 'Rohan', secondName);
    assert.ok(second && second !== first, 'the second company has its own address');
    assert.equal(path(pippin), `/campaigns/${cid}/my-company/${second}`);
    const c = await chips(pippin);
    assert.equal(c.length, 2, `two chips: ${JSON.stringify(c)}`);
    assert.deepEqual(c.map((x) => x.current), [false, true], 'the company on screen is marked');
    assert.ok(c[0].text.includes(firstName) && c[1].text.includes(secondName), 'the chips are named by company');
    assert.ok(!(await pippin.has('New company')), 'at the limit there is no "New company" chip');
    assert.ok(await pippin.has('All 2 companies created'), 'a note says all are created');
    await pippin.goto(`/campaigns/${cid}`);
    assert.ok(await pippin.has('Your companies'), 'the Overview card turns plural');
    assert.ok(await pippin.has('You have all 2 companies this campaign allows.'), 'and says the limit is reached');
    assert.ok(!(await pippin.has('Create another company')), 'with no control to create more');
    assert.equal(await pippin.page.$$eval('main a', (as) => as.filter((a) => a.innerText.includes('Manage company')).length), 2, 'one Manage company per company');
  });

  await h.step('several companies: the chips switch between the companies, in My Company and in the Armory', async () => {
    await pippin.goto(`/campaigns/${cid}/my-company`);
    assert.equal(path(pippin), `/campaigns/${cid}/my-company/${first}`, 'the alias opens the oldest company');
    assert.equal((await pippin.page.title()).startsWith(firstName), true, 'with several companies the title names the company');
    await pippin.click(`${SWITCHER} a`, secondName, { nav: true });
    assert.equal(path(pippin), `/campaigns/${cid}/my-company/${second}`);
    assert.ok((await pippin.text()).includes(secondName) && (await chips(pippin))[1].current);
    // each company is its own: renaming one leaves the other alone
    await pippin.click('button[aria-label="Rename the company"]');
    await pippin.fill('input[name=name]', 'Renamed second band');
    await pippin.page.keyboard.press('Enter');
    await pippin.waitText('Renamed second band');
    assert.ok((await chips(pippin))[0].text.includes(firstName) && (await chips(pippin))[1].text.includes('Renamed second band'));
    await pippin.click('a.tab', 'Armory', { nav: true });
    assert.equal(path(pippin), `/campaigns/${cid}/armory?company=${first}`, 'the Armory tab opens the oldest company');
    assert.equal((await chips(pippin)).length, 2, 'the switcher is in the Armory');
    await pippin.click(`${SWITCHER} a`, 'Renamed second band', { nav: true });
    assert.equal(path(pippin), `/campaigns/${cid}/armory?company=${second}`);
    assert.ok((await chips(pippin))[1].current, 'the chip of the armory in view is marked');
    assert.ok(await pippin.has('Renamed second band can buy'), 'the armory is that company\'s');
  });

  await h.step('several companies: lowering the limit keeps the companies and only refuses new ones', async () => {
    await setCap(1);
    await pippin.goto(`/campaigns/${cid}`);
    assert.ok(await pippin.has('This campaign now allows 1 company per player; you keep your 2.'), 'the Overview says so');
    assert.equal(await pippin.page.$$eval('main a', (as) => as.filter((a) => a.innerText.includes('Manage company')).length), 2, 'both companies are still there');
    assert.ok(!(await pippin.has('Create another company')));
    await pippin.goto(`/campaigns/${cid}/my-company/${first}`);
    assert.equal((await chips(pippin)).length, 2, 'the switcher keeps both');
    assert.ok(!(await pippin.has('New company')), 'without a "New company" chip');
    const r = await post(pippin, `/campaigns/${cid}/company`, 'faction=Rohan&name=Third&leader=0&sergeant=1&sergeant=2');
    assert.equal(r.status, 422, 'a new company is refused above the limit');
    await organiser.goto(`/campaigns/${cid}/companies`);
    assert.equal(await organiser.page.$$eval('main li a h2', (e) => e.length), 3, 'the Companies page lists the 3 companies of the campaign');
    assert.equal(await organiser.page.$$eval('main li a', (as) => as.filter((a) => a.innerText.includes('Pippin')).length), 2, 'with the owner name on each');
  });

  let bands;
  await h.step('several companies: in Two bands the chips show for Éowyn (below the limit) and Galadriel (at it), not for Gimli', async () => {
    bands = await campaignId(eowyn, 'Two bands');
    await eowyn.goto(`/campaigns/${bands}/my-company`);
    const e = await chips(eowyn);
    assert.equal(e.length, 3, `Éowyn: two companies and "New company": ${JSON.stringify(e)}`);
    assert.ok(e[2].text.includes('New company') && e[2].href === `/campaigns/${bands}/company/new`);
    assert.ok(e[0].current && e[0].text.includes('Eorlings of the Mark'));
    await galadriel.goto(`/campaigns/${bands}/my-company`);
    const g = await chips(galadriel);
    assert.equal(g.length, 3, `Galadriel: three companies: ${JSON.stringify(g)}`);
    assert.ok(!(await galadriel.has('New company')) && await galadriel.has('All 3 companies created'));
    await galadriel.goto(`/campaigns/${bands}`);
    assert.ok(await galadriel.has('You have all 3 companies this campaign allows.'));
    await gimli.goto(`/campaigns/${bands}/my-company`);
    assert.equal(await hasSwitcher(gimli), false, 'one company, no chip, even when the campaign allows three');
    await gimli.goto(`/campaigns/${bands}`);
    assert.ok(await gimli.has('Create another company') && await gimli.has('Your company') && !(await gimli.has('Your companies')));
    // a company of the same player in the other campaign is not reachable here
    eowyn.allow.push(/404/);
    const res = await eowyn.page.goto(eowyn.url(`/campaigns/${bands}/my-company/${first}`), { waitUntil: 'load' });
    assert.equal(res.status(), 404, 'a company of another campaign is 404');
  });

  await h.step('several companies: a match with two companies of one player carries the "Same player" note; the new match form shows it as you build', async () => {
    await eowyn.goto(`/campaigns/${bands}/matches`);
    const href = await eowyn.page.evaluate(() => [...document.querySelectorAll('main a[href]')].map((a) => a.getAttribute('href')).find((x) => /\/matches\/\d+$/.test(x) && document.querySelector(`a[href="${x}"]`).innerText.includes('Shieldmaidens')));
    assert.ok(href, 'the match of Éowyn\'s two companies is listed');
    await eowyn.goto(href);
    assert.ok((await eowyn.text()).includes('Same player: both of these companies are Éowyn'), 'the match page says so');
    assert.ok(await eowyn.has('Eorlings of the Mark') && await eowyn.has('Shieldmaidens of the Deep') && await eowyn.has('Vanguard of the Black Gate'), 'all three companies are on it');
    // the form: side 1 starts with her first company
    await eowyn.goto(`/campaigns/${bands}/matches/new`);
    assert.ok(await eowyn.has('Mine'), 'the Mine chip is there for a player with companies');
    const search = (i) => `section:nth-of-type(${i}) input[type=search]`;
    await eowyn.page.waitForSelector('input[type=search]');
    const boxes = await eowyn.page.$$('input[type=search]');
    await boxes[1].click();
    await boxes[1].type('Shield');
    await eowyn.click('ul[role=listbox] button', 'Shieldmaidens of the Deep');
    await eowyn.waitText('Same player: Éowyn plays both sides');
    await eowyn.click('button[aria-label^="Remove Shieldmaidens"]');
    await eowyn.waitGone('Same player: Éowyn plays both sides');
    await (await eowyn.page.$$('input[type=search]'))[0].click();
    await (await eowyn.page.$$('input[type=search]'))[0].type('Shield');
    await eowyn.click('ul[role=listbox] button', 'Shieldmaidens of the Deep');
    await eowyn.waitText("Both are Éowyn's");
    void search;
    // Mine narrows the search to her own
    await eowyn.click('button', '=Mine');
    await (await eowyn.page.$$('input[type=search]'))[1].click();
    const listed = await eowyn.page.$$eval('ul[role=listbox] li[role=option]', (l) => l.filter((x) => x.getBoundingClientRect().height).map((x) => x.innerText));
    assert.ok(listed.length === 0 || listed.every((t) => t.includes('Éowyn')), `Mine lists only her companies: ${listed}`);
  });

  for (const scheme of ['light', 'dark']) for (const w of SIZES) {
    await h.step(`several companies: the pages at ${w} px, ${scheme}: no horizontal scroll, axe clean`, async () => {
      const pages = [
        [organiser, `/campaigns/${cid}`, 'main'], [pippin, `/campaigns/${cid}`, 'main'], [pippin, `/campaigns/${cid}/my-company/${first}`, 'main'],
        [pippin, `/campaigns/${cid}/armory?company=${second}`, 'main'], [eowyn, `/campaigns/${bands}/my-company`, 'main'],
        [galadriel, `/campaigns/${bands}`, 'main'], [eowyn, `/campaigns/${bands}/matches/new`, 'main'], [eowyn, '/styleguide', '#sg-switcher'],
      ];
      for (const [s, url, ctx] of pages) {
        await layout(s, w, scheme);
        await s.goto(url);
        assert.ok(await s.noHScroll(), `no horizontal scroll: ${url} at ${w} px ${scheme}`);
        if (url.includes('my-company') && s === pippin || url.includes('my-company') && s === eowyn) {
          const row = await s.page.$eval(SWITCHER, (n) => { const r = n.getBoundingClientRect(); return { right: r.right, w: innerWidth, chips: [...n.querySelectorAll('a.chip')].every((c) => c.getBoundingClientRect().height >= 32) }; });
          assert.ok(row.right <= row.w + 1, 'the switcher stays inside the page (it scrolls sideways when it is long)');
          assert.ok(row.chips, 'the chips are tall enough to hit');
        }
        const bad = await axeOn(s, ctx);
        assert.equal(bad.length, 0, `axe ${url}: ${bad.join('; ')}`);
      }
      await (await eowyn.page.$('#sg-switcher')).screenshot({ path: `out/several-companies-styleguide-${w}-${scheme}.png` });
      await layout(pippin, w, scheme);
      await pippin.goto(`/campaigns/${cid}/my-company/${second}`);
      await (await pippin.page.$(SWITCHER)).screenshot({ path: `out/several-companies-chips-${w}-${scheme}.png` });
    });
  }
  for (const s of [organiser, pippin, frodo, eowyn, galadriel, gimli]) await layout(s, 1280, 'light', 900);
}
