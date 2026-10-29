// Browser-side cost of the big pages (docs/performance.md, "Rendering cost"): what Chrome spends after the bytes arrived.
// `node rendercost.mjs [--label name] [--reps 5] [--cpu 4] [--only text]`; `make perf` measures the network, this measures the page itself.
// For each page (the static files already cached, no network latency): the CDP Performance metrics of the load (style, layout and script time,
// whole task time, elements, layout count), the long tasks (over 50 ms) of the page, first contentful paint, largest contentful paint and the
// layout shift score. `--cpu 4` slows the CPU four times (Emulation.setCPUThrottlingRate), a rough phone. The armoury also gets its two
// interactions: typing into the search and switching "Hide what ... cannot take". Median of --reps runs. Same scratch-database environment as
// `make perf` (PERF_BASE_URL reuses a running app). Only reads. Writes tests/e2e/out/rendercost-<label>.json and prints a Markdown table.
import { Env, outDir } from './lib/env.mjs';
import { Harness } from './lib/harness.mjs';
import { campaignId } from './lib/util.mjs';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const args = process.argv.slice(2);
const opt = (n, d) => (args.includes(n) ? args[args.indexOf(n) + 1] : d);
const label = opt('--label', 'run');
const reps = Number(opt('--reps', 5));
const cpu = Number(opt('--cpu', 1));
const only = opt('--only', '');
const latency = Number(opt('--latency', 0)); // ms added per request (CDP network emulation)
const cold = args.includes('--cold'); // browser cache disabled: a first visit
const log = (m) => console.error(`[rendercost] ${m}`);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const median = (a) => { const s = a.filter((x) => x != null).sort((x, y) => x - y); return s.length ? s[Math.floor(s.length / 2)] : null; };

const OBSERVE = () => {
  window.__lt = []; window.__cls = 0; window.__lcp = 0;
  new PerformanceObserver((l) => { for (const e of l.getEntries()) window.__lt.push(e.duration); }).observe({ type: 'longtask', buffered: true });
  new PerformanceObserver((l) => { for (const e of l.getEntries()) if (!e.hadRecentInput) { window.__cls += e.value; try { (window.__shifts ||= []).push(Math.round(e.startTime) + 'ms ' + e.value.toFixed(3) + ' ' + (e.sources || []).map((x) => { const n = x.node; return (n ? n.nodeName + '.' + String((n.getAttribute && n.getAttribute('class')) || '').slice(0, 40) + '#' + (n.id || '') : '?') + ' ' + [x.previousRect.x, x.previousRect.y, x.previousRect.width, x.previousRect.height].map(Math.round) + '->' + [x.currentRect.x, x.currentRect.y, x.currentRect.width, x.currentRect.height].map(Math.round); }).join(' | ')); } catch (err) { /* sources are for debugging only */ } } }).observe({ type: 'layout-shift', buffered: true });
  new PerformanceObserver((l) => { for (const e of l.getEntries()) window.__lcp = e.startTime; }).observe({ type: 'largest-contentful-paint', buffered: true });
};

async function metrics(cdp) {
  const { metrics } = await cdp.send('Performance.getMetrics');
  return Object.fromEntries(metrics.map((m) => [m.name, m.value]));
}

async function one(s, path, interact) {
  const cdp = await s.page.createCDPSession();
  await cdp.send('Performance.enable');
  if (cpu > 1) await cdp.send('Emulation.setCPUThrottlingRate', { rate: cpu });
  if (!s.observed) { await s.page.evaluateOnNewDocument(OBSERVE); s.observed = true; } // once: every registration would add the same entries again
  await cdp.send('Network.enable');
  await cdp.send('Network.setCacheDisabled', { cacheDisabled: cold });
  await cdp.send('Network.emulateNetworkConditions', { offline: false, latency, downloadThroughput: -1, uploadThroughput: -1 });
  await s.page.goto('about:blank');
  const m0 = await metrics(cdp);
  await s.page.goto(s.url(path), { waitUntil: 'load' });
  await sleep(1200 + 3 * latency); // lets Alpine, htmx cards and the paint settle
  const m1 = await metrics(cdp);
  const page = await s.page.evaluate(() => {
    const fcp = performance.getEntriesByName('first-contentful-paint')[0];
    return { fcp: fcp ? fcp.startTime : null, lcp: window.__lcp, cls: window.__cls, long: window.__lt.length, longMs: window.__lt.reduce((a, b) => a + b, 0), nodes: document.getElementsByTagName('*').length };
  });
  if (process.env.SHOW_SHIFTS && page.cls > 0) console.error((await s.page.evaluate(() => window.__shifts)).join('\n'));
  const d = (k) => Math.round((m1[k] - m0[k]) * 1000);
  const out = { ...page, style: d('RecalcStyleDuration'), layout: d('LayoutDuration'), script: d('ScriptDuration'), task: d('TaskDuration'), layouts: Math.round(m1.LayoutCount - m0.LayoutCount), recalcs: Math.round(m1.RecalcStyleCount - m0.RecalcStyleCount), heapMB: +(m1.JSHeapUsedSize / 1048576).toFixed(1) };
  if (interact) Object.assign(out, await interact(s, cdp));
  if (cpu > 1) await cdp.send('Emulation.setCPUThrottlingRate', { rate: 1 });
  await cdp.send('Network.setCacheDisabled', { cacheDisabled: false });
  await cdp.send('Network.emulateNetworkConditions', { offline: false, latency: 0, downloadThroughput: -1, uploadThroughput: -1 });
  await cdp.detach().catch(() => {});
  return out;
}

// Time from the input event until the next frame after Alpine updated, taken inside the page.
const frameTime = (s, fn) => s.page.evaluate(async (src) => {
  const f = new Function('return (' + src + ')')();
  const t0 = performance.now();
  await f();
  await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
  return Math.round(performance.now() - t0);
}, fn.toString());

async function armouryActions(s, cdp) {
  const search = await frameTime(s, async () => {
    const i = document.getElementById('armory-search'); i.value = 'sword'; i.dispatchEvent(new Event('input', { bubbles: true }));
  });
  const clear = await frameTime(s, async () => {
    const i = document.getElementById('armory-search'); i.value = ''; i.dispatchEvent(new Event('input', { bubbles: true }));
  });
  const toggle = await frameTime(s, async () => {
    document.querySelector('button[role=switch]').click();
  });
  const open = await frameTime(s, async () => {
    const b = [...document.querySelectorAll('li[data-name] button[aria-expanded]')].find((x) => x.getBoundingClientRect().width > 0);
    b.click();
  });
  return { searchMs: search, clearMs: clear, toggleMs: toggle, openMs: open };
}

async function main() {
  const env = new Env();
  let h;
  const stop = async () => { await h?.close(); await env.cleanup(); };
  try {
    mkdirSync(outDir, { recursive: true });
    if (process.env.PERF_BASE_URL) env.base = process.env.PERF_BASE_URL;
    else { await env.prepare(log); await env.startServer(); }
    h = new Harness(env.base);
    await h.launch();
    const admin = await h.session('admin@example.com');
    const m = await campaignId(admin, 'Matches: every scenario');
    await admin.goto(`/campaigns/${m}/matches`);
    const matchLinks = await admin.page.evaluate(() => [...new Set([...document.querySelectorAll('main a[href]')].map((a) => a.getAttribute('href')).filter((x) => /\/matches\/\d+$/.test(x)))]);
    let match = matchLinks[0], live = null;
    for (const ml of matchLinks) {
      await admin.goto(ml);
      const cos = await admin.page.evaluate(() => [...new Set([...document.querySelectorAll('main a[href]')].map((a) => a.getAttribute('href')).filter((x) => /\/companies\/\d+$/.test(x)))]);
      for (const c of cos.slice(0, 2)) { await admin.goto(c); if (await admin.page.evaluate(() => !!document.getElementById('live-dot-text'))) { live = c; match = ml; break; } }
      if (live) break;
    }
    const eowyn = await h.session('eowyn@example.com');
    await eowyn.goto(`/campaigns/${m}/my-company`);
    const company = new URL(eowyn.page.url()).pathname;
    const pages = [
      { name: 'my company', s: eowyn, path: company },
      { name: 'armoury', s: eowyn, path: `/campaigns/${m}/armory?company=${company.split('/').pop()}`, interact: armouryActions },
      { name: 'overview', s: eowyn, path: `/campaigns/${m}` },
      { name: 'live company page', s: admin, path: live },
      { name: 'match page', s: admin, path: match },
    ].filter((p) => !only || p.name.includes(only));
    const results = [];
    for (const p of pages) {
      await p.s.goto(p.path); // warms the static files
      const runs = [];
      for (let i = 0; i < reps; i++) runs.push(await one(p.s, p.path, p.interact));
      const keys = Object.keys(runs[0]);
      const med = Object.fromEntries(keys.map((k) => [k, median(runs.map((r) => r[k]))]));
      results.push({ page: p.name, path: p.path, ...med });
      console.error(`  ${p.name.padEnd(18)} nodes ${med.nodes} task ${med.task} ms (style ${med.style}, layout ${med.layout}, script ${med.script})  long ${med.long}/${Math.round(med.longMs)} ms  fcp ${Math.round(med.fcp)} lcp ${Math.round(med.lcp)} cls ${med.cls.toFixed(3)}${med.searchMs != null ? `  search ${med.searchMs} clear ${med.clearMs} toggle ${med.toggleMs} open ${med.openMs}` : ''}`);
    }
    writeFileSync(join(outDir, `rendercost-${label}.json`), JSON.stringify({ label, cpu, reps, when: new Date().toISOString(), results }, null, 1));
    console.log(`\n| Page (CPU x${cpu}) | Elements | Task ms | Style ms | Layout ms | Script ms | Layouts | Long tasks (ms) | FCP | LCP | CLS | Search / clear / toggle / open ms |\n|---|---|---|---|---|---|---|---|---|---|---|---|`);
    for (const r of results) console.log(`| ${r.page} | ${r.nodes} | ${r.task} | ${r.style} | ${r.layout} | ${r.script} | ${r.layouts} | ${r.long} (${Math.round(r.longMs)}) | ${Math.round(r.fcp)} | ${Math.round(r.lcp)} | ${r.cls.toFixed(3)} | ${r.searchMs != null ? `${r.searchMs} / ${r.clearMs} / ${r.toggleMs} / ${r.openMs}` : '-'} |`);
  } finally { await stop(); }
}

main().then(() => process.exit(0), (e) => { console.error(e); process.exit(1); });
