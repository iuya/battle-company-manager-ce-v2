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

// The dice lint (docs/testing.md, "Dice in the flows"): a loop or retry around a click that throws dice makes the flow depend on what the
// dice happen to show (and multiplies a slow throw). Such a loop must either have a dice source the flow decides (the style guide's "values
// decided beforehand" switch, `serverValues`, or a server dice seam: put its marker, `diceSeed`, in the loop) or be in DICE_LOOP_ALLOWED
// with the reason. A click is a dice click when its selector or label is one of these.
const DICE_CLICK = /data-roll-again|Roll again|Roll the scenario|Roll instead|Roll D3|Roll on the special chart|Roll ·|'=Roll'|rollNewScenario\(|\bthrowDice\b/;
const DICE_SOURCE = /serverValues|diceSeed/;
// "<flow>: <dice click text of the loop>" -> why it is fine as it is.
export const DICE_LOOP_ALLOWED = {
  '110-new-match-form: rollNewScenario(': 'two plain throws, each waited for in full; the scenario shown is read, never compared with a number (no retry, no pair is needed)',
  '86-dice3d: Roll on the special chart': 'finishRecruit ends the recruit whatever the server rolled; each pass takes the branch the page offers and the loop is bounded at 5',
};

// The text of every `for`/`while` statement of `src` (header and body), found by matching brackets; strings and comments are not parsed,
// which is enough for these flow files (a bracket inside a string only makes the body longer, never shorter than a real one in practice).
export function loops(src) {
  const out = [];
  for (const m of src.matchAll(/\b(?:for|while)\s*\(/g)) {
    let i = m.index + m[0].length, depth = 1;
    while (i < src.length && depth) { depth += src[i] === '(' ? 1 : src[i] === ')' ? -1 : 0; i++; }
    while (/\s/.test(src[i] || '')) i++;
    let end = i;
    if (src[i] === '{') { depth = 0; do { depth += src[end] === '{' ? 1 : src[end] === '}' ? -1 : 0; end++; } while (end < src.length && depth); }
    else end = src.indexOf(';', i) + 1 || src.length;
    out.push(src.slice(m.index, end));
  }
  return out;
}

// Returns the problems (empty when every loop with a dice click has a dice source or an allow-list entry). `files` maps a flow name to its source.
export function lintDiceLoops(files = readFlowSources()) {
  const problems = [];
  for (const [flow, src] of Object.entries(files)) {
    for (const loop of loops(src)) {
      const click = loop.match(DICE_CLICK);
      if (!click || DICE_SOURCE.test(loop)) continue;
      const key = `${flow}: ${click[0]}`;
      if (!DICE_LOOP_ALLOWED[key]) problems.push(`${key}: a loop throws dice with no dice source decided by the flow (serverValues, diceSeed); decide the dice, or add the loop to DICE_LOOP_ALLOWED in tests/e2e/lib/groups.mjs with the reason:\n      ${loop.split('\n')[0].trim().slice(0, 140)}`);
    }
  }
  for (const key of Object.keys(DICE_LOOP_ALLOWED)) {
    const flow = key.split(':')[0];
    if (!files[flow] || !loops(files[flow]).some((l) => DICE_CLICK.test(l) && l.includes(key.slice(flow.length + 2)))) problems.push(`DICE_LOOP_ALLOWED has an entry nothing needs any more: ${key}`);
  }
  return problems;
}

function readFlowSources() {
  return Object.fromEntries(flowNames().map((f) => [f, readFileSync(join(e2eDir, 'flows', f + '.mjs'), 'utf8')]));
}

// node tests/e2e/lib/groups.mjs [--names]: checks the grouping and the dice loops (exit 1 on a problem); --names prints the group names as a JSON array (the CI matrix).
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const problems = [...checkGroups(), ...lintDiceLoops()];
  if (problems.length) { console.error(problems.map((p) => 'e2e groups: ' + p).join('\n')); process.exit(1); }
  if (process.argv.includes('--names')) console.log(JSON.stringify(Object.keys(loadGroups())));
  else console.log(`e2e groups: ${flowNames().length} flows in ${Object.keys(loadGroups()).length} groups, each in exactly one`);
}
