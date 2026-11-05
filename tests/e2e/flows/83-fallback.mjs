// The Warrior-to-Hero fallback of the aftermath: a Leader or Sergeant fell and no hero is left, so a Warrior takes the place
// (the one with the most Experience, ties by rating: the player chooses among the tied). Companies of "Aftermath ward 3".
import { assert, campaignId, layout, SIZES, MODAL } from '../lib/util.mjs';

export default async function (h) {
  const sauron = await h.session('sauron@example.com');
  const gimli = await h.session('gimli@example.com');
  const aragorn = await h.session('aragorn@example.com');
  const urls = {};

  // The aftermath page of a company of the ward, found through the match page.
  async function open(s, name) {
    if (!urls[name]) {
      const cid = await campaignId(s, 'Aftermath ward 3');
      await s.goto(`/campaigns/${cid}/matches`);
      const links = await s.page.$$eval('main a[href]', (a) => [...new Set(a.map((x) => x.getAttribute('href')).filter((x) => /\/matches\/\d+$/.test(x)))]);
      await s.goto(links[0]);
      urls[name] = await s.page.evaluate((n) => { const a = [...document.querySelectorAll('main a[href]')].find((x) => /\/aftermath\/\d+$/.test(x.getAttribute('href')) && x.innerText.includes(n)); return a && a.getAttribute('href'); }, name);
      assert.ok(urls[name], `an aftermath link for ${name}`);
    }
    await s.goto(urls[name]);
  }
  // The succession form: its legend and the cards of the Warriors, each answered by its own button.
  const form = (s) => s.page.evaluate(() => {
    const f = [...document.querySelectorAll('form')].find((x) => /has no hero left/.test(x.innerText));
    if (!f) return null;
    const answers = [...f.querySelectorAll('button[name=answer]')].map((b) => ({ value: b.value, label: b.closest('article').innerText.replace(/\s+/g, ' ').trim() }));
    return { legend: f.querySelector('legend').innerText.replace(/\s+/g, ' ').trim(), answers, radios: f.querySelectorAll('input[type=radio]').length, text: f.innerText.replace(/\s+/g, ' ') };
  });

  await h.step('fallback: one Warrior ahead in Experience is offered alone, with no "nobody" option and one button', async () => {
    await open(sauron, 'Orcs with one clear veteran');
    const f = await form(sauron);
    assert.ok(f, 'the succession form is on the page');
    assert.match(f.legend, /A Sergeant has fallen and the company has no hero left\./);
    assert.match(f.legend, /The Warrior with the most Experience becomes a Sergeant/);
    assert.equal(f.answers.length, 1, 'one Warrior listed');
    assert.equal(f.radios, 0, 'nothing to tick first');
    assert.match(f.answers[0].label, /Mordor Orc Warrior.*pts.*Experience.*Make Sergeant/);
    assert.ok(!f.answers.some((r) => r.value === ''), 'there is no "nobody" answer');
    assert.ok(!/Nobody/i.test(f.text), 'the word nobody is not offered');
  });

  await h.step('fallback: two Warriors tied for the most Experience are both offered, each with its own button', async () => {
    await open(gimli, 'Dwarves with two equal veterans');
    const f = await form(gimli);
    assert.ok(f);
    assert.match(f.legend, /2 Warriors are tied for the most Experience \(then the highest rating\): choose which one becomes a Sergeant/);
    assert.equal(f.answers.length, 2);
    assert.ok(!f.answers.some((r) => r.value === ''), 'no "nobody" answer');
    const xps = f.answers.map((r) => r.label.match(/(\d+) Experience/)[1]);
    assert.equal(xps[0], xps[1], 'the tied Warriors have the same Experience');
    assert.ok(Number(xps[0]) >= 3);
  });

  await h.step('fallback: a Leader who fell with both Sergeants is replaced from the Warriors, and the answer is mandatory', async () => {
    await open(aragorn, 'Rangers who lost every officer');
    const f = await form(aragorn);
    assert.ok(f);
    assert.match(f.legend, /The Leader has fallen and the company has no hero left\./);
    assert.match(f.legend, /becomes the Leader|choose which one becomes the Leader/);
    assert.ok(f.answers.length >= 1 && !f.answers.some((r) => r.value === ''));
  });

  await h.step('fallback: the screen fits 360, 768 and 1280 px in light and dark', async () => {
    for (const [s, name] of [[gimli, 'Dwarves with two equal veterans'], [sauron, 'Orcs with one clear veteran']]) for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(s, w, scheme);
      await open(s, name);
      assert.ok(await s.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      const el = await s.page.evaluateHandle(() => [...document.querySelectorAll('form')].find((x) => /has no hero left/.test(x.innerText)));
      await el.asElement().screenshot({ path: `out/fallback-${name.split(' ')[0].toLowerCase()}-${w}-${scheme}.png` });
    }
    await layout(gimli, 1280, 'light'); await layout(sauron, 1280, 'light');
  });

  await h.step('fallback: the chosen Warrior becomes the Sergeant and is asked for its path', async () => {
    await open(gimli, 'Dwarves with two equal veterans');
    const f = await form(gimli);
    // one press anywhere on the card answers: it is clicked away from the button
    const card = await gimli.page.$(`button[name=answer][value="${f.answers[1].value}"]`);
    const plate = await card.evaluateHandle((b) => b.closest('article').querySelector('.unit-head'));
    await plate.asElement().evaluate((e) => e.scrollIntoView({ block: 'center' }));
    const box = await plate.asElement().boundingBox();
    const hit = await gimli.page.evaluate((x, y) => { const e = document.elementFromPoint(x, y); return e && `${e.tagName}.${e.className}`; }, box.x + box.width / 2, box.y + box.height / 2);
    assert.match(hit, /unit-choice-go/, 'the card answers wherever it is pressed');
    await gimli.page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
    await gimli.settle(500);
    const t = await gimli.text();
    assert.match(t, /Dwarf Warrior 2 becomes a Sergeant \(Hero in the Making: choose its path\)/);
    assert.ok(!(await form(gimli)), 'the succession question is gone');
    assert.match(t, /Sergeant\) 1: Dead/, 'the fallen Sergeant is dead');
    // the path choice is on My Company: the promoted Warrior is a Sergeant with a path to choose, required
    const cid = await campaignId(gimli, 'Aftermath ward 3');
    await gimli.goto(`/campaigns/${cid}/my-company`);
    const promo = await gimli.page.$$eval('main button.promo', (b) => b.map((x) => x.innerText.replace(/\s+/g, ' ').trim()));
    assert.deepEqual(promo, ['Choose a path Required'], 'exactly one unit asks for its path');
    await gimli.click('main button.promo', 'Choose a path');
    await gimli.page.waitForSelector(MODAL + ' form input[name=path]', { visible: true });
    const modal = await gimli.page.$eval('#modal-body', (m) => m.innerText);
    assert.match(modal, /is a sergeant of the company and has to follow a path/i);
    assert.ok(await gimli.page.$$eval(MODAL + ' input[name=path]', (r) => r.length >= 2 && r.every((x) => x.required)), 'at least two paths, a choice is required');
    await gimli.page.keyboard.press('Escape');
    // the last answer finished the aftermath on its own (nothing else to decide), so the path can be taken at once
    await open(gimli, 'Dwarves with two equal veterans');
    assert.ok(await gimli.page.$('[data-change-summary]'), 'the aftermath finished by itself after the last decision');
    await gimli.goto(`/campaigns/${cid}/my-company`);
    await gimli.click('main button.promo', 'Choose a path');
    await gimli.page.waitForSelector(MODAL + ' form input[name=path]', { visible: true });
    await gimli.click(MODAL + ' label', '');
    await gimli.click(MODAL + ' button', 'Take this path');
    await gimli.settle(500);
    await gimli.goto(`/campaigns/${cid}/my-company`);
    assert.equal(await gimli.page.$$eval('main button.promo', (b) => b.length), 0, 'no unit asks for a path any more');
  });
}
