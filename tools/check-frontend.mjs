#!/usr/bin/env node
// Frontend architecture check, run in CI: every script stays under 400 lines and every function body
// under 60; no static inline style and no colour literal outside the token file.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const errors = [];

function walk(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    if (name === 'vendor' || name === 'node_modules') continue;
    const p = join(dir, name);
    if (statSync(p).isDirectory()) out.push(...walk(p));
    else if (p.endsWith('.js') || p.endsWith('.html') || p.endsWith('.css')) out.push(p);
  }
  return out;
}

// Strings and comments become spaces, so braces inside text do not count.
function mask(src) {
  const out = src.split('');
  let i = 0, state = null;
  const blank = (from, to) => {
    for (let k = from; k < to && k < out.length; k++) if (out[k] !== '\n') out[k] = ' ';
  };
  while (i < src.length) {
    const c = src[i];
    if (!state) {
      if (c === '/' && src[i + 1] === '/') {
        const end = src.indexOf('\n', i);
        const stop = end < 0 ? src.length : end;
        blank(i, stop);
        i = stop;
        continue;
      }
      if (c === '/' && src[i + 1] === '*') {
        const end = src.indexOf('*/', i + 2);
        const stop = end < 0 ? src.length : end + 2;
        blank(i, stop);
        i = stop;
        continue;
      }
      if (c === "'" || c === '"' || c === '`') { state = c; blank(i, i + 1); i++; continue; }
      i++;
      continue;
    }
    if (c === '\\') { blank(i, i + 2); i += 2; continue; }
    if (c === state) { blank(i, i + 1); state = null; i++; continue; }
    blank(i, i + 1);
    i++;
  }
  return out.join('');
}

function lineOf(src, pos) { return src.slice(0, pos).split('\n').length; }

function functions(src) {
  const masked = mask(src);
  const patterns = [
    /\b(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(/g,
    /\b(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>\s*\{/g,
  ];
  const out = [];
  for (const re of patterns) {
    let m;
    while ((m = re.exec(masked)) !== null) {
      const open = masked.indexOf('{', m.index + m[0].length - 1);
      if (open < 0) continue;
      let depth = 0, end = -1;
      for (let i = open; i < masked.length; i++) {
        if (masked[i] === '{') depth++;
        else if (masked[i] === '}' && --depth === 0) { end = i; break; }
      }
      if (end > open) out.push({ name: m[1], line: lineOf(src, m.index), lines: masked.slice(open, end).split('\n').length - 1 });
    }
  }
  return out;
}

for (const file of walk(join(root, 'ui'))) {
  const text = readFileSync(file, 'utf8');
  const rel = relative(root, file).replaceAll('\\', '/');
  if (file.endsWith('.js')) {
    const fileLines = text.split('\n').length - (text.endsWith('\n') ? 1 : 0);
    if (fileLines > 400) errors.push(`${rel}: ${fileLines} lines (limit 400)`);
    for (const f of functions(text)) {
      if (f.lines > 60) errors.push(`${rel}:${f.line} ${f.name} has ${f.lines} lines (limit 60)`);
    }
  }
  for (const m of text.matchAll(/style="([^"]*)"/g)) {
    const decl = m[1].trim();
    if (decl.includes('${')) continue; // dynamic values are custom properties by convention
    for (const part of decl.split(';')) {
      if (part.trim() && !/^\s*--[\w-]+\s*:/.test(part)) {
        errors.push(`${rel}: static inline style "${decl}"`);
        break;
      }
    }
  }
  if (!file.endsWith('tokens.css') && (file.endsWith('.css') || file.endsWith('.js') || file.endsWith('.html'))) {
    const colour = text.match(/#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/);
    if (colour) errors.push(`${rel}: colour literal ${colour[0]} outside tokens.css`);
  }
}

if (errors.length) {
  for (const e of errors) console.error(e);
  process.exit(1);
}
console.log('frontend architecture: ok');
