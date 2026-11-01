#!/usr/bin/env node
// check-i18n — extracts every t('key') from ui/src (+ data-i18n in index.html) and verifies the shared
// server catalog covers them in both languages. There is no browser-side catalog to drift from.
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const catalog = JSON.parse(readFileSync(join(root, 'internal/core/catalog.json'), 'utf8'));
const NAMESPACES = new Set(Object.keys(catalog).map(k => k.split('.')[0]));
const FILE_EXTS = new Set(['js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx', 'json', 'css', 'html', 'md', 'txt', 'yml', 'yaml', 'toml', 'sh', 'exe', 'py', 'go', 'png', 'svg', 'jpg', 'jpeg', 'webp', 'zip', 'gz', 'whl', 'php', 'rb', 'rs', 'java', 'kt', 'cs', 'vue', 'svelte']);
const srcDir = join(root, 'ui', 'src');
const used = new Set();
const files = [];
const walk = d => {
  for (const f of readdirSync(d)) {
    const p = join(d, f);
    if (statSync(p).isDirectory()) walk(p);
    else if (f.endsWith('.js')) files.push(p);
  }
};
walk(srcDir);
files.push(join(root, 'ui', 'index.html'));
for (const f of files) {
  const src = readFileSync(f, 'utf8');
  for (const m of src.matchAll(/\bt\(\s*'([^']+)'/g)) if (!m[1].endsWith('.')) used.add(m[1]);
  for (const m of src.matchAll(/\bt\(\s*"([^"]+)"/g)) if (!m[1].endsWith('.')) used.add(m[1]);
  // setRow(label, desc, …) and toast(msg) route their first args through t() inside the helpers
  for (const m of src.matchAll(/\bsetRow\(\s*'([^']+)'\s*,\s*'([^']+)'/g)) { used.add(m[1]); used.add(m[2]); }
  for (const m of src.matchAll(/\bsetRow\(\s*'([^']+)'\s*,\s*[^']/g)) used.add(m[1]);
  for (const m of src.matchAll(/\btoast\(\s*'([^']+)'/g)) used.add(m[1]);
  for (const m of src.matchAll(/data-i18n(?:-ph|-title|-aria)?="([^"]+)"/g)) used.add(m[1]);
  // page section tables pass their labels through t() at render time
  for (const m of src.matchAll(/\['[^']+',\s*'((?:settings|cust)\.[^']+)'\]/g)) used.add(m[1]);
  // keys reached through a variable (t(a) with a from a table) are still catalog ids: any dotted
  // literal whose first segment is a namespace the catalog already uses must exist.
  for (const m of src.matchAll(/'([a-z][a-zA-Z0-9]*\.[a-zA-Z][\w.]*)'/g)) {
    const key = m[1];
    if (catalog[key] || key.endsWith('.') || key.includes('/') || key.includes('://')) continue;
    const ext = key.split('.').pop().toLowerCase();
    if (FILE_EXTS.has(ext)) continue;
    if (!NAMESPACES.has(key.split('.')[0])) continue;
    used.add(key);
  }
}
// dynamic key families (composed at runtime) — always required:
for (const k of [
  'mode.ask', 'mode.acceptEdits', 'mode.plan', 'mode.bypass',
  'effort.low', 'effort.medium', 'effort.high', 'effort.max',
  'verdict.aprovado', 'verdict.reprovado', 'verdict.erro', 'verdict.parada', 'verdict.impasse', 'verdict.orcamento',
  'agent.kind.general', 'agent.kind.explore', 'agent.kind.review',
  'ban.kind.tool', 'ban.kind.command', 'ban.kind.path', 'ban.kind.action',
  'plan.noteStoppedTask', 'tasks.weekday.0', 'tasks.weekday.1', 'tasks.weekday.2', 'tasks.weekday.3', 'tasks.weekday.4', 'tasks.weekday.5', 'tasks.weekday.6',
  'System instructions', 'Tools', 'Your messages', 'Responses', 'Tool calls', 'Tool results', 'Images',
]) used.add(k);

let bad = 0;
const missingPt = [...used].filter(k => !catalog[k]?.['pt-BR']);
const missingEn = [...used].filter(k => !catalog[k]?.en);
if (missingPt.length) { bad++; console.error('MISSING in pt-BR:', missingPt.join(', ')); }
if (missingEn.length) { bad++; console.error('MISSING in en:', missingEn.join(', ')); }
console.log(`check-i18n: ${used.size} keys used, ${Object.keys(catalog).length} in the shared catalog`);
if (bad) process.exit(1);
