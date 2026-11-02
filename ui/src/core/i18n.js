// i18n — semantic keys + ICU MessageFormat subset ({name} interpolation, plural, select),
// categories chosen by Intl.PluralRules (real CLDR), Intl date/number formatters per locale.
// Messages live in the shared Go catalog (internal/core/catalog.json) and arrive from /api/i18n;
// there is no second translation table in the browser. Coverage is enforced by scripts/check-i18n.mjs.
import { api } from './api.js';

let catalog = {};
let table = {};
let loaded = false;
let locale = 'pt-BR';

export async function loadCatalog() {
  const data = await api('/api/i18n');
  if (!data || typeof data !== 'object') throw new Error('invalid message catalog');
  catalog = data;
  loaded = true;
  table = tableFor(locale);
  applyStatic();
}

export const t = (key, params) => format(table[key] ?? key, params);
export const lang = () => (locale === 'en' ? 'en' : 'pt');

export function setLocale(l) {
  locale = l?.startsWith('en') ? 'en' : 'pt-BR';
  if (loaded) table = tableFor(locale);
  document.documentElement.lang = locale;
  applyStatic();
}

function tableFor(l) {
  const out = {};
  for (const [key, langs] of Object.entries(catalog)) out[key] = langs[l] ?? langs['pt-BR'] ?? langs.en ?? key;
  return out;
}

function applyStatic() {
  if (!loaded) return;
  for (const el of document.querySelectorAll('[data-i18n]')) el.textContent = t(el.dataset.i18n);
  for (const el of document.querySelectorAll('[data-i18n-ph]')) el.placeholder = t(el.dataset.i18nPh);
  for (const el of document.querySelectorAll('[data-i18n-title]')) el.title = t(el.dataset.i18nTitle);
  for (const el of document.querySelectorAll('[data-i18n-aria]')) el.setAttribute('aria-label', t(el.dataset.i18nAria));
}

function format(msg, params) {
  if (params == null) return msg;
  return msg.replace(/\{(\w+)(?:,\s*(\w+)\s*,\s*((?:[^{}]|\{[^{}]*\})*))?\}/g, (m, name, kind, rest) => {
    const v = params[name];
    if (!kind) return String(v ?? '');
    const clauses = splitClauses(rest);
    if (kind === 'plural') {
      const n = Number(v);
      const cat = new Intl.PluralRules(locale).select(n);
      const chosen = clauses[`=${n}`] ?? clauses[cat] ?? clauses.other;
      return render(chosen ?? '', params).replace(/#/g, fmtNum(n));
    }
    if (kind === 'select') {
      const chosen = clauses[v] ?? clauses.other;
      return render(chosen ?? '', params);
    }
    if (kind === 'number') return fmtNum(Number(v));
    if (kind === 'date') return new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(v));
    return String(v ?? '');
  });
}

function splitClauses(rest) {
  const out = {};
  for (const m of (rest || '').matchAll(/([=\w-]+)\s*\{((?:[^{}]|\{[^{}]*\})*)\}/g)) out[m[1]] = m[2];
  return out;
}
const render = (s, params) => s.replace(/\{(\w+)\}/g, (m, k) => String(params[k] ?? ''));

export const fmtNum = n => new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(n);
export const fmtMoney = usd => new Intl.NumberFormat(locale, { style: 'currency', currency: 'USD' }).format(usd || 0);
// A turn can cost a fraction of a cent: below one cent the two usual decimals would round it to nothing.
export const fmtMoneyFine = usd => { const digits = usd > 0 && usd < 0.01 ? 4 : 2; return new Intl.NumberFormat(locale, { style: 'currency', currency: 'USD', minimumFractionDigits: digits, maximumFractionDigits: digits }).format(usd || 0); };
export const fmtDateTime = ts => new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(ts));
export const fmtTime = ts => new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit' }).format(new Date(ts));
export const fmtDate = ts => new Intl.DateTimeFormat(locale, { dateStyle: 'short' }).format(new Date(ts));
