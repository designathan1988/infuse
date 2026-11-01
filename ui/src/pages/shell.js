// pages/shell — full pages replace the session area (Settings, Scheduled, Customize). Shared frame:
// side nav + content, plus the settings-row controls everything else is built from.
import { $, h, esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { ui } from '../core/store.js';

export function showPage(name, arg) {
  ui.page.set(name);
  ui.pageArg.set(arg || null);
  let pg = $('#page');
  if (!name) {
    if (pg) pg.remove();
    $('#views').classList.remove('hidden');
    import('../panes/index.js').then(p => { p.render(); });
    return null;
  }
  if (!pg) {
    pg = h('<div id="page" class="page"></div>');
    $('#main').prepend(pg);
  }
  pg.innerHTML = '';
  $('#views').classList.add('hidden');
  $('#panes').classList.add('empty');
  return pg;
}

export function pageShell(title, sections, active, onSelect) {
  const pg = showPage(title);
  const nav = h(`<nav class="page-nav" aria-label="${esc(title)}"><div class="side-nav shell-note" tabindex="0" role="button">← ${t('common.back')}</div></nav>`);
  nav.firstChild.onclick = () => showPage(null);
  nav.firstChild.onkeydown = e => { if (e.key === 'Enter') showPage(null); };
  for (const [id, label] of sections) {
    const n = h(`<div class="side-nav${id === active ? ' on' : ''}" tabindex="0" role="button">${esc(t(label))}</div>`);
    n.onclick = () => onSelect(id);
    nav.appendChild(n);
  }
  const content = h('<div class="page-content"><div class="inner"></div></div>');
  pg.append(nav, content);
  return content.querySelector('.inner');
}

export const setRow = (label, desc, control) => {
  const r = h(`<div class="settings-row"><div class="l"><b>${esc(t(label))}</b>${desc ? `<span>${esc(t(desc))}</span>` : ''}</div></div>`);
  r.appendChild(control);
  return r;
};
export const toggleEl = (on, f) => {
  const s = h(`<span class="switch${on ? ' on' : ''} clickable" role="switch" tabindex="0" aria-checked="${!!on}"></span>`);
  const flip = () => { s.classList.toggle('on'); s.setAttribute('aria-checked', s.classList.contains('on')); f(s.classList.contains('on')); };
  s.onclick = flip;
  s.onkeydown = e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); flip(); } };
  return s;
};
export const selectEl = (opts, val, f) => {
  const s = h(`<select>${opts.map(([v, l]) => `<option value="${esc(v)}" ${v === val ? 'selected' : ''}>${esc(t(l) || l)}</option>`).join('')}</select>`);
  s.onchange = () => f(s.value);
  return s;
};
export const inputEl = (val, f, type = 'text', ph = '') => {
  const i = h(`<input type="${type}" value="${esc(val ?? '')}" placeholder="${esc(t(ph) || ph)}">`);
  i.onchange = () => f(type === 'number' ? Number(i.value) : i.value);
  return i;
};
