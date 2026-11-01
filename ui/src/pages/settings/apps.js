// settings/apps — installed editors and CLIs; a missing CLI opens a terminal at the install command.
import { h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { api } from '../../core/api.js';
import { curRoot } from '../../core/store.js';
import { toast } from '../../components/toast.js';

export async function renderApps({ el }) {
  el.append(h(`<h1>${t('settings.appsTitle')}</h1>`), h(`<p class="lead">${t('settings.appsDesc')}</p>`));
  const apps = await api('/api/apps', { root: curRoot() }).catch(() => []);
  for (const a of apps) el.append(appRow(a));
  const eds = await api('/api/editors', { root: '' }).catch(() => []);
  el.append(h(`<h2>${t('settings.editors')}</h2>`), h(`<p class="lead">${eds.length ? eds.map(e => esc(e.name)).join(', ') : t('settings.noEditors')} — ${t('settings.editorsDesc')}</p>`));
}

function appRow(a) {
  const r = h(`<div class="provider-row"><div class="top"><span class="nm">${esc(a.name)}</span><span class="tag ${a.found ? 'ok' : ''}">${t(a.found ? 'apps.found' : 'apps.notFound')}</span><span class="faint mono trunc grow">${esc(a.appId || a.path || '')}</span>${a.found ? `<button class="btn sm line">${t('common.open')}</button>` : a.kind === 'cli' ? `<button class="btn sm line">${t('apps.install')}</button>` : `<button class="btn sm line" disabled>${t('common.open')}</button>`}</div></div>`);
  r.querySelector('button').onclick = async () => {
    if (a.kind === 'cli') return runAppInTerminal(a.id, !a.found);
    const res = await api('/api/apps/open', { body: { id: a.id }, root: curRoot() }).catch(e => ({ error: String(e) }));
    toast(res.ok ? t('apps.opening') : (res.error || ''));
  };
  return r;
}

async function runAppInTerminal(id, install) {
  const r = await api('/api/apps/command', { body: { id, install: install ? '1' : '' }, root: '' }).catch(e => ({ error: String(e) }));
  if (!r.ok) return toast(r.error || '');
  const { showPage } = await import('../shell.js');
  showPage(null);
  const panes = await import('../../panes/index.js');
  panes.togglePane('terminal', true);
  const { curDir } = await import('../../core/store.js');
  setTimeout(() => panes.paneCall('terminal', 'openAt', curDir(), r.command, r.name), 80);
}
