// A three-company match on the Limits campaign: Fog of War's secret objectives on the match page, a cavalry company's live card
// (granted rules, the mount lost), and a tie in the MVP votes of the end checks (two opposing players vote for different units).
import { assert, campaignId, MODAL, layout, SIZES } from '../lib/util.mjs';

export default async function (h) {
  const you = await h.session('you@example.com');
  const eowyn = await h.session('eowyn@example.com');
  const gimli = await h.session('gimli@example.com');
  const sauron = await h.session('sauron@example.com');
  let cid, mid, eCo, gCo, sCo;

  await h.step('match: a cavalry company and the Dwarves against the Helm\'s Deep garrison, Fog of War', async () => {
    cid = await campaignId(you, 'Limits, creatures');
    await you.goto(`/campaigns/${cid}/matches/new`);
    const pick = async (side, text) => {
      await you.page.evaluate((side, text) => {
        let el = document.querySelector(`[name=side-name-${side}]`);
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
    await you.click('main button', 'Choose scenario');
    await you.click('main label', 'Fog of War');
    await you.click('main button', 'Use this scenario');
    await you.waitText('Fog of War');
  });
  if (!mid) return;

  await h.step('secret objectives: the match page lists the three Fog of War objectives by label, with distinct heroes to note, and keeps them private', async () => {
    await eowyn.goto(`/campaigns/${cid}/matches/${mid}`);
    await eowyn.page.waitForSelector('#secret-objectives');
    await eowyn.settle(500);
    const labels = await eowyn.page.$$eval('#secret-objectives fieldset > legend', (l) => l.map((x) => x.innerText.trim()));
    assert.deepEqual(labels, ['Own hero survives', 'Enemy hero harmed', 'Terrain piece'], 'the objectives are listed by their label, not their key');
    assert.ok(!/ownHeroSurvives|enemyHeroHarmed|terrainPiece/.test(await eowyn.page.$eval('#secret-objectives', (e) => e.innerText)), 'no internal key is shown');
    // the heroes to choose from have distinct names (two Sergeants of one kind are told apart)
    const opts = await eowyn.page.$$eval('#secret-objectives select[name=unit]', (s) => s.map((x) => [...x.options].map((o) => o.text)));
    assert.equal(opts.length, 2, 'two nomination lists (own hero, enemy hero)');
    for (const list of opts) assert.equal(new Set(list).size, list.length, `heroes are told apart: ${list}`);
    assert.ok(opts[0].some((t) => /Sergeant\) \d/.test(t)) || opts[0].length >= 2, 'own heroes listed');
    // note a hero and a tier, and see them come back
    const own = await eowyn.page.$eval('#secret-objectives select[name=unit]', (s) => { const o = [...s.options].find((x) => x.value !== '0'); return { value: o.value, text: o.text }; });
    await eowyn.page.select('#secret-objectives select[name=unit]', own.value);
    const mark = eowyn.reqs.length;
    await eowyn.click('#secret-objectives form button', 'Save note');
    await eowyn.settle(400);
    await eowyn.waitText(`Noted: ${own.text}`);
    const sent = eowyn.reqs.slice(mark).filter((r) => r.includes('> '));
    assert.equal(sent.filter((r) => /GET .*\/secret$/.test(r)).length, 0, `a click inside the card must not refetch it (it raced the save): ${sent}`);
    assert.equal(await eowyn.page.$eval('#secret-objectives select[name=unit]', (s) => s.options[s.selectedIndex].text), own.text, 'the noted hero is the one selected after the save');
    await eowyn.page.evaluate(() => document.querySelector('#secret-objectives input[name=tier][value="2"]').click());
    await eowyn.page.evaluate(() => [...document.querySelectorAll('#secret-objectives form')].find((f) => f.querySelector('input[name=tier]')).querySelector('button').click());
    await eowyn.settle(400);
    assert.ok(await eowyn.page.$eval('#secret-objectives input[name=tier][value="2"]', (r) => r.checked), 'the tier is kept');
    // the opposing player sees that Riders noted something, not what
    await sauron.goto(`/campaigns/${cid}/matches/${mid}`);
    await sauron.page.waitForSelector('#secret-objectives');
    const st = await sauron.page.$eval('#secret-objectives', (e) => e.innerText);
    assert.match(st, /Noted already: Riders on every horse/);
    assert.ok(!st.includes(own.text.split(' (')[0] + ' (Sergeant)') || own.text === '' || !/Noted: /.test(st), 'the other player sees no note of Éowyn');
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(eowyn, w, scheme);
      assert.ok(await eowyn.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      await (await eowyn.page.$('#secret-objectives')).screenshot({ path: `out/secret-objectives-${w}-${scheme}.png` });
    }
    await layout(eowyn, 1280, 'light');
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
    await eowyn.click('main button', '=Actions');
    await eowyn.click(MODAL + ' button', 'Mount removed');
    await eowyn.click(MODAL + ' button', '=Confirm');
    await eowyn.waitText('On foot');
    const after = await cards(eowyn);
    assert.equal(after.filter((c) => c.onFoot).length, 1, 'one unit is on foot');
    assert.ok(after[0].onFoot, 'the Rider is on foot');
    assert.equal(after[0].gear.find((g) => /^horse/i.test(g.name)).struck, 'the mount is lost', 'its horse is struck through with the reason');
    assert.ok(after[0].rules.filter((r) => r.struck).every((r) => r.name === 'Cavalry'), 'only the rules the mount gave are struck');
    for (let i = 1; i < after.length; i++) assert.deepEqual(after[i].gear, before[i].gear, `the other units keep their gear (${after[i].title})`);
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

  // The MVP votes of the end checks: each player votes for a unit of the company he fought.
  const vote = async (s, unitIndex) => {
    await s.goto(`/campaigns/${cid}/matches/${mid}`);
    await s.settle(400);
    await s.page.evaluate((i) => [...document.querySelectorAll('[role=radiogroup][aria-label^="MVP of"] button[role=radio]')][i].click(), unitIndex);
    await s.settle(500);
  };
  const tally = (s) => s.page.evaluate(() => {
    const h3 = [...document.querySelectorAll('h3')].find((x) => /^Votes ·/.test(x.innerText));
    if (!h3) return { lines: [], tied: '' };
    const box = h3.parentElement;
    return { lines: [...box.querySelectorAll('li')].map((l) => l.innerText.replace(/\s+/g, ' ').trim()), tied: (box.innerText.match(/Tied:[^\n]*/) || [''])[0] };
  });

  await h.step('MVP votes: two opposing players vote for different units, the end checks say it is a tie; one more vote breaks it', async () => {
    await vote(eowyn, 0);
    let t = await tally(eowyn);
    assert.equal(t.lines.length, 1, 'one unit has a vote');
    assert.match(t.lines[0], /1 vote$/);
    assert.equal(t.tied, '', 'no tie with a single vote');
    await vote(gimli, 1);
    t = await tally(gimli);
    assert.equal(t.lines.length, 2, 'two units have one vote each');
    assert.ok(t.lines.every((l) => /1 vote$/.test(l)));
    assert.equal(t.tied, "Tied: the owner of Garrison of Helm's Deep picks in the aftermath.", 'the tie is named and says who decides');
    // the voter's own choice is marked
    assert.equal(await gimli.page.$$eval('[role=radiogroup][aria-label^="MVP of"] button[aria-checked=true]', (b) => b.length), 1);
    // Gimli changes his vote to Éowyn's unit: one unit with two votes, no tie
    await vote(gimli, 0);
    t = await tally(gimli);
    assert.equal(t.lines.length, 1);
    assert.match(t.lines[0], /2 votes$/);
    assert.equal(t.tied, '', 'two votes for one unit is no tie');
    // and back to a tie, to be settled in the aftermath
    await vote(gimli, 1);
    t = await tally(gimli);
    assert.match(t.tied, /^Tied: the owner of Garrison of Helm's Deep picks in the aftermath\.$/);
    await gimli.page.screenshot({ path: 'out/mvp-tie.png' });
  });

  await h.step('match end: all three players confirm; the secret notes are revealed with their score and the tie is left to the owner in the aftermath', async () => {
    await eowyn.goto(`/campaigns/${cid}/matches/${mid}`);
    await eowyn.click('main label', 'Draw');
    await eowyn.click('main button', 'Save result');
    await eowyn.click('main button', 'Propose to end');
    await eowyn.click('main button', 'Propose anyway');
    await eowyn.waitText('proposed');
    for (const p of [gimli, sauron]) {
      await p.goto(`/campaigns/${cid}/matches/${mid}`);
      await p.click('main button', 'Confirm the end');
    }
    await sauron.waitText('Ended');
    await sauron.goto(`/campaigns/${cid}/matches/${mid}`);
    await sauron.page.waitForSelector('#secret-objectives');
    const rev = await sauron.page.$eval('#secret-objectives', (e) => e.innerText);
    assert.match(rev, /The match is over: the notes are revealed\./);
    assert.match(rev, /Riders on every horse · Own hero survives/, 'the label is shown, with the company');
    assert.match(rev, /Noted: Warrior of Rohan \(Sergeant\) \d/);
    assert.match(rev, /Scored 3 VP/, 'tier 2 is worth 3 VP');
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
