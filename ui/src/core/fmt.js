// fmt — display formatting on top of Intl (locale-aware), plus compact token counts and durations.
import { t, fmtMoney, fmtTime, fmtDate, lang } from './i18n.js';

export const money = v => fmtMoney(v || 0);

export function ktok(n) {
  n = n || 0;
  if (n >= 1e6) return new Intl.NumberFormat(lang() === 'en' ? 'en' : 'pt-BR', { maximumFractionDigits: 2 }).format(n / 1e6) + 'M';
  if (n >= 1000) return (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k';
  return String(n);
}
export const fmtTok = n => n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k' : String(n || 0);

export function fmtDur(ms) {
  if (ms == null) return '';
  if (ms < 1000) return ms + ' ms';
  const s = ms / 1000;
  if (s < 60) return (s < 10 ? s.toFixed(1) : Math.round(s)) + ' s';
  const m = Math.floor(s / 60);
  return `${m}m ${Math.round(s % 60)}s`;
}

export function elapsed(from, to = Date.now()) {
  const s = Math.max(0, Math.round(((to || Date.now()) - from) / 1000));
  if (s < 60) return s + 's';
  if (s < 3600) return Math.floor(s / 60) + 'min ' + (s % 60) + 's';
  return Math.floor(s / 3600) + 'h ' + Math.floor((s % 3600) / 60) + 'min';
}

// "hoje, 14:32" / "ontem, 09:10" / "31 out, 18:02" — words come from the locale table, dates from Intl.
export function fmtWhen(ts) {
  if (!ts) return '';
  const d = new Date(ts), now = new Date(), day = 86400000;
  const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const hm = fmtTime(ts);
  if (ts >= midnight) return t('time.today') + ', ' + hm;
  if (ts >= midnight - day) return t('time.yesterday') + ', ' + hm;
  return fmtDate(ts) + ', ' + hm;
}

export function ago(ts) {
  if (!ts) return '';
  const s = (Date.now() - ts) / 1000;
  if (s < 60) return t('time.now');
  if (s < 3600) return t('time.minAgo', { n: Math.floor(s / 60) });
  if (s < 86400) return t('time.hourAgo', { n: Math.floor(s / 3600) });
  return t('time.dayAgo', { n: Math.round(s / 86400) });
}
