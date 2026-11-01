// An organiser awards their own company too (GitHub issue 60; docs/matches.md, "Organiser grants and scenario rewards"): the organiser "you" also plays
// the Fiefdoms of Gondor in the demo campaign "Aura parade". The Grants card at the bottom of the My Company page has the award form, a grant is
// applied to the company and written to its history, and the card is checked at 360/768/1280 px in light and dark (no horizontal scroll, axe on the card).
import { assert, campaignId } from '../lib/util.mjs';
import { axeScoped, eachLook, resetLook, setLook } from '../lib/axe.mjs';

const CARD = '#grants';
const gold = (s) => s.page.$eval(`${CARD} form p.text-muted`, (p) => Number(p.querySelectorAll('strong')[0].innerText));

export default async function (h, { only }) {
  const s = await h.session('you@example.com');
  let cid;
  const open = async () => {
    cid ||= await campaignId(s, 'Aura parade');
    await s.goto(`/campaigns/${cid}/my-company`);
    await s.page.waitForSelector(CARD);
  };

  await h.step('organiser awards own company: the form is on the Grants card, the last card of the page', async () => {
    await open();
    assert.ok(await s.page.$(`${CARD} form[hx-post$="/grants"]`), 'the Grant reward form is on the own company page');
    const last = await s.page.evaluate((c) => {
      const cards = [...document.querySelectorAll('main section.card')];
      return cards[cards.length - 1]?.matches(c);
    }, CARD);
    assert.ok(last, 'the Grants card stays at the bottom');
  }, { only });

  await h.step('organiser awards own company: 4 Gold and a note are granted and show in the history', async () => {
    await open();
    const before = await gold(s);
    await s.fill('#grant-gold', '4');
    await s.fill('#grant-note', 'Own purse, e2e');
    await s.click(`${CARD} form button.btn-primary`, 'Grant reward');
    await s.page.waitForFunction((c) => document.querySelector(c)?.innerText.includes('Own purse, e2e'), { timeout: 8000 }, CARD);
    assert.match(await s.page.$eval(CARD, (e) => e.innerText), /\+4 Gold/);
    assert.equal(await gold(s), before + 4, 'the company has 4 Gold more');
  }, { only });

  await eachLook(h, 'organiser awards own company', only, async (w, scheme, contrast, axeHere) => {
    await setLook(s, w, scheme, contrast);
    await open();
    assert.ok(await s.noHScroll(), 'no horizontal scroll');
    assert.ok(await s.page.$(`${CARD} form`), 'the form is there');
    if (axeHere) assert.deepEqual(await axeScoped(s, [CARD]), [], 'axe on the Grants card');
  });

  await h.step('organiser awards own company: back to the default look', async () => { await resetLook(s); }, { only });
}
