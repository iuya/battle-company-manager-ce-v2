// axe-core scoped to the content a flow is about (the whole page is slow and has timed out on CI), and the looks every page of the
// administration is checked in: 360/768/1280 px, light and dark, normal and high contrast.
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { layout, SIZES } from './util.mjs';

const axeSource = readFileSync(createRequire(import.meta.url).resolve('axe-core/axe.min.js'), 'utf8');

// Serious and critical WCAG 2.1 A/AA violations inside the given selectors ([] when none).
export async function axeScoped(s, include) {
  await s.page.evaluate(axeSource);
  return s.page.evaluate((include) => axe.run({ include: include.map((x) => [x]) }, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] }, resultTypes: ['violations'] })
    .then((r) => r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => `${v.id} (${v.impact}, ${v.nodes.length}) ${v.nodes.slice(0, 2).map((n) => n.target.join(' ')).join(' | ')}`)), include);
}

export const LOOKS = [['light', 'normal'], ['dark', 'normal'], ['light', 'high'], ['dark', 'high']];

// Puts the session in a look (width, colour scheme, contrast cookie).
export async function setLook(s, w, scheme, contrast) {
  const ck = { name: 'bcm-contrast', value: contrast, url: s.h.base };
  if (s.page.setCookie) await s.page.setCookie(ck); else await s.ctx.setCookie(ck);
  await layout(s, w, scheme, w === 360 ? 780 : 900);
}

export async function resetLook(s) {
  await s.page.deleteCookie({ name: 'bcm-contrast', url: s.h.base }).catch(() => {});
  await layout(s, 1280, 'light');
}

// One step per look; `check(w, scheme, contrast, axeHere)` runs inside it. axe runs at 360 and 1280 in every look but 768, as flow 101 does.
export async function eachLook(h, title, only, check) {
  for (const [scheme, contrast] of LOOKS) {
    for (const w of SIZES) {
      await h.step(`${title} at ${w} px, ${scheme}${contrast === 'high' ? ', high contrast' : ''}`, async () => {
        await check(w, scheme, contrast, w !== 768);
      }, { only });
    }
  }
}
