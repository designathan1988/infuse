'use strict';
// core
// Page shell and form primitives (loaded before the pages).

'use strict';
// Full pages that replace the session area: Settings, Scheduled, Customize. Every control applies on change.

function showPage(name) {
  UI.set('page', name); let pg = $('#page');
  if (!name) { if (pg) pg.remove(); $('#views').classList.remove('hidden'); renderPanes(); renderSidebar(); return null; }
  if (!pg) { pg = h('<div id="page" class="page"></div>'); $('#main').prepend(pg); }
  pg.innerHTML = ''; $('#views').classList.add('hidden'); $('#panes').classList.add('empty'); renderSidebar();
  return pg;
}
function pageShell(title, sections, active, onSelect) {
  const pg = showPage(title);
  const nav = h(`<nav class="pnav"><div class="nav u-mb12px-c--text3">← ${t('ui.page.voltar.1505ad')}</div></nav>`);
  nav.firstChild.onclick = () => showPage(null);
  sections.forEach(([id, label]) => { const n = h(`<div class="nav${id === active ? ' on' : ''}">${esc(t(label))}</div>`); n.onclick = () => onSelect(id); nav.appendChild(n); });
  const content = h('<div class="pcontent"><div class="inner"></div></div>');
  pg.append(nav, content); return $('.inner', content);
}
const setRow = (label, desc, control) => { const r = h(`<div class="set"><div class="l"><b>${esc(t(label))}</b>${desc ? `<span>${esc(t(desc))}</span>` : ''}</div></div>`); r.appendChild(control); return r; };
const toggleEl = (on, f) => { const s = h(`<span class="sw${on ? ' on' : ''} u-curpointer"></span>`); s.onclick = () => { s.classList.toggle('on'); f(s.classList.contains('on')); }; return s; };
const selectEl = (opts, val, f) => { const s = h(`<select>${opts.map(([v, l]) => `<option value="${esc(v)}" ${v === val ? 'selected' : ''}>${esc(t(l))}</option>`).join('')}</select>`); s.onchange = () => f(s.value); return s; };
const inputEl = (val, f, type = 'text', ph = '') => { const i = h(`<input type="${type}" value="${esc(val ?? '')}" placeholder="${esc(t(ph))}">`); i.onchange = () => f(type === 'number' ? Number(i.value) : i.value); return i; };
