// journal — every write that passed through the filter (and every blocked attempt), as a sortable table.
import { esc, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { fmtDateTime } from '../core/i18n.js';
import { api } from '../core/api.js';
import { dataTable } from '../components/data-table.js';

export function mount(pb, extra, c) {
  const draw = async () => {
    const list = await api('/api/journal', { root: c.root }).catch(() => []);
    const table = dataTable({
      columns: [
        { key: 'ts', label: 'journal.when', render: r => `<span class="faint nowrap">${fmtDateTime(r.ts)}</span>` },
        { key: 'kind', label: 'journal.kind', render: r => `<span class="tag ${r.kind === 'blocked' ? 'bad' : ''}">${esc(r.kind)}</span>` },
        { key: 'path', label: 'journal.what', render: r => `<span class="mono sel break-all">${esc(r.path || r.command || '')}</span>${r.detail ? ` <span class="faint">${esc(r.detail)}</span>` : ''}` },
      ],
      rows: list || [],
      empty: 'journal.empty'
    });
    pb.innerHTML = '';
    pb.appendChild(table.el);
  };
  draw();
  return { refresh: debounce(draw, 800) };
}
