// The flow groups of tests/e2e/groups.json: CI runs one job per group (e2e.yml), `make e2e-groups` runs them one after the other.
// Every file in flows/ must be in exactly one group, so a new flow cannot be forgotten.
import { readdirSync, readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const e2eDir = join(dirname(fileURLToPath(import.meta.url)), '..');

export function loadGroups() {
  return JSON.parse(readFileSync(join(e2eDir, 'groups.json'), 'utf8')).groups;
}

export function flowNames() {
  return readdirSync(join(e2eDir, 'flows')).filter((f) => f.endsWith('.mjs')).map((f) => f.replace(/\.mjs$/, '')).sort();
}

// Returns the list of problems (empty when the grouping is complete and disjoint).
export function checkGroups(groups = loadGroups(), flows = flowNames()) {
  const problems = [];
  const seen = new Map();
  for (const [g, list] of Object.entries(groups)) {
    if (!Array.isArray(list) || !list.length) problems.push(`group ${g} is empty`);
    for (const f of list) {
      if (!flows.includes(f)) problems.push(`group ${g}: ${f} is not a file in tests/e2e/flows`);
      if (seen.has(f)) problems.push(`${f} is in two groups: ${seen.get(f)} and ${g}`);
      seen.set(f, g);
    }
  }
  for (const f of flows) if (!seen.has(f)) problems.push(`${f} is in no group: add it to tests/e2e/groups.json`);
  return problems;
}

// node tests/e2e/lib/groups.mjs [--names]: checks the grouping (exit 1 on a problem); --names prints the group names as a JSON array (the CI matrix).
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const problems = checkGroups();
  if (problems.length) { console.error(problems.map((p) => 'e2e groups: ' + p).join('\n')); process.exit(1); }
  if (process.argv.includes('--names')) console.log(JSON.stringify(Object.keys(loadGroups())));
  else console.log(`e2e groups: ${flowNames().length} flows in ${Object.keys(loadGroups()).length} groups, each in exactly one`);
}
