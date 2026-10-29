// The main page types at 360, 768 and 1280 px in light and dark: no horizontal scroll, no console errors, no failed requests.
import { assert, campaignId } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
// Rules the pass leaves out, each with the reason (empty: none).
const AXE_OFF = [];

const SIZES = [[360, 780], [768, 1024], [1280, 900]];

export default async function (h) {
  const admin = await h.session('admin@example.com');
  const anon = await h.session('anon', { login: false });
  const first = (s, re) => s.page.evaluate((src) => {
    const a = [...document.querySelectorAll('main a[href]')].find((x) => new RegExp(src).test(x.getAttribute('href')));
    return a && a.getAttribute('href');
  }, re.source);

  // Collect the urls once.
  const urls = [];
  await h.step('pages: collect one page of each type from the demo world', async () => {
    const ids = {};
    for (const n of ['Matches: every scenario', 'Aftermath ward 1', 'Level-ups waiting']) ids[n] = await campaignId(admin, n);
    const m = ids['Matches: every scenario'];
    await admin.goto(`/campaigns/${m}/matches`);
    const match = await first(admin, /\/matches\/\d+$/);
    assert.ok(match, 'a match link');
    await admin.goto(match);
    const live = await first(admin, /\/companies\/\d+$/);
    const a = ids['Aftermath ward 1'];
    await admin.goto(`/campaigns/${a}/matches`);
    const m2 = await first(admin, /\/matches\/\d+$/);
    await admin.goto(m2);
    const aft = await first(admin, /\/aftermath\/\d+$/);
    assert.ok(live && aft, `a company page and an aftermath page were found (${live}, ${aft})`);
    for (const [who, path] of [
      [anon, '/login'], [admin, '/campaigns'], [admin, '/campaigns/new'],
      [admin, `/campaigns/${m}`], [admin, `/campaigns/${m}/my-company`], [admin, `/campaigns/${m}/armory`],
      [admin, `/campaigns/${m}/companies`], [admin, `/campaigns/${m}/rankings`], [admin, `/campaigns/${m}/matches`],
      [admin, `/campaigns/${m}/glossary`], [admin, match], [admin, live], [admin, aft],
      [admin, `/campaigns/${ids['Level-ups waiting']}/my-company`],
      [admin, `/campaigns/${m}/company/new`], [admin, '/glossary'], [admin, '/account'], [admin, '/admin'],
      [admin, '/admin/users'], [admin, '/admin/rulesets'], [admin, '/styleguide'],
    ]) if (path) urls.push([who, path]);
  });

  for (const [w, ht] of SIZES) {
    for (const scheme of ['light', 'dark']) {
      await h.step(`pages at ${w} px, ${scheme}: ${'no horizontal scroll, no console errors'}`, async () => {
        const bad = [];
        for (const s of [admin, anon]) {
          await s.page.setViewport({ width: w, height: ht });
          await s.page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: scheme }]);
        }
        for (const [s, path] of urls) {
          s.issues();
          await s.goto(path);
          await s.settle(250);
          const dark = await s.page.evaluate(() => { const c = getComputedStyle(document.body).backgroundColor.match(/\d+/g).map(Number); return (c[0] + c[1] + c[2]) / 3 < 128; });
          if (dark !== (scheme === 'dark')) bad.push(`${path}: the page is ${dark ? 'dark' : 'light'} in ${scheme} mode`);
          if (!(await s.noHScroll())) bad.push(`${path}: horizontal scroll (${await s.page.evaluate(() => document.documentElement.scrollWidth)} px wide)`);
          for (const i of s.issues()) bad.push(`${path}: ${i}`);
        }
        assert.equal(bad.length, 0, '\n  ' + bad.join('\n  '));
      });
    }
  }

  // axe-core: serious and critical violations of WCAG 2.x A/AA on every collected page, light and dark.
  for (const [w, ht, scheme] of [[1280, 900, 'light'], [1280, 900, 'dark'], [360, 780, 'light'], [360, 780, 'dark']]) {
    await h.step(`accessibility (axe-core) at ${w} px, ${scheme}: no serious or critical violations`, async () => {
      const bad = [];
      for (const s of [admin, anon]) {
        await s.page.setViewport({ width: w, height: ht });
        await s.page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: scheme }]);
      }
      for (const [s, path] of urls) {
        await s.goto(path);
        await s.settle(250);
        await s.page.evaluate(axeSource);
        const res = await s.page.evaluate((off) => axe.run(document, {
          runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] },
          rules: Object.fromEntries(off.map((r) => [r, { enabled: false }])),
          resultTypes: ['violations'],
        }).then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical')
          .map((v) => ({ id: v.id, impact: v.impact, n: v.nodes.length, sample: v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' :: ' + (x.any[0]?.message || x.all[0]?.message || x.none[0]?.message || '')) }))), AXE_OFF);
        s.issues();
        for (const v of res) bad.push(`${path}: ${v.id} (${v.impact}, ${v.n} nodes) ${v.sample.join(' | ')}`);
      }
      assert.equal(bad.length, 0, '\n  ' + bad.join('\n  '));
    });
  }
}
