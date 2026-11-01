// The armoury lists every purchase a company could ever make and hides only the impossible ones (docs/frontend.md, "The Armory tab"; GitHub issue 57).
// Aragorn's fresh Arnor company of "Companies being built": its officers are Warriors of Arnor in Heavy Armour and its warriors may not take armour, so
// no unit can buy Armour or Heavy Armour now, yet a Ranger of the North may. Both are listed, greyed, with the reason (aria-disabled, described by the
// reason, reachable with the keyboard, still opening the per-unit reasons); a Warg and a halberd, which no Arnor unit can ever have, are hidden by the
// switch, which brings them back; the list is checked at 360/768/1280 px in light, dark and high contrast (no horizontal scroll, axe on the list).
import { assert, campaignId } from '../lib/util.mjs';
import { axeScoped, eachLook, resetLook, setLook } from '../lib/axe.mjs';

const LIST = '#armory-items';
const row = (name) => `${LIST} li[data-name="${name}"]`;

export default async function (h, { only }) {
  const s = await h.session('aragorn@example.com');
  let cid;
  const open = async () => {
    cid ||= await campaignId(s, 'Companies being built');
    await s.goto(`/campaigns/${cid}/armory`);
    await s.page.waitForSelector(LIST);
  };
  // What a person sees of an item: shown or not, its button, and the reason that describes the button.
  const look = (name) => s.page.evaluate((sel) => {
    const li = document.querySelector(sel);
    if (!li) return null;
    const b = li.querySelector('button[aria-expanded]');
    const d = b.getAttribute('aria-describedby');
    return {
      shown: li.getBoundingClientRect().height > 0,
      label: b.innerText.trim(),
      disabled: b.getAttribute('aria-disabled'),
      focusable: !b.disabled && b.tabIndex >= 0,
      why: d ? (document.getElementById(d)?.innerText || '').replace(/\s+/g, ' ').trim() : '',
    };
  }, row(name));

  await h.step('armoury (fresh Arnor): Armour and Heavy Armour are listed, greyed, with the reason and who could take them later; a Warg and a halberd are not', async () => {
    await open();
    await s.page.evaluate(() => { try { localStorage.clear(); } catch (e) { /* none */ } });
    await open();
    const armour = await look('Armour');
    assert.ok(armour?.shown, 'Armour is listed');
    assert.equal(armour.label, 'No unit can take it now');
    assert.equal(armour.disabled, 'true', 'the button is aria-disabled');
    assert.ok(armour.focusable, 'and still reachable with the keyboard');
    assert.match(armour.why, /^No unit can take it now: /);
    assert.match(armour.why, /Ranger of the North may take it/, 'the reason says who could take it later');
    const heavy = await look('Heavy Armour');
    assert.ok(heavy?.shown, 'Heavy Armour is listed');
    assert.match(heavy.why, /Ranger of the North may take it after Armour\./, 'armour goes one stage at a time');
    for (const gone of ['Warg', 'Easterling Halberd', 'Fell Warg']) {
      const l = await look(gone);
      assert.ok(!l || !l.shown, `${gone} can never be bought in Arnor and is not shown`);
    }
    assert.match(await s.text(), /hidden because no unit of the company could ever take them/);
    assert.match(await s.text(), /Hide what .+ could never take/);
  }, { only });

  await h.step('armoury (fresh Arnor): the greyed button is reached with Tab and opens the reasons of every unit with Enter', async () => {
    await open();
    await s.page.$eval(`${row('Armour')} button[aria-expanded]`, (b) => b.scrollIntoView({ block: 'center' }));
    // walk the page with Tab until the focus lands on the button: it is in the tab order
    let landed = false;
    for (let i = 0; i < 400 && !landed; i++) {
      await s.page.keyboard.press('Tab');
      landed = await s.page.evaluate((sel) => document.activeElement === document.querySelector(`${sel} button[aria-expanded]`), row('Armour'));
    }
    assert.ok(landed, 'Tab reaches the Armour button');
    await s.page.keyboard.press('Enter');
    await s.page.waitForSelector(`${row('Armour')} ul[aria-label=Units] li`);
    assert.equal(await s.page.$eval(`${row('Armour')} button[aria-expanded]`, (b) => b.getAttribute('aria-expanded')), 'true');
    const text = await s.page.$eval(`${row('Armour')} ul[aria-label=Units]`, (u) => u.innerText.replace(/\s+/g, ' '));
    assert.match(text, /Already wears this armour or better/, 'every unit of a fresh Arnor company wears Armour or better');
    assert.equal(await s.page.$$eval(`${row('Armour')} form[action$="/armory/buy"]`, (f) => f.length), 0, 'nobody can buy it: no buy form');
  }, { only });

  await h.step('armoury (fresh Arnor): the switch brings back what no unit could ever take, and its choice is remembered', async () => {
    await open();
    const sw = () => s.page.$eval('button[role=switch]', (b) => b.getAttribute('aria-checked'));
    assert.equal(await sw(), 'true');
    await s.click('button[role=switch]');
    assert.equal(await sw(), 'false');
    const warg = await look('Warg');
    assert.ok(warg?.shown, 'the Warg shows with the switch off');
    await open();
    assert.equal(await sw(), 'false', 'remembered after a reload');
    await s.click('button[role=switch]');
    assert.ok(!(await look('Warg')).shown, 'and hidden again with the switch on');
  }, { only });

  await eachLook(h, 'armoury (fresh Arnor)', only, async (w, scheme, contrast, axeHere) => {
    await setLook(s, w, scheme, contrast);
    await open();
    assert.ok(await s.noHScroll(), 'no horizontal scroll');
    const armour = await look('Armour');
    assert.ok(armour.shown && armour.disabled === 'true', 'Armour is listed and greyed');
    // the button is greyed with tokens, not dimmed: its text reads against its own background
    const c = await s.page.$eval(`${row('Armour')} button[aria-expanded]`, (b) => {
      const st = getComputedStyle(b);
      return { fg: st.color, bg: st.backgroundColor, opacity: st.opacity };
    });
    assert.equal(c.opacity, '1', 'not dimmed with opacity');
    if (axeHere) assert.deepEqual(await axeScoped(s, [LIST]), [], 'axe on the armoury list');
    await (await s.page.$(row('Heavy Armour'))).screenshot({ path: `out/armoury-possible-${w}-${scheme}${contrast === 'high' ? '-high' : ''}.png` });
  });

  await h.step('armoury (fresh Arnor): back to the default look', async () => { await resetLook(s); }, { only });
}
