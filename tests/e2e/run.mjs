// Browser smoke tests (docs/testing.md). `make e2e`; `node run.mjs --only <text>` runs the flow files whose name contains it (for example `--only match`);
// `--group <name>` (or E2E_GROUP) runs the flows of one group of groups.json (CI runs one job per group);
// `E2E_BASE_URL=http://127.0.0.1:18080 node run.mjs` runs against a server you started yourself (scratch data only!).
import { Env, outDir } from './lib/env.mjs';
import { Harness, clearOut } from './lib/harness.mjs';
import { readdirSync, readFileSync } from 'node:fs';
import { loadGroups, checkGroups } from './lib/groups.mjs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const args = process.argv.slice(2);
const only = args.includes('--only') ? args[args.indexOf('--only') + 1] : process.env.E2E_ONLY;
const group = args.includes('--group') ? args[args.indexOf('--group') + 1] : process.env.E2E_GROUP; // a group of groups.json
const prepareOnly = args.includes('--prepare');
const log = (m) => console.log(`[e2e] ${m}`);

const env = new Env();
let h;
let code = 1;
const t0 = Date.now();
const stop = async () => { await h?.close(); await env.cleanup(); };
process.on('SIGINT', async () => { await stop(); process.exit(130); });
process.on('SIGTERM', async () => { await stop(); process.exit(143); });

try {
  clearOut();
  const problems = checkGroups();
  if (problems.length) throw new Error('tests/e2e/groups.json:\n  ' + problems.join('\n  '));
  if (process.env.E2E_BASE_URL) { env.base = process.env.E2E_BASE_URL; }
  else {
    await env.prepare(log);
    if (prepareOnly) { log('template ready: ' + env.tpl); await env.cleanup(); process.exit(0); }
    await env.startServer();
  }
  log(`app at ${env.base}`);
  h = new Harness(env.base);
  await h.launch();
  const dir = join(dirname(fileURLToPath(import.meta.url)), 'flows');
  let members = null;
  if (group) { members = loadGroups()[group]; if (!members) throw new Error(`no group ${group} in tests/e2e/groups.json`); log(`group ${group}: ${members.join(', ')}`); }
  const files = readdirSync(dir).filter((f) => f.endsWith('.mjs') && (!only || f.includes(only)) && (!members || members.includes(f.replace(/\.mjs$/, '')))).sort();
  const shared = {}; // flows hand objects to later flows (the campaign and the match they created)
  for (const f of files) {
    const mod = await import(join(dir, f));
    console.log(`\n${f.replace(/\.mjs$/, '')}`);
    try { await mod.default(h, { shared, env }); }
    catch (e) { console.log(`  FAIL  ${f}: ${e.stack || e}`); h.results.push({ name: f, ok: false, error: String(e.message) }); await h.dump(f); }
  }
  // The server logged every report the browsers sent to /csp-report (a second witness next to the securitypolicyviolation events).
  try {
    const reports = readFileSync(join(outDir, 'server.log'), 'utf8').split('\n').filter((l) => l.includes('csp violation') && !l.includes('csp-control'));
    if (!process.env.E2E_BASE_URL) {
      h.results.push(reports.length ? { name: 'csp: no violation reports in the server log', ok: false, error: reports.slice(0, 5).join('\n') } : { name: 'csp: no violation reports in the server log', ok: true });
      console.log(reports.length ? `  FAIL  csp reports in the server log (${reports.length}):\n        ${reports.slice(0, 5).join('\n        ')}` : '\n  ok    csp: no violation reports in the server log');
    }
  } catch {}
  const failed = h.results.filter((r) => !r.ok);
  console.log(`\n[e2e] ${h.results.length - failed.length} passed, ${failed.length} failed in ${Math.round((Date.now() - t0) / 1000)} s${h.retries ? `, ${h.retries} click(s) repeated because no request was sent` : ''}`);
  for (const r of failed) console.log(`  FAILED: ${r.name}`);
  code = failed.length ? 1 : 0;
} catch (e) {
  console.error('[e2e] setup failed:', e.stack || e);
} finally {
  await stop();
}
process.exit(code);
