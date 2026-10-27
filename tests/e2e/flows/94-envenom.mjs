// Envenom Weapon (X): a weapon modifier that gives the UNIT Poisoned Attacks (<weapon>) for the weapon the player chooses at purchase
// (docs/frontend.md, "Weapon modifiers"). Bought from the buy window and from the Armory tab with the same weapon chooser; the weapon
// stays plain, the choice is final (no chooser, no "Change weapon"), dropping the weapon keeps the rule and buying it again shows the rule once, a weapon already poisoned is not offered, and a unit that cannot take it gets the reason instead of a button
// (the "no weapon left" reason itself is a handler test: no demo unit reaches it). Mordor's gallery company (60 influence, orc warriors with one small slot each) and Harad's (a Bow poisoned by the profile).
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
const axeOn = async (s, ctx) => {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((ctx) => axe.run(ctx, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id}: ${v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' ' + (x.any[0]?.message || '')).join(' | ')}`)), ctx);
};

const ENV = 'li[data-name="Envenom Weapon (X)"]';
const norm = (t) => t.replace(/\s+/g, ' ').trim();

export default async function (h) {
  const s = await h.session('mordor@example.com');
  let g;

  // The units of the Envenom entry on the Armory tab: id, the weapons the chooser offers, the reason when there is no button.
  const rows = async () => {
    await s.goto(`/campaigns/${g}/armory`);
    await s.click(`${ENV} button`, 'Buy for');
    return s.page.evaluate((ENV) => [...document.querySelectorAll(`${ENV} ul li`)].map((l) => ({
      id: l.querySelector('[name=unit]')?.value || null,
      weapons: [...l.querySelectorAll('select[name=weapon] option')].map((o) => o.value).filter(Boolean),
      text: l.innerText.replace(/\s+/g, ' ').trim(),
    })), ENV);
  };
  const withWeapons = (all, ...ws) => all.find((r) => r.id && r.weapons.join('|') === ws.join('|'));
  // Buys an item of the Armory tab for a unit (the chosen weapon when the item asks for one).
  const armoryBuy = async (item, unit, weapon) => {
    await s.goto(`/campaigns/${g}/armory`);
    await s.click(`li[data-name="${item}"] button`, 'Buy for');
    const form = `li[data-name="${item}"] form:has(input[name=unit][value="${unit}"])`;
    if (weapon) await s.select(`${form} select[name=weapon]`, weapon);
    await s.click(`${form} button`, '=Buy · 1 influence');
    await s.waitText(`Bought ${item}.`);
  };
  // The expanded card of a unit on the company page.
  const card = async (unit) => {
    await s.goto(`/campaigns/${g}/my-company`);
    await s.click(`#unit-${unit} .unit-corner`);
    await s.page.waitForSelector(`#unit-${unit}.unit-expanded`, { visible: true });
    return s.page.$eval(`#unit-${unit}`, (a) => ({
      text: a.innerText.replace(/\s+/g, ' '),
      items: [...a.querySelectorAll('li.eq-melee p.font-medium, li.eq-two_handed p.font-medium, li.eq-spear p.font-medium, li.eq-bow p.font-medium, li[class*="eq-"] p.font-medium')].map((p) => p.innerText.replace(/\s+/g, ' ').trim()),
      choices: [...a.querySelectorAll('form[action$="/equipment/weapon"] select option')].map((o) => o.value).filter(Boolean),
    }));
  };
  const count = (text, needle) => text.split(needle).length - 1;

  let a, x, y, z; // the units: bow, spear, two-handed, hand weapon only

  await h.step('envenom: the Armory tab asks for the weapon of every unit that has one, built from that unit\'s weapons', async () => {
    g = await campaignId(s, 'Gallery');
    const all = await rows();
    a = withWeapons(all, 'Hand weapon', 'Orc Bow');
    x = withWeapons(all, 'Hand weapon', 'Spear');
    y = withWeapons(all, 'Hand weapon', 'Two-handed weapon');
    z = withWeapons(all, 'Hand weapon');
    assert.ok(a && x && y && z, 'the gallery Mordor company has orcs with a bow, a spear, a two-handed weapon and only a hand weapon');
    assert.match(await s.page.$eval(`${ENV} ul li select[name=weapon]`, (e) => e.options[0].text), /Choose a weapon/, 'no weapon is preselected');
    assert.equal(await s.page.$eval(`${ENV} ul li select[name=weapon]`, (e) => e.required), true);
  });
  if (!a || !x || !y || !z) return;

  await h.step('envenom: the buy window of a unit asks for the weapon; the unit gets Poisoned Attacks (Orc Bow) once and the bow stays plain', async () => {
    await s.goto(`/campaigns/${g}/my-company`);
    await s.click(`.unit-slot[data-unit-id="${a.id}"] button[aria-label^="Buy equipment for"]`);
    const li = '#modal-body li[data-find^="Envenom Weapon"]';
    await s.page.waitForSelector(`${li} select[name=weapon]`, { visible: true });
    assert.deepEqual(await s.page.$$eval(`${li} select[name=weapon] option`, (o) => o.map((e) => e.value).filter(Boolean)), ['Hand weapon', 'Orc Bow'], 'the unit\'s own weapons');
    await s.select(`${li} select[name=weapon]`, 'Orc Bow');
    await s.click(`${li} button`, '=Buy · 1 influence');
    await s.waitText('Bought Envenom Weapon (X).');
    const c = await card(a.id);
    assert.equal(count(c.text, 'Poisoned Attacks (Orc Bow)'), 1, 'the rule is on the card once: ' + c.text.slice(0, 400));
    assert.ok(c.items.some((t) => /^Orc Bow\b/.test(t)) && !c.items.some((t) => /^Orc Bow\b.*oison/i.test(t)), 'the bow is a plain item: ' + c.items.join(' / '));
    assert.ok(c.items.some((t) => /^Envenom Weapon \(Orc Bow\)/.test(t)), 'the modifier names its weapon: ' + c.items.join(' / '));
  });

  await h.step('envenom: the Armory tab buys it with a chosen weapon; the choice is final (no chooser, no Change weapon, no Choose weapon)', async () => {
    await armoryBuy('Envenom Weapon (X)', x.id, 'Spear');
    const c = await card(x.id);
    assert.equal(count(c.text, 'Poisoned Attacks (Spear)'), 1, 'once: ' + c.text.slice(0, 400));
    assert.ok(!c.items.some((t) => /^Spear\b.*oison/i.test(t)), 'the spear stays plain');
    assert.ok(c.items.some((t) => /^Envenom Weapon \(Spear\)/.test(t)), 'the modifier names its weapon: ' + c.items.join(' / '));
    assert.deepEqual(c.choices, [], 'nothing to choose any more');
    assert.ok(!/Change weapon|Choose weapon/.test(c.text), 'no way to change or choose: ' + c.text.slice(0, 400));
  });

  await h.step('envenom: dropping the poisoned weapon keeps the rule, buying it again shows the rule once', async () => {
    await card(x.id);
    await s.click(`#unit-${x.id} form[action$="/equipment/drop"]:has(input[name=item][value="Spear"]) button`);
    await s.waitText('Dropped Spear.');
    let c = await card(x.id);
    assert.ok(!c.items.some((t) => /^Spear\b/.test(t)), 'the spear is gone: ' + c.items.join(' / '));
    assert.equal(count(c.text, 'Poisoned Attacks (Spear)'), 1, 'the rule stays once: ' + c.text.slice(0, 400));
    assert.ok(c.items.some((t) => /^Envenom Weapon \(Spear\)/.test(t)), 'the modifier stays: ' + c.items.join(' / '));
    assert.ok(!/Change weapon|Choose weapon/.test(c.text));
    await armoryBuy('Spear', x.id);
    c = await card(x.id);
    assert.ok(c.items.some((t) => /^Spear\b/.test(t)), 'the spear is back: ' + c.items.join(' / '));
    assert.equal(count(c.text, 'Poisoned Attacks (Spear)'), 1, 'poisoned again, shown once: ' + c.text.slice(0, 400));
    assert.ok(!/Change weapon|Choose weapon/.test(c.text));
  });

  await h.step('envenom: a weapon already poisoned is not offered (the Haradrim Warrior\'s profile poisons its Bow)', async () => {
    const harad = await h.session('harad@example.com');
    const hg = await campaignId(harad, 'Gallery');
    await harad.goto(`/campaigns/${hg}/armory`);
    await harad.click(`${ENV} button`, 'Buy for');
    const unit = await harad.page.evaluate((ENV) => [...document.querySelectorAll(`${ENV} ul li`)].filter((l) => l.querySelector('select')).map((l) => ({ id: l.querySelector('[name=unit]').value, w: [...l.querySelectorAll('select option')].map((o) => o.value).filter(Boolean) })).find((r) => r.w.join('|') === 'Hand weapon' || r.w.join('|') === 'Hand weapon|Spear'), ENV);
    assert.ok(unit, 'a Haradrim Warrior in the armory');
    await harad.goto(`/campaigns/${hg}/my-company`);
    await harad.click(`#unit-${unit.id} .unit-corner`);
    const text = await harad.page.$eval(`#unit-${unit.id}`, (a) => a.innerText.replace(/\s+/g, ' '));
    assert.match(text, /Poisoned Attacks \(Bow\)/, 'the profile poisons the Bow');
    assert.ok(!unit.w.includes('Bow'), 'the poisoned Bow is not offered: ' + unit.w);
  });

  await h.step('envenom: a unit that carries it cannot buy it again: the reason replaces the button (Armory tab; the buy window does not list it)', async () => {
    await armoryBuy('Envenom Weapon (X)', z.id, 'Hand weapon');
    const c = await card(z.id);
    assert.equal(count(c.text, 'Poisoned Attacks (Hand weapon)'), 1, c.text.slice(0, 400));
    assert.equal((await rows()).find((r) => !r.id && /Already carried/.test(r.text)) !== undefined, true, 'the unit has a reason and no form');
    assert.equal(await s.page.$$eval(`${ENV} ul li`, (l) => l.filter((e) => /Already carried/.test(e.innerText) && e.querySelector('form')).length), 0, 'no button');
    await s.goto(`/campaigns/${g}/my-company`);
    await s.click(`.unit-slot[data-unit-id="${z.id}"] button[aria-label^="Buy equipment for"]`);
    await s.page.waitForSelector('#modal-body li[data-find]', { visible: true });
    assert.equal(await s.page.$('#modal-body li[data-find^="Envenom Weapon"] form'), null, 'no buy form in the window (the item is not even listed)');
    await s.page.keyboard.press('Escape');
  });

  await h.step('envenom: 360/768/1280 light and dark, no horizontal scroll, no axe violation (Armory tab with the entry open, the buy window)', async () => {
    for (const w of SIZES) for (const scheme of ['light', 'dark']) {
      await layout(s, w, scheme);
      const all = await rows();
      assert.ok(all.length > 0);
      assert.equal(await s.noHScroll(), true, `armory ${w} ${scheme}`);
      assert.deepEqual(await axeOn(s, 'main'), [], `axe armory ${w} ${scheme}`);
      await s.page.screenshot({ path: `out/envenom-armory-${w}-${scheme}.png`, fullPage: true });
      await s.goto(`/campaigns/${g}/my-company`);
      await s.click(`.unit-slot[data-unit-id="${a.id}"] button[aria-label^="Buy equipment for"]`);
      await s.page.waitForSelector('#modal-body li[data-find]', { visible: true });
      assert.equal(await s.noHScroll(), true, `window ${w} ${scheme}`);
      assert.deepEqual(await axeOn(s, '#modal-body'), [], `axe window ${w} ${scheme}`);
      await s.page.keyboard.press('Escape');
      await s.settle(200);
    }
    await layout(s, 1280, 'light');
  });
}
