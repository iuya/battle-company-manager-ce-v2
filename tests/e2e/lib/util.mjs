import assert from 'node:assert/strict';
export { assert };
export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Finds a campaign's id by (a prefix of) its name from the navigation links of any signed-in page.
export async function campaignId(s, namePrefix) {
  await s.goto('/campaigns');
  const id = await s.page.evaluate((p) => {
    for (const a of document.querySelectorAll('a[href^="/campaigns/"]')) {
      const m = a.getAttribute('href').match(/^\/campaigns\/(\d+)(?:\/my-company)?$/); // the lists link to My Company
      if (m && a.innerText.trim().startsWith(p)) return m[1];
    }
    return null;
  }, namePrefix);
  assert.ok(id, `no campaign named "${namePrefix}..." for ${s.email}`);
  return id;
}

// The app's modal sheets are role=dialog elements (the visibility check in Session.click skips the closed ones).
export const MODAL = '[role=dialog]';

// Clicks the button of the open confirmation dialog.
export async function confirmDialog(s, label = '=Yes', opts = {}) {
  await s.click(`dialog[open] button, ${MODAL} button`, label, opts); // a native <dialog> (take command) or a sheet
}

// Reads the first number found after a label in a block of text ("Influence\n\n0" -> 0).
export function numberAfter(text, label) {
  const m = text.match(new RegExp(label + '\\s*(-?\\d+)'));
  return m ? Number(m[1]) : NaN;
}

// Sets a viewport and colour scheme on a session (the app follows prefers-color-scheme).
export async function layout(s, width, scheme, height = 900) {
  await s.page.setViewport({ width, height });
  await s.page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: scheme }]);
  await s.settle(200);
}
export const SIZES = [360, 768, 1280];

// WCAG contrast ratio of two css colours ("rgb(r, g, b)" / "rgba(...)"); a translucent background is composited on `under`.
export function contrast(fg, bg, under = [255, 255, 255]) {
  const parse = (c) => c.match(/[\d.]+/g).map(Number);
  const over = (c) => { const [r, g, b, a = 1] = parse(c); return [r, g, b].map((v, i) => v * a + under[i] * (1 - a)); };
  const lum = (rgb) => { const [r, g, b] = rgb.map((v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
  const L1 = lum(over(fg)), L2 = lum(over(bg));
  return (Math.max(L1, L2) + 0.05) / (Math.min(L1, L2) + 0.05);
}
