// scheduled/list — every task as a clickable row, newest scheduling state first.
import { h, esc } from '../../core/dom.js';
import { t, fmtDateTime } from '../../core/i18n.js';
import { I } from '../../core/icons.js';
import { projectLabel } from '../../core/store.js';
import { presetText } from './preset.js';

export function renderList(el, tasks, open) {
  el.append(
    h(`<div class="row mb-3"><h1 class="grow">${t('nav.scheduled')}</h1><button class="btn line nt">${I.plus} ${t('tasks.new')}</button></div>`),
    h(`<p class="lead">${t('tasks.intro')}</p>`)
  );
  el.querySelector('.nt').onclick = () => open('');
  if (!tasks.length) el.append(h(`<div class="empty">${t('tasks.empty')}</div>`));
  for (const tk of tasks) {
    const r = h(`<div class="task-row" tabindex="0" role="button"><span class="status-dot ${tk.active ? 'busy' : ''} no-animation" aria-hidden="true"></span><div class="grow"><b>${esc(tk.name)}</b><div class="faint trunc">${esc(presetText(tk))} · ${esc(projectLabel(tk.root))}</div></div><span class="faint">${tk.nextRun ? t('tasks.next') + ' ' + fmtDateTime(tk.nextRun) : t(tk.active ? 'tasks.manual' : 'tasks.paused')}</span></div>`);
    r.onclick = () => open(tk.name);
    r.onkeydown = e => { if (e.key === 'Enter') open(tk.name); };
    el.append(r);
  }
}
