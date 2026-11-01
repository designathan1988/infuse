// vices — observed vices (deterministic signature + remedy), as a sortable table.
import { esc, debounce } from '../core/dom.js';
import { fmtDateTime } from '../core/i18n.js';
import { api } from '../core/api.js';
import { dataTable } from '../components/data-table.js';

export function mount(pb, extra, c) {
  const draw = async () => {
    const list = await api('/api/lessons', { root: c.root }).catch(() => []);
    const table = dataTable({
      columns: [
        { key: 'ts', label: 'vices.when', render: r => `<span class="faint nowrap">${fmtDateTime(r.ts)}</span>` },
        { key: 'vice', label: 'vices.name', render: r => `<span class="tag warn">${esc(r.vice)}</span>` },
        { key: 'detail', label: 'vices.detail', render: r => `<span class="sel">${esc(r.detail)}</span>` },
      ],
      rows: list || [],
      empty: 'vices.empty'
    });
    pb.innerHTML = '';
    pb.appendChild(table.el);
  };
  draw();
  return { refresh: debounce(draw, 800) };
}
