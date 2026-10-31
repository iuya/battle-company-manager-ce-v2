// Account invitations (docs/auth.md, "Account invitations"): an administrator (steward) creates a named and an anonymous invitation, copies the
// message, people register through the links in fresh browsers (the ?email= of a named link is only for reading), land on the campaigns page signed in;
// a used, revoked or foreign link, an address that already has an account and a malformed link all show the same page "Talk with the administrator.";
// the registration is throttled; the creation card and the list at 360/768/1280 px in light, dark and high contrast.
import { assert } from '../lib/util.mjs';
import { axeScoped, eachLook, resetLook, setLook } from '../lib/axe.mjs';

const GENERIC = 'Talk with the administrator.';
const PW = 'a-long-password-1';

export default async function (h, { only }) {
  const steward = await h.session('steward@example.com');
  let named, anonymous, revoked;
  const stranger = async () => { const s = await h.session('stranger', { login: false }); s.allow.push(/404|422|429/); return s; };
  const messageOf = (s) => s.page.$eval('textarea[readonly]', (t) => t.value);
  const linkOf = async (s) => {
    const m = (await messageOf(s)).match(/https?:\/\/[^\s]+/);
    assert.ok(m, 'the message holds a link');
    return new URL(m[0]);
  };
  const create = async (fill) => {
    await steward.goto('/admin/invitations');
    await fill();
    await steward.waitText('Invitation created.');
    return linkOf(steward);
  };
  const register = async (s, url, { email, name, password = PW, confirm = password }) => {
    await s.goto(url.pathname + url.search);
    if (email) await s.fill('input[name=email]', email);
    await s.fill('input[name=name]', name);
    await s.fill('input[name=password]', password);
    await s.fill('input[name=confirm]', confirm);
    await Promise.all([s.page.waitForNavigation({ waitUntil: 'load' }), s.page.click('button[type=submit]')]);
  };

  await h.step('invitations: a named invitation is made with the keyboard; the message is ready to copy, read-only and only shown once', async () => {
    named = await create(async () => {
      await steward.fill('#inv-email', 'e2e-named@example.com');
      await steward.page.keyboard.press('Enter');
    });
    const msg = await messageOf(steward);
    assert.match(msg, /^Open this link to create your account: http\S+\/register\/[A-Za-z0-9_-]{43}\?email=e2e-named%40example\.com\nIt works once and expires on /);
    assert.ok(await steward.page.$eval('textarea[readonly]', (t) => t.readOnly));
    await steward.click('button', '=Copy');
    await steward.goto('/admin/invitations');
    assert.ok(await steward.has('e2e-named@example.com'), 'the list shows the invitation');
    assert.ok(!(await steward.page.evaluate((t) => document.documentElement.outerHTML.includes(t), named.pathname.split('/')[2])), 'the list does not carry the token');
    const inline = await steward.page.evaluate(() => [...document.scripts].filter((x) => !x.src && !x.type.includes('json')).length);
    assert.equal(inline, 0, 'no inline script');
  }, { only });

  await h.step('invitations: the person registers (a forged ?email= is ignored) and lands on the campaigns page signed in; the link is then dead', async () => {
    const s = await stranger();
    const forged = new URL(named.pathname + '?email=forged@evil.example', named.origin);
    await s.goto(forged.pathname + forged.search);
    assert.equal(await s.page.$eval('#reg-email', (i) => i.value), 'e2e-named@example.com', 'the address of the invitation, not the query');
    assert.ok(await s.page.$eval('#reg-email', (i) => i.readOnly));
    await register(s, forged, { name: 'Named Person' });
    assert.equal(new URL(s.page.url()).pathname, '/campaigns');
    await s.waitText('Named Person');
    const again = await stranger();
    await again.goto(named.pathname);
    await again.waitText(GENERIC);
    await again.goto('/login');
    await again.fill('input[name=email]', 'e2e-named@example.com');
    await again.fill('input[name=password]', PW);
    await Promise.all([again.page.waitForNavigation({ waitUntil: 'load' }), again.page.click('button[type=submit]')]);
    assert.equal(new URL(again.page.url()).pathname, '/', 'the new password works');
    // the list says used, with the account
    await steward.goto('/admin/invitations');
    assert.ok(await steward.page.evaluate(() => !!document.querySelector('[data-state=used]') && document.body.innerText.includes('Named Person')), 'the list shows it used');
    // a signed-in visitor opening a link is sent home
    await s.goto(named.pathname);
    assert.equal(new URL(s.page.url()).pathname, '/');
  }, { only });

  await h.step('invitations: an anonymous invitation for two people; the third, and an address that has an account, get the generic page', async () => {
    anonymous = await create(async () => {
      await steward.click('input[name=kind][value=anonymous]');
      await steward.fill('#inv-max', '2');
      await steward.click('button', '=Create invitation');
    });
    assert.ok(!anonymous.search, 'an anonymous link carries no address');
    assert.match(await messageOf(steward), /up to 2 people/);
    const one = await stranger();
    await register(one, anonymous, { email: 'e2e-anon1@example.com', name: 'Anon One' });
    assert.equal(new URL(one.page.url()).pathname, '/campaigns');
    // an address that already has an account looks like any other dead link
    const taken = await stranger();
    await taken.goto(anonymous.pathname);
    await taken.fill('input[name=email]', 'eowyn@example.com');
    await taken.fill('input[name=name]', 'Impostor');
    await taken.fill('input[name=password]', PW);
    await taken.fill('input[name=confirm]', PW);
    const [resp] = await Promise.all([taken.page.waitForNavigation({ waitUntil: 'load' }), taken.page.click('button[type=submit]')]);
    assert.equal(resp.status(), 422);
    const takenText = await taken.text();
    assert.ok(takenText.includes(GENERIC));
    const two = await stranger();
    await register(two, anonymous, { email: 'e2e-anon2@example.com', name: 'Anon Two' });
    assert.equal(new URL(two.page.url()).pathname, '/campaigns');
    const three = await stranger();
    await three.goto(anonymous.pathname);
    assert.equal(await three.text(), takenText, 'a used-up link shows the very same page');
    await register(three, anonymous, { email: 'e2e-anon3@example.com', name: 'Anon Three' }).catch(() => {});
    assert.ok(await three.has(GENERIC));
  }, { only });

  await h.step('invitations: field mistakes keep the form; a revoked invitation and a malformed link show the generic page', async () => {
    revoked = await create(async () => { await steward.fill('#inv-email', 'e2e-revoked@example.com'); await steward.click('button', '=Create invitation'); });
    const s = await stranger();
    await register(s, revoked, { name: 'Weak', password: 'short', confirm: 'short' });
    assert.ok(await s.has('between 10 and 128'), 'the weak password is explained');
    assert.equal(await s.page.$eval('#reg-email', (i) => i.value), 'e2e-revoked@example.com');
    await s.fill('input[name=password]', PW);
    await s.fill('input[name=confirm]', PW + 'x');
    await Promise.all([s.page.waitForNavigation({ waitUntil: 'load' }), s.page.click('button[type=submit]')]);
    assert.ok(await s.has('do not match'));
    // the administrator revokes it (inline confirmation), the list says revoked
    await steward.goto('/admin/invitations');
    await steward.click('li[data-state=pending] button', '=Revoke');
    await steward.click('li[data-state=pending] [role=alertdialog] button', '=Revoke');
    await steward.waitGone('Revoke this invitation?');
    assert.ok(await steward.page.evaluate(() => !!document.querySelector('li[data-state=revoked]')), 'revoked in the list');
    const late = await stranger();
    await late.goto(revoked.pathname);
    await late.waitText(GENERIC);
    const bad = await stranger();
    await bad.goto('/register/not-a-token');
    await bad.waitText(GENERIC);
  }, { only });

  await h.step('invitations: the filter chips narrow the list', async () => {
    await steward.goto('/admin/invitations');
    await steward.click('nav[aria-label="Filter invitations"] a', '=Revoked');
    await steward.settle(300);
    assert.ok(await steward.page.evaluate(() => [...document.querySelectorAll('#invitation-list li[data-state]')].every((l) => l.dataset.state === 'revoked')));
    assert.ok(await steward.has('e2e-revoked@example.com'));
  }, { only });

  await h.step('invitations: the registration is throttled per link', async () => {
    const s = await stranger();
    const bad = '/register/' + 'z'.repeat(43);
    // looking at a dead link never counts; failed attempts to register with it do
    for (let i = 0; i < 7; i++) assert.equal((await s.page.goto(s.url(bad))).status(), 404);
    await s.page.goto(s.url('/login'));
    const codes = await s.page.evaluate(async (u) => { const out = []; for (let i = 0; i < 6; i++) out.push((await fetch(u, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body: 'email=a%40example.com&name=A&password=longenough1&confirm=longenough1' })).status); return out; }, bad);
    assert.deepEqual(codes, [422, 422, 422, 422, 422, 429]);
    assert.equal((await s.page.goto(s.url(bad))).status(), 429);
    assert.ok(await s.has('Too many attempts'));
  }, { only });

  let open;
  await h.step('invitations: one more named invitation stays pending for the registration page to look at', async () => {
    open = await create(async () => { await steward.fill('#inv-email', 'e2e-look@example.com'); await steward.click('button', '=Create invitation'); });
  }, { only });

  await eachLook(h, 'invitations: the page, the registration and the generic page', only, async (w, scheme, contrast, withAxe) => {
    await setLook(steward, w, scheme, contrast);
    await steward.goto('/admin/invitations');
    assert.ok(await steward.noHScroll(), 'no horizontal scroll (Invitations)');
    if (withAxe) assert.deepEqual(await axeScoped(steward, ['#invitation-create', 'nav[aria-label=Administration]', '#invitation-list']), [], 'axe (Invitations)');
    steward.issues();
    const s = await stranger();
    await setLook(s, w, scheme, contrast);
    await s.goto(open.pathname);
    assert.ok(await s.noHScroll(), 'no horizontal scroll (registration)');
    if (withAxe) assert.deepEqual(await axeScoped(s, ['main']), [], 'axe (registration)');
    await s.goto('/register/' + `${w}${scheme}${contrast}`.padEnd(43, 'q')); // a dead link: the generic page
    assert.ok(await s.noHScroll(), 'no horizontal scroll (generic)');
    if (withAxe) assert.deepEqual(await axeScoped(s, ['main']), [], 'axe (generic page)');
    await s.close();
  });
  await resetLook(steward);
}
