// The online validator (/tools/validate), open to every signed-in player: the Battle Companies file (and optionally the vanilla file and the
// glossary) is checked as the "Check this file" dry run does, nothing is stored and no campaign is created. Checked at 360, 768 and 1280 px in
// light and dark. One account makes at most 6 checks a minute, so this flow makes five.
import { assert, SIZES, layout } from '../lib/util.mjs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve, join } from 'node:path';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';

const EX = resolve(dirname(fileURLToPath(import.meta.url)), '../../../examples/customizing-rulesets');

export default async function (h, { only }) {
  const admin = h.sessions.find((s) => s.email === 'admin@example.com') || await h.session('admin@example.com');
  const you = h.sessions.find((s) => s.email === 'you@example.com') || await h.session('you@example.com', { height: 1800 });
  you.allow.push(/422|404/);
  const cards = () => admin.page.$$eval('#ruleset-panel li[id^="rs-"]', (l) => l.length);
  const result = () => you.page.$eval('#validator-result', (e) => e.innerText.replace(/\s+/g, ' '));
  const pick = async (id, file) => (await you.page.$(id)).uploadFile(file);
  const check = async () => { await you.click('button', 'Check files'); };
  const swatches = async () => {
    for (const scheme of ['light', 'dark']) for (const w of SIZES) {
      await layout(you, w, scheme);
      assert.ok(await you.noHScroll(), `no horizontal scroll at ${w}px ${scheme}`);
      const wide = await you.page.$$eval('#validator-result *, form input[type=file]', (els) => els.filter((e) => e.getBoundingClientRect().right > innerWidth + 1).length);
      assert.equal(wide, 0, `nothing sticks out at ${w}px ${scheme}`);
    }
    await layout(you, 1280, 'light');
  };
  let before = 0, campaigns = '';

  await h.step('online validator: linked from the new-campaign form and the rules page; labelled fields', async () => {
    await admin.goto('/admin/rulesets');
    before = await cards();
    assert.ok(await admin.page.$('a[href="/tools/validate"]'), 'the rules page links to the checker');
    await you.goto('/');
    campaigns = await you.page.evaluate(() => document.querySelector('main, #main')?.innerText || '');
    await you.goto('/campaigns/new');
    assert.ok(await you.page.$('a[href="/tools/validate"]'), 'the new-campaign form links to the checker');
    assert.ok(await you.has('Check it first'));
    await you.goto('/tools/validate');
    await you.waitText('Check rules files');
    for (const [id, text] of [['v-battle', 'Battle Companies file'], ['v-vanilla', 'Vanilla rules file'], ['v-glossary', 'Glossary']]) {
      assert.ok(await you.page.$eval(`label[for=${id}]`, (l) => l.innerText.includes('')), `label for ${id}`);
      assert.ok((await you.page.$eval(`label[for=${id}]`, (l) => l.innerText)).includes(text));
    }
    assert.ok(await you.page.$eval('#validator-result', (e) => e.getAttribute('aria-live') === 'polite'), 'a live region');
  }, { only });

  await h.step('online validator: the example files pass, the focus moves to the answer', async () => {
    await pick('#v-battle', `${EX}/minimal.json`);
    await check();
    await you.waitText('No problems found');
    assert.ok((await result()).includes('Nothing was stored and no campaign was created.'));
    assert.equal(await you.page.evaluate(() => document.activeElement && document.activeElement.id), 'validator-result', 'the answer takes the focus');
    for (const f of ['custom-unit.json', 'parameter-override.json']) {
      await pick('#v-battle', `${EX}/${f}`);
      await check();
      await you.waitText('No problems found');
      assert.ok((await result()).includes('the shipped one'), `${f} is checked against the shipped vanilla file and glossary`);
    }
    await swatches();
  }, { only });

  await h.step('online validator: the typos example lists every problem with the closest names', async () => {
    await pick('#v-battle', `${EX}/broken/typos.json`);
    await check();
    await you.waitText('4 problems found');
    const text = await result();
    for (const want of ['battle_companies.json', 'companies[0].colour', 'Greenway Lor', 'Greenway Watchr', 'Closest match', 'Greenway Lore', 'rolls cover 1-5']) {
      assert.ok(text.includes(want), `the report names ${want}`);
    }
    assert.equal(await you.page.$$eval('#validator-result [data-ruleset-report] li', (l) => l.length), 4, 'one row per problem');
    assert.ok(await you.page.$eval('#validator-result code', (c) => getComputedStyle(c).fontFamily.toLowerCase().includes('mono')), 'paths are monospace');
    await swatches();
  }, { only });

  await h.step('online validator: a vanilla file that is not JSON is reported as the vanilla file; nothing is stored', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'validator-'));
    const NOT_JSON = join(dir, 'vanilla.json');
    writeFileSync(NOT_JSON, '{nope');
    await pick('#v-vanilla', NOT_JSON);
    await pick('#v-battle', `${EX}/minimal.json`);
    await check();
    await you.waitText('problem');
    const text = await result();
    assert.ok(text.includes('vanilla.json') && text.includes('not valid JSON'), text);
    assert.ok(text.includes('Vanilla rules data: your file'));
    await swatches();
    await you.goto('/');
    assert.equal(await you.page.evaluate(() => document.querySelector('main, #main')?.innerText || ''), campaigns, 'no campaign was created');
    await admin.goto('/admin/rulesets');
    assert.equal(await cards(), before, 'no rules file was stored');
  }, { only });
}
