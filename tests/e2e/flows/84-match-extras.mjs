// A three-company match on the Limits campaign: Fog of War without a secret objectives card, a cavalry company's live card
// (granted rules, the mount lost), and a tie in the MVP votes of the end sheets (two opposing players vote for different units).
import { chooseScenario, endMatch, confirmEnd, assert, campaignId, MODAL, layout, SIZES } from '../lib/util.mjs';

export default async function (h) {
  const you = await h.session('you@example.com');
  const eowyn = await h.session('eowyn@example.com');
  const gimli = await h.session('gimli@example.com');
  const sauron = await h.session('sauron@example.com');
  let cid, mid, eCo, gCo, sCo;

  await h.step('failed courage: a card of a Broken company marks it with one press, with only the border highlighted', async () => {
    const aragorn = await h.session('aragorn@example.com');
    const camp = await campaignId(aragorn, 'Fog of War: retreats');
    await aragorn.goto(`/campaigns/${camp}/my-company`);
    const marks = () => aragorn.page.$$eval('main .courage-failed', (b) => b.length);
    const before = await marks();
    assert.ok(before > 1, `the Rangers in the Field have the mark (${before})`);
    assert.equal(await aragorn.page.$('main [hx-get$="/sheet/courage"]'), null, 'the Courage test window is gone');
    for (const scheme of ['light', 'dark']) {
      for (const width of SIZES) {
        await layout(aragorn, width, scheme);
        const look = await aragorn.page.$eval('main .courage-failed', (b) => {
          const c = getComputedStyle(b);
          return { fill: c.backgroundColor, border: c.borderTopColor, icon: !!b.querySelector('svg use'), wide: document.documentElement.scrollWidth > innerWidth };
        });
        assert.equal(look.fill, 'rgba(0, 0, 0, 0)', 'no filled background');
        assert.notEqual(look.border, 'rgba(0, 0, 0, 0)', 'the border stands out');
        assert.ok(look.icon && !look.wide, `${width} ${scheme}: the icon, and no horizontal scroll`);
        await aragorn.page.$eval('main .courage-failed', (b) => b.scrollIntoView({ block: 'center' }));
        await aragorn.page.screenshot({ path: `out/failed-courage-${width}-${scheme}.png` });
      }
    }
    await layout(aragorn, 1280, 'light');
    await aragorn.page.click('main .courage-failed');
    await aragorn.page.waitForFunction((n) => document.querySelectorAll('main .courage-failed').length === n - 1, { timeout: 8000 }, before);
    assert.equal(await aragorn.page.$eval(MODAL, (m) => m.offsetParent !== null), false, 'no window opened');
    assert.match(await aragorn.text(), /Failed courage/);
  });

  await h.step('match: a cavalry company and the Dwarves against the Helm\'s Deep garrison, Fog of War', async () => {
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
    await pick(1, 'Erebor');
    await pick(2, 'Garrison of Helm');
    await you.click('main button', 'Create match', { nav: true }).catch(() => {});
    await you.settle(400);
    mid = (new URL(you.page.url()).pathname.match(/matches\/(\d+)/) || [])[1];
    assert.ok(mid, 'redirected to the new match: ' + you.page.url());
    const ids = await you.page.evaluate(() => [...new Set([...document.querySelectorAll('main a[href*="/companies/"]')].map((a) => a.getAttribute('href').match(/companies\/(\d+)/)[1]))]);
    assert.equal(ids.length, 3, 'three companies in the match');
    [eCo, gCo, sCo] = ids;
    await chooseScenario(you, 'Fog of War');
  });
  if (!mid) return;

  await h.step('secret objectives: a Fog of War match has no card for them, the rules still print how it is scored', async () => {
    await eowyn.goto(`/campaigns/${cid}/matches/${mid}`);
    assert.equal(await eowyn.page.$('#secret-objectives'), null, 'no secret objectives card');
    assert.match(await eowyn.page.$eval('main', (m) => m.textContent), /secretly note down/, 'the printed objectives are still there to read');
  });

  // The live card of every unit of the company: its mount chip, struck state and special rules.
  const cards = (s) => s.page.$$eval('main .unit-slot', (u) => u.map((x) => ({
    title: x.querySelector('h3')?.innerText.trim(),
    onFoot: /On foot/.test(x.innerText),
    gear: [...x.querySelectorAll('ul[aria-label=Wargear] li')].map((l) => ({ name: l.innerText.replace(/\s+/g, ' ').trim(), struck: l.className.includes('line-through') ? l.title : '' })),
    rules: [...x.querySelectorAll('ul[aria-label="Special rules"] li')].map((l) => ({ name: l.innerText.trim(), struck: l.className.includes('line-through'), tip: l.dataset.tipText || '' })),
  })));

  await h.step('live cavalry: the Rohan company shows its mounts and granted rules on the live cards; losing a mount strikes only that unit\'s horse', async () => {
    await eowyn.goto(`/campaigns/${cid}/companies/${eCo}`);
    await eowyn.waitText('is running');
    const before = await cards(eowyn);
    const mounted = before.filter((c) => c.gear.some((g) => /^(Armoured )?horse/i.test(g.name)));
    assert.ok(mounted.length >= 3, `the company has several mounted units (${mounted.length})`);
    assert.ok(mounted.every((c) => c.gear.every((g) => !g.struck) && !c.onFoot), 'nobody is on foot at the start');
    const arise = before.filter((c) => c.rules.some((r) => /^Arise, Riders of Rohan!/.test(r.name)));
    assert.ok(arise.length >= 3 && arise.length < before.length, `the rule granted to the heroes is on some cards, not on all (${arise.length} of ${before.length})`);
    assert.ok(arise.every((c) => c.rules.find((r) => /^Arise/.test(r.name)).tip.length > 20), 'the granted rule has its text as a tooltip');
    const rider = before[0];
    assert.equal(rider.title, 'Rider of Rohan');
    assert.ok(rider.rules.some((r) => r.name === 'Expert Rider'), 'the cavalry rule Expert Rider is on the Rider\'s card');
    // keywords (docs/unit-keywords.md): the chips are race and factions; Cavalry and the mount are the icon's and the gear's, so a lost mount changes none
    const kwOf = () => eowyn.page.$eval('main .unit-slot', (u) => ({
      row: [...u.querySelectorAll('[data-keywords] ul[aria-label=Keywords] > li.kw-chip')].map((c) => c.textContent.trim()),
      all: [...u.querySelectorAll('.kw-full li.kw-chip')].map((c) => c.textContent.trim()), groups: [...u.querySelectorAll('.kw-full dt')].map((d) => d.textContent.trim()),
    }));
    const kw0 = await kwOf();
    assert.equal(kw0.row[0], 'Man', `the Rider's keywords start with the race: ${kw0.row}`);
    assert.ok(!kw0.all.some((k) => ['Cavalry', 'Mount', 'Hero', 'Warrior', 'Infantry'].includes(k)) && !kw0.groups.some((g) => g.startsWith('Mount')), `no obvious keyword: ${kw0.all}`);
    await eowyn.liveMount(false); // no button on the card for now: the action it sent
    await eowyn.waitText('On foot');
    const after = await cards(eowyn);
    assert.equal(after.filter((c) => c.onFoot).length, 1, 'one unit is on foot');
    assert.ok(after[0].onFoot, 'the Rider is on foot');
    assert.equal(after[0].gear.find((g) => /^horse/i.test(g.name)).struck, 'the mount is lost', 'its horse is struck through with the reason');
    assert.ok(after[0].rules.filter((r) => r.struck).every((r) => r.name === 'Cavalry'), 'only the rules the mount gave are struck');
    for (let i = 1; i < after.length; i++) assert.deepEqual(after[i].gear, before[i].gear, `the other units keep their gear (${after[i].title})`);
    assert.deepEqual(await kwOf(), kw0, 'on foot: the keywords do not change');
    // the viewer sees it too
    await gimli.goto(`/campaigns/${cid}/companies/${eCo}`);
    await gimli.waitText('On foot');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(eowyn, w, scheme);
      assert.ok(await eowyn.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      await (await eowyn.page.$('main .unit-slot')).screenshot({ path: `out/live-cavalry-${w}-${scheme}.png` });
    }
    await layout(eowyn, 1280, 'light');
  });

  await h.step('MVP votes: End match asks the proposer for a vote, each Confirm the others; the two opposing players vote different units', async () => {
    const path = `/campaigns/${cid}/matches/${mid}`;
    // without a vote the sheet is refused with who is missing
    eowyn.allow.push(/422/);
    await eowyn.goto(path);
    await eowyn.click('main [data-end-match]', '');
    await eowyn.page.waitForSelector(MODAL + ' [data-end-sheet]');
    assert.deepEqual(await eowyn.page.$$eval(MODAL + ' [data-mvp-ballot]', (b) => b.map((x) => x.dataset.mvpBallot)), ["Garrison of Helm's Deep"], 'one ballot: the opposing company');
    await eowyn.click(MODAL + ' label', 'Draw');
    await eowyn.click(MODAL + ' [data-end-sheet] button.btn-primary', '=End match');
    await eowyn.page.waitForFunction((m) => /MVP/.test(document.querySelector(m + ' [data-end-sheet] [role=alert]')?.innerText || ''), { timeout: 8000 }, MODAL)
      .catch(() => { throw new Error('an End match without the MVP vote must be refused'); });
    await endMatch(eowyn, path, 'Draw', { mvp: 0 });
    await eowyn.waitText('proposes to end');
    await confirmEnd(gimli, path, { mvp: 1 });
    await confirmEnd(sauron, path, { mvp: 0 });
    await gimli.page.screenshot({ path: 'out/mvp-tie.png' });
  });

  await h.step('match end: all three players confirm with the winner and the MVP alone; the tie is left to the owner in the aftermath', async () => {
    await sauron.waitText('Ended');
    await sauron.goto(`/campaigns/${cid}/matches/${mid}`);
    assert.equal(await sauron.page.$('#secret-objectives'), null, 'nothing to reveal at the end');
    // the tie: the owner of the voted company picks in the aftermath
    await sauron.goto(`/campaigns/${cid}/matches/${mid}/aftermath/${sCo}`);
    const txt = await sauron.text();
    assert.match(txt, /MVP votes from the live match/);
    assert.match(txt, /Tied: .*Choose the MVP among them below \(or none\)\./);
    assert.equal(await sauron.page.$$eval('#mvp-hint li', (l) => l.length), 2, 'both voted units are listed with their votes');
    assert.equal(await sauron.page.$$eval('input[name$="-mvp"]:checked', (c) => c.length), 0, 'no MVP is ticked for the player');
    await sauron.page.screenshot({ path: 'out/aftermath-mvp-tie.png' });
  });
}
