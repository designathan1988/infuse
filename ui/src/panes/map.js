// map — the project map: what each file exports, who depends on it, the hotspots, and per-directory
// files/lines. Regenerable; it follows the agent's writes through the pane refresh.
import { esc, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { fmtDateTime } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { api } from '../core/api.js';
import { fileRows, mostUsed } from '../core/mapdata.js';
import { dataTable } from '../components/data-table.js';

const SHOWN_EXPORTS = 4;
const pathLink = (path, tail = '') => `<div class="mono path-link trunc" data-path="${esc(path)}" tabindex="0" role="button" title="${esc(path)}">${esc(path)}${tail}</div>`;

function exportsCell(r) {
  const more = r.nExports - SHOWN_EXPORTS;
  return `<span class="mono map-names" title="${esc(r.exports.join(', '))}">${esc(r.exports.slice(0, SHOWN_EXPORTS).join(', '))}${more > 0 ? ` +${more}` : ''}</span>`;
}

const filesTable = m => dataTable({
  columns: [
    { key: 'path', label: 'map.file', render: r => pathLink(r.path) },
    { key: 'nExports', label: 'map.exports', render: exportsCell },
    { key: 'nDependents', label: 'map.usedBy', render: r => `<span title="${esc(r.dependents.join('\n'))}">${r.nDependents}</span>` },
    { key: 'lines', label: 'map.lines', render: r => String(r.lines) },
  ],
  rows: fileRows(m),
  empty: 'map.empty'
});

const dirsTable = m => dataTable({
  columns: [
    { key: '0', label: 'map.dir', render: r => `<span class="mono">${esc(r[0])}</span>` },
    { key: 'files', label: 'map.files', render: r => String((r[1].files || []).length) },
    { key: 'lines', label: 'map.lines', render: r => String(r[1].lines || 0) },
  ],
  rows: Object.entries(m.dirs || {}).sort(),
  empty: 'map.empty'
});

function summaryHTML(m) {
  const hot = (m.hotspots || []).map(x => pathLink(x.path, ` — ${x.lines} ${t('common.lines')}`)).join('');
  const used = mostUsed(m).map(r => pathLink(r.path, ` — ${r.nDependents}`)).join('');
  return `${hot ? `<div class="rail-card mb-5"><b>${t('map.hotspots')}</b>${hot}</div>` : ''}
    ${used ? `<div class="rail-card mb-5"><b>${t('map.mostUsed')}</b>${used}</div>` : ''}`;
}

export function mount(pb, extra, c) {
  extra.innerHTML = `<button class="icon-btn" title="${esc(t('map.regenerate'))}" aria-label="${esc(t('map.regenerate'))}">${I.reload}</button>`;
  let view = 'files', m = {};
  const render = () => {
    const top = pb.scrollTop;
    pb.innerHTML = `<div class="pane-list"><div class="faint mb-4">${Object.keys(m.files || {}).length} ${t('common.files')} · ${fmtDateTime(m.generatedAt || Date.now())}</div>
      ${summaryHTML(m)}
      <div class="seg-group mb-4" role="group">${[['files', 'map.viewFiles'], ['dirs', 'map.viewDirs']].map(([k, l]) => `<button class="${k === view ? 'on' : ''}" data-view="${k}">${t(l)}</button>`).join('')}</div></div>`;
    pb.querySelector('.pane-list').appendChild((view === 'files' ? filesTable(m) : dirsTable(m)).el);
    pb.scrollTop = top;
  };
  const draw = async refresh => {
    m = await api('/api/map', { root: c.dir, ...(refresh ? { body: {} } : {}) }).catch(() => ({}));
    render();
  };
  pb.onclick = e => {
    const v = e.target.closest('[data-view]');
    if (v) { view = v.dataset.view; render(); return; }
    const p = e.target.closest('[data-path]');
    if (p) import('./index.js').then(x => x.openFileInPane(p.dataset.path));
  };
  extra.querySelector('button').onclick = () => draw(true);
  draw();
  return { refresh: debounce(() => draw(), 800) };
}
