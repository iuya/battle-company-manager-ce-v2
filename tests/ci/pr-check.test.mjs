// Tests the pull request check of .github/workflows/pr-check.yml without GitHub: extracts its inline script and runs it against sample
// pull requests. Dependency-free: node tests/ci/pr-check.test.mjs (Node 18+). Exit code 1 on a failed expectation.
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
const lines = readFileSync(path.join(root, '.github/workflows/pr-check.yml'), 'utf8').split('\n');
const start = lines.findIndex(l => /^\s+script: \|\s*$/.test(l));
if (start < 0) throw new Error('script block not found in pr-check.yml');
const indent = /^\s*/.exec(lines[start + 1])[0].length;
const source = lines.slice(start + 1).map(l => l.slice(indent)).join('\n');
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const run = new AsyncFunction('context', 'core', source);

const template = readFileSync(path.join(root, '.github/PULL_REQUEST_TEMPLATE.md'), 'utf8');
const good = template
  .replace(/^(\s*- )\[ \]/gm, '$1[x]')
  .replace(/^(## Summary\s*\n)/m, '$1Fixes the roster wrap.\n')
  .replace(/^(## Linked issue\s*\n)/m, '$1Closes #12\n')
  .replace(/^(## How it was tested\s*\n)/m, '$1make test.\n');

async function check(title, body, type = 'User') {
  const result = { failed: null };
  const core = { info() {}, error() {}, setFailed(m) { result.failed = m; } };
  await run({ payload: { pull_request: { title, body, user: { type } } } }, core);
  return result.failed;
}

let failures = 0;
async function expect(name, wantFail, title, body, type) {
  const failed = await check(title, body, type);
  const ok = wantFail ? failed !== null : failed === null;
  if (!ok) { failures++; console.error('FAIL ' + name + (failed ? '\n' + failed : '')); } else console.log('ok   ' + name);
}

const fill = (s, from, to) => s.replace(from, to);
await expect('filled template passes', false, 'feat(web): add roster export', good);
await expect('CRLF line endings pass', false, 'fix: wrap', good.replace(/\n/g, '\r\n'));
await expect('breaking change marker passes', false, 'refactor(core)!: drop the old API', good);
await expect('scope with dots and slashes passes', false, 'chore(deps/go.mod): bump', good);
await expect('bot pull requests are skipped', false, 'Bump x from 1 to 2', '', 'Bot');
for (const t of ['Add roster export', 'feat add roster', 'feat:no space', 'Feat: capital', 'feature: x', 'feat(): x', 'feat(Web): x', 'feat(web) add', 'feat: ', '', 'fix:  ']) {
  await expect('bad title "' + t + '"', true, t, good);
}
await expect('empty body fails', true, 'fix: x', '');
await expect('untouched template fails', true, 'fix: x', template);
await expect('missing section fails', true, 'fix: x', fill(good, /## Linked issue[\s\S]*?(?=## )/, ''));
await expect('no type ticked fails', true, 'fix: x', fill(good, /(## Type[\s\S]*?)(?=## Linked)/, m => m.replace(/\[x\]/g, '[ ]')));
await expect('mandatory box unticked fails', true, 'fix: x', fill(good, /\[x\](.*make test)/, '[ ]$1'));
await expect('secrets box unticked fails', true, 'fix: x', fill(good, /\[x\](.*no secrets)/i, '[ ]$1'));
await expect('licence box unticked fails', true, 'fix: x', fill(good, /\[x\](.*licen)/i, '[ ]$1'));
await expect('conditional box unticked passes', false, 'fix: x', fill(good, /\[x\](.*make validate)/, '[ ]$1'));
await expect('summary only a comment fails', true, 'fix: x', fill(good, 'Fixes the roster wrap.', '<!-- describe -->'));

if (failures) { console.error(failures + ' failure(s)'); process.exit(1); }
console.log('all pr-check cases pass');
