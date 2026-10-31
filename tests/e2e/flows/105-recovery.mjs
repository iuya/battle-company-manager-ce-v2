// Password recovery links (docs/auth.md, "Password recovery"): a superuser (admin) makes a link for an account, copies the message, the person sets a new
// password in a fresh browser (no automatic sign-in, the old password stops working, their sessions are gone, the link is dead); a link is invalidated
// when the person signs in or opens the app first; revoke; a newer link replaces the older; an administrator has no recovery controls; the page at
// 360/768/1280 px in light, dark and high contrast. Uses the dedicated recover-* accounts of the demo world and never touches recover-pending.
import { assert } from '../lib/util.mjs';
import { PASSWORD } from '../lib/harness.mjs';
import { axeScoped, eachLook, resetLook, setLook } from '../lib/axe.mjs';

const GENERIC = 'Talk with the administrator.';
const NEW = 'brand-new-password-9';
const rowId = (s, email) => s.page.evaluate((email) => {
  const li = [...document.querySelectorAll('li[id^=user-]')].find((l) => l.innerText.includes(email));
  return li ? Number(li.id.slice(5)) : 0;
}, email);

export default async function (h, { only }) {
  const admin = await h.session('admin@example.com');
  const steward = await h.session('steward@example.com');
  const anon = async () => { const s = await h.session('anon', { login: false }); s.allow.push(/404|422|429/); return s; };

  // Opens the Recovery card of the account and returns its row id.
  const card = async (email) => {
    await admin.goto('/admin/users?q=' + encodeURIComponent(email));
    const id = await rowId(admin, email);
    assert.ok(id, 'the account is listed');
    await admin.page.evaluate((id) => { document.querySelectorAll(`#user-${id} details`).forEach((d) => { d.open = true; }); }, id);
    return id;
  };
  const generate = async (email, { hours } = {}) => {
    const id = await card(email);
    if (hours) await admin.select(`#rec-hours-${id}`, hours);
    await admin.click(`#user-${id} button`, '=Generate link');
    await admin.click(`#user-${id} [role=alertdialog] button`, '=Confirm');
    await admin.waitText('Shown only now');
    const msg = await admin.page.$eval(`#recovery-${id}`, (t) => t.value);
    const m = msg.match(/https?:\/\/\S+\/recover\/[A-Za-z0-9_-]{43}/);
    assert.ok(m, 'the message holds the link: ' + msg);
    assert.match(msg, /^Open this link to set a new password: /);
    return { id, url: new URL(m[0]) };
  };
  const state = (id) => admin.page.$eval(`#user-${id} [data-recovery-state]`, (p) => p.dataset.recoveryState + ': ' + p.innerText.replace(/\s+/g, ' ').trim());
  const dead = async (url) => { const s = await anon(); const r = await s.page.goto(url.href); assert.equal(r.status(), 404); assert.ok(await s.has(GENERIC)); await s.close(); };
  const signIn = async (s, email, password) => {
    await s.goto('/login');
    await s.fill('input[name=email]', email);
    await s.fill('input[name=password]', password);
    await Promise.all([s.page.waitForNavigation({ waitUntil: 'load', timeout: 4000 }).catch(() => {}), s.page.click('button[type=submit]')]); // a refusal swaps the form in place
    return new URL(s.page.url()).pathname;
  };
  const submit = (s) => Promise.all([s.page.waitForNavigation({ waitUntil: 'load' }), s.page.click('button[type=submit]')]);
  let used, pendingUrl;

  await h.step('recovery: the demo states are worded on the Users page', async () => {
    for (const [email, want] of [['recover-used@example.com', 'used'], ['recover-expired@example.com', 'expired'], ['recover-revoked@example.com', 'revoked'], ['recover-login@example.com', 'the person signed in'], ['recover-access@example.com', 'the person opened the app'], ['recover-pending@example.com', 'pending']]) {
      const id = await card(email);
      assert.ok((await state(id)).includes(want), `${email}: ${await state(id)}`);
    }
  }, { only });

  await h.step('recovery: the superuser makes a link; the person sets a new password; no automatic sign-in, old password and sessions gone, link dead', async () => {
    const old = await h.session('recover-expired@example.com'); // a session that the new password must end
    const { id, url } = await generate('recover-expired@example.com', { hours: 72 });
    used = url;
    assert.ok((await state(id)).startsWith('pending'), await state(id));
    // opening the link in a browser that is signed in as somebody else does not use it up (the page never reads the session)
    const s = await anon();
    s.allow.push(/422/);
    await s.goto(url.pathname);
    await s.waitText('Set a new password');
    await s.fill('input[name=password]', NEW);
    await s.fill('input[name=confirm]', NEW + 'x');
    await submit(s);
    await s.waitText('do not match');
    await s.fill('input[name=password]', 'short');
    await s.fill('input[name=confirm]', 'short');
    await submit(s);
    await s.waitText('between 10 and 128');
    await s.fill('input[name=password]', NEW);
    await s.fill('input[name=confirm]', NEW);
    await submit(s);
    assert.equal(new URL(s.page.url()).pathname, '/login', 'no automatic sign-in');
    await s.waitText('Your password was changed');
    assert.equal(await signIn(s, 'recover-expired@example.com', PASSWORD), '/login', 'the old password is refused');
    assert.equal(await signIn(s, 'recover-expired@example.com', NEW), '/', 'the new one works');
    await old.goto('/');
    assert.equal(new URL(old.page.url()).pathname, '/login', 'the old session is gone');
    await dead(url);
    await admin.goto('/admin/users?q=recover-expired');
    assert.ok((await state(id)).startsWith('used'), await state(id));
  }, { only });

  await h.step('recovery: signing in first invalidates the link', async () => {
    const { id, url } = await generate('recover-login@example.com');
    const person = await anon();
    assert.equal(await signIn(person, 'recover-login@example.com', PASSWORD), '/');
    await dead(url);
    await card('recover-login@example.com');
    assert.ok((await state(id)).includes('the person signed in'), await state(id));
  }, { only });

  await h.step('recovery: opening the app with a session first invalidates the link', async () => {
    const person = await h.session('recover-access@example.com');
    const { id, url } = await generate('recover-access@example.com');
    await person.goto('/campaigns');
    await dead(url);
    await card('recover-access@example.com');
    assert.ok((await state(id)).includes('the person opened the app'), await state(id));
  }, { only });

  await h.step('recovery: revoke (with a confirmation) and replace', async () => {
    const { id, url } = await generate('recover-revoked@example.com');
    await admin.click(`#user-${id} button`, '=Revoke link');
    await admin.click(`#user-${id} [role=alertdialog] button`, '=Revoke');
    await admin.waitText('The link is revoked.');
    assert.ok((await state(id)).startsWith('revoked'), await state(id));
    await dead(url);
    const first = await generate('recover-replaced@example.com');
    const second = await generate('recover-replaced@example.com');
    assert.notEqual(first.url.href, second.url.href);
    pendingUrl = second.url;
    await dead(first.url);
    const s = await anon();
    await s.goto(second.url.pathname);
    await s.waitText('Set a new password');
    await s.close();
  }, { only });

  await h.step('recovery: an administrator sees no recovery controls and cannot post', async () => {
    await steward.goto('/admin/users?q=recover-login');
    assert.ok(!(await steward.has('Recovery link')));
    const id = await rowId(steward, 'recover-login@example.com');
    steward.allow.push(/404/);
    const status = await steward.page.evaluate(async (id) => (await fetch(`/admin/users/${id}/recovery`, { method: 'POST', credentials: 'same-origin' })).status, id);
    assert.equal(status, 404);
    // and a link used cannot be used again
    const s = await anon();
    const r = await s.page.goto(used.href);
    assert.equal(r.status(), 404);
  }, { only });

  await eachLook(h, 'recovery: the card and the page', only, async (w, scheme, contrast, withAxe) => {
    await setLook(admin, w, scheme, contrast);
    const id = await card('recover-replaced@example.com');
    assert.ok(await admin.noHScroll(), 'no horizontal scroll (Users)');
    if (withAxe) assert.deepEqual(await axeScoped(admin, [`#user-${id}`]), [], 'axe (recovery card)');
    admin.issues();
    const s = await anon();
    await setLook(s, w, scheme, contrast);
    await s.goto(pendingUrl.pathname);
    await s.waitText('Set a new password');
    assert.ok(await s.noHScroll(), 'no horizontal scroll (set a password)');
    if (withAxe) assert.deepEqual(await axeScoped(s, ['main']), [], 'axe (set a password)');
    await s.goto('/recover/' + `${w}${scheme}${contrast}`.padEnd(43, 'q'));
    assert.ok(await s.noHScroll(), 'no horizontal scroll (generic)');
    if (withAxe) assert.deepEqual(await axeScoped(s, ['main']), [], 'axe (generic)');
    await s.close();
  });
  await resetLook(admin);
}
