'use strict';
// Interface language: one catalog served by the API (the same entries Go uses). Messages are stable
// ids with named arguments; the Portuguese source text is only a fallback for labels that come from
// data (a menu label, a task note), so adding a language never touches the rendering code.

let BUNDLE = {};
let PT_INDEX = {};
let LANG = 'pt-BR';

function langKey(lang) {
  return String(lang || '').toLowerCase().startsWith('en') ? 'en' : 'pt-BR';
}

async function loadMessages(lang) {
  LANG = langKey(lang);
  try {
    const r = await fetch('/api/i18n', { cache: 'no-cache' });
    BUNDLE = await r.json();
  } catch { BUNDLE = {}; }
  PT_INDEX = {};
  for (const [id, langs] of Object.entries(BUNDLE)) {
    if (!id.startsWith('ui.')) continue;
    const pt = langs['pt-BR'];
    if (pt && PT_INDEX[pt.trim()] === undefined) PT_INDEX[pt.trim()] = id;
  }
}

function t(id, args) {
  if (typeof id !== 'string') return id;
  let entry = BUNDLE[id];
  if (!entry) {
    const mapped = PT_INDEX[id.trim()];
    if (mapped) entry = BUNDLE[mapped];
  }
  if (!entry) return id;
  let s = entry[LANG] ?? entry['en'] ?? id;
  if (args) s = s.replace(/\{([A-Za-z_][A-Za-z0-9_]*)\}/g, (m, k) => args[k] !== undefined ? String(args[k]) : m);
  return s;
}

function applyStaticTranslations(root = document) {
  root.querySelectorAll('[data-i18n]').forEach(el => { const v = t(el.dataset.i18n); if (v !== el.textContent) el.textContent = v; });
  root.querySelectorAll('[data-i18n-title]').forEach(el => el.setAttribute('title', t(el.dataset.i18nTitle)));
  root.querySelectorAll('[data-i18n-placeholder]').forEach(el => el.setAttribute('placeholder', t(el.dataset.i18nPlaceholder)));
}

function startTranslation() {
  document.documentElement.lang = LANG === 'en' ? 'en' : 'pt-BR';
  applyStaticTranslations();
}

// Switching language re-renders the interface in place: no reload, no lost connection.
async function setLanguage(lang) {
  await loadMessages(lang);
  startTranslation();
  if (typeof remountUI === 'function') await remountUI();
}
