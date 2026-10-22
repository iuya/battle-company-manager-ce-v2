// Sign in and out, account page (timezone), glossary, administration.
import { assert } from '../lib/util.mjs';

export default async function (h, { only }) {
  const anon = await h.session('anon', { login: false });
  await h.step('login: wrong password is refused, right one signs in, logout signs out', async () => {
    anon.allow.push(/401|403|422/);
    await anon.goto('/login');
    await anon.fill('input[name=email]', 'you@example.com');
    await anon.fill('input[name=password]', 'wrong-password-x');
    await anon.click('button[type=submit]', 'Sign in');
    await anon.waitText('Invalid email or password');
    assert.equal(new URL(anon.page.url()).pathname, '/login');
    await anon.goto('/');
    assert.equal(new URL(anon.page.url()).pathname, '/login', 'a signed-out visit to / goes to the login page');
    await h.login(anon, 'you@example.com');
    await anon.waitText('New campaign');
    await anon.click('button', 'Sign out', { nav: true });
    assert.equal(new URL(anon.page.url()).pathname, '/login');
    await anon.goto('/campaigns/new');
    assert.equal(new URL(anon.page.url()).pathname, '/login', 'the session is gone');
  }, { only });

  const you = await h.session('you@example.com');
  await h.step('account: timezone is saved and shown', async () => {
    await you.goto('/account');
    await you.fill('input[name=zone]', 'Pacific/Auckland');
    await you.page.$eval('input[name=zone]', (e) => e.form.requestSubmit());
    await you.settle(400);
    await you.goto('/account');
    const v = await you.page.$eval('input[name=zone]', (e) => e.value);
    assert.equal(v, 'Pacific/Auckland');
  }, { only });

  await h.step('glossary: a cross-reference scrolls to its term', async () => {
    await you.goto('/glossary');
    const target = await you.page.evaluate(() => {
      const link = [...document.querySelectorAll('main a[href^="#t-"]')].find((a) => {
        const el = document.getElementById(a.getAttribute('href').slice(1));
        return el && el.getBoundingClientRect().top > innerHeight * 1.5;
      });
      if (link) { link.dataset.e2e = '1'; return link.getAttribute('href'); }
      return null;
    });
    assert.ok(target, 'a link to a term far below the fold');
    await you.page.click('a[data-e2e="1"]');
    await you.settle(700);
    const top = await you.page.evaluate((t) => document.getElementById(t.slice(1)).getBoundingClientRect().top, target);
    assert.ok(top >= -2 && top < 600, `the term ${target} is scrolled into view (top ${top})`);
  }, { only });

  const admin = await h.session('admin@example.com');
  await h.step('admin: overview, users and rules files pages load; a user is refused /admin', async () => {
    for (const [path, text] of [['/admin', 'Administration'], ['/admin/users', 'admin@example.com'], ['/admin/rulesets', 'Battle Companies']]) {
      await admin.goto(path);
      await admin.waitText(text);
    }
    you.allow.push(/403|404/);
    await you.goto('/admin/users');
    assert.ok(!(await you.has('pippin@example.com')), 'a plain user does not see the user list');
  }, { only });
}
