// scheduled/editor — name, prompt, schedule chips, actions and run history for one task.
import { h, esc } from '../../core/dom.js';
import { t, fmtDateTime } from '../../core/i18n.js';
import { api } from '../../core/api.js';
import { cache, curRoot, reloadSessions, openSession } from '../../core/store.js';
import { toast } from '../../components/toast.js';
import { drawChips } from './chips.js';

export async function renderEditor(el, tasks, name, open) {
  const tk = tasks.find(x => x.name === name) || {
    name: '', prompt: '', root: curRoot() || cache.projects.get()[0]?.root || '',
    preset: 'daily', hour: 9, minute: 0, weekday: 1, active: true, history: [], alwaysAllowed: []
  };
  const prev = tk.name;
  el.append(h(`<div class="side-nav sched-back" tabindex="0" role="button">← ${t('nav.scheduled')}</div>`));
  el.lastChild.onclick = () => open();
  const doc = h(`<div class="doc-editor-box"><input class="name" placeholder="${esc(t('tasks.namePlaceholder'))}" value="${esc(tk.name)}" aria-label="${esc(t('tasks.namePlaceholder'))}"><textarea placeholder="${esc(t('tasks.promptPlaceholder'))}" aria-label="${esc(t('tasks.promptPlaceholder'))}">${esc(tk.prompt)}</textarea><div class="chips sched-chips"></div></div>`);
  el.append(doc);
  drawChips(doc.querySelector('.chips'), tk);
  const actions = h(`<div class="row sched-actions"><button class="btn pri sv">${t(prev ? 'tasks.saveChanges' : 'tasks.create')}</button>${prev ? `<button class="btn line rn">${t('tasks.runNow')}</button><span class="grow"></span><button class="btn danger dl">${t('common.delete')}</button>` : ''}</div>`);
  el.append(actions);
  bindActions(actions, doc, tk, prev, open);
  if (prev) renderHistory(el, tk);
}

function bindActions(actions, doc, tk, prev, open) {
  actions.querySelector('.sv').onclick = async () => {
    tk.name = doc.querySelector('.name').value;
    tk.prompt = doc.querySelector('textarea').value;
    const r = await api('/api/tasks', { method: 'PUT', body: { ...tk, previousName: prev }, root: '' }).catch(e => ({ error: String(e) }));
    if (r.error) return toast(r.error);
    toast(t('tasks.saved'));
    open(r.name);
  };
  const rn = actions.querySelector('.rn');
  if (rn) rn.onclick = async () => {
    const r = await api('/api/tasks/run', { body: { name: prev }, root: '' }).catch(e => ({ error: String(e) }));
    if (r.error) return toast(r.error);
    await reloadSessions();
    openSession(r.root, r.session);
  };
  const dl = actions.querySelector('.dl');
  if (dl) dl.onclick = async () => {
    if (!confirm(t('tasks.deleteConfirm'))) return;
    await api('/api/tasks/delete', { body: { name: prev }, root: '' }).catch(() => { });
    open();
  };
}

function renderHistory(el, tk) {
  el.append(h(`<h2>${t('tasks.history')}</h2>`));
  if (!(tk.history || []).length) el.append(h(`<div class="faint">${t('tasks.neverRan')}</div>`));
  for (const r of tk.history || []) {
    const row = h(`<div class="task-row sched-history-row"><span class="tag ${r.status === 'started' ? 'ok' : r.status === 'failed' ? 'bad' : 'warn'}">${t(r.status === 'started' ? 'tasks.ran' : r.status === 'failed' ? 'tasks.failed' : 'tasks.skipped')}</span><span class="grow">${fmtDateTime(r.ts)}</span><span class="faint">${esc(r.note || '')}</span></div>`);
    if (r.session) row.onclick = () => openSession(tk.root, r.session);
    el.append(row);
  }
}
