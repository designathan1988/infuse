// settings/providers — connection cards; keys go to the OS vault, test/save and default live here.
import { $, $$, h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { api } from '../../core/api.js';
import { cache, ensureModels, reloadSettings } from '../../core/store.js';
import { toast } from '../../components/toast.js';

export async function renderProviders({ el }) {
  el.append(h(`<h1>${t('settings.providers')}</h1>`), h(`<p class="lead">${t('settings.providersDesc')}</p>`));
  const conns = await api('/api/connections', { root: '' }).catch(() => []);
  const list = h('<div></div>');
  el.append(list);
  drawProviders(list, conns);
}

function drawProviders(list, rows) {
  list.innerHTML = '';
  (rows || []).forEach(p => list.appendChild(providerRow(p, list)));
}

async function reload(list) {
  drawProviders(list, await api('/api/connections', { root: '' }).catch(() => []));
}

function providerRow(p, list) {
  const r = h(`<div class="provider-row"><div class="top"><span class="nm">${esc(p.label)}</span>${p.connected ? `<span class="tag ok">${t('prov.connected')}</span>${p.keyHint ? `<span class="faint mono">${esc(p.keyHint)}</span>` : ''}${p.default ? `<span class="tag">${t('prov.default')}</span>` : ''}` : `<span class="faint">${p.local ? t('prov.local') : ''}</span>`}<span class="grow"></span>${p.connected ? `<button class="btn sm mk">${t('prov.makeDefault')}</button><button class="btn sm ed">${t('common.edit')}</button><button class="btn sm danger dc">${t('prov.disconnect')}</button>` : `<button class="btn sm line cn">${t('prov.connect')}</button>`}</div></div>`);
  const expand = () => toggleExpand(r, p, list);
  r.querySelector('.cn')?.addEventListener('click', expand);
  r.querySelector('.ed')?.addEventListener('click', expand);
  r.querySelector('.dc')?.addEventListener('click', async () => {
    await api('/api/connections/disconnect', { body: { id: p.id }, root: '' }).catch(() => { });
    await ensureModels(); await reloadSettings();
    await reload(list);
  });
  r.querySelector('.mk')?.addEventListener('click', async () => {
    await api('/api/connections/default', { body: { id: p.id, model: p.models?.[0] || '' }, root: '' }).catch(() => { });
    await reloadSettings();
    await reload(list);
  });
  return r;
}

function toggleExpand(r, p, list) {
  const open = r.querySelector('.ex');
  if (open) { open.remove(); return; }
  const ex = providerEditor(p, list);
  r.appendChild(ex);
  r.querySelector('input')?.focus();
}

function providerEditor(p, list) {
  const ex = h(`<div class="ex">${p.needKey || p.id === 'custom' ? `<div class="f"><input class="key" type="password" placeholder="${esc(p.connected ? t('prov.newKey') : t('prov.pasteKey'))}" aria-label="${esc(t('prov.pasteKey'))}">${p.keyUrl ? `<a href="${esc(p.keyUrl)}" target="_blank" rel="noreferrer" class="faint">${t('prov.getKey')}</a>` : ''}</div>` : ''}
    <div class="f"><input class="base" placeholder="${esc(p.defaultBaseUrl || 'https://…')}" value="${esc(p.baseUrl !== p.defaultBaseUrl ? p.baseUrl : '')}" aria-label="${t('prov.endpoint')}"><span class="faint">${t('prov.endpoint')}</span></div>
    <div class="row"><button class="btn sm pri go">${t('prov.testAndSave')}</button><span class="faint res" aria-live="polite"></span></div>${p.models?.length ? `<div class="model-chips">${p.models.slice(0, 60).map(m => `<span>${esc(m)}</span>`).join('')}</div>` : ''}</div>`);
  const go = async () => {
    const res = ex.querySelector('.res');
    res.textContent = t('prov.testing');
    const out = await api('/api/connections/connect', { body: { id: p.id, apiKey: ex.querySelector('.key')?.value || '', baseUrl: ex.querySelector('.base').value.trim() }, root: '' }).catch(e => ({ error: String(e) }));
    if (!out.ok) { res.innerHTML = `<span class="del">${esc(out.error)}</span>`; return; }
    toast(t('prov.connectedLabel') + ` · ${out.connection?.models?.length || 0} ${t('prov.models')}`);
    await ensureModels(); await reloadSettings();
    await reload(list);
  };
  ex.querySelector('.go').onclick = go;
  $$('.ex input', ex).forEach(i => i.onkeydown = e => { if (e.key === 'Enter') go(); });
  return ex;
}
