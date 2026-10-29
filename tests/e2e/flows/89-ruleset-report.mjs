// The ruleset issue report: a rules file with twelve mistakes is refused with every problem listed (file, path, hint, closest
// match), by "Check this file" (a dry run), by the admin upload and by the new-campaign form; nothing is stored and no campaign
// is created. Checked at 360, 768 and 1280 px in light and dark.
import { assert, SIZES, layout } from '../lib/util.mjs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const BAD = resolve(dirname(fileURLToPath(import.meta.url)), '../../../internal/core/lean/testdata/invalid/many_mistakes.json');

export default async function (h, { only }) {
  const admin = h.sessions.find((s) => s.email === 'admin@example.com') || await h.session('admin@example.com');
  const you = h.sessions.find((s) => s.email === 'you@example.com') || await h.session('you@example.com', { height: 1800 });
  admin.allow.push(/422/);
  you.allow.push(/422|404/); // 404: the picker asks whether the server knows the file
  const cards = () => admin.page.$$eval('#ruleset-panel li[id^="rs-"]', (l) => l.length);
  const report = (s) => s.page.$eval('[data-ruleset-report]', (e) => e.innerText.replace(/\s+/g, ' '));
  const swatches = async (s) => {
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(s, w, scheme);
      assert.ok(await s.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      const wide = await s.page.$$eval('[data-ruleset-report] *', (els) => els.filter((e) => e.getBoundingClientRect().right > innerWidth + 1).length);
      assert.equal(wide, 0, `nothing of the report sticks out at ${w}px ${scheme}`);
    }
    await layout(s, 1280, 'light');
  };
  let before = 0;

  await h.step('ruleset report: "Check this file" lists every problem and stores nothing', async () => {
    await admin.goto('/admin/rulesets');
    before = await cards();
    await admin.select('#rs-kind', 'battle_companies');
    await (await admin.page.$('#rs-file')).uploadFile(BAD);
    await admin.click('button', 'Check this file');
    await admin.waitText('12 problems found');
    const text = await report(admin);
    for (const want of ['battle_companies.json', 'campaign.recruitment.adjust.maxPerAttempt', 'Ranger of Arnr', 'Closest match', 'Ranger of Arnor', 'is already defined at gear[0]', 'put the old spelling in aka', 'Nothing was stored.']) {
      assert.ok(text.includes(want), `the report names ${want}`);
    }
    assert.equal(await admin.page.$$eval('[data-ruleset-report] li', (l) => l.length), 12, 'one row per problem');
    assert.ok(await admin.page.$eval('[data-ruleset-report] code', (c) => getComputedStyle(c).fontFamily.toLowerCase().includes('mono')), 'paths are monospace');
    await swatches(admin);
    assert.equal(await cards(), before, 'nothing is stored');
  }, { only });

  await h.step('ruleset report: the admin upload shows the same report, no card is added', async () => {
    await admin.goto('/admin/rulesets');
    await admin.select('#rs-kind', 'battle_companies');
    await (await admin.page.$('#rs-file')).uploadFile(BAD);
    await admin.click('button', '=Upload');
    await admin.waitText('12 problems found');
    assert.equal(await admin.page.$$eval('[data-ruleset-report] li', (l) => l.length), 12);
    assert.equal(await cards(), before, 'nothing is stored');
    await swatches(admin);
  }, { only });

  await h.step('ruleset report: the campaign form shows every problem, keeps the name and creates no campaign', async () => {
    const name = `E2E bad rules ${Date.now() % 100000}`;
    await you.goto('/campaigns/new');
    await you.fill('input[name=name]', name);
    await you.page.click('input[name=battle_companies-mode][value=file]');
    await (await you.page.$('input[name=battle_companies-file]')).uploadFile(BAD);
    await you.waitText('Fixture');
    await you.click('button', 'Create campaign', { nav: true });
    await you.waitText('12 problems found');
    const text = await report(you);
    assert.ok(text.includes('Nothing was stored and no campaign was created.'));
    assert.ok(text.includes('Ranger of Arnor'), 'the closest match is shown');
    assert.equal(await you.page.$eval('input[name=name]', (i) => i.value), name, 'the form keeps the name');
    await swatches(you);
    await you.goto('/campaigns');
    assert.ok(!(await you.has(name)), 'no campaign was created');
    await admin.goto('/admin/rulesets');
    assert.equal(await cards(), before, 'no rules file was stored');
  }, { only });
}
