'use strict';
// boot
// Settings/usage loaders and boot.

// ---------------------------------------------------------------- settings helpers & boot
async function saveSetting(patch) { const r = await api('/api/settings', { method: 'PUT', body: patch, root: '' }); if (r.error) toast(r.error); Server.set('settings', { ...S.settings, ...patch }); return r; }
async function loadSettings() { const r = await api('/api/settings/full', { root: '' }); Server.set('settings', r.settings || {}); Server.set('version', r.version); Server.set('shell', r.shell); }
async function loadUsage() { const u = await api('/api/usage', { root: '' }); Server.set('usage', u); $('#usageToday').textContent = u.today ? t('hoje ') + money(u.today) : ''; }

async function boot() {
  await loadSettings(); applyTheme();
  UI.set('lang', (S.settings.language || '').toLowerCase().startsWith('en') ? 'en' : 'pt');
  await loadLocale(S.lang);
  startTranslation();
  await loadSessions();
  api('/api/editors', { root: '' }).then(x => Server.set('editors', Array.isArray(x) ? x : []));
  api('/api/models', { root: '' }).then(x => Server.set('models', Array.isArray(x) ? x : []));
  loadUsage();
  const lay = lsGet('layout', null);
  UI.viewsSet([newView()]);
  if (lay?.views?.length) {
    UI.viewsSet(lay.views.map(() => newView()));
    UI.set('focus', Math.min(lay.focus || 0, S.views.length - 1));
    for (let i = 0; i < lay.views.length; i++) { const lv = lay.views[i]; const s = S.sessions.find(x => x.id === lv.sid); if (s) { UI.set('focus', i); View.set(S.views[i].id, { sid: '' }); await openSession(s.root, s.id); } else if (lv.root) View.draft(S.views[i].id, { root: lv.root }); }
    UI.set('focus', Math.min(lay.focus || 0, S.views.length - 1));
  }
  mountViews(); renderSidebar(); restorePanes();
  if (!S.settings.provider) setTimeout(() => toast('Conecte um provedor de IA em Configurações > Provedores'), 600);
  $('#navNew').onclick = () => newSessionScreen(); $('#navScheduled').onclick = () => openScheduled(); $('#navCustomize').onclick = () => openCustomize(); $('#btnSettings').onclick = () => openSettings();
  setInterval(loadSessions, 30000);
}

boot();
