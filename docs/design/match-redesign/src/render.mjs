// Renders ./pages/*.html to ./out/*.png (360x800 @2x, and 1280x900 @1x for B) and writes out/heights.json (scrollHeight at 360).
import puppeteer from 'puppeteer-core';
import fs from 'fs';
import path from 'path';
const chrome = process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const b = await puppeteer.launch({ executablePath: chrome, headless: true });
fs.mkdirSync('out', { recursive: true });
const heights = {};
const only = process.argv[2];
for (const f of fs.readdirSync('pages').filter(f => f.endsWith('.html') && (!only || f.includes(only)))) {
  const name = f.replace('.html', '');
  const url = 'file://' + path.resolve('pages', f);
  const p = await b.newPage();
  await p.setViewport({ width: 360, height: 800, deviceScaleFactor: 2 });
  await p.goto(url); await p.evaluate(() => document.fonts.ready);
  heights[name] = await p.evaluate(() => Math.round(document.body.getBoundingClientRect().height));
  heights[name + '-actionsY'] = await p.evaluate(() => { const e = [...document.querySelectorAll('button,span')].find(b => /^(Unit )?[Aa]ctions( ▾)?$/.test(b.textContent.trim()) && b.children.length === 0); return e ? Math.round(e.getBoundingClientRect().top + scrollY) : null; });
  const hsx = await p.evaluate(() => document.documentElement.scrollWidth);
  if (hsx > 360) console.log('HORIZONTAL OVERFLOW', name, hsx);
  await p.screenshot({ path: `out/${name}-360.png` });
  if (/-B(-dark)?$/.test(name) && !name.startsWith('actions') && !name.startsWith('endchecks')) {
    await p.setViewport({ width: 1280, height: 900, deviceScaleFactor: 1 });
    await p.goto(url); await p.evaluate(() => document.fonts.ready);
    await p.screenshot({ path: `out/${name}-1280.png` });
    heights[name + '-1280'] = await p.evaluate(() => Math.round(document.body.getBoundingClientRect().height));
  }
  await p.close();
}
const old = fs.existsSync('out/heights.json') ? JSON.parse(fs.readFileSync('out/heights.json')) : {};
fs.writeFileSync('out/heights.json', JSON.stringify({ ...old, ...heights }, null, 1));
console.log(heights);
await b.close();
