// Tests of the checks in groups.mjs and of the template key (node --test; run by make e2e-check-groups, part of make test).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { writeFileSync, rmSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { lintDiceLoops, loops } from './groups.mjs';
import { templateKey, root } from './env.mjs';

test('loops finds the body of a braced and of a one-statement loop', () => {
  const found = loops('a(); for (let i = 0; i < 3; i++) { x(`{`); } while (go()) y(); z();');
  assert.equal(found.length, 2);
  assert.match(found[0], /x\(/);
  assert.match(found[1], /y\(\);$/);
});

test('a loop around a dice click needs a dice source or an allow-list entry', () => {
  const retry = "for (let i = 0; i < 6; i++) { await s.click('#scenario [data-roll-again]', ''); if (await s.page.$('#r')) break; }";
  assert.equal(lintDiceLoops({ '99-x': retry, '110-new-match-form': '', '86-dice3d': '' }).filter((p) => p.startsWith('99-x')).length, 1);
  const seeded = "for (let i = 0; i < 6; i++) { await s.page.evaluate(() => serverValues(2)); await s.click('#scenario [data-roll-again]', ''); }";
  assert.deepEqual(lintDiceLoops({ '99-x': seeded, '110-new-match-form': '', '86-dice3d': '' }).filter((p) => p.startsWith('99-x')), []);
  const plain = "await s.click('#scenario [data-roll-again]', ''); for (const w of [1, 2]) { await layout(s, w); }";
  assert.deepEqual(lintDiceLoops({ '99-x': plain, '110-new-match-form': '', '86-dice3d': '' }).filter((p) => p.startsWith('99-x')), []);
});

test('an allow-list entry nothing needs is reported', () => {
  assert.ok(lintDiceLoops({ '110-new-match-form': '', '86-dice3d': '' }).some((p) => /nothing needs/.test(p)));
});

test('the shipped flows pass the dice lint', () => {
  assert.deepEqual(lintDiceLoops(), []);
});

test('the seeded template key follows internal/core and go.mod, not internal/web', () => {
  const before = templateKey();
  const probe = join(root, 'internal/core/zz_template_key_probe.go');
  const webProbe = join(root, 'internal/web/zz_template_key_probe.go');
  try {
    writeFileSync(probe, 'package core\n');
    assert.notEqual(templateKey(), before, 'a new file under internal/core changes the key');
    rmSync(probe);
    assert.equal(templateKey(), before, 'and removing it restores it');
    writeFileSync(webProbe, 'package web\n');
    assert.equal(templateKey(), before, 'a file under internal/web does not');
  } finally { rmSync(probe, { force: true }); rmSync(webProbe, { force: true }); }
  assert.ok(existsSync(join(root, 'go.mod')) && existsSync(join(root, 'go.sum')), 'the files the key reads exist');
});
