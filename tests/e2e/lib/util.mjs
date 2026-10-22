import assert from 'node:assert/strict';
export { assert };
export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Finds a campaign's id by (a prefix of) its name from the navigation links of any signed-in page.
export async function campaignId(s, namePrefix) {
  await s.goto('/');
  const id = await s.page.evaluate((p) => {
    for (const a of document.querySelectorAll('a[href^="/campaigns/"]')) {
      const m = a.getAttribute('href').match(/^\/campaigns\/(\d+)$/);
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
