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

// Ending a match (docs/matches.md, "The sheets"): the End match sheet (winner, MVP of every opposing company, note) and the Confirm sheet.
// `mvp` is the index of the unit voted in every ballot (the first by default); null votes nothing (a practice match needs no vote).
async function voteInSheet(s, mvp) {
  if (mvp === null) return;
  await s.page.evaluate((m, i) => {
    for (const ballot of document.querySelectorAll(m + ' [data-mvp-ballot]')) {
      const units = [...ballot.querySelectorAll('[role=listbox] button')];
      (units[i] || units[0])?.click();
    }
  }, MODAL, mvp);
  await s.settle(150);
}

// Opens End match on the match page and proposes the end: winner is the label of a side ("Rohan + Gondor") or "Draw".
export async function endMatch(s, matchPath, winner, { mvp = 0 } = {}) {
  await s.goto(matchPath);
  await s.click('main [data-end-match]', '');
  await s.page.waitForSelector(MODAL + ' [data-end-sheet]');
  await s.click(MODAL + ' label', winner);
  await voteInSheet(s, mvp);
  await s.click(MODAL + ' [data-end-sheet] button.btn-primary', '=End match');
  await s.settle(600);
}

// Opens Confirm on the match page (an end is pending) and confirms with the viewer's MVP votes.
export async function confirmEnd(s, matchPath, { mvp = 0 } = {}) {
  await s.goto(matchPath);
  await s.click('main [data-confirm-end]', '');
  await s.page.waitForSelector(MODAL + ' [data-end-sheet]');
  await voteInSheet(s, mvp);
  await s.click(MODAL + ' [data-end-sheet] button.btn-primary', '=Confirm');
  await s.settle(600);
}

// Confirms Against the Odds when the match page offers it to this viewer (docs/matches.md, "Against the Odds before the game"), taking what still
// fits until nothing does: nothing can be noted and the match cannot end until it is decided. It does nothing on a match with nothing to decide.
// The session must be on the match page.
export async function decideATO(s) {
  if (!(await s.page.$('#ato [data-ato-confirm]'))) return;
  for (let i = 0; i < 40 && (await s.page.$eval('#ato [data-ato-confirm]', (b) => b.disabled)); i++) {
    const pressed = await s.page.evaluate(() => {
      const free = (css) => [...document.querySelectorAll(css)].find((e) => !e.disabled && !e.checked && e.offsetParent !== null);
      const next = free('#ato [data-ato-add]') || free('#ato input[name=kit]') || free('#ato input[name=wanderer]') || free('#ato input[name=bonus][value=toolKit]') || free('#ato input[name=bonus][value=wanderer]');
      next?.click();
      return !!next;
    });
    if (!pressed) break;
    await s.settle(50);
  }
  await s.click('#ato [data-ato-confirm]', '');
  await s.waitText('Against the Odds confirmed.');
}

// Rolls the new-match form's scenario card until it shows "Rolled N (table T) and N". With Shake the table on the browser throws physical
// dice, and a pair the rules file places no scenario on leaves the card with a "roll again" note, so a roll can need another throw.
// `first` is the text of the button that starts the first throw ("Roll the scenario", "Roll again", "Roll instead"); retries press the card's roll button.
export async function rollNewScenario(s, first, { tries = 8, timeout = 30000 } = {}) {
  for (let i = 0; i < tries; i++) {
    if (i === 0) await s.click('#new-scenario button', first);
    else await s.click('#new-scenario [data-roll-again]', '');
    await s.page.waitForFunction(() => /Rolled \d \(table \d\) and \d|found no scenario/.test(document.querySelector('#new-scenario').innerText), { timeout });
    if (/Rolled \d \(table \d\) and \d/.test(await s.page.$eval('#new-scenario', (e) => e.innerText))) return;
  }
  throw new Error(`the dice found no scenario in ${tries} throws`);
}

// Sets the scenario of an open match before the game: Change scenario, Choose another, then the scenario's button.
export async function chooseScenario(s, name) {
  await s.click('main [data-change-scenario]', '').catch(() => {}); // a match without a scenario shows the choices at once
  await s.click('main [data-choose-another]', '');
  await s.click('main #scenario button', name);
  await s.waitText(name);
}
