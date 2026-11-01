#!/usr/bin/env node
// check-language — fails when Portuguese appears outside the shared catalog. The repository is
// English (code, comments, identifiers, tests, scripts); the one permitted Portuguese is the pt-BR
// value inside internal/core/catalog.json. A line is Portuguese when it carries a Portuguese-specific
// accented letter or a decisive Portuguese word — both are code signals, not a translation guess.
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, extname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const SCANNED = ['internal', 'cmd', 'ui/src', 'ui/index.html', 'scripts', 'tools'];
const EXTS = new Set(['.go', '.js', '.mjs', '.html']);

const ACCENT = /[áàâãéêíóôõúüçÁÀÂÃÉÊÍÓÔÕÚÜÇ]/;
const WORDS = /\b(não|sessão|sessões|arquivo|arquivos|usuário|usuários|pasta|pastas|erro|erros|falha|falhas|falhou|tela|telas|pronto|filtro|filtros|regra|regras|teste|testes|mensagem|mensagens|janela|fechar|abrir|salvar|excluir|remover|adicionar|criar|editar|confirmar|cancelar|configuração|configurações|também|então|isso|isto|aqui|agora|ainda|sempre|nunca|depois|antes|durante|porque|quando|onde|qual|quais|muito|pouco|você|vocês|está|estão|são|há|nós|ele|ela|eles|elas|mas|mesmo|mesma|pode|podem|deve|devem|foi)\b/i;

const files = [];
const SELF = new Set(['scripts/check-language.mjs', 'scripts/check-i18n.mjs']); // the language tooling itself
const walk = p => {
  const abs = join(root, p);
  let st;
  try { st = statSync(abs); } catch { return; }
  if (st.isDirectory()) {
    for (const f of readdirSync(abs)) walk(join(p, f));
    return;
  }
  if (EXTS.has(extname(abs)) && !SELF.has(p.split('\\').join('/'))) files.push(p);
};
for (const p of SCANNED) walk(p);

// Reviewed exceptions: protocol values stored and compared by code, and word lists that detect
// language. Every line carries its reason; nothing else is exempt.
const allow = readFileSync(join(root, 'scripts/language-allowlist.txt'), 'utf8').split('\n')
  .filter(l => l.trim() && !l.trim().startsWith('#'))
  .map(l => l.split('\t')[0]);
let allowed = 0;
let bad = 0;
for (const f of files) {
  const lines = readFileSync(join(root, f), 'utf8').split('\n');
  lines.forEach((line, i) => {
    if (!ACCENT.test(line) && !WORDS.test(line)) return;
    // the language's own name is data, never a translation: it is read from the catalog elsewhere
    if (/languageNames|Português \(Brasil\)/.test(line)) return;
    if (allow.some(a => a && line.includes(a))) { allowed++; return; }
    bad++;
    console.error(`${relative('.', join(root, f))}:${i + 1}: ${line.trim().slice(0, 140)}`);
  });
}
console.log(`check-language: ${files.length} files scanned, ${bad} Portuguese line(s) outside the catalog, ${allowed} reviewed exception(s)`);
if (bad) process.exit(1);
