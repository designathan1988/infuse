// map — the project map: hotspots and per-directory files/lines, regenerable.
import { $, $$, esc, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { fmtDateTime } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { api } from '../core/api.js';
import { dataTable } from '../components/data-table.js';

export function mount(pb, extra, c) {
  extra.innerHTML = `<button class="icon-btn" title="${esc(t('map.regenerate'))}" aria-label="${esc(t('map.regenerate'))}">${I.reload}</button>`;
  const draw = async refresh => {
    const m = await api('/api/map' + (refresh ? '?refresh=1' : ''), { root: c.dir }).catch(() => ({}));
    const dirs = Object.entries(m.dirs || {}).sort();
    const table = dataTable({
      columns: [
        { key: '0', label: 'map.dir', render: r => `<span class="mono">${esc(r[0])}</span>` },
        { key: 'files', label: 'map.files', render: r => String((r[1].files || []).length) },
        { key: 'lines', label: 'map.lines', render: r => String(r[1].lines || 0) },
      ],
      rows: dirs,
      empty: 'map.empty'
    });
    const features = m.features || [];
    const featurePath = p => /\.[^/]+$/.test(p)
      ? `<span class="mono path-link" data-path="${esc(p)}" tabindex="0" role="button">${esc(p)}</span>`
      : `<span class="mono">${esc(p)}</span>`;
    const featureCard = features.length ? `<div class="rail-card mb-5"><b>${t('map.features')}</b>${features.slice(0, 30).map(f => `<div class="mb-2">
        <div><span class="mono">${esc(f.name)}</span> <span class="faint">· ${t('map.fileCount', { count: f.files })} · ${t('map.lineCount', { count: f.lines })}</span></div>
        <div class="faint">${(f.paths || []).map(featurePath).join(' · ')}${f.pathCount > (f.paths || []).length ? ` · +${f.pathCount - f.paths.length}` : ''}</div>
        ${(f.routes || []).length ? `<div class="mono">${f.routes.map(r => esc(r)).join(' · ')}</div>` : ''}
      </div>`).join('')}</div>` : '';
    const routes = m.routes || [];
    const routeCard = routes.length ? `<div class="rail-card mb-5"><b>${t('map.routes')}</b>${routes.slice(0, 40).map(r => `<div class="mono path-link" data-path="${esc(r.file)}" tabindex="0" role="button"><span class="faint">${esc(r.method)}</span> ${esc(r.path)} — ${esc(r.file)}</div>`).join('')}</div>` : '';
    pb.innerHTML = `<div class="pane-list"><div class="faint mb-4">${t('map.fileCount', { count: Object.keys(m.files || {}).length })} · ${fmtDateTime(m.generatedAt || Date.now())}</div>
      ${featureCard}${routeCard}
      ${(m.hotspots || []).length ? `<div class="rail-card mb-5"><b>${t('map.hotspots')}</b>${m.hotspots.map(x => `<div class="mono path-link" data-path="${esc(x.path)}" tabindex="0" role="button">${esc(x.path)} — ${t('map.lineCount', { count: x.lines })}</div>`).join('')}</div>` : ''}</div>`;
    pb.querySelector('.pane-list').appendChild(table.el);
    $$('[data-path]', pb).forEach(x => x.onclick = () => import('./index.js').then(p => p.openFileInPane(x.dataset.path)));
  };
  extra.querySelector('button').onclick = () => draw(true);
  draw();
  return { refresh: debounce(() => draw(), 800) };
}
