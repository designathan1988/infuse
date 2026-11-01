// data-table — declarative table: columns {key, label, render(row), class, sort(a,b) | true}, rows.
// Client-side sorting on header click; tbody rebuilt with keyed rows. Replaces the three
// copy-pasted <table> strings (journal/vices/map) with one tested component.
import { h, esc } from '../core/dom.js';
import { t } from '../core/i18n.js';

export function dataTable({ columns, rows, empty }) {
  const wrap = h('<div class="data-table-wrap"></div>');
  let sortKey = null, sortDir = 1;
  const draw = () => {
    let list = [...(rows || [])];
    const col = columns.find(c => c.key === sortKey);
    if (col && list.length > 1) {
      list.sort((a, b) => {
        const va = a[col.key], vb = b[col.key];
        const r = typeof va === 'number' && typeof vb === 'number' ? va - vb : String(va ?? '').localeCompare(String(vb ?? ''));
        return r * sortDir;
      });
    }
    if (!list.length) { wrap.innerHTML = `<div class="empty">${esc(t(empty || 'common.empty'))}</div>`; return; }
    wrap.innerHTML = `<table class="data-table">
      <thead><tr>${columns.map(c => `<th data-k="${esc(c.key)}" aria-sort="${sortKey === c.key ? (sortDir > 0 ? 'ascending' : 'descending') : 'none'}">${esc(t(c.label))}${sortKey === c.key ? (sortDir > 0 ? ' ↑' : ' ↓') : ''}</th>`).join('')}</tr></thead>
      <tbody>${list.map((r, i) => `<tr data-i="${i}">${columns.map(c => `<td class="${esc(c.class || '')}">${c.render ? c.render(r) : esc(r[c.key] ?? '')}</td>`).join('')}</tr>`).join('')}</tbody></table>`;
    wrap.querySelectorAll('th[data-k]').forEach(th => th.onclick = () => {
      const k = th.dataset.k;
      if (sortKey === k) sortDir = -sortDir; else { sortKey = k; sortDir = 1; }
      draw();
    });
  };
  draw();
  return { el: wrap, redraw: draw };
}
