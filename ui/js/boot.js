'use strict';
// boot
// Settings/usage loaders and boot.

// ---------------------------------------------------------------- settings helpers & boot
async function saveSetting(patch) { const r = await api('/api/settings', { method: 'PUT', body: patch, root: '' }); if (r.error) toast(r.error); Object.assign(S.settings, patch); return r; }
async function loadSettings() { const r = await api('/api/settings/full', { root: '' }); S.settings = r.settings || {}; S.version = r.version; S.shell = r.shell; }
async function loadUsage() { const u = await api('/api/usage', { root: '' }); S.usage = u; $('#usageToday').textContent = u.today ? t('hoje ') + money(u.today) : ''; }

async function boot() {
  await loadSettings(); applyTheme();
  S.lang = (S.settings.language || '').toLowerCase().startsWith('en') ? 'en' : 'pt';
  await loadLocale(S.lang);
  startTranslation();
  await loadSessions();
  api('/api/editors', { root: '' }).then(x => S.editors = Array.isArray(x) ? x : []);
  api('/api/models', { root: '' }).then(x => { S.models = Array.isArray(x) ? x : []; });
  loadUsage();
  const lay = lsGet('layout', null);
  S.views = [newView()];
  if (lay?.views?.length) {
    S.views = lay.views.map(() => newView()); S.focus = Math.min(lay.focus || 0, S.views.length - 1);
    for (let i = 0; i < lay.views.length; i++) { const lv = lay.views[i]; const s = S.sessions.find(x => x.id === lv.sid); if (s) { S.focus = i; S.views[i].sid = ''; await openSession(s.root, s.id); } else if (lv.root) S.views[i].draft.root = lv.root; }
    S.focus = Math.min(lay.focus || 0, S.views.length - 1);
  }
  mountViews(); renderSidebar(); restorePanes();
  if (!S.settings.provider) setTimeout(() => toast('Conecte um provedor de IA em Configurações > Provedores'), 600);
  $('#navNew').onclick = () => newSessionScreen(); $('#navScheduled').onclick = () => openScheduled(); $('#navCustomize').onclick = () => openCustomize(); $('#btnSettings').onclick = () => openSettings();
  setInterval(loadSessions, 30000);
}

boot();
