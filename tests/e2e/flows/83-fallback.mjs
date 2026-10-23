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
  // The succession form: its legend, radios and button.
  const form = (s) => s.page.evaluate(() => {
    const f = [...document.querySelectorAll('form')].find((x) => /has no hero left/.test(x.innerText));
    if (!f) return null;
    const radios = [...f.querySelectorAll('input[type=radio]')].map((r) => ({ value: r.value, checked: r.checked, required: r.required, label: r.closest('label').innerText.replace(/\s+/g, ' ').trim() }));
    return { legend: f.querySelector('legend').innerText.replace(/\s+/g, ' ').trim(), radios, button: f.querySelector('button')?.innerText.trim(), text: f.innerText.replace(/\s+/g, ' ') };
  });

  await h.step('fallback: one Warrior ahead in Experience is offered alone, already chosen, with no "nobody" option', async () => {
    await open(sauron, 'Orcs with one clear veteran');
    const f = await form(sauron);
    assert.ok(f, 'the succession form is on the page');
    assert.match(f.legend, /A Sergeant has fallen and the company has no hero left\./);
    assert.match(f.legend, /The Warrior with the most Experience becomes a Sergeant/);
    assert.equal(f.radios.length, 1, 'one Warrior listed');
    assert.ok(f.radios[0].checked && f.radios[0].required, 'it is chosen and the answer is mandatory');
    assert.match(f.radios[0].label, /Mordor Orc Warrior.*Experience.*pts/);
    assert.ok(!f.radios.some((r) => r.value === ''), 'there is no "nobody" answer');
    assert.ok(!/Nobody/i.test(f.text), 'the word nobody is not offered');
    assert.equal(f.button, 'Choose');
  });

  await h.step('fallback: two Warriors tied for the most Experience are both listed, nobody is chosen for the player, an answer is required', async () => {
    await open(gimli, 'Dwarves with two equal veterans');
    const f = await form(gimli);
    assert.ok(f);
    assert.match(f.legend, /2 Warriors are tied for the most Experience \(then the highest rating\): choose which one becomes a Sergeant/);
    assert.equal(f.radios.length, 2);
    assert.ok(f.radios.every((r) => !r.checked && r.required), 'none is pre-selected and each is required');
    assert.ok(!f.radios.some((r) => r.value === ''), 'no "nobody" answer');
    const xps = f.radios.map((r) => r.label.match(/(\d+) Experience/)[1]);
    assert.equal(xps[0], xps[1], 'the tied Warriors have the same Experience');
    assert.ok(Number(xps[0]) >= 3);
    // submitting without choosing is refused by the browser (required radio), nothing is sent
    const before = gimli.reqCount;
    await gimli.page.evaluate(() => [...document.querySelectorAll('form')].find((x) => /has no hero left/.test(x.innerText)).querySelector('button').click());
    await gimli.settle(300);
    assert.equal(gimli.reqCount, before, 'no request without an answer');
    assert.ok(await gimli.page.evaluate(() => !!document.querySelector('input[type=radio]:invalid')), 'the browser flags the missing answer');
  });

  await h.step('fallback: a Leader who fell with both Sergeants is replaced from the Warriors, and the answer is mandatory', async () => {
    await open(aragorn, 'Rangers who lost every officer');
    const f = await form(aragorn);
    assert.ok(f);
    assert.match(f.legend, /The Leader has fallen and the company has no hero left\./);
    assert.match(f.legend, /becomes the Leader|choose which one becomes the Leader/);
    assert.ok(f.radios.length >= 1 && f.radios.every((r) => r.required) && !f.radios.some((r) => r.value === ''));
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
    const pick = f.radios[1];
    await gimli.page.evaluate((v) => document.querySelector(`input[type=radio][value="${v}"]`).click(), pick.value);
    await gimli.click('form button', '=Choose');
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
    // a path cannot be taken while the aftermath is open: the refusal says so and nothing changes
    gimli.allow.push(/422/);
    await gimli.click(MODAL + ' label', '');
    await gimli.click(MODAL + ' button', 'Take this path');
    await gimli.settle(400);
    assert.match(await gimli.page.$eval('#modal-body', (m) => m.innerText), /Finish the aftermath of Match 1 first\./);
    await gimli.page.keyboard.press('Escape');
    // finish the aftermath, then the path can be taken
    await open(gimli, 'Dwarves with two equal veterans');
    await gimli.click('button', 'Finish aftermath');
    await gimli.settle(500);
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
