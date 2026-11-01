// scheduled — the page shell: list, or the editor for one named task.
import { h } from '../core/dom.js';
import { api } from '../core/api.js';
import { showPage } from './shell.js';
import { renderList } from './scheduled/list.js';
import { renderEditor } from './scheduled/editor.js';

export async function openScheduled(name) {
  const pg = showPage('scheduled');
  const content = h('<div class="page-content sched-page"><div class="inner"></div></div>');
  pg.appendChild(content);
  const el = content.querySelector('.inner');
  const tasks = await api('/api/tasks', { root: '' }).catch(() => []);
  if (name === undefined || name === null) return renderList(el, tasks, openScheduled);
  await renderEditor(el, tasks, name, openScheduled);
}
