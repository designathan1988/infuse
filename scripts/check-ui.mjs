#!/usr/bin/env node
// check-ui — parses every ui/src module (ESM) so a syntax slip fails the build instead of the app.
// Usage: node --experimental-vm-modules scripts/check-ui.mjs
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
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

// The DOM-free modules are executed against the payload shapes the server really sends.
const load = rel => import(pathToFileURL(join(srcDir, rel)).href);
const same = (got, want, what) => {
  if (JSON.stringify(got) !== JSON.stringify(want)) { bad++; console.error('LOGIC', what, '→', JSON.stringify(got)); }
};
const { writePolicyValues } = await load('pages/writePolicy.js');
const policy = writePolicyValues({ settings: { requireReview: false, maxFileLines: 250, protectedPaths: ['a/**', 'b/**'] }, version: '1' });
same([policy.toggles.requireReview, policy.toggles.diagnostics], [false, true], 'a switch reads the nested settings; an unset one is on');
same([policy.maxFileLines, policy.protectedPaths], [250, 'a/**, b/**'], 'limit and protected paths read the nested settings');
same(writePolicyValues({ requireReview: false }), null, 'a payload without settings is a failed read, not defaults');

const { fileRows, mostUsed, fileOverlay } = await load('core/mapdata.js');
const map = {
  files: {
    'a.js': { lines: 10, exports: ['x'], dependents: ['b.js', 'c.js'] },
    'b.js': { lines: 900, exports: null, dependents: [] },
    'c.js': { lines: 5, exports: ['y', 'z'], dependents: ['b.js'] },
  },
  hotspots: [{ path: 'b.js', lines: 900 }],
};
same(fileRows(map).map(r => [r.path, r.nExports, r.nDependents]), [['a.js', 1, 2], ['c.js', 2, 1], ['b.js', 0, 0]], 'rows, most depended-on first');
same(mostUsed(map, 1).map(r => r.path), ['a.js'], 'most used');
same(fileOverlay(map), { 'a.js': { dependents: 2, lines: 10 }, 'c.js': { dependents: 1, lines: 5 }, 'b.js': { dependents: 0, lines: 900, hotspot: true } }, 'overlay');
same(fileOverlay({}), {}, 'an empty map overlays nothing');

const { isLowerBound, isUnverified, turnCostOf, turnNumberAt, spendPatch } = await load('core/spend.js');
const spent = { turn: 2, turnStarts: [0, 4], turnCosts: [0.5, 0.25], unpriced: ['local-x'], outcome: 'unverified' };
same([isLowerBound(spent), isLowerBound({ unpriced: null }), isUnverified(spent), isUnverified({ outcome: '' })], [true, false, true, false], 'unpriced and outcome read the snapshot');
same([turnCostOf(spent, 2), turnCostOf(spent, 3), turnCostOf({}, 1)], [0.25, 0, 0], 'a turn cost is found by turn number');
same(turnNumberAt([0, 0, 3], 0), 2, 'after a compaction the last turn at an index owns it');
same(spendPatch(spent, { turnCost: 0.75, unpriced: ['local-x'] }).turnCosts, [0.5, 0.75], 'a message updates the running turn');
same(spendPatch({ turn: 1 }, {}).turnCosts, [], 'an event without accounts changes nothing');

const { localValues, localPayload, ENV_MASK } = await load('pages/customize/localValues.js');
const layers = [{ name: 'user', settings: {} }, { name: 'local', exists: true, settings: { model: 'm', outputStyle: 'concise', protectedPaths: ['a', 'b'], permissions: { Bash: 'deny' }, env: { B: ENV_MASK, A: ENV_MASK } } }];
const form = localValues(layers);
same([form.env.map(r => r.name), form.protectedPaths, form.env[0].value], [['A', 'B'], 'a, b', ''], 'the form starts from the local layer and holds no value');
form.env[1].value = 'new'; form.env.push({ name: ' C ', value: 'c', stored: false }, { name: '', value: 'x', stored: false });
form.permissions.Write = '';
const saved = localPayload(form);
same(saved.env, { A: ENV_MASK, B: 'new', C: 'c' }, 'untouched values go back as the mask, a retyped one as itself');
same([saved.outputStyle, saved.permissions, saved.protectedPaths], ['concise', { Bash: 'deny' }, ['a', 'b']], 'a save keeps the rest of the file; inherit drops the key');

if (bad) { console.error(`check-ui: ${bad} failure(s)`); process.exit(1); }
console.log(`check-ui: ${count} modules OK`);
