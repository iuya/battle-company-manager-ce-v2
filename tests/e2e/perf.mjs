// Frontend performance baseline (docs/performance.md). `make perf` or `node perf.mjs [--only text] [--reps 3] [--latencies 0,100,300]`.
// Same scratch-database environment as the smoke tests (lib/env.mjs), with PERF_TRACE=true so the server answers with a Server-Timing
// header (time and database statements, internal/perftrace). Chrome's network emulation (CDP Network.emulateNetworkConditions) adds the
// round-trip latency between browser and server; nothing in the server is changed for it. The flows only read (they open panels, they do not save).
import { Env, outDir } from './lib/env.mjs';
import { Harness } from './lib/harness.mjs';
import { campaignId, MODAL } from './lib/util.mjs';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const args = process.argv.slice(2);
const opt = (name, def) => (args.includes(name) ? args[args.indexOf(name) + 1] : def);
const only = opt('--only', process.env.PERF_ONLY);
const reps = Number(opt('--reps', process.env.PERF_REPS || 3));
const latencies = opt('--latencies', process.env.PERF_LATENCIES || '0,100,300').split(',').map(Number);
const kbps = Number(opt('--kbps', process.env.PERF_KBPS || 0)); // download speed in KB/s (0: unlimited); 200 is about a 1.6 Mbit/s mobile link
const label = opt('--label', process.env.PERF_LABEL || 'run');
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const log = (m) => console.error(`[perf] ${m}`);

// Records every request of a page through CDP: when it started and ended, type, bytes on the wire, decoded bytes and the Server-Timing header.
class Recorder {
  constructor(page) { this.page = page; this.reset(); }
  async attach() {
    const c = this.cdp = await this.page.createCDPSession();
    await c.send('Network.enable');
    c.on('Network.requestWillBeSent', (e) => {
      if (e.request.url.startsWith('data:')) return;
      if (e.redirectResponse && this.byId.has(e.requestId)) { const prev = this.byId.get(e.requestId); prev.t1 = e.timestamp; prev.status = e.redirectResponse.status; this.inflight.delete(prev); }
      const r = { id: e.requestId, url: e.request.url, method: e.request.method, type: e.type, t0: e.timestamp, t1: null, bytes: 0, decoded: 0, status: 0, app: 0, db: 0, stmts: 0, cache: false, headers: {} };
      this.byId.set(e.requestId, r); this.list.push(r); this.inflight.add(r); this.last = Date.now();
    });
    c.on('Network.responseReceived', (e) => {
      const r = this.byId.get(e.requestId); if (!r) return;
      r.status = e.response.status; r.cache = !!(e.response.fromDiskCache || e.response.fromPrefetchCache);
      r.headers = e.response.headers || {};
      const st = Object.entries(r.headers).find(([k]) => k.toLowerCase() === 'server-timing');
      if (st) {
        const app = /app;dur=([\d.]+)/.exec(st[1]); const db = /db;dur=([\d.]+);desc="(\d+)"/.exec(st[1]);
        if (app) r.app = Number(app[1]); if (db) { r.db = Number(db[1]); r.stmts = Number(db[2]); }
      }
    });
    c.on('Network.requestServedFromCache', (e) => { const r = this.byId.get(e.requestId); if (r) r.cache = true; });
    c.on('Network.dataReceived', (e) => { const r = this.byId.get(e.requestId); if (r) r.decoded += e.dataLength; });
    c.on('Network.loadingFinished', (e) => { const r = this.byId.get(e.requestId); if (!r) return; r.t1 = e.timestamp; r.bytes = e.encodedDataLength; this.inflight.delete(r); this.last = Date.now(); });
    c.on('Network.loadingFailed', (e) => { const r = this.byId.get(e.requestId); if (!r) return; r.t1 = e.timestamp; r.failed = e.errorText; this.inflight.delete(r); this.last = Date.now(); });
  }
  reset() { this.byId = new Map(); this.list = []; this.inflight = new Set(); this.last = Date.now(); }
  // Streams (SSE) stay open for ever and do not count as "in flight" for settling.
  get busy() { return [...this.inflight].some((r) => r.type !== 'EventSource'); }
  async idle(quiet = 300, max = 20000) {
    const t = Date.now();
    while (Date.now() - t < max) { if (!this.busy && Date.now() - this.last >= quiet) return; await sleep(25); }
  }
  // The longest chain of requests where each starts after the previous one ended (the round trips the user waits for in series).
  chain() {
    const rs = this.list.filter((r) => r.type !== 'EventSource' && r.t1 !== null && !r.cache).sort((a, b) => a.t0 - b.t0);
    const lvl = new Map();
    let best = 0;
    for (const r of rs) {
      let l = 1;
      for (const p of rs) if (p !== r && p.t1 <= r.t0 + 0.0005 && lvl.has(p)) l = Math.max(l, lvl.get(p) + 1);
      lvl.set(r, l); best = Math.max(best, l);
    }
    return best;
  }
  summary() {
    const all = this.list;
    for (const r of all) if (r.status === 200 && r.t1 !== null && r.bytes === 0 && r.type !== 'EventSource') r.cache = true; // memory cache hits report no bytes
    const rs = all.filter((r) => !r.cache); // what went over the network; answers from the browser cache are only counted in `cached`
    const sum = (f) => rs.reduce((a, r) => a + f(r), 0);
    const kind = (r) => r.type === 'Document' ? 'doc' : (r.type === 'XHR' || r.type === 'Fetch') ? 'xhr' : r.type === 'EventSource' ? 'sse' : 'static';
    const n = { doc: 0, xhr: 0, sse: 0, static: 0 };
    for (const r of rs) n[kind(r)]++;
    const dynamic = rs.filter((r) => kind(r) === 'doc' || kind(r) === 'xhr');
    return {
      requests: rs.length, ...n, cached: all.length - rs.length,
      chain: this.chain(), bytes: Math.round(sum((r) => r.bytes)), htmlBytes: Math.round(dynamic.reduce((a, r) => a + r.bytes, 0)),
      htmlDecoded: Math.round(dynamic.reduce((a, r) => a + r.decoded, 0)),
      serverMs: +dynamic.reduce((a, r) => a + r.app, 0).toFixed(1), dbMs: +dynamic.reduce((a, r) => a + r.db, 0).toFixed(1), stmts: dynamic.reduce((a, r) => a + r.stmts, 0),
      detail: all.map((r) => ({ at: Math.round((r.t0 - all[0].t0) * 1000), end: r.t1 ? Math.round((r.t1 - all[0].t0) * 1000) : null, m: r.method, u: r.url.replace(/^https?:\/\/[^/]+/, ''), t: r.type, s: r.status, b: Math.round(r.bytes), d: Math.round(r.decoded), app: r.app, stmts: r.stmts, cache: r.cache })),
    };
  }
}

async function throttle(page, rec, ms) {
  await rec.cdp.send('Network.emulateNetworkConditions', { offline: false, latency: ms, downloadThroughput: kbps ? kbps * 1024 : -1, uploadThroughput: kbps ? kbps * 1024 : -1 });
}

const median = (a) => { const s = [...a].sort((x, y) => x - y); return s[Math.floor(s.length / 2)]; };

// One measured run of fn(): clears the recorder, runs, waits until the network is quiet, returns the summary and the timings.
async function measure(s, rec, fn, { observe = 0 } = {}) {
  rec.reset();
  const t0 = Date.now();
  await fn();
  await rec.idle();
  const settled = Date.now() - t0 - 300; // the quiet period of idle() is not part of the user's wait
  if (observe) { const c0 = rec.list.length; await sleep(observe); var background = rec.list.length - c0; }
  const nav = await s.page.evaluate(() => {
    const n = performance.getEntriesByType('navigation')[0];
    const fcp = performance.getEntriesByName('first-contentful-paint')[0];
    // ttfb: navigation start to the first byte of the document; nodes: elements in the DOM once the page settled
    return n ? { load: Math.round(n.loadEventEnd), fcp: fcp ? Math.round(fcp.startTime) : null, dom: Math.round(n.domContentLoadedEventEnd), ttfb: Math.round(n.responseStart), nodes: document.getElementsByTagName('*').length } : { nodes: document.getElementsByTagName('*').length };
  }).catch(() => ({}));
  return { ...rec.summary(), settledMs: Math.max(0, settled), ...nav, background };
}

async function main() {
  const env = new Env();
  env.extraEnv = { PERF_TRACE: 'true' };
  let h;
  const stop = async () => { await h?.close(); await env.cleanup(); };
  process.on('SIGINT', async () => { await stop(); process.exit(130); });
  try {
    mkdirSync(outDir, { recursive: true });
    if (process.env.PERF_BASE_URL) env.base = process.env.PERF_BASE_URL;
    else { await env.prepare(log); await env.startServer(); }
    log(`app at ${env.base}`);
    h = new Harness(env.base);
    await h.launch();

    // Discover the pages once (read-only, no latency).
    const admin = await h.session('admin@example.com');
    const ids = {};
    for (const n of ['Matches: every scenario', 'Aftermath ward 1', 'Level-ups waiting', 'Companies being built']) ids[n] = await campaignId(admin, n);
    const m = ids['Matches: every scenario'];
    const first = (re) => admin.page.evaluate((src) => { const a = [...document.querySelectorAll('main a[href]')].find((x) => new RegExp(src).test(x.getAttribute('href'))); return a && a.getAttribute('href'); }, re.source);
    await admin.goto(`/campaigns/${m}/matches`);
    const matchLinks = await admin.page.evaluate(() => [...new Set([...document.querySelectorAll('main a[href]')].map((a) => a.getAttribute('href')).filter((x) => /\/matches\/\d+$/.test(x)))]);
    let match = matchLinks[0], live = null;
    for (const ml of matchLinks) {
      await admin.goto(ml);
      const cos = await admin.page.evaluate(() => [...new Set([...document.querySelectorAll('main a[href]')].map((a) => a.getAttribute('href')).filter((x) => /\/companies\/\d+$/.test(x)))]);
      for (const c of cos.slice(0, 2)) {
        await admin.goto(c);
        if (await admin.page.evaluate(() => !!document.getElementById('live-dot-text'))) { live = c; match = ml; break; }
      }
      if (live) break;
    }
    await admin.goto(`/campaigns/${ids['Aftermath ward 1']}/matches`);
    const m2 = await first(/\/matches\/\d+$/);
    await admin.goto(m2);
    const aftermath = await first(/\/aftermath\/\d+$/);
    log(`pages: match ${match}, live ${live}, aftermath ${aftermath}`);
    await admin.close();

    const cm = ids['Companies being built'], lu = ids['Level-ups waiting'];
    // Each flow: who (a signed-in user), what to run (the navigation or the click), what to wait for. They only read.
    const flows = [
      { name: 'login', anon: true, run: async (s) => {
        await s.page.goto(env.base + '/login', { waitUntil: 'load' });
        await s.page.$eval('input[name=email]', (e) => { e.value = 'eowyn@example.com'; });
        await s.page.$eval('input[name=password]', (e) => { e.value = 'demo-password'; });
        await Promise.all([s.page.waitForNavigation({ waitUntil: 'load' }), s.page.click('button[type=submit]')]);
      } },
      { name: 'home', user: 'eowyn@example.com', run: (s) => s.page.goto(env.base + '/', { waitUntil: 'load' }) },
      { name: 'home, cold cache', user: 'eowyn@example.com', cold: true, run: (s) => s.page.goto(env.base + '/', { waitUntil: 'load' }) },
      { name: 'campaign overview', user: 'eowyn@example.com', run: (s) => s.page.goto(`${env.base}/campaigns/${m}`, { waitUntil: 'load' }) },
      { name: 'my company', user: 'eowyn@example.com', run: (s) => s.page.goto(`${env.base}/campaigns/${m}/my-company`, { waitUntil: 'load' }) },
      { name: 'recruit window (click)', user: 'eowyn@example.com', prep: (s) => s.page.goto(`${env.base}/campaigns/${cm}/my-company`, { waitUntil: 'load' }),
        run: async (s) => { await s.click('main button', 'Recruit'); await s.page.waitForFunction((q) => [...document.querySelectorAll(q)].some((e) => e.getBoundingClientRect().width > 0), {}, MODAL); } },
      { name: 'level-up window (click)', user: 'galadriel@example.com', prep: (s) => s.page.goto(`${env.base}/campaigns/${lu}/my-company`, { waitUntil: 'load' }),
        run: async (s) => { await s.click('main button', 'Level Up'); await s.page.waitForFunction((q) => [...document.querySelectorAll(q)].some((e) => e.getBoundingClientRect().width > 0), {}, MODAL); } },
      { name: 'click: overview to my company (htmx)', user: 'eowyn@example.com', prep: (s) => s.page.goto(`${env.base}/campaigns/${m}`, { waitUntil: 'load' }),
        run: async (s) => { await s.page.evaluate((h) => document.querySelector(`#sidebar a[href^="${h}"]`).click(), `/campaigns/${m}/my-company`); await s.page.waitForFunction((h) => location.pathname.startsWith(h), {}, `/campaigns/${m}/my-company`); } },
      { name: 'click: home to campaign overview (htmx)', user: 'eowyn@example.com', prep: (s) => s.page.goto(`${env.base}/`, { waitUntil: 'load' }),
        run: async (s) => { await s.page.evaluate((h) => document.querySelector(`main a[href="${h}"]`).click(), `/campaigns/${m}`); await s.page.waitForFunction((h) => location.pathname === h, {}, `/campaigns/${m}`); } },
      { name: 'armoury', user: 'eowyn@example.com', run: (s) => s.page.goto(`${env.base}/campaigns/${m}/armory`, { waitUntil: 'load' }) },
      { name: 'matches list', user: 'eowyn@example.com', run: (s) => s.page.goto(`${env.base}/campaigns/${m}/matches`, { waitUntil: 'load' }) },
      { name: 'match page', user: 'admin@example.com', run: (s) => s.page.goto(env.base + match, { waitUntil: 'load' }) },
      { name: 'live match (+3 s observed)', user: 'admin@example.com', observe: 3000, run: async (s) => {
        await s.page.goto(env.base + live, { waitUntil: 'load' });
        await s.page.waitForFunction(() => document.getElementById('live-dot-text')?.textContent === 'Live', { timeout: 15000 }).catch(() => {});
      } },
      { name: 'aftermath', user: 'admin@example.com', run: (s) => s.page.goto(env.base + aftermath, { waitUntil: 'load' }) },
    ].filter((f) => !only || f.name.includes(only));

    const results = [];
    for (const L of latencies) {
      log(`latency ${L} ms`);
      const sessions = {};
      for (const f of flows) {
        const key = f.anon ? 'anon' : f.user;
        const runs = [];
        for (let i = 0; i < reps; i++) {
          let s;
          if (f.anon) s = await h.session('anon', { login: false });
          else { s = sessions[key] ??= await h.session(key); }
          const rec = new Recorder(s.page); await rec.attach();
          await throttle(s.page, rec, 0);
          if (f.cold) await rec.cdp.send('Network.setCacheDisabled', { cacheDisabled: true });
          if (f.prep) { await f.prep(s); await rec.idle(); }
          else if (!f.anon && i === 0) { await s.page.goto(env.base + '/', { waitUntil: 'load' }); await rec.idle(); } // warms the static files, as a user who is browsing
          await throttle(s.page, rec, L);
          try { runs.push(await measure(s, rec, () => f.run(s), { observe: f.observe })); }
          catch (e) { await s.page.screenshot({ path: join(outDir, `perf-fail-${f.name.replace(/\W+/g, '-')}.png`) }).catch(() => {}); throw new Error(`${f.name} at ${L} ms: ${e.message}`); }
          await rec.cdp.send('Network.setCacheDisabled', { cacheDisabled: false });
          await rec.cdp.detach().catch(() => {});
          if (f.anon) await s.close();
        }
        // the run with the median settle time stands for the flow
        const med = median(runs.map((r) => r.settledMs));
        const pick = runs.find((r) => r.settledMs === med) || runs[0];
        results.push({ flow: f.name, latency: L, ...pick, settledAll: runs.map((r) => r.settledMs) });
        console.error(`  ${String(L).padStart(3)} ms  ${f.name.padEnd(28)} req ${pick.requests}+${pick.cached}c (doc ${pick.doc} xhr ${pick.xhr} sse ${pick.sse} static ${pick.static})  chain ${pick.chain}  ${pick.bytes} B  settled ${pick.settledMs} ms  fcp ${pick.fcp}  load ${pick.load}  server ${pick.serverMs} ms  stmts ${pick.stmts}`);
      }
      for (const s of Object.values(sessions)) await s.close();
    }
    writeFileSync(join(outDir, `perf-${label}.json`), JSON.stringify({ label, kbps, reps, latencies, when: new Date().toISOString(), results }, null, 1));
    printTable(results);
  } finally { await stop(); }
}

function printTable(results) {
  const byFlow = new Map();
  for (const r of results) { if (!byFlow.has(r.flow)) byFlow.set(r.flow, []); byFlow.get(r.flow).push(r); }
  console.log('\n| Flow | Network requests (doc/xhr/sse/static), cached | Chain | KB wire | KB HTML | Server ms | DB stmts | DOM elements | TTFB ms @ ' + latencies.join(' / ') + ' | FCP ms @ ' + latencies.join(' / ') + ' | Load ms @ ' + latencies.join(' / ') + ' | Settled ms @ ' + latencies.join(' / ') + ' |');
  console.log('|---|---|---|---|---|---|---|---|---|---|---|---|');
  for (const [f, rs] of byFlow) {
    const a = rs[0];
    console.log(`| ${f} | ${a.requests} (${a.doc}/${a.xhr}/${a.sse}/${a.static}) +${a.cached} cached | ${a.chain} | ${(a.bytes / 1024).toFixed(1)} | ${(a.htmlBytes / 1024).toFixed(1)} | ${a.serverMs} | ${a.stmts} | ${a.nodes ?? '-'} | ${rs.map((r) => r.ttfb ?? '-').join(' / ')} | ${rs.map((r) => r.fcp ?? '-').join(' / ')} | ${rs.map((r) => r.load ?? '-').join(' / ')} | ${rs.map((r) => r.settledMs).join(' / ')} |`);
  }
  const live = results.find((r) => r.flow.startsWith('live') && r.background !== undefined);
  if (live) console.log(`\nLive match: ${live.background} extra requests in the 3 s after it settled.`);
}

main().then(() => process.exit(0), (e) => { console.error(e); process.exit(1); });
