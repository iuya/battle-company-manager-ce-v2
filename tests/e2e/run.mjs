// Browser smoke tests (docs/testing.md). `make e2e`; `node run.mjs --only <text>` runs the flow files whose name contains it (for example `--only match`);
// `E2E_BASE_URL=http://127.0.0.1:18080 node run.mjs` runs against a server you started yourself (scratch data only!).
import { Env, outDir } from './lib/env.mjs';
import { Harness, clearOut } from './lib/harness.mjs';
import { readdirSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const args = process.argv.slice(2);
const only = args.includes('--only') ? args[args.indexOf('--only') + 1] : process.env.E2E_ONLY;
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
  const files = readdirSync(dir).filter((f) => f.endsWith('.mjs') && (!only || f.includes(only))).sort();
  const shared = {}; // flows hand objects to later flows (the campaign and the match they created)
  for (const f of files) {
    const mod = await import(join(dir, f));
    console.log(`\n${f.replace(/\.mjs$/, '')}`);
    try { await mod.default(h, { shared, env }); }
    catch (e) { console.log(`  FAIL  ${f}: ${e.stack || e}`); h.results.push({ name: f, ok: false, error: String(e.message) }); await h.dump(f); }
  }
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
