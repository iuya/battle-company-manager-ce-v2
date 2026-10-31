// The redesigned unit cards in every look the app has: light and dark, the high-contrast variant of the palettes, the gear style
// (chips or icon tiles), at 360, 768 and 1280 px: no horizontal scroll, no console errors, axe (serious and critical) clean.
// Pages: the Veterans demo campaign (heroes at twenty levels: its running match with the live trackers, hearts and milestones,
// both company pages, the rankings), an Elven company on My Company (Aftermath ward 1) and a campaign with injured units
// (Injuries and treatment). 60-pages already runs every page type in light and dark with the default look.
import { assert, campaignId } from '../lib/util.mjs';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');
// A company locked by a waiting aftermath draws its controls half transparent on purpose (inactive components are exempt from
// WCAG 1.4.3; see the data-lock rule in tailwind.css), so axe leaves them out.
const LOCKED = '[data-lock]:not([data-lock=""]) [data-lock]';
const SIZES = [[360, 780], [768, 1024], [1280, 900]];
// light/dark x contrast x gear; the default look (light and dark, normal contrast, chips) is 60-pages.
const LOOKS = [
  { scheme: 'light', contrast: 'high', gear: 'chips', axe: [360, 768, 1280] },
  { scheme: 'dark', contrast: 'high', gear: 'chips', axe: [360, 1280] },
  { scheme: 'light', contrast: 'normal', gear: 'tiles', axe: [360, 1280] },
  { scheme: 'dark', contrast: 'high', gear: 'tiles', axe: [360, 1280] },
];

export default async function (h) {
  const admin = await h.session('admin@example.com');
  const galadriel = await h.session('galadriel@example.com');
  const eowyn = await h.session('eowyn@example.com');
  const urls = [];
  const seen = { insignia: 0, live: 0, tracks: 0, away: 0, xp: 0, elven: 0 };

  await h.step('card variants: collect the Veterans, an Elven company and injured units', async () => {
    const vet = await campaignId(admin, 'Veterans');
    await admin.goto(`/campaigns/${vet}/matches`);
    const match = await admin.page.evaluate(() => [...document.querySelectorAll('main a[href]')].map((a) => a.getAttribute('href')).find((x) => /\/matches\/\d+$/.test(x)));
    assert.ok(match, 'the Veterans match');
    await admin.goto(match);
    const companies = await admin.page.evaluate(() => [...new Set([...document.querySelectorAll('main a[href]')].map((a) => a.getAttribute('href')).filter((x) => /\/companies\/\d+$/.test(x)))]);
    assert.ok(companies.length >= 2, `both Veterans companies are linked from the match (${companies.length})`);
    const ward = await campaignId(galadriel, 'Aftermath ward 1');
    const inj = await campaignId(eowyn, 'Injuries and treatment');
    urls.push(...companies.map((p) => [admin, p]), [admin, match], [admin, `/campaigns/${vet}/rankings`],
      [galadriel, `/campaigns/${ward}/my-company`], [eowyn, `/campaigns/${inj}/my-company`]);
  });

  const look = async (l, w, ht) => {
    for (const s of [admin, galadriel, eowyn]) {
      await s.page.setViewport({ width: w, height: ht });
      await s.page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: l.scheme }]);
      for (const [name, value] of [['bcm-contrast', l.contrast], ['bcm-gear', l.gear]]) {
        const c = { name, value, url: s.h.base };
        if (s.page.setCookie) await s.page.setCookie(c); else await s.ctx.setCookie(c);
      }
    }
  };
  const open = async (s, path, l) => {
    s.issues();
    await s.goto(path);
    await s.settle(250);
    const st = await s.page.evaluate(() => ({ c: document.documentElement.dataset.contrast || '', g: document.documentElement.dataset.gear || '' }));
    return st.c === (l.contrast === 'high' ? 'high' : '') && st.g === (l.gear === 'tiles' ? 'tiles' : '') ? '' : `${path}: the look did not apply (${JSON.stringify(st)})`;
  };

  for (const l of LOOKS) {
    const name = `${l.scheme}, ${l.contrast} contrast, gear ${l.gear}`;
    for (const [w, ht] of SIZES) {
      await h.step(`card variants at ${w} px (${name}): no horizontal scroll, no console errors`, async () => {
        await look(l, w, ht);
        const bad = [];
        for (const [s, path] of urls) {
          const m = await open(s, path, l);
          if (m) bad.push(m);
          if (!(await s.noHScroll())) bad.push(`${path}: horizontal scroll (${await s.page.evaluate(() => document.documentElement.scrollWidth)} px wide)`);
          for (const i of s.issues()) bad.push(`${path}: ${i}`);
          const n = await s.page.evaluate(() => ({
            insignia: document.querySelectorAll('main .unit-rank').length, live: document.querySelectorAll('main .live-trackers').length,
            tracks: document.querySelectorAll('main .tracker-seg').length,
            away: document.querySelectorAll('main .unit-away').length, xp: document.querySelectorAll('main .xp-bar').length,
            elven: [...document.querySelectorAll('main li.eq-chip, main ul[aria-label="Unit upgrades"] li')].filter((e) => /Elven/.test(e.innerText)).length,
          }));
          for (const k of Object.keys(seen)) seen[k] += n[k];
        }
        assert.equal(bad.length, 0, '\n  ' + bad.join('\n  '));
      });
    }
    for (const w of l.axe) {
      await h.step(`card variants, accessibility (axe-core) at ${w} px (${name}): no serious or critical violations`, async () => {
        await look(l, w, SIZES.find((x) => x[0] === w)[1]);
        const bad = [];
        for (const [s, path] of urls) {
          const m = await open(s, path, l);
          if (m) bad.push(m);
          await s.page.evaluate(axeSource);
          const res = await s.page.evaluate((locked) => axe.run({ include: [['body']], exclude: [[locked]] }, {
            runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'],
          }).then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical')
            .map((v) => ({ id: v.id, impact: v.impact, n: v.nodes.length, sample: v.nodes.slice(0, 2).map((x) => x.target.join(' ') + ' :: ' + (x.any[0]?.message || x.all[0]?.message || x.none[0]?.message || '')) }))), LOCKED);
          s.issues();
          for (const v of res) bad.push(`${path}: ${v.id} (${v.impact}, ${v.n} nodes) ${v.sample.join(' | ')}`);
        }
        assert.equal(bad.length, 0, '\n  ' + bad.join('\n  '));
      });
    }
  }

  await h.step('card variants: the pages held every kind of card (insignia, live trackers, injured, experience bar, Elven gear)', async () => {
    for (const k of Object.keys(seen)) assert.ok(seen[k] > 0, `no ${k} on any page: ${JSON.stringify(seen)}`);
  });
}
