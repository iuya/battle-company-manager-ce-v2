// The footer of every page names the release, the license and the source (GitHub issue: footer with version and credits): signed out on the
// login page and signed in on a campaign page, it holds at 360 px in light and dark with no horizontal scroll. The e2e server is a build from
// a checkout, so the release reads "dev build"; the link to a release's notes is covered by the unit test.
import { assert, campaignId, layout } from '../lib/util.mjs';

const footer = (s) => s.page.$eval('[data-app-footer]', (e) => ({ text: e.innerText.replace(/\s+/g, ' ').trim(), links: [...e.querySelectorAll('a')].map((a) => a.href) }));

export default async function (h) {
  const admin = await h.session('admin@example.com', { height: 1000 });

  await h.step('footer: the login page names the release, the license and the source', async () => {
    const anon = await h.session('anon', { height: 900, login: false });
    await anon.goto('/login');
    const f = await footer(anon);
    assert.match(f.text, /dev build/, `the release: ${f.text}`);
    assert.match(f.text, /CC BY-NC-SA 4\.0/, `the license: ${f.text}`);
    assert.ok(f.links.some((l) => l === 'https://github.com/iuya/battle-company-manager-ce-v2'), `the source link: ${f.links}`);
    assert.ok(f.links.some((l) => l.startsWith('https://creativecommons.org/licenses/by-nc-sa/4.0')), `the license link: ${f.links}`);
  });

  await h.step('footer: a campaign page has it too, and it holds at 360 px in light and dark', async () => {
    await admin.goto(`/campaigns/${await campaignId(admin, 'Matches: every scenario')}/glossary`);
    assert.match((await footer(admin)).text, /CC BY-NC-SA 4\.0/);
    for (const scheme of ['light', 'dark']) {
      await layout(admin, 360, scheme);
      await admin.page.$eval('[data-app-footer]', (e) => e.scrollIntoView({ block: 'end' }));
      const over = await admin.page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(over <= 0, `no horizontal scroll at 360 px ${scheme} (${over})`);
    }
    await layout(admin, 1280, 'light');
  });
}
