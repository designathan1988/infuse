#!/usr/bin/env node
// check-ui — parses every ui/src module (ESM) so a syntax slip fails the build instead of the app.
// Usage: node --experimental-vm-modules scripts/check-ui.mjs
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const srcDir = join(root, 'ui', 'src');
let bad = 0, count = 0;
const walk = d => {
  for (const f of readdirSync(d)) {
    const p = join(d, f);
    if (statSync(p).isDirectory()) walk(p);
    else if (f.endsWith('.js')) {
      count++;
      try { new vm.SourceTextModule(readFileSync(p, 'utf8'), { identifier: relative(root, p) }); }
      catch (e) { bad++; console.error('SYNTAX', relative(root, p), '→', e.message.slice(0, 160)); }
    }
  }
};
walk(srcDir);
if (bad) { console.error(`${bad} of ${count} modules failed to parse`); process.exit(1); }
console.log(`check-ui: ${count} modules OK`);
