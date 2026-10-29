// Open the last campaign directly (docs/auth.md, "Last campaign"; docs/multi-company-spec.md, section 8). In the demo world: a fresh
// browser sees the campaign list at "/", opening a campaign makes "/" land on it, opening a second company makes the My Company alias
// land on that one, the brand link and the "Campaigns" links reach the list (which never redirects), and a cookie that is another
// user's, malformed, finished or for a campaign the user cannot open is ignored, cleared, and the list shows.
// 360/768/1280 px in both themes, no horizontal scroll.
import { assert, campaignId, layout, SIZES } from '../lib/util.mjs';

const pathOf = (s) => new URL(s.page.url()).pathname;
const lastCookie = async (s) => (await s.page.cookies()).find((c) => c.name === 'bcm-last');
const setLast = async (s, value) => {
  const c = { name: 'bcm-last', value, url: s.h.base, httpOnly: true, sameSite: 'Lax' };
  if (s.page.setCookie) await s.page.setCookie(c); else await s.ctx.setCookie(c);
};
const isList = (s) => s.page.evaluate(() => document.querySelector('main h1')?.innerText.trim() === 'Campaigns');

export default async function (h) {
  // Fresh sessions: no bcm-last cookie yet (the other flows' sessions of the same people may have one).
  const eowyn = await h.session('eowyn@example.com', { height: 1200 });
  const sauron = await h.session('sauron@example.com', { height: 1200 });
  let bands, first, second;

  await h.step('last campaign: a new browser has no memory, "/" shows the list', async () => {
    assert.equal(await lastCookie(eowyn), undefined, 'no cookie after signing in');
    await eowyn.goto('/');
    assert.equal(pathOf(eowyn), '/', 'no redirect without a cookie');
    assert.ok(await isList(eowyn), 'the list is shown');
    await eowyn.goto('/campaigns');
    assert.equal(pathOf(eowyn), '/campaigns');
    assert.ok(await isList(eowyn));
    bands = await campaignId(eowyn, 'Two bands');
  });

  await h.step('last campaign: opening a campaign makes "/" land on it; the cookie is HttpOnly ids only', async () => {
    await eowyn.goto(`/campaigns/${bands}/rankings`);
    const c = await lastCookie(eowyn);
    assert.ok(c && /^\d+\.\d+$/.test(c.value) && c.value.endsWith('.' + bands), `cookie ${JSON.stringify(c)}`);
    assert.ok(c.httpOnly && c.sameSite === 'Lax' && c.path === '/', 'HttpOnly, Lax, Path=/');
    assert.ok(c.expires > Date.now() / 1000 + 80 * 86400, 'about 90 days');
    assert.equal(await eowyn.page.evaluate(() => document.cookie.includes('bcm-last')), false, 'scripts cannot read it');
    await eowyn.goto('/');
    assert.equal(pathOf(eowyn), `/campaigns/${bands}`, 'the Overview of the last campaign');
  });

  await h.step('last campaign: the brand link, "Campaigns" in the sidebar and "← Campaigns" reach the list, which never redirects', async () => {
    await eowyn.click('header a', 'Battle Companies', { nav: true });
    assert.equal(pathOf(eowyn), '/campaigns');
    assert.ok(await isList(eowyn));
    await eowyn.goto(`/campaigns/${bands}`);
    await eowyn.click('main a', '← Campaigns', { nav: true });
    assert.equal(pathOf(eowyn), '/campaigns');
    await eowyn.goto(`/campaigns/${bands}`);
    await eowyn.click('#sidebar a.nav-link', '=Campaigns', { nav: true });
    assert.equal(pathOf(eowyn), '/campaigns');
    await eowyn.goto('/campaigns');
    assert.equal(pathOf(eowyn), '/campaigns', 'reloading the list stays on it (no loop)');
  });

  await h.step('last campaign: the My Company alias opens the last company opened, else the first', async () => {
    await eowyn.goto(`/campaigns/${bands}/my-company`);
    first = pathOf(eowyn).match(/my-company\/(\d+)$/)?.[1];
    assert.ok(first, 'the alias lands on a company');
    const hrefs = await eowyn.page.$$eval('nav[aria-label="Your companies"] a.chip', (as) => as.map((a) => a.getAttribute('href')).filter((x) => /my-company\/\d+$/.test(x)));
    assert.equal(hrefs.length, 2, `Éowyn has two companies: ${hrefs}`);
    second = hrefs.map((x) => x.match(/(\d+)$/)[1]).find((x) => x !== first);
    await eowyn.goto(`/campaigns/${bands}/my-company/${second}`);
    assert.equal((await lastCookie(eowyn)).value.split('.').at(-1), second);
    await eowyn.goto(`/campaigns/${bands}/rankings`); // another tab of the campaign keeps the company
    await eowyn.goto(`/campaigns/${bands}/my-company`);
    assert.equal(pathOf(eowyn), `/campaigns/${bands}/my-company/${second}`, 'the second company');
    await eowyn.goto(`/campaigns/${bands}/my-company/${first}`);
    await eowyn.goto(`/campaigns/${bands}/my-company`);
    assert.equal(pathOf(eowyn), `/campaigns/${bands}/my-company/${first}`, 'and back to the first');
  });

  await h.step('last campaign: a cookie that is another user\'s, malformed, for a stranger campaign or finished is ignored and cleared', async () => {
    await sauron.goto(`/campaigns/${bands}`);
    const mine = (await lastCookie(sauron)).value.split('.')[0];
    await sauron.page.setCookie({ name: 'bcm-finished', value: '1', url: sauron.h.base });
    const finished = await campaignId(sauron, 'Finished campaign');
    const cases = [
      ['another user\'s', `${Number(mine) + 1000}.${bands}`],
      ['malformed', `${mine}.${bands}.x.9`],
      ['too long', '9'.repeat(200)],
      ['a campaign that does not exist', `${mine}.999999`],
    ];
    cases.push(['finished', `${mine}.${finished}`]);
    for (const [what, value] of cases) {
      await setLast(sauron, value);
      await sauron.goto('/');
      assert.equal(pathOf(sauron), '/', `${what}: no redirect`);
      assert.ok(await isList(sauron), `${what}: the list is shown`);
      assert.equal(await lastCookie(sauron), undefined, `${what}: the cookie is cleared`);
    }
    // a valid one for the same user still works afterwards
    await sauron.goto(`/campaigns/${bands}`);
    await sauron.goto('/');
    assert.equal(pathOf(sauron), `/campaigns/${bands}`);
    // a finished campaign opened from the list is not remembered
    {
      await sauron.goto(`/campaigns/${finished}`);
      assert.ok((await lastCookie(sauron)).value.endsWith('.' + bands), 'still the previous campaign');
    }
  });

  await h.step('last campaign: signing out and in as someone else ignores the cookie', async () => {
    const other = await h.session('galadriel@example.com', { login: false });
    const c = await lastCookie(eowyn);
    await other.goto('/login');
    await setLast(other, c.value); // Éowyn's value in Galadriel's browser
    await h.login(other, 'galadriel@example.com');
    assert.equal(pathOf(other), '/', 'the list, not a redirect');
    assert.equal(await lastCookie(other), undefined, 'cleared');
  });

  for (const scheme of ['light', 'dark']) for (const w of SIZES) {
    await h.step(`last campaign: the list and the landing page at ${w} px, ${scheme}: no horizontal scroll`, async () => {
      await layout(eowyn, w, scheme);
      for (const url of ['/campaigns', '/', `/campaigns/${bands}/my-company`]) {
        await eowyn.goto(url);
        assert.ok(await eowyn.noHScroll(), `no horizontal scroll: ${url} at ${w} px ${scheme}`);
      }
      await eowyn.goto('/campaigns');
      assert.ok(await eowyn.page.$('header a[href="/campaigns"]'), 'the brand link points at the list');
    });
  }
  await layout(eowyn, 1280, 'light', 1200);
}
