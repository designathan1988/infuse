// settings/integration — the interposed shell shim and its install/remove actions.
import { h } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { api } from '../../core/api.js';
import { cache } from '../../core/store.js';
import { setRow } from '../shell.js';
import { toast } from '../../components/toast.js';

export function renderIntegration({ el, open }) {
  const st = cache.shell || {};
  el.append(h(`<h1>${t('settings.integration')}</h1>`), h(`<p class="lead">${t('settings.integrationDesc')}</p>`));
  const ctl = h(`<span class="row"><span class="tag ${st.shimInstalled ? 'ok' : ''}">${t(st.shimInstalled ? 'integration.installed' : 'integration.notInstalled')}</span><button class="btn sm line in">${t('apps.install')}</button><button class="btn sm danger un">${t('common.remove')}</button></span>`);
  ctl.querySelector('.in').onclick = async () => {
    const r = await api('/api/install', { body: {}, root: '' }).catch(e => ({ error: String(e) }));
    toast(r.error || t('integration.installedToast'));
    open('integration');
  };
  ctl.querySelector('.un').onclick = async () => {
    await api('/api/uninstall', { body: {}, root: '' }).catch(() => { });
    toast(t('integration.removed'));
    open('integration');
  };
  el.append(setRow('integration.interposedShell', st.shim || '', ctl));
}
