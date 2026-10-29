// Checks that every relative link in the Markdown files of the repository resolves: the file or folder exists and, for a link
// with a #fragment into a Markdown file, the heading exists (GitHub's anchor rules). External links (http, https, mailto) are not
// fetched. Run: node tests/ci/links.mjs [file.md ...]   (make docs-check; part of make test)
import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs';
import { dirname, join, resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const skipDirs = new Set(['.git', 'node_modules', 'sources', 'tmp', 'backups', '.private', '.beads', 'out']);

function walk(dir, found) {
  for (const name of readdirSync(dir)) {
    if (skipDirs.has(name)) continue;
    const path = join(dir, name);
    const st = statSync(path);
    if (st.isDirectory()) walk(path, found);
    else if (name.endsWith('.md')) found.push(path);
  }
  return found;
}

// Lines of a Markdown file outside fenced code blocks, with inline code spans blanked.
function proseLines(text) {
  let fence = null;
  return text.split('\n').map((line) => {
    const m = line.match(/^\s*(```+|~~~+)/);
    if (m) {
      if (!fence) fence = m[1][0];
      else if (m[1][0] === fence) fence = null;
      return '';
    }
    return fence ? '' : line.replace(/`[^`]*`/g, (c) => ' '.repeat(c.length));
  });
}

// GitHub's heading anchors: lower case, punctuation removed, spaces to hyphens, repeats numbered.
function anchors(text) {
  const seen = new Map();
  const out = new Set();
  for (const line of proseLines(text)) {
    const m = line.match(/^#{1,6}\s+(.*?)\s*#*\s*$/);
    if (!m) continue;
    const plain = m[1].replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/[`*_]/g, '');
    let slug = plain.toLowerCase().replace(/[^\p{L}\p{N}\s-]/gu, '').trim().replace(/\s/g, '-');
    const n = seen.get(slug) || 0;
    seen.set(slug, n + 1);
    out.add(n ? `${slug}-${n}` : slug);
  }
  return out;
}

const cache = new Map();
const anchorsOf = (file) => {
  if (!cache.has(file)) cache.set(file, anchors(readFileSync(file, 'utf8')));
  return cache.get(file);
};

const files = process.argv.length > 2 ? process.argv.slice(2).map((f) => resolve(f)) : walk(root, []);
const problems = [];
let checked = 0;

for (const file of files) {
  const lines = proseLines(readFileSync(file, 'utf8'));
  lines.forEach((line, i) => {
    for (const m of line.matchAll(/\[(?:[^\]\\]|\\.)*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)/g)) {
      const target = m[1];
      if (/^[a-z][a-z0-9+.-]*:/i.test(target) || target.startsWith('//')) continue;
      checked++;
      const [pathPart, fragment] = target.split('#');
      const dest = pathPart === '' ? file : resolve(dirname(file), decodeURIComponent(pathPart));
      const where = `${relative(root, file)}:${i + 1}`;
      if (!existsSync(dest)) {
        problems.push(`${where}: ${target} does not exist`);
        continue;
      }
      if (fragment && dest.endsWith('.md') && statSync(dest).isFile() && !anchorsOf(dest).has(decodeURIComponent(fragment).toLowerCase())) {
        problems.push(`${where}: ${target}: no heading "${fragment}" in ${relative(root, dest)}`);
      }
    }
  });
}

if (problems.length) {
  console.error(problems.join('\n'));
  console.error(`\n${problems.length} broken link(s) of ${checked} checked in ${files.length} Markdown files`);
  process.exit(1);
}
console.log(`links ok: ${checked} relative links in ${files.length} Markdown files`);
