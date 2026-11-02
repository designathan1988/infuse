// customize/plugins — the installed packages (install from git, approve, remove) and the marketplace
// index the settings name. Approving is the only step that lets a plugin's hooks and servers run.
import { h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { I } from '../../core/icons.js';
import { api } from '../../core/api.js';
import { cache, saveSettings } from '../../core/store.js';
import { toast } from '../../components/toast.js';

function pluginCard(p, act) {
  const runs = (p.runs || []).length
    ? `<span>${t('plugin.approveWarning')}</span><ul class="plugin-runs">${p.runs.map(x => `<li class="mono">${esc(x)}</li>`).join('')}</ul>`
    : `<span>${t('plugin.approveNothing')}</span>`;
  const card = h(`<div class="connector-card"><div class="row"><span class="conn-dot ${p.approved ? 'ok' : 'warn'}"></span><b>${esc(p.name)}</b>${p.version ? `<span class="tag mono">${esc(p.version)}</span>` : ''}<span class="faint">${t(p.approved ? 'plugin.approved' : 'mcp.awaiting')}</span><span class="grow"></span><button class="icon-btn del" aria-label="${esc(t('common.remove'))}">${I.trash}</button></div>
    ${p.description ? `<div class="faint">${esc(p.description)}</div>` : ''}
    <div class="mono faint connector-target ml-0">${esc(p.source || p.dir)}${p.commit ? ' @ ' + esc(p.commit.slice(0, 8)) : ''}</div>
    ${p.approved ? '' : `<div class="connector-approve plugin-approve"><div class="grow">${runs}</div><button class="btn sm pri ap">${t('mcp.approve')}</button></div>`}</div>`);
  card.querySelector('.del').onclick = () => act('/api/plugins/remove', p.name);
  card.querySelector('.ap')?.addEventListener('click', () => act('/api/plugins/approve', p.name));
  return card;
}

const marketCard = (e, have) => `<div class="connector-preset${have ? ' added' : ''}"><b>${esc(e.name)}</b>${e.description ? `<span class="faint">${esc(e.description)}</span>` : ''}<span class="mono faint">${esc(e.url)}</span><div class="row">${e.version ? `<span class="tag mono">${esc(e.version)}</span>` : ''}${have ? `<span class="tag ok">${t('mcp.added')}</span>` : `<button class="btn sm" data-url="${esc(e.url)}">${I.plus} ${t('plugin.install')}</button>`}</div></div>`;

function installBox(install) {
  const box = h(`<div class="connector-custom"><div class="crit-sentence">${t('plugin.installFrom')} <input class="slot mono pu field-wide-3" placeholder="https://github.com/org/plugin.git" spellcheck="false" aria-label="${esc(t('plugin.installFrom'))}"> <button class="btn sm pri pi">${t('plugin.install')}</button></div></div>`);
  const go = () => { const u = box.querySelector('.pu').value.trim(); if (u) install(u, box.querySelector('.pi')); };
  box.querySelector('.pi').onclick = go;
  box.querySelector('.pu').onkeydown = e => { if (e.key === 'Enter') go(); };
  return box;
}

function marketBox(reload) {
  const box = h(`<div><h2>${t('plugin.marketplace')}</h2><p class="faint field-note">${t('plugin.marketplaceDesc')}</p><div class="connector-custom field-my"><div class="crit-sentence">${t('plugin.marketplaceAddress')} <input class="slot mono mu field-wide-3" placeholder="https://example.com/plugins.json" spellcheck="false" aria-label="${esc(t('plugin.marketplaceAddress'))}" value="${esc(cache.settings.get().marketplaceUrl || '')}"> <button class="btn sm ms">${t('common.save')}</button></div></div><div class="market-list"></div></div>`);
  box.querySelector('.ms').onclick = async () => {
    await saveSettings({ marketplaceUrl: box.querySelector('.mu').value.trim() }).catch(e => toast(String(e)));
    reload();
  };
  return box;
}

async function drawMarket(list, root, installed, install) {
  if (!cache.settings.get().marketplaceUrl) { list.innerHTML = `<div class="empty">${t('plugin.marketplaceNeeded')}</div>`; return; }
  const r = await api('/api/plugins/marketplace', { root }).catch(e => ({ error: String(e) }));
  if (r.error) { list.innerHTML = `<div class="errbox" role="alert">${esc(r.error)}</div>`; return; }
  const entries = r.plugins || [];
  list.innerHTML = entries.length ? `<div class="connector-grid">${entries.map(e => marketCard(e, installed.has(e.name))).join('')}</div>` : `<div class="empty">${t('plugin.marketplaceEmpty')}</div>`;
  list.querySelectorAll('[data-url]').forEach(b => b.onclick = () => install(b.dataset.url, b));
}

export async function pluginsSection(el, root) {
  el.append(h(`<h1>${t('cust.plugins')}</h1>`), h(`<p class="lead">${t('cust.pluginsDesc')}</p>`));
  const list = h('<div class="connector-list mt-6"></div>');
  const act = async (route, name) => {
    const r = await api(route, { body: { name }, root }).catch(e => ({ error: String(e) }));
    if (r.error) toast(r.error);
    draw();
  };
  const install = async (url, btn) => {
    const label = btn.innerHTML;
    btn.disabled = true; btn.textContent = t('plugin.installing');
    const r = await api('/api/plugins/install', { body: { url }, root }).catch(e => ({ error: String(e) }));
    btn.disabled = false; btn.innerHTML = label;
    toast(r.error || t('plugin.installed'));
    if (!r.error) draw();
  };
  const draw = async () => {
    const plugins = await api('/api/plugins', { root }).catch(e => { toast(String(e)); return []; });
    list.innerHTML = plugins.length ? '' : `<div class="empty">${t('plugin.noneYet')}</div>`;
    for (const p of plugins) list.append(pluginCard(p, act));
    drawMarket(market.querySelector('.market-list'), root, new Set(plugins.map(p => p.name)), install);
  };
  const market = marketBox(() => draw());
  el.append(installBox(install), list, market);
  await draw();
}
