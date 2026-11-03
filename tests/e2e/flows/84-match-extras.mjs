// A three-company match on the Limits campaign: Fog of War's secret objectives on the match page, a cavalry company's live card
// (granted rules, the mount lost), and a tie in the MVP votes of the end sheets (two opposing players vote for different units).
import { chooseScenario, endMatch, confirmEnd, assert, campaignId, MODAL, layout, SIZES } from '../lib/util.mjs';

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

  await h.step('match end: all three players confirm; the secret notes are revealed with their score and the tie is left to the owner in the aftermath', async () => {
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
