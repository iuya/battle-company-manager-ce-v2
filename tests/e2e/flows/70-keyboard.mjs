// Keyboard walk (R10): Tab reaches the controls in order with a visible focus ring, never gets trapped, and a modal sheet keeps the
// focus inside, closes on Escape and gives the focus back.
import { assert, campaignId, MODAL } from '../lib/util.mjs';

// Presses Tab `max` times and describes every focused element: visible ring, inside the viewport once scrolled.
async function walk(s, max) {
  const seen = [];
  for (let i = 0; i < max; i++) {
    await s.page.keyboard.press('Tab');
    const f = await s.page.evaluate(() => {
      const e = document.activeElement;
      if (!e || e === document.body) return { body: true };
      const cs = getComputedStyle(e);
      const ring = (cs.outlineStyle !== 'none' && parseFloat(cs.outlineWidth) > 0) || cs.boxShadow !== 'none';
      const r = e.getBoundingClientRect();
      const name = (e.getAttribute('aria-label') || e.innerText || e.value || e.getAttribute('href') || '').replace(/\s+/g, ' ').trim().slice(0, 40);
      return { tag: e.tagName.toLowerCase(), name, ring, size: r.width > 0 && r.height > 0, key: e.tagName + '|' + name + '|' + Math.round(r.top + scrollY) + '|' + Math.round(r.left) };
    });
    if (f.body) break; // the focus left the page (the browser chrome): one full round
    if (seen.some((x) => x.key === f.key)) break; // wrapped around
    seen.push(f);
  }
  return seen;
}

export default async function (h) {
  const eowyn = await h.session('eowyn@example.com');
  const anon = await h.session('anon', { login: false });
  let cid;

  await h.step('keyboard: the sign-in form is reachable in order with a visible focus ring', async () => {
    await anon.goto('/login');
    const seen = await walk(anon, 30);
    assert.ok(seen.length >= 3, `Tab reaches the controls (${seen.length})`);
    const bad = seen.filter((f) => !f.ring || !f.size).map((f) => `${f.tag} "${f.name}"`);
    assert.equal(bad.length, 0, 'no focus ring: ' + bad.join(', '));
    assert.ok(seen.some((f) => f.tag === 'input'), 'an input is reached');
  });

  await h.step('keyboard: a company page, every stop has a focus ring and nothing traps the focus', async () => {
    cid = await campaignId(eowyn, 'Companies being built');
    await eowyn.goto(`/campaigns/${cid}/my-company`);
    const seen = await walk(eowyn, 120);
    assert.ok(seen.length >= 10, `Tab reaches the controls (${seen.length})`);
    const bad = seen.filter((f) => f.size && !f.ring).map((f) => `${f.tag} "${f.name}"`);
    assert.equal(bad.length, 0, 'no focus ring: ' + [...new Set(bad)].join(', '));
  });

  await h.step('keyboard: a sheet opens with Enter, keeps the focus inside, closes with Escape and returns the focus', async () => {
    await eowyn.goto(`/campaigns/${cid}/my-company`);
    await eowyn.page.evaluate(() => { [...document.querySelectorAll('main button')].find((b) => b.innerText.includes('Recruit')).focus(); });
    const opener = await eowyn.page.evaluate(() => document.activeElement.innerText);
    await eowyn.page.keyboard.press('Enter');
    await eowyn.settle(500);
    assert.ok(await eowyn.page.$(MODAL), 'the sheet is open');
    for (let i = 0; i < 25; i++) {
      await eowyn.page.keyboard.press('Tab');
      const inside = await eowyn.page.evaluate((m) => !!document.activeElement.closest(m), MODAL);
      assert.ok(inside, `the focus stays inside the sheet (Tab ${i + 1})`);
    }
    await eowyn.page.keyboard.press('Escape');
    await eowyn.settle(400);
    const open = await eowyn.page.evaluate((m) => [...document.querySelectorAll(m)].some((e) => e.getBoundingClientRect().height > 0 && getComputedStyle(e).visibility !== 'hidden'), MODAL);
    assert.ok(!open, 'Escape closes the sheet');
    const back = await eowyn.page.evaluate(() => document.activeElement.innerText);
    assert.equal(back, opener, 'the focus returns to the button that opened the sheet');
  });
}
