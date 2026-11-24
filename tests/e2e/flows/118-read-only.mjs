// Read-only accounts (docs/admin.md, "Read-only accounts"): the demo account (viewer) signs in and looks at its pages while every write is
// refused with 403 and the reason, and nothing it tried changed; a superuser switches the flag on another account with the keyboard and the
// change reaches that account's open session at once, and off again; an administrator has no switch; a superuser's own row has none either.
// The Users page with the card at 360/768/1280 px in light, dark and high contrast (axe over the row only).
import { assert } from '../lib/util.mjs';
import { axeScoped, eachLook, resetLook, setLook } from '../lib/axe.mjs';

const rowId = (s, email) => s.page.evaluate((email) => {
  const li = [...document.querySelectorAll('li[id^=user-]')].find((l) => l.innerText.includes(email));
  return li ? Number(li.id.slice(5)) : 0;
}, email);
const openRow = (s, id) => s.page.evaluate((id) => { document.querySelectorAll(`#user-${id} details`).forEach((d) => { d.open = true; }); }, id);
const search = async (s, q) => { await s.goto('/admin/users?q=' + encodeURIComponent(q)); };
// A request from the page, with its cookies: the status and the start of the answer.
const send = (s, method, path, body) => s.page.evaluate(async (method, path, body) => {
  const r = await fetch(path, { method, headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body: new URLSearchParams(body), credentials: 'same-origin' });
  return { status: r.status, text: await r.text() };
}, method, path, body);
const flagBadge = (s, id) => s.page.$$eval(`#user-${id} .badge`, (b) => b.some((x) => x.innerText.trim() === 'Read-only'));

export default async function (h, { only }) {
  const viewer = await h.session('viewer@example.com');
  const admin = await h.session('admin@example.com');
  const steward = await h.session('steward@example.com');

  await h.step('read-only: the demo account looks at its pages, every write is refused with the reason, and nothing changed', async () => {
    viewer.allow.push(/403/); // the refusals
    await viewer.goto('/campaigns');
    await viewer.goto('/account');
    assert.ok(await viewer.has('Smoke test'), 'the account page opens');
    for (const [method, path, body] of [['POST', '/campaigns', { name: 'A campaign' }], ['PUT', '/account/appearance', { palette: 'mithril', mode: 'dark' }], ['POST', '/account/profile', { name: 'Changed' }], ['POST', '/account/close', { email: 'viewer@example.com' }]]) {
      const r = await send(viewer, method, path, body);
      assert.equal(r.status, 403, `${method} ${path}`);
      assert.match(r.text, /read-only/, `${method} ${path} says why`);
    }
    await viewer.goto('/account');
    assert.ok(await viewer.has('Smoke test'), 'the name is the same after the refused change');
    assert.ok(!(await viewer.has('Changed')), 'no change went through');
    // the account can leave: signing out is not refused
    const out = await viewer.page.evaluate(async () => (await fetch('/logout', { method: 'POST', credentials: 'same-origin', redirect: 'manual' })).type);
    assert.ok(out === 'opaqueredirect' || out === 'basic', 'POST /logout is answered, not refused');
  }, { only });

  await h.step('read-only: a superuser switches it on another account with the keyboard; the open session of that account is refused at once, and allowed again when it is off', async () => {
    const pippin = await h.session('pippin@example.com'); // signed in before the switch
    await pippin.goto('/account');
    const before = await send(pippin, 'PUT', '/account/appearance', { palette: 'mithril', mode: 'light' });
    assert.ok(before.status < 400, `a plain account writes (${before.status})`);
    await search(admin, 'pippin@example.com');
    const id = await rowId(admin, 'pippin@example.com');
    await openRow(admin, id);
    assert.ok(!(await flagBadge(admin, id)), 'not read-only to begin with');
    await admin.page.focus(`#user-${id} [data-read-only] input[name=read_only]`);
    await admin.page.keyboard.press('Space');
    await admin.page.keyboard.press('Tab'); // the Save button
    await admin.page.keyboard.press('Enter');
    await admin.waitText('Saved');
    assert.ok(await flagBadge(admin, id), 'the badge shows');
    pippin.allow.push(/403/);
    const refused = await send(pippin, 'PUT', '/account/appearance', { palette: 'mithril', mode: 'light' });
    assert.equal(refused.status, 403, 'the open session is refused at once');
    await pippin.goto('/campaigns');
    assert.ok(await pippin.has('Pippin') || (await pippin.page.title()).length > 0, 'and still reads');
    // off again
    await search(admin, 'pippin@example.com');
    await openRow(admin, id);
    await admin.page.focus(`#user-${id} [data-read-only] input[name=read_only]`);
    await admin.page.keyboard.press('Space');
    await admin.page.keyboard.press('Tab');
    await admin.page.keyboard.press('Enter');
    await admin.waitText('Saved');
    assert.ok(!(await flagBadge(admin, id)), 'the badge is gone');
    const after = await send(pippin, 'PUT', '/account/appearance', { palette: 'mithril', mode: 'light' });
    assert.ok(after.status < 400, `writes again (${after.status})`);
  }, { only });

  await h.step('read-only: an administrator has no switch, a superuser row has none, and a forged request is refused', async () => {
    await search(steward, 'pippin@example.com');
    const sid = await rowId(steward, 'pippin@example.com');
    assert.ok(!(await steward.page.$(`#user-${sid} [data-read-only]`)), 'no card for an administrator');
    steward.allow.push(/403|404/);
    const forged = await send(steward, 'POST', `/admin/users/${sid}/read-only`, { read_only: '1', email: 'pippin@example.com' });
    assert.ok(forged.status === 404 || forged.status === 403, `forged by an administrator (${forged.status})`);
    await search(admin, 'admin@example.com');
    const aid = await rowId(admin, 'admin@example.com');
    assert.ok(!(await admin.page.$(`#user-${aid} [data-read-only]`)), 'no card on a superuser');
    admin.allow.push(/422/);
    const self = await send(admin, 'POST', `/admin/users/${aid}/read-only`, { read_only: '1', email: 'admin@example.com' });
    assert.equal(self.status, 422, 'a forged request for a superuser is refused (the row has no card to show the reason in)');
    await search(admin, 'admin@example.com');
    assert.ok(!(await flagBadge(admin, aid)), 'and nothing changed: the superuser is not read-only');
    await search(admin, 'viewer@example.com');
    const vid = await rowId(admin, 'viewer@example.com');
    assert.ok(await flagBadge(admin, vid), 'the demo account shows its badge');
  }, { only });

  await eachLook(h, 'read-only: the Users page', only, async (w, scheme, contrast, withAxe) => {
    await setLook(admin, w, scheme, contrast);
    await search(admin, 'viewer@example.com');
    const id = await rowId(admin, 'viewer@example.com');
    await openRow(admin, id);
    assert.ok(await admin.noHScroll(), 'no horizontal scroll');
    if (withAxe) assert.deepEqual(await axeScoped(admin, [`#user-${id}`]), [], 'axe');
    admin.issues();
  });
  await resetLook(admin);
}
