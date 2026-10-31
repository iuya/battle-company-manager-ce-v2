// Puts before/after screenshots side by side under a title: node _compose.mjs shots outDir
import puppeteer from 'puppeteer-core';
import fs from 'fs';
import path from 'path';
const [dir, outDir] = process.argv.slice(2);
const img = f => 'data:image/png;base64,' + fs.readFileSync(path.join(dir, f)).toString('base64');
// each sheet: name, title, columns [label, file, cssWidth]
const B = 'Before: main', A = 'After: feat/visual-redesign';
const sheets = [
  ['1-leader-card-live', 'Leader in a live match (Veterans, 360 px)', [[B, 'before-light-card0.png', 328], [A, 'after-light-card0.png', 328]]],
  ['2-leader-card', 'Leader outside a match (Two bands, 360 px)', [[B, 'before-c21-card0.png', 328], [A, 'after-c21-card0.png', 328]]],
  ['3-sergeant-and-warrior-live', 'Sergeant and warrior in a live match (360 px)', [[B, 'before-light-card1.png', 328], [A, 'after-light-card1.png', 328], [B, 'before-light-card3.png', 328], [A, 'after-light-card3.png', 328]]],
  ['4-elven-company', 'An Elven company: unit and gear icons, Elven leaf (360 px)', [[B, 'before-elf-card0.png', 328], [A, 'after-elf-card0.png', 328], [B, 'before-elf-card7.png', 328], [A, 'after-elf-card7.png', 328]]],
  ['5-dark', 'Dark mode, live match (360 px)', [[B, 'before-dark-card0.png', 328], [A, 'after-dark-card0.png', 328], [B, 'before-dark-card3.png', 328], [A, 'after-dark-card3.png', 328]]],
  ['6-desktop-1280', 'My Company at 1280 px (Two bands)', [[B, 'before-desk.png', 640], [A, 'after-desk.png', 640]]],
  ['7-new-options', 'New on the branch: gear as chips or tiles, high contrast (360 px)', [['Gear: chips (default)', 'after-c21-card0.png', 328], ['Gear: tiles', 'after-tiles-card0.png', 328], ['High contrast, light', 'after-hc-card0.png', 328], ['High contrast, dark', 'after-hcdark-card0.png', 328]]],
];
const b = await puppeteer.launch({ executablePath: process.env.CHROME_PATH || 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true });
const p = await b.newPage();
for (const [name, title, cols] of sheets) {
  const html = `<!doctype html><meta charset=utf-8><style>
  body{margin:0;padding:24px;background:#f3f1ea;font:15px system-ui,sans-serif;color:#1f2a24;display:inline-block}
  h1{font:600 22px Georgia,serif;margin:0 0 16px}
  .row{display:flex;gap:20px;align-items:flex-start}
  figure{margin:0} figcaption{font-weight:600;margin-bottom:8px;font-size:14px}
  figcaption.before{color:#8a5a00} figcaption.after{color:#1d6b3a}
  img{display:block;border-radius:6px;box-shadow:0 1px 4px #0003}
  </style><h1>${title}</h1><div class=row>${cols.map(([l, f, w]) => `<figure><figcaption class="${l.startsWith('Before') ? 'before' : l.startsWith('After') ? 'after' : ''}">${l}</figcaption><img src="${img(f)}" style="width:${w}px"></figure>`).join('')}</div>`;
  await p.setViewport({ width: 400, height: 400, deviceScaleFactor: 2 });
  await p.setContent(html, { waitUntil: 'load' });
  const box = await p.evaluate(() => { const r = document.body.getBoundingClientRect(); return { w: Math.ceil(r.width), h: Math.ceil(r.height) }; });
  await p.setViewport({ width: box.w, height: box.h, deviceScaleFactor: name.startsWith('6') ? 1.5 : 2 });
  await p.screenshot({ path: path.join(outDir, name + '.png'), clip: { x: 0, y: 0, width: box.w, height: box.h } });
  console.log(name, box);
}
await b.close();
