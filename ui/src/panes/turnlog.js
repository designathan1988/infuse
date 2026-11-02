// turnlog — the session's per-turn log: each model request and tool call, with times and usage.
import { esc, debounce } from '../core/dom.js';
import { t, fmtDateTime } from '../core/i18n.js';
import { api } from '../core/api.js';
import { dataTable } from '../components/data-table.js';

// what a record says, by its kind
const what = r => (r.kind === 'tool'
  ? `<span class="mono sel break-all">${esc(r.tool?.name || '')}</span> <span class="faint">${esc(r.tool?.input || '')}</span>`
  : `<span class="mono">${esc(r.model || '')}</span> <span class="faint">${r.usage?.input ?? 0} + ${r.usage?.output ?? 0}</span>`);

export function mount(pb, extra, c) {
  const sid = c.L?.id || '';
  const draw = async () => {
    if (!sid) { pb.innerHTML = `<div class="empty">${esc(t('turnlog.noSession'))}</div>`; return; }
    const list = await api('/api/turnlog?session=' + encodeURIComponent(sid), { root: c.root }).catch(() => []);
    const table = dataTable({
      columns: [
        { key: 'ts', label: 'turnlog.when', render: r => `<span class="faint nowrap">${fmtDateTime(r.ts)}</span>` },
        { key: 'turn', label: 'turnlog.turn', render: r => String(r.turn) },
        { key: 'kind', label: 'turnlog.kind', render: r => `<span class="tag ${r.tool?.isError ? 'bad' : ''}">${esc(r.kind)}</span>` },
        { key: 'what', label: 'turnlog.what', render: what },
        { key: 'ms', label: 'turnlog.ms', render: r => String(r.ms ?? 0) },
      ],
      rows: (list || []).slice().reverse(),
      empty: 'turnlog.empty'
    });
    pb.innerHTML = '';
    pb.appendChild(table.el);
  };
  draw();
  return { refresh: debounce(draw, 800) };
}
