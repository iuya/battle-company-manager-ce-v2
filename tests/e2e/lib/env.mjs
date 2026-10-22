// Test environment: a scratch database copied from a cached, seeded template, the app on a free port, Chrome.
// Everything it starts it stops by pid; nothing here uses pkill, and the development database is never touched.
import { spawn, execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readdirSync, readFileSync, existsSync, mkdirSync, statSync, openSync } from 'node:fs';
import { createServer } from 'node:net';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export const here = dirname(fileURLToPath(import.meta.url));
export const e2eDir = resolve(here, '..');
export const root = resolve(e2eDir, '..', '..');
export const outDir = join(e2eDir, 'out');
const compose = ['compose', '-f', join(root, 'compose.dev.yaml')];

export function sh(cmd, args, opts = {}) {
  return execFileSync(cmd, args, { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], ...opts });
}

// psql runs inside the compose PostgreSQL container (the same one `make test-integration` uses); E2E_ADMIN_URL
// (a superuser-ish URL of any PostgreSQL) replaces it when psql is installed locally.
function psql(sql, db = 'postgres') {
  if (process.env.E2E_ADMIN_URL) {
    return sh('psql', [process.env.E2E_ADMIN_URL.replace(/\/[^/?]*(\?|$)/, `/${db}$1`), '-v', 'ON_ERROR_STOP=1', '-tAc', sql]);
  }
  return sh('docker', [...compose, 'exec', '-T', 'postgres', 'psql', '-U', 'bcm', '-d', db, '-v', 'ON_ERROR_STOP=1', '-tAc', sql]);
}

export function dbUrl(name) {
  const base = process.env.E2E_DB_URL_BASE || 'postgres://bcm:bcm@localhost:5432';
  return `${base}/${name}?sslmode=disable`;
}

function hashTree(paths) {
  const h = createHash('sha1');
  const walk = (p) => {
    if (!existsSync(p)) return;
    if (statSync(p).isDirectory()) { for (const f of readdirSync(p).sort()) walk(join(p, f)); return; }
    h.update(p); h.update(readFileSync(p));
  };
  for (const p of paths) walk(p);
  return h.digest('hex').slice(0, 10);
}

export async function freePort() {
  return new Promise((res, rej) => {
    const s = createServer();
    s.listen(0, '127.0.0.1', () => { const { port } = s.address(); s.close(() => res(port)); });
    s.on('error', rej);
  });
}

export function findChrome() {
  const c = [process.env.CHROME_PATH, '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/usr/bin/google-chrome', '/usr/bin/google-chrome-stable', '/usr/bin/chromium', '/usr/bin/chromium-browser',
    '/snap/bin/chromium'].filter(Boolean);
  const f = c.find((p) => existsSync(p));
  if (!f) throw new Error('No Chrome found: install Google Chrome or Chromium, or set CHROME_PATH');
  return f;
}

export class Env {
  constructor() { this.procs = []; this.db = null; this.port = null; }

  // Builds the server binary and makes sure the seeded template exists (the world takes minutes to seed, so it is
  // done once per version of the seed script, the migrations and the rules files, then copied per run).
  async prepare(log) {
    mkdirSync(outDir, { recursive: true });
    this.bin = join(outDir, 'bcm-e2e-server');
    log('building the server');
    sh('go', ['build', '-o', this.bin, './cmd/server']);
    const tpl = 'bcm_e2e_tpl_' + hashTree([join(root, 'cmd/server'), join(root, 'internal/postgres/migrations'), join(root, 'definitions')]);
    const exists = psql(`SELECT 1 FROM pg_database WHERE datname='${tpl}'`).trim() === '1';
    if (!exists || process.env.E2E_RESEED) {
      log(`seeding the demo world into ${tpl} (a few minutes, cached for later runs)`);
      psql(`DROP DATABASE IF EXISTS ${tpl}`);
      psql(`CREATE DATABASE ${tpl}`);
      sh(this.bin, ['seed', '--world'], { env: { ...process.env, DATABASE_URL: dbUrl(tpl) }, maxBuffer: 1 << 28 });
    }
    this.tpl = tpl;
    this.db = `bcm_e2e_run_${process.pid}`;
    psql(`DROP DATABASE IF EXISTS ${this.db}`);
    psql(`CREATE DATABASE ${this.db} TEMPLATE ${tpl}`);
    this.port = await freePort();
    this.base = `http://127.0.0.1:${this.port}`;
  }

  async startServer() {
    const logFd = openSync(join(outDir, 'server.log'), 'w');
    const p = spawn(this.bin, [], {
      cwd: root, stdio: ['ignore', logFd, logFd],
      env: { ...process.env, DATABASE_URL: dbUrl(this.db), PORT: String(this.port), APP_ENV: 'development', CSP_MODE: 'report', STYLEGUIDE: 'true' },
    });
    this.procs.push(p);
    for (let i = 0; i < 100; i++) {
      try { const r = await fetch(this.base + '/readyz'); if (r.ok) return; } catch {}
      await new Promise((r) => setTimeout(r, 200));
    }
    throw new Error('the server did not become ready (tests/e2e/out/server.log)');
  }

  // Stops only what this run started, then drops its scratch database. The cached template stays.
  async cleanup() {
    for (const p of this.procs) { try { p.kill('SIGTERM'); } catch {} }
    await new Promise((r) => setTimeout(r, 500));
    for (const p of this.procs) { try { if (p.exitCode === null) p.kill('SIGKILL'); } catch {} }
    if (this.db) { try { psql(`DROP DATABASE IF EXISTS ${this.db} WITH (FORCE)`); } catch (e) { console.error('could not drop', this.db, e.message); } }
  }
}
