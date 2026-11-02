// Item descriptions link the glossary (GitHub issue 62): in the armoury and in the "+" buy window of a unit's card, a term or a base unit an item's text names
// is a link with the usual tooltip, and an item never links its own name. Aragorn's fresh Arnor company of "Companies being built".
// Mountain Boots say they give Mountain Dweller, the War spear counts as a lance and a spear, the Company Drum is a war drum, the Backpack's own name stays plain.
import { assert, campaignId, MODAL } from '../lib/util.mjs';

const LIST = '#armory-items';

// The term links inside a block: their text, tooltip title and tooltip text.
const links = (root) => `${root} a.term-link`;
const readLinks = (s, sel) => s.page.$$eval(sel, (as) => as.map((a) => ({ text: a.textContent.trim(), title: a.dataset.tipTitle || '', tip: a.dataset.tipText || '', href: a.getAttribute('href') })));

export default async function (h, { only }) {
  const s = await h.session('aragorn@example.com');
  let cid;
  const open = async () => {
    cid ||= await campaignId(s, 'Companies being built');
    await s.goto(`/campaigns/${cid}/armory`);
    await s.page.waitForSelector(LIST);
  };
  const row = (name) => `${LIST} li[data-name="${name}"]`;

  await h.step('armoury: the descriptions link glossary terms with a tooltip, and an item never links its own name', async () => {
    await open();
    const boots = await readLinks(s, links(row('Mountain Boots')));
    const dweller = boots.filter((l) => l.text.toLowerCase() === 'mountain dweller');
    assert.ok(dweller.length >= 1, 'Mountain Boots link Mountain Dweller');
    assert.ok(dweller.every((l) => l.tip.length > 10 && l.title === 'Mountain Dweller' && /glossary#t-/.test(l.href)), 'with a tooltip and a glossary address');

    const spear = (await readLinks(s, links(row('War spear')))).map((l) => l.text.toLowerCase());
    assert.ok(spear.includes('lance') && spear.includes('spear'), `War spear links lance and spear (got ${spear})`);

    const drum = (await readLinks(s, links(row('Company Drum')))).map((l) => l.text.toLowerCase());
    assert.ok(drum.includes('war drum'), `Company Drum links war drum (got ${drum})`);

    const pack = (await readLinks(s, links(row('Backpack')))).map((l) => l.text.toLowerCase());
    assert.ok(!pack.includes('backpack'), 'the Backpack does not link its own name');
    assert.match(await s.page.$eval(row('Backpack'), (li) => li.innerText), /A model with a Backpack may carry/, 'and its text is still there');
  }, { only });

  await h.step('armoury: hovering a linked term shows its tooltip', async () => {
    await open();
    const sel = `${row('War spear')} a.term-link`;
    await s.page.$eval(sel, (a) => a.scrollIntoView({ block: 'center' }));
    await s.page.hover(sel);
    await s.page.waitForFunction(() => [...document.querySelectorAll('[role=tooltip]')].some((t) => t.getBoundingClientRect().width && t.innerText.trim()), { timeout: 3000 });
  }, { only });

  await h.step('the "+" buy window of a unit: the equipment list links its descriptions too', async () => {
    cid ||= await campaignId(s, 'Companies being built');
    await s.goto(`/campaigns/${cid}/my-company`);
    await s.page.waitForSelector('button.eq-add');
    // the unit's "+" next to Equipment (the buy window: kind=equipment)
    const add = await s.page.$('button.eq-add[hx-get$="kind=equipment"]');
    assert.ok(add, 'a unit has the "+" of its equipment');
    await add.evaluate((b) => b.scrollIntoView({ block: 'center' }));
    await add.click();
    await s.page.waitForSelector(`${MODAL} #armoury-find`);
    const items = await s.page.$$eval(`${MODAL} li[data-find]`, (lis) => lis.map((l) => l.dataset.find));
    assert.ok(items.length > 0, 'the window lists items');
    const all = await readLinks(s, `${MODAL} a.term-link`);
    assert.ok(all.length > 0, 'at least one description in the window is linked');
    assert.ok(all.every((l) => l.tip.length > 0), 'each with its tooltip');
    // no item links its own name
    const own = await s.page.$$eval(`${MODAL} li[data-find]`, (lis) => lis.flatMap((li) => {
      const name = li.querySelector('p.font-medium').firstChild.textContent.trim().toLowerCase();
      return [...li.querySelectorAll('a.term-link')].filter((a) => a.textContent.trim().toLowerCase() === name).map(() => name);
    }));
    assert.deepEqual(own, [], 'no item links its own name');
  }, { only });
}
