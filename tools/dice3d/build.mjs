// Bundles src/index.js with three.js and cannon-es into ../../internal/web/static/js/dice3d.js. esbuild keeps only what the
// source imports (tree shaking), so the output holds a fraction of both libraries.
import { build } from 'esbuild';
import { statSync } from 'node:fs';
import { gzipSync } from 'node:zlib';
import { readFileSync } from 'node:fs';

const out = '../../internal/web/static/js/dice3d.js';
await build({ entryPoints: ['src/index.js'], bundle: true, minify: true, format: 'iife', target: 'es2020', outfile: out, legalComments: 'none' });
const size = statSync(out).size, gz = gzipSync(readFileSync(out)).length;
console.log(`${out}: ${(size / 1024).toFixed(0)} KB, ${(gz / 1024).toFixed(0)} KB gzipped`);
