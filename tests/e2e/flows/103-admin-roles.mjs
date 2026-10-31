// Roles in the administration (docs/admin.md, "Roles"): an administrator (steward) sees Users and Invitations but no Rules files, has the
// Organiser switch on plain users and organisers and nothing on administrators and superusers; a forged role is refused with 403; a
// superuser (admin) also has the Administrator switch, never a Superuser option; the new administrator sees the section. Keyboard use of the
// switches, and the page at 360/768/1280 px in light, dark and high contrast (axe over the row and the sub-navigation only).
import { assert } from '../lib/util.mjs';
import { axeScoped, eachLook, resetLook, setLook } from '../lib/axe.mjs';

// The id of the account row with this email, from its element id (user-<id>).
const rowId = (s, email) => s.page.evaluate((email) => {
  const li = [...document.querySelectorAll('li[id^=user-]')].find((l) => l.innerText.includes(email));
  return li ? Number(li.id.slice(5)) : 0;
}, email);
const openRow = (s, id) => s.page.evaluate((id) => { document.querySelectorAll(`#user-${id} details`).forEach((d) => { d.open = true; }); }, id);
const post = (s, path, body) => s.page.evaluate(async (path, body) => {
  const r = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body: new URLSearchParams(body), credentials: 'same-origin' });
  return r.status;
}, path, body);
const badge = (s, id) => s.page.$$eval(`#user-${id} .badge`, (b) => b.some((x) => x.innerText.includes('Can create campaigns')));
const search = async (s, q) => { await s.goto('/admin/users?q=' + encodeURIComponent(q)); };

export default async function (h, { only }) {
  const steward = await h.session('steward@example.com');
  const admin = await h.session('admin@example.com');

  await h.step('roles: the administrator sees Users and Invitations, not Rules files; the Organiser switch only, on plain users', async () => {
    await steward.goto('/admin/users');
    await steward.waitText('Users');
    const nav = await steward.page.$$eval('nav[aria-label=Administration] a', (a) => a.map((x) => x.innerText.trim()));
    assert.deepEqual(nav, ['Users', 'Invitations']);
    const side = await steward.page.evaluate(() => !!document.querySelector('#sidebar a[href="/admin"]'));
    assert.ok(side, 'the sidebar offers Administration');
    await search(steward, 'pippin@example.com');
    const id = await rowId(steward, 'pippin@example.com');
    await openRow(steward, id);
    const sw = await steward.page.$$eval(`#user-${id} input[role=switch]`, (i) => i.map((x) => x.name));
    assert.deepEqual(sw, ['can_create'], 'only the Organiser switch');
    assert.ok(!(await steward.has('Superuser</')), 'no Superuser option');
    await search(steward, 'steward@example.com');
    assert.ok(await steward.has('Administrators are managed by superusers.'), 'own row: no controls');
    await search(steward, 'admin@example.com');
    assert.ok(await steward.has('Superusers are set from the console.'));
    steward.allow.push(/404/);
    await steward.goto('/admin/rulesets');
    assert.ok(await steward.has('Page not found') || await steward.has('nothing at this address'), 'Rules files are superuser-only');
  }, { only });

  await h.step('roles: the administrator makes pippin an organiser with the keyboard and takes it away again', async () => {
    await search(steward, 'pippin@example.com');
    const id = await rowId(steward, 'pippin@example.com');
    await openRow(steward, id);
    assert.ok(!(await badge(steward, id)), 'pippin cannot create campaigns to begin with');
    await steward.page.focus(`#user-${id} input[name=can_create]`);
    await steward.page.keyboard.press('Space');
    await steward.page.keyboard.press('Tab'); // the Save button
    await steward.page.keyboard.press('Enter');
    await steward.waitText('Saved');
    assert.ok(await badge(steward, id), 'the badge shows');
    const pippin = await h.session('pippin@example.com');
    await pippin.goto('/campaigns');
    assert.ok(await pippin.has('New campaign'), 'the change applies at once');
    await openRow(steward, id);
    await steward.page.focus(`#user-${id} input[name=can_create]`);
    await steward.page.keyboard.press('Space');
    await steward.click(`#user-${id} button`, '=Save');
    await steward.waitText('Saved');
    await pippin.goto('/campaigns');
    assert.ok(!(await pippin.has('New campaign')), 'and so does taking it away');
    await pippin.close();
  }, { only });

  await h.step('roles: a forged role by the administrator is refused with 403, the recovery routes are 404', async () => {
    await search(steward, 'pippin@example.com');
    const id = await rowId(steward, 'pippin@example.com');
    steward.allow.push(/403|404/);
    assert.equal(await post(steward, `/admin/users/${id}/permissions`, { role: 'admin', email: 'pippin@example.com' }), 403);
    assert.equal(await post(steward, `/admin/users/${id}/permissions`, { role: 'superuser', email: 'pippin@example.com' }), 403);
    const adminId = await (async () => { await search(steward, 'admin@example.com'); return rowId(steward, 'admin@example.com'); })();
    assert.equal(await post(steward, `/admin/users/${adminId}/permissions`, { role: 'user', email: 'admin@example.com' }), 403);
    assert.equal(await post(steward, `/admin/users/${id}/recovery`, {}), 404);
    admin.allow.push(/403/);
    assert.equal(await post(admin, `/admin/users/${id}/permissions`, { role: 'superuser', email: 'pippin@example.com' }), 403, 'nobody makes a superuser on the web');
  }, { only });

  await h.step('roles: the superuser has both switches, no Superuser option; the new administrator sees the section', async () => {
    await search(admin, 'pippin@example.com');
    const id = await rowId(admin, 'pippin@example.com');
    await openRow(admin, id);
    const sw = await admin.page.$$eval(`#user-${id} input[role=switch]`, (i) => i.map((x) => x.name));
    assert.deepEqual(sw, ['role', 'can_create']);
    assert.ok(!(await admin.page.evaluate(() => document.body.innerHTML.includes('value="superuser"'))), 'no Superuser option');
    await search(admin, 'admin@example.com');
    assert.ok(await admin.has('Set from the console (make grant).'));
    await search(admin, 'pippin@example.com');
    await openRow(admin, id);
    await admin.page.focus(`#user-${id} input[name=role]`);
    await admin.page.keyboard.press('Space');
    const dis = await admin.page.$eval(`#user-${id} input[name=can_create]`, (i) => ({ d: i.disabled, c: i.checked }));
    assert.deepEqual(dis, { d: true, c: true }, 'an administrator always creates campaigns');
    await admin.click(`#user-${id} button`, '=Save');
    await admin.waitText('Saved');
    const pippin = await h.session('pippin@example.com');
    await pippin.goto('/campaigns');
    assert.ok(await pippin.page.evaluate(() => !!document.querySelector('#sidebar a[href="/admin"]')), 'the section shows for the new administrator');
    await pippin.goto('/admin/invitations');
    await pippin.waitText('Invite someone');
    // and back: the Organiser flag it had is restored
    await search(admin, 'pippin@example.com');
    await openRow(admin, id);
    await admin.page.focus(`#user-${id} input[name=role]`);
    await admin.page.keyboard.press('Space');
    await admin.click(`#user-${id} button`, '=Save');
    await admin.waitText('Saved');
    pippin.allow.push(/404/);
    await pippin.goto('/admin/users');
    assert.ok(!(await pippin.has('Invite someone')), 'the section is gone');
    await pippin.goto('/campaigns');
    assert.ok(!(await pippin.has('New campaign')), 'pippin is a plain user again');
    await pippin.close();
  }, { only });

  await eachLook(h, 'roles: the Users page', only, async (w, scheme, contrast, withAxe) => {
    for (const [s, who] of [[steward, 'administrator'], [admin, 'superuser']]) {
      await setLook(s, w, scheme, contrast);
      await search(s, 'pippin@example.com');
      const id = await rowId(s, 'pippin@example.com');
      await openRow(s, id);
      assert.ok(await s.noHScroll(), `no horizontal scroll (${who})`);
      if (withAxe) assert.deepEqual(await axeScoped(s, [`#user-${id}`, 'nav[aria-label=Administration]']), [], `axe (${who})`);
      s.issues();
    }
  });
  await resetLook(steward); await resetLook(admin);
}
