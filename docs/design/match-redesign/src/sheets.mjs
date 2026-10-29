// Composes the comparison sheets from out/*.png (mockups), cur/*.png (audit screenshots) and out/heights.json.
// Usage: node sheets.mjs   -> sheets/*.png
import puppeteer from 'puppeteer-core';
import fs from 'fs';
import path from 'path';
const H = JSON.parse(fs.readFileSync('out/heights.json'));
const fmt = n => n.toLocaleString('en-US');
const CUR = { match: 4525, live: 10376, aftermath: 10973 };
const CURY = { live: 1582 };

const CSS = `
*{box-sizing:border-box}
body{margin:0;padding:28px;background:#e6e8ea;font:14px/1.4 system-ui,-apple-system,"Segoe UI",sans-serif;color:#1b1f23;width:max-content}
h1{font:700 26px/1.2 system-ui;margin:0 0 4px}
.sub{color:#444;margin:0 0 18px;max-width:1500px}
.grid{display:flex;gap:28px;align-items:flex-start}
.col{width:360px}
.lab{display:flex;align-items:baseline;gap:8px;font-weight:700;font-size:17px;margin-bottom:2px}
.lab small{font-weight:500;color:#555;font-size:12px}
.tag{display:inline-block;color:#fff;border-radius:6px;padding:1px 9px;font-weight:800}
.t-cur{background:#666}.t-A{background:#3a6fb0}.t-B{background:#2f7a3e}.t-C{background:#a0602a}
.cap{height:54px;font-size:12px;color:#333;margin-bottom:6px}
.fr{width:360px;height:800px;overflow:hidden;border:2px solid #9aa0a6;border-radius:6px;background:#bbb;position:relative}
.fr img{width:360px;display:block}
.fr.rec{border-color:#2f7a3e;border-width:3px}
.pl{margin-top:8px;font-size:12px}
.pl .bg{height:12px;background:#d0d3d6;border-radius:3px;position:relative;margin-top:2px}
.pl .bg i{position:absolute;left:0;top:0;bottom:0;border-radius:3px}
.pl .vp{position:absolute;top:-3px;bottom:-3px;width:2px;background:#000}
.note{font-size:12px;color:#444;margin-top:2px}
.arrow{align-self:center;font-size:44px;color:#2f7a3e;font-weight:800;margin-top:360px}
.rowt{font:700 20px system-ui;margin:22px 0 8px}
`;
const col = ({ k, label, sub, cap, img, h, note, rec, y }) => {
  const bar = h ? `<div class="pl"><b>${fmt(h)} px</b> ${k === 'cur' ? 'page height (measured)' : 'page height (mockup)'}${y ? `<br>first Actions ${fmt(y)} px below the live block start${k === 'cur' ? ' (y = 2,317 on the page)' : ''}` : ''}<div class="bg"><i style="width:${Math.min(100, h / MAXH * 100)}%;background:${{ cur: '#666', A: '#3a6fb0', B: '#2f7a3e', C: '#a0602a' }[k]}"></i><span class="vp" style="left:${800 / MAXH * 100}%"></span></div>${note ? `<div class="note">${note}</div>` : ''}</div>` : (note ? `<div class="note">${note}</div>` : '');
  return `<div class="col"><div class="lab"><span class="tag t-${k}">${k === 'cur' ? 'Today' : k}</span>${label}${sub ? `<small>${sub}</small>` : ''}</div><div class="cap">${cap}</div><div class="fr${rec ? ' rec' : ''}"><img src="${img}"></div>${bar}</div>`;
};
let MAXH = 1;
const sheetHtml = (title, sub, cols, rows) => { MAXH = Math.max(...cols.map(c => c.h || 0), 1); return `<style>${CSS}</style><h1>${title}</h1><p class="sub">${sub}</p><div class="grid">${cols.map(col).join('')}</div>`; };

const sheets = {};
const std = (name, curImg, cap, key, notes = {}) => ({
  title: '', cols: [
    { k: 'cur', label: 'Current', sub: 'audit screenshot', img: curImg, cap: cap.cur, h: CUR[key], y: CURY[key] },
    { k: 'A', label: 'Density pass', sub: 'same structure', img: `out/${name}-A-360.png`, cap: cap.A, h: key ? H[`${name}-A`] : undefined, y: key ? H[`${name}-A-actionsY`] : undefined },
    { k: 'B', label: 'Task-oriented', sub: 'recommended', img: `out/${name}-B-360.png`, cap: cap.B, h: key ? H[`${name}-B`] : undefined, y: key ? H[`${name}-B-actionsY`] : undefined, rec: true },
    { k: 'C', label: 'Scoreboard + log', sub: 'side drawer', img: `out/${name}-C-360.png`, cap: cap.C, h: key ? H[`${name}-C`] : undefined, y: key ? H[`${name}-C-actionsY`] : undefined },
  ]
});

const defs = [
  ['1-match-page', 'Screen 1: the match page (360 px)', 'Match open, first screen of each design. Same scenario (Chance Encounter), same two companies.',
    std('match', 'cur/m1.png', { cur: 'Header buttons, one card per company, then the scenario card; End checks (27 rows) and the result lie further down.', A: 'Same cards, fewer: both sides in one card, End checks collapsed ("0 of 27 set"), sentences behind (i).', B: 'Accordion cards with one-line summaries; the bar at the bottom holds End checks and Propose to end.', C: 'Scoreboard and log only; every other card moves into the "Match tools" sheet.' }, 'match')],
  ['2-live-company-page', 'Screen 2: the live company page (360 px)', 'Scrolled to the live block (the campaign heading and tabs above it are left out in every frame). 15 units, nothing played yet.',
    std('live', 'cur/l1.png', { cur: 'Banner, 6 summary tiles, limits, then 15 full unit cards; Actions is the last control of each card (first one far below).', A: 'Banner with one "Match" menu; Actions moves into the header of each card. Cards stay full.', B: 'One compact row per unit (Wounds - / + and Actions); the full card opens on tap. Bar: Start turn, Match menu.', C: 'Dense table: name and Wounds only; a unit opens in a sheet. Bar: Unit actions, Match tools.' }, 'live')],
  ['3-live-actions', 'Screen 3a: the Actions menu during a match (360 px)', 'The sheet that opens from a unit. Viewport 360x800, page behind it dimmed.',
    std('actions', 'cur/act.png', { cur: 'Centered window, 6 rows of 58 px with hint lines, a cross and a Close button.', A: 'Same window, one 48 px line per row, no hints, no Close button.', B: 'Bottom sheet of 2-column tiles (56 px), label only.', C: 'Bottom sheet with a "quick action" field and a flat list.' }, null)],
  ['4-live-end-checks', 'Screen 3b: End checks during a match (360 px)', 'The objective and MVP window: 15 + 12 units.',
    std('endchecks', 'cur/end.png', { cur: '27 full-width rows (15 of them end with the word "No"), two sentences, same list as the match page.', A: 'Two-column toggles, a checkmark instead of "No", the sentence behind (i).', B: 'Wrapped toggle chips grouped by company: about 8 lines instead of 27 rows.', C: 'A table: Unit, Got it, MVP; one line per unit.' }, null)],
  ['5-aftermath', 'Screen 4: the aftermath, Sheet step (360 px)', 'First screen of step 1 (Sheet) before starting the aftermath, 15 units.',
    std('aftermath', 'cur/a1.png', { cur: 'Long explanations, the result box, then Against the Odds (6 bonuses) and one block per unit repeating six long XP sentences.', A: 'Short chip names with the sentence behind (i); Against the Odds collapsed. Still one block per unit.', B: 'Defaults stated once; one line per unit ("Survived +0 XP"), only the casualty is open. Save / Start in the bar.', C: 'XP and Influence on top, one dense two-line row per unit (outcome, took part, six toggles).' }, 'aftermath')],
];

const b = await puppeteer.launch({ executablePath: process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true });
fs.mkdirSync('sheets', { recursive: true });
const shoot = async (file, html, w, scale) => {
  fs.writeFileSync(`_${file}.html`, `<!doctype html><meta charset=utf-8><body>${html}`);
  const p = await b.newPage();
  await p.setViewport({ width: w, height: 600, deviceScaleFactor: scale });
  await p.goto('file://' + path.resolve(`_${file}.html`));
  await p.evaluate(() => Promise.all([...document.images].map(i => i.decode())));
  await p.screenshot({ path: `sheets/${file}.png`, fullPage: true });
  await p.close();
};

for (const [file, title, sub, s] of defs) {
  const notes = { match: 'Mockup heights are lower bounds: they draw only the collapsed/visible parts.', live: 'Mockup height counts the live block only (no campaign heading above it).' };
  await shoot(file, sheetHtml(title, sub, s.cols), 1620, 1.4);
}

/* ---- B in two steps ---- */
{
  const stepCol = (k, label, sub, cap, img, h) => ({ k, label, sub, cap, img, h });
  const live = [
    stepCol('cur', 'Current', '', 'Full cards, Actions at the bottom of each.', 'cur/l1.png', CUR.live),
    stepCol('A', 'Step 1', 'A: no-regret changes', 'Smaller buttons, Actions in the card header, names and sentences merged. Quick, reversible.', 'out/live-A-360.png', H['live-A']),
    stepCol('B', 'Step 2', 'B: structure', 'Compact unit rows, sticky bottom bar, bottom-sheet tiles, collapsible cards.', 'out/live-B-360.png', H['live-B']),
  ];
  const match = [
    stepCol('cur', 'Current', '', 'Two company cards, 27-row End checks inline.', 'cur/m1.png', CUR.match),
    stepCol('A', 'Step 1', 'A: no-regret changes', 'One sides block, End checks collapsed, tooltips instead of sentences.', 'out/match-A-360.png', H['match-A']),
    stepCol('B', 'Step 2', 'B: structure', 'Accordion cards and the bottom action bar.', 'out/match-B-360.png', H['match-B']),
  ];
  const row = (t, cols) => { MAXH = Math.max(...cols.map(c => c.h)); return `<div class="rowt">${t}</div><div class="grid">${cols.map((c, i) => (i ? '<div class="arrow">→</div>' : '') + col({ ...c, rec: c.k === 'B' })).join('')}</div>`; };
  await shoot('6-b-in-two-steps', `<style>${CSS}.arrow{margin-top:340px}</style><h1>Recommendation: B in two steps (360 px)</h1><p class="sub">Step 1 ships the no-regret changes of design A on today's structure. Step 2 adds B's compact rows, bottom bar and collapsible cards on top.</p>` + row('Live company page', live) + row('Match page', match), 1260, 1.15);
}

/* ---- desktop B ---- */
{
  const fr = (img, h) => `<div style="border:3px solid #2f7a3e;border-radius:6px;overflow:hidden;width:1280px;height:900px"><img src="${img}" style="display:block;width:1280px"></div>`;
  await shoot('7-b-desktop-1280', `<style>${CSS}body{width:1340px}</style><h1>Design B at 1280 px</h1><p class="sub">Match page and live company page. At this width B uses a sticky right panel instead of a bottom bar; on the live page the selected unit's full card and its actions are inline (no sheet). Mouse pointer: 36 px buttons.</p>
<div class="rowt">Match page (End checks, Result and Roles collapsed, as in the wireframe)</div>${fr('out/match-B-1280.png')}
<div class="rowt">Live company page (first 900 px; the list continues for 15 units, ${fmt(H['live-B-1280'])} px in all)</div>${fr('out/live-B-1280.png')}`, 1340, 1);
}

/* ---- dark ---- */
{
  const mk = (k, label, img, cap) => `<div class="col"><div class="lab"><span class="tag t-B">B</span>${label}</div><div class="cap">${cap}</div><div class="fr rec"><img src="${img}"></div></div>`;
  await shoot('8-b-dark', `<style>${CSS}</style><h1>Design B, light and dark (360 px)</h1><p class="sub">Same mockups with data-mode="dark" on the root: only semantic tokens, so nothing is redrawn.</p><div class="grid">${[
    mk('', 'Match page, light', 'out/match-B-360.png', ''), mk('', 'Match page, dark', 'out/match-B-dark-360.png', ''),
    mk('', 'Live page, light', 'out/live-B-360.png', ''), mk('', 'Live page, dark', 'out/live-B-dark-360.png', '')].join('')}</div>`, 1620, 1.4);
}
await b.close();
for (const f of fs.readdirSync('sheets').filter(f => f.endsWith('.png'))) console.log(f, Math.round(fs.statSync('sheets/' + f).size / 1024) + ' KB');
