// Signs in to a running server and saves a screenshot of a page (full or a clip) and of chosen unit cards; run from tests/e2e (puppeteer-core).
// node shot.mjs <base> <email> <path> <out.png> <width> [light|dark] [normal|high] [chips|tiles|-] [card indexes, e.g. 0,3] [clip y:height]
import puppeteer from 'puppeteer-core';
// node _shot.mjs base email path out width [mode] [contrast] [gear]
const [base, email, path, out, width, mode, contrast, gear] = process.argv.slice(2);
const b = await puppeteer.launch({ executablePath: process.env.CHROME_PATH || 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true, args: ['--hide-scrollbars'] });
const p = await b.newPage(); await p.setViewport({ width: +width, height: 800, deviceScaleFactor: +width < 700 ? 2 : 1 });
await p.goto(base + '/login'); await p.type('input[name=email]', email); await p.type('input[name=password]', 'demo-password');
await Promise.all([p.waitForNavigation(), p.click('button[type=submit]')]);
await p.goto(base + path, { waitUntil: 'load' });
await p.evaluate((mode, contrast, gear) => { const r = document.documentElement; if (mode) r.dataset.mode = mode; if (contrast === 'high') r.dataset.contrast = 'high'; if (gear && gear !== '-') r.dataset.gear = gear; }, mode, contrast, gear);
await p.evaluate(() => document.fonts.ready); await new Promise(r => setTimeout(r, 1200));
const cardsArg = process.argv[10];
if (cardsArg) { await p.evaluate(() => { for (const e of document.querySelectorAll('body *')) { const pos = getComputedStyle(e).position; if (pos === 'sticky' || pos === 'fixed') e.style.setProperty('position', 'static', 'important'); } }); const els = await p.$$('article.unit'); for (const i of cardsArg.split(',')) await els[+i].screenshot({ path: out.replace('.png', `-card${i}.png`) }); }
const clipArg = process.argv[11];
if (clipArg) { const [y, h] = clipArg.split(':').map(Number); await p.screenshot({ path: out, clip: { x: 0, y, width: +width, height: h }, captureBeyondViewport: true }); }
else await p.screenshot({ path: out, fullPage: true });
const info = await p.evaluate(() => ({ h: document.body.scrollHeight, sw: document.documentElement.scrollWidth, cards: [...document.querySelectorAll('article.unit')].map(e => { const r = e.getBoundingClientRect(); return [Math.round(r.left), Math.round(r.top + scrollY), Math.round(r.width), Math.round(r.height)]; }) }));
console.log(JSON.stringify(info));
await b.close();
