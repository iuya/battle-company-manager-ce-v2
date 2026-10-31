// Server-side cost of the main pages, taken over plain HTTP (no browser): docs/optimization-report.md, "Method".
// `node pagetimes.mjs [--label name] [--reps 20] [--warm 5]`. Same scratch-database environment as the smoke tests (lib/env.mjs), the server
// started with PERF_TRACE=true so that every answer carries Server-Timing (app ms to the first byte, database ms and statements).
// For each page: the median of --reps requests after --warm warm-up requests of the Server-Timing figures, the wall time of the request as
// the client sees it, the HTML raw and gzip, and the CSS and JS the page loads, raw and gzip. It also takes the server's startup time to
// the first successful /healthz and /login and its resident memory (RSS) after start, after the pages were loaded once, and after 200 more
// page requests. Writes tests/e2e/out/pagetimes-<label>.json and prints Markdown tables. Only reads; the development database is not touched.
import http from 'node:http';
import zlib from 'node:zlib';
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, openSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { Env, outDir, dbUrl, root } from './lib/env.mjs';

const args = process.argv.slice(2);
const opt = (n, d) => (args.includes(n) ? args[args.indexOf(n) + 1] : d);
const label = opt('--label', 'run');
const reps = Number(opt('--reps', 20));
const warm = Number(opt('--warm', 5));
const log = (m) => console.error(`[pagetimes] ${m}`);
const agent = new http.Agent({ keepAlive: true, maxSockets: 4 });
const median = (a) => { const s = [...a].sort((x, y) => x - y); return s.length % 2 ? s[(s.length - 1) / 2] : (s[s.length / 2 - 1] + s[s.length / 2]) / 2; };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const jars = new Map();
// One request. gzip: ask for it (the wire bytes are then the compressed body); otherwise identity. Returns the status, headers, wire and decoded sizes, ms.
function request(base, path, { user, gzip = true, htmx = false, method = 'GET', form } = {}) {
  return new Promise((resolve, reject) => {
    const u = new URL(path, base);
    const headers = { 'Accept-Encoding': gzip ? 'gzip' : 'identity' };
    if (user && jars.get(user)) headers.Cookie = jars.get(user);
    if (htmx) headers['HX-Request'] = 'true';
    let body = null;
    if (form) { body = new URLSearchParams(form).toString(); headers['Content-Type'] = 'application/x-www-form-urlencoded'; headers['Content-Length'] = Buffer.byteLength(body); }
    const t0 = process.hrtime.bigint();
    const req = http.request({ host: u.hostname, port: u.port, path: u.pathname + u.search, method, headers, agent }, (res) => {
      const chunks = []; let ttfb = Number(process.hrtime.bigint() - t0) / 1e6;
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => {
        const wire = Buffer.concat(chunks);
        const ms = Number(process.hrtime.bigint() - t0) / 1e6;
        const enc = res.headers['content-encoding'];
        const raw = enc === 'gzip' ? zlib.gunzipSync(wire) : wire;
        const set = res.headers['set-cookie'];
        if (set && user) { // merge by cookie name: an answer may set only the appearance cookie
          const jar = new Map((jars.get(user) || '').split('; ').filter(Boolean).map((c) => [c.split('=')[0], c]));
          for (const c of set) { const kv = c.split(';')[0]; jar.set(kv.split('=')[0], kv); }
          jars.set(user, [...jar.values()].join('; '));
        }
        const st = res.headers['server-timing'] || '';
        const app = /app;dur=([\d.]+)/.exec(st), db = /db;dur=([\d.]+);desc="(\d+)"/.exec(st);
        resolve({ status: res.statusCode, headers: res.headers, wire: wire.length, raw: raw.length, text: raw.toString('utf8'), ms, ttfb, app: app ? +app[1] : null, db: db ? +db[1] : null, stmts: db ? +db[2] : null, location: res.headers.location });
      });
    });
    req.on('error', reject);
    if (body) req.write(body);
    req.end();
  });
}

// Follows redirects (the my-company alias) and returns the last answer plus the final path.
async function get(base, path, o = {}) {
  let r = await request(base, path, o), p = path;
  for (let i = 0; i < 5 && r.status >= 300 && r.status < 400 && r.location; i++) { p = r.location; r = await request(base, p, o); }
  r.path = p;
  return r;
}

async function login(base, user) {
  jars.delete(user);
  await request(base, '/login', { user });
  const r = await request(base, '/login', { user, method: 'POST', form: { email: user, password: 'demo-password' } });
  if (r.status !== 303 && r.status !== 302) throw new Error(`sign-in as ${user} failed: ${r.status}`);
}

const rssKB = (pid) => Number(execFileSync('ps', ['-o', 'rss=', '-p', String(pid)], { encoding: 'utf8' }).trim());

async function main() {
  const env = new Env();
  env.extraEnv = { PERF_TRACE: 'true' };
  const out = { label, reps, warm, when: new Date().toISOString() };
  try {
    mkdirSync(outDir, { recursive: true });
    await env.prepare(log);
    // The server is started here (not by env.startServer) so that the time to the first answer is measured with a tight poll.
    const spawnServer = () => {
      const fd = openSync(join(outDir, 'server.log'), 'w');
      return spawn(env.bin, [], { cwd: root, stdio: ['ignore', fd, fd], env: { ...process.env, DATABASE_URL: dbUrl(env.db), PORT: String(env.port), APP_ENV: 'development', CSP_MODE: 'report', STYLEGUIDE: 'true', PERF_TRACE: 'true' } });
    };
    const startups = [];
    for (let i = 0; i < 6; i++) { // the first one also warms the page cache of the binary; it is reported but not part of the median of the rest
      const t0 = process.hrtime.bigint();
      const p = spawnServer(); env.procs.push(p);
      let health = null, loginPage = null;
      for (;;) {
        if (health === null) { try { const r = await request(env.base, '/healthz'); if (r.status === 200) health = Number(process.hrtime.bigint() - t0) / 1e6; } catch {} }
        if (health !== null) { try { const r = await request(env.base, '/login'); if (r.status === 200) { loginPage = Number(process.hrtime.bigint() - t0) / 1e6; break; } } catch {} }
        if (Number(process.hrtime.bigint() - t0) / 1e9 > 60) throw new Error('server did not start');
        await sleep(5);
      }
      startups.push({ healthzMs: Math.round(health), loginMs: Math.round(loginPage) });
      if (i < 5) { p.kill('SIGTERM'); await new Promise((r) => p.on('exit', r)); }
      else { env.server = p; }
    }
    out.startups = startups;
    const server = env.server;
    out.rss = { startupKB: rssKB(server.pid) };

    // Discover the pages like perf.mjs does.
    await login(env.base, 'eowyn@example.com'); await login(env.base, 'admin@example.com');
    const E = 'eowyn@example.com', A = 'admin@example.com';
    const campaigns = {};
    const list = await get(env.base, '/campaigns', { user: E });
    for (const m of list.text.matchAll(/href="\/campaigns\/(\d+)(?:\/my-company)?"[^>]*>([\s\S]*?)<\/a>/g)) campaigns[m[2].replace(/<[^>]+>/g, '').trim()] = m[1];
    const adminList = await get(env.base, '/campaigns', { user: A });
    const adminCampaigns = {};
    for (const m of adminList.text.matchAll(/href="\/campaigns\/(\d+)(?:\/my-company)?"[^>]*>([\s\S]*?)<\/a>/g)) adminCampaigns[m[2].replace(/<[^>]+>/g, '').trim()] = m[1];
    const byPrefix = (map, p) => { const k = Object.keys(map).find((n) => n.startsWith(p)); if (!k) throw new Error('no campaign ' + p + ' in ' + JSON.stringify(Object.keys(map))); return map[k]; };
    const m = byPrefix(campaigns, 'Matches: every scenario');
    const am = byPrefix(adminCampaigns, 'Aftermath ward 1'), mm = byPrefix(adminCampaigns, 'Matches: every scenario');
    const links = (text, re) => [...new Set([...text.matchAll(/href="([^"]+)"/g)].map((x) => x[1]).filter((x) => re.test(x)))];
    const mine = await get(env.base, `/campaigns/${m}/my-company`, { user: E });
    const cmatch = /\/my-company\/(\d+)/.exec(mine.path); if (!cmatch) throw new Error(`my-company answered ${mine.status} at ${mine.path}: ${mine.text.slice(0, 300)}; campaigns ${JSON.stringify(campaigns)}`);
    const companyId = cmatch[1];
    const matchesPage = await get(env.base, `/campaigns/${mm}/matches`, { user: A });
    const matchLinks = links(matchesPage.text, /\/matches\/\d+$/);
    let match = matchLinks[0], live = null;
    for (const ml of matchLinks) {
      const page = await get(env.base, ml, { user: A });
      for (const c of links(page.text, /\/companies\/\d+$/).slice(0, 2)) {
        const cp = await get(env.base, c, { user: A });
        if (cp.text.includes('live-dot-text')) { live = c; match = ml; break; }
      }
      if (live) break;
    }
    const am2 = await get(env.base, `/campaigns/${am}/matches`, { user: A });
    const amMatch = links(am2.text, /\/matches\/\d+$/)[0];
    const am3 = await get(env.base, amMatch, { user: A });
    const aftermath = links(am3.text, /\/aftermath\/\d+$/)[0];
    // The new-company page answers only a member who may still found a company (otherwise it redirects): the first campaign and user it opens for.
    let newCompany = null;
    for (const id of Object.values(campaigns)) for (const u of [E, A]) {
      if (newCompany) break;
      if ((await request(env.base, `/campaigns/${id}/company/new`, { user: u })).status === 200) newCompany = { user: u, path: `/campaigns/${id}/company/new` };
    }
    if (!newCompany) throw new Error('no campaign opens the new-company page');
    log(`campaign ${m}, company ${companyId}, match ${match}, live ${live}, aftermath ${aftermath}, new company ${newCompany.path}`);
    out.ids = { m, companyId, match, live, aftermath, newCompany };

    const pages = [
      { name: 'Campaign list', user: E, path: '/campaigns' },
      { name: 'Campaign overview', user: E, path: `/campaigns/${m}` },
      { name: 'My Company', user: E, path: `/campaigns/${m}/my-company/${companyId}` },
      { name: 'Matches tab', user: E, path: `/campaigns/${m}/matches` },
      { name: 'Armory', user: E, path: `/campaigns/${m}/armory` },
      { name: 'Campaign glossary', user: E, path: `/campaigns/${m}/glossary` },
      { name: 'Glossary (sidebar)', user: E, path: '/glossary' },
      { name: 'Glossary unit cards (fetched on demand)', user: E, path: `/campaigns/${m}/glossary/units`, htmx: true, fragment: true },
      { name: 'Company page: Arnor (rule texts with unit names)', user: newCompany.user, path: `${newCompany.path.replace(/new$/, 'info')}?c=Arnor`, htmx: true, fragment: true },
      { name: 'Company page: Fornost (rule texts with unit names)', user: newCompany.user, path: `${newCompany.path.replace(/new$/, 'info')}?c=Fornost`, htmx: true, fragment: true },
      { name: 'New company', user: newCompany.user, path: newCompany.path },
      { name: 'Match page', user: A, path: match },
      { name: 'Live match (company page)', user: A, path: live },
      { name: 'Aftermath', user: A, path: aftermath },
      { name: 'Tabletop Simulator dialog, aura rings on', user: E, path: `/campaigns/${m}/companies/${companyId}/tts`, htmx: true, fragment: true },
      { name: 'Tabletop Simulator dialog, aura rings off', user: E, path: `/campaigns/${m}/companies/${companyId}/tts?auras=0`, htmx: true, fragment: true },
      { name: 'Administration: Users (superuser)', user: A, path: '/admin/users' },
      ...(process.env.PT_NO_INVITATIONS ? [] : [{ name: 'Administration: Invitations (superuser)', user: A, path: '/admin/invitations' }]), // PT_NO_INVITATIONS=1 on a tree before the page existed
    ];

    // First visit of every page (rule sets get loaded, caches fill), then the memory figure "after the demo world is loaded".
    for (const p of pages) { const r = await get(env.base, p.path, p); if (r.status !== 200) throw new Error(`${p.name}: ${p.path} answered ${r.status}`); }
    out.rss.afterPagesKB = rssKB(server.pid);

    // 200 more page requests, round robin.
    for (let i = 0; i < 200; i++) { const p = pages[i % pages.length]; await get(env.base, p.path, p); }
    out.rss.after200KB = rssKB(server.pid);

    // Static files the pages load (shared by every page: measured once per address).
    const statics = new Map();
    const assetsOf = (html) => [...html.matchAll(/(?:href|src)="(\/static\/[^"]+\.(?:css|js))(?:\?[^"]*)?"/g)].map((x) => x[0].replace(/^(?:href|src)="/, '').slice(0, -1));
    const sizeOfStatic = async (u) => {
      if (!statics.has(u)) {
        const g = await request(env.base, u, { user: E, gzip: true }), r = await request(env.base, u, { user: E, gzip: false });
        statics.set(u, { raw: r.wire, gzip: g.headers['content-encoding'] === 'gzip' ? g.wire : null });
      }
      return statics.get(u);
    };

    out.pages = [];
    for (const p of pages) {
      for (let i = 0; i < warm; i++) await get(env.base, p.path, p);
      const runs = [];
      for (let i = 0; i < reps; i++) runs.push(await get(env.base, p.path, p));
      const raw = await get(env.base, p.path, { ...p, gzip: false }), gz = await get(env.base, p.path, p);
      let css = { raw: 0, gzip: 0, n: 0 }, js = { raw: 0, gzip: 0, n: 0 };
      if (!p.fragment) for (const u of assetsOf(raw.text)) {
        const s = await sizeOfStatic(u), t = u.includes('.css') ? css : js;
        t.raw += s.raw; t.gzip += s.gzip ?? s.raw; t.n++;
      }
      out.pages.push({
        name: p.name, path: p.path, status: gz.status,
        appMs: +median(runs.map((r) => r.app)).toFixed(1), dbMs: +median(runs.map((r) => r.db)).toFixed(1), stmts: median(runs.map((r) => r.stmts)),
        stmtsRange: [Math.min(...runs.map((r) => r.stmts)), Math.max(...runs.map((r) => r.stmts))],
        clientMs: +median(runs.map((r) => r.ms)).toFixed(1), ttfbMs: +median(runs.map((r) => r.ttfb)).toFixed(1),
        htmlRaw: raw.wire, htmlGzip: gz.headers['content-encoding'] === 'gzip' ? gz.wire : null, css, js,
      });
      log(`${p.name.padEnd(42)} app ${out.pages.at(-1).appMs} ms  db ${out.pages.at(-1).dbMs} ms  stmts ${out.pages.at(-1).stmts}  html ${raw.wire} B (gzip ${out.pages.at(-1).htmlGzip})`);
    }
    out.rss.afterAllKB = rssKB(server.pid);
    writeFileSync(join(outDir, `pagetimes-${label}.json`), JSON.stringify(out, null, 1));
    printTables(out);
  } finally { agent.destroy(); await env.cleanup(); }
}

function printTables(o) {
  const kb = (n) => (n == null ? '-' : (n / 1024).toFixed(1));
  console.log(`\nStartup (ms from spawn): healthz ${o.startups.map((s) => s.healthzMs).join(', ')}; /login ${o.startups.map((s) => s.loginMs).join(', ')}`);
  console.log(`RSS MB: startup ${(o.rss.startupKB / 1024).toFixed(1)}, after the pages were loaded once ${(o.rss.afterPagesKB / 1024).toFixed(1)}, after 200 more requests ${(o.rss.after200KB / 1024).toFixed(1)}, after the measurement ${(o.rss.afterAllKB / 1024).toFixed(1)}`);
  console.log('\n| Page | Server ms (app) | DB ms | Statements | Client ms | HTML KB raw | HTML KB gzip | CSS KB raw / gzip | JS KB raw / gzip |');
  console.log('|---|---|---|---|---|---|---|---|---|');
  for (const p of o.pages) console.log(`| ${p.name} | ${p.appMs} | ${p.dbMs} | ${p.stmts} | ${p.clientMs} | ${kb(p.htmlRaw)} | ${kb(p.htmlGzip)} | ${p.css.n ? `${kb(p.css.raw)} / ${kb(p.css.gzip)}` : '-'} | ${p.js.n ? `${kb(p.js.raw)} / ${kb(p.js.gzip)}` : '-'} |`);
}

main().then(() => process.exit(0), (e) => { console.error(e); process.exit(1); });
