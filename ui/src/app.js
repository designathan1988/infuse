// app — boot and application chrome: theme (tokens + data-theme), declarative keyboard map,
// drag & drop (folders become projects, files attachments), notifications, the views host, and the
// attention bus (SSE events → system notifications when you are not looking).
import { $, $$, h, esc } from './core/dom.js';
import { t, setLocale, loadCatalog } from './core/i18n.js';
import { money } from './core/fmt.js';
import { I } from './core/icons.js';
import { effect } from './core/signals.js';
import { api, readToken } from './core/api.js';
import {
  ui, cache, pref, bus, prefsSig, reloadSessions, reloadSettings, reloadUsage, ensureModels,
  openSession, newSessionScreen, setFocus, liveOf, busy, focusedView, closeLive
} from './core/store.js';
import { buildView } from './components/view.js';
import { mountSidebar } from './components/sidebar.js';
import { render as renderPanes, togglePane, bindPanes } from './panes/index.js';
import { toast } from './components/toast.js';
import { menuOpen } from './components/menu.js';
import { dialogOpen } from './components/dialog.js';

// ---------------------------------------------------------------- theme (tokens carry the colors)
export function applyTheme() {
  const th = cache.settings.get().theme || 'system';
  const dark = th === 'dark' || (th === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
}
matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyTheme);

// ---------------------------------------------------------------- views host
function mountViews() {
  const host = $('#views');
  host.innerHTML = '';
  host.classList.toggle('split', ui.views.get().length > 1);
  ui.views.get().forEach((v, i) => {
    // a view binds to one session; when it switches sessions (open/new), rebuild its subtree
    if (!v.el || v._lastSid !== v.sid) {
      v.el?.remove();
      v.sideEl?.remove(); v.sideEl = null;
      buildView(v);
      v._lastSid = v.sid;
    }
    v.el.classList.toggle('focused', i === ui.focus.get());
    host.appendChild(v.el);
  });
}
effect(() => { ui.views.get(); ui.focus.get(); mountViews(); });

// ---------------------------------------------------------------- attention: notifications when away
bus.on('attention', ({ L, kind }) => {
  if (cache.settings.get().notify === false) return;
  const viewing = ui.views.get()[ui.focus.get()]?.sid === L.id && document.hasFocus();
  if (viewing) return;
  const msg = {
    permission: ['notify.needsYou.title', 'notify.needsYou.body'],
    question: ['notify.question.title', 'notify.question.body'],
    done: ['notify.done.title', L.snap.get()?.title || ''],
    error: ['notify.error.title', 'infuse'],
  }[kind] || ['', ''];
  nat('notify', t(msg[0]), typeof msg[1] === 'string' && msg[1] ? msg[1] : t(msg[1]));
  if (kind !== 'done' || !ui.views.get().some(x => x.sid === L.id) || !document.hasFocus()) {
    if (kind === 'done') import('../src/core/store.js').then(st => st.setUnread(L.id, true));
  }
});
const nat = async (method, ...p) => {
  if (window.infuse) return infuse.call(method, ...p);
  if (method === 'notify') {
    if (!document.hasFocus() && 'Notification' in window) {
      if (Notification.permission === 'granted') new Notification(p[0], { body: p[1] });
      else Notification.requestPermission();
    }
    return;
  }
};
bus.on('gate', d => {
  const text = String(d?.display || d?.message || d || '').replace(/^\[infuse\] /, '').split('\n')[0].slice(0, 140);
  if (d === 'choose-folder') return toast(t('composer.chooseFolder'));
  toast(t('gate.intervened') + text);
});
bus.on('api-error', e => {
  const code = e?.body?.code;
  const translated = code ? t(code, e.body.args) : '';
  toast(translated && translated !== code ? translated : String(e?.message || e));
});
bus.on('compacted', () => toast(t('chat.compactedToast')));
bus.on('sessions-changed', () => reloadSessions().catch(() => { }));
bus.on('close-page', () => import('./pages/shell.js').then(s => s.showPage(null))); // a session click leaves any page

// CI auto-fix: check PRs of sessions with autofix on, every 2 minutes
setInterval(async () => {
  for (const v of ui.views.get()) {
    const L = v.sid ? liveOf(v.sid) : null;
    if (!L || !L.changes.get()?.isGit) continue;
    if (L.autofix === undefined) L.autofix = !!pref('autofix')[L.id];
    if (!L.autofix || busy(L)) continue;
    try {
      const pr = await api('/api/pr', { root: L.root, dir: L.snap.get().workRoot || L.root });
      const p = pr?.pr;
      if (!p) continue;
      const failed = (p.statusCheckRollup || []).filter(chk => /FAIL|ERROR/i.test(chk.conclusion || chk.state || ''));
      const sig = p.number + ':' + failed.map(chk => chk.name + chk.completedAt).join('|');
      if (failed.length && L.autofixSig !== sig) {
        L.autofixSig = sig;
        toast(t('pr.ciFailedSendingLog'));
        const r = await api('/api/pr', { body: { action: 'failed-logs' }, root: L.root, dir: L.snap.get().workRoot || L.root });
        if (r.ok && v._promptEl) {
          v._promptEl.value = `${t('pr.ciFailedPrompt', { n: p.number })}\n\n\`\`\`\n${r.output}\n\`\`\``;
          v._sendFn?.();
        }
      }
    } catch { /* offline for a beat */ }
  }
}, 120000);

// ---------------------------------------------------------------- drag & drop: folders → projects, files → attachments
let dragDepth = 0;
addEventListener('dragenter', e => { if (e.dataTransfer.types.includes('Files')) { dragDepth++; $('#drop').classList.add('show'); } });
addEventListener('dragleave', () => { if (--dragDepth <= 0) { dragDepth = 0; $('#drop').classList.remove('show'); } });
addEventListener('dragover', e => { if (e.dataTransfer.types.includes('Files')) e.preventDefault(); });
addEventListener('drop', async e => {
  dragDepth = 0;
  $('#drop').classList.remove('show');
  if (!e.dataTransfer.files.length) return;
  e.preventDefault();
  const v = focusedView();
  const files = [...e.dataTransfer.files];
  if (window.infuse) {
    const r = await infuse.drop(files);
    if (r.projects?.length) { await reloadSessions(); newSessionScreen(r.projects[0]); }
    for (const a of r.attachments || []) if (!v.atts.find(x => x.path === a.path)) v.atts.push(a);
    v.renderAtts?.();
    ui.views.set([...ui.views.get()]);
  } else {
    for (const f of files) {
      if (f.type.startsWith('image/')) {
        const r = new FileReader();
        r.onload = () => { v.images.push({ mediaType: f.type, data: r.result.split(',')[1] }); v.renderAtts?.(); };
        r.readAsDataURL(f);
      } else {
        const r = new FileReader();
        r.onload = () => { if (v._promptEl) v._promptEl.value += `\n<file name="${f.name}">\n${r.result.slice(0, 200000)}\n</file>`; };
        r.readAsText(f);
      }
    }
    if ([...e.dataTransfer.items].some(i => i.webkitGetAsEntry?.()?.isDirectory)) toast(t('drop.folderHint'));
  }
});

// ---------------------------------------------------------------- keyboard: declarative map, no if-chains
// Each binding: chord → {desc, run}. Inside the terminal only chords in TERMINAL_PASS escape to the app.
const pane = (id, force) => () => togglePane(id, force);
const SHORTCUTS = {
  'ctrl+k': { run: () => import('./components/dialogs.js').then(d => d.palette()) },
  'ctrl+n': { run: () => newSessionScreen() },
  'ctrl+,': { run: () => import('./pages/settings.js').then(p => p.openSettings()) },
  'ctrl+/': { run: () => import('./components/dialogs.js').then(d => d.shortcutsDialog()) },
  'ctrl+b': { run: () => document.body.classList.toggle('sidebar-collapsed') },
  'ctrl+w': { run: () => { if (ui.views.get().length > 1) { import('./core/store.js').then(st => st.exitSplit()); } else { const v = focusedView(); if (v?.sid) { v.sid = ''; ui.views.set([...ui.views.get()]); } } } },
  'ctrl+alt+w': { run: () => import('./core/store.js').then(st => st.exitSplit()) },
  'ctrl+tab': { run: () => {
    const ids = pref('recent').filter(id => cache.sessions.get().find(s => s.id === id));
    if (ids.length > 1) { const s = cache.sessions.get().find(x => x.id === ids[1]); if (s) openSession(s.root, s.id); }
  } },
  'ctrl+shift+]': { run: () => cycleSession(1) },
  'ctrl+shift+[': { run: () => cycleSession(-1) },
  'ctrl+shift+d': { run: pane('changes') },
  'ctrl+shift+b': { run: pane('browser') },
  'ctrl+shift+a': { run: pane('agents') },
  'ctrl+shift+p': { run: pane('procs') },
  'ctrl+shift+s': { run: () => { togglePane('browser', true); setTimeout(() => import('./panes/index.js').then(p => p.paneCall('browser', 'pick')), 100); } },
  'ctrl+`': { run: pane('terminal') },
  'ctrl+\\': { run: () => import('./panes/index.js').then(p => p.closeFocusedPane()) },
  'ctrl+;': { run: () => import('./components/view.js').then(m => m.openSideChat(focusedView())) },
  'ctrl+o': { run: () => {
    const order = ['normal', 'thinking', 'verbose'];
    pref('viewMode', order[(order.indexOf(pref('viewMode')) + 1) % 3]);
  } },
  'ctrl+shift+m': { run: () => $('.modechip, .chip[class*="mode-"]', focusedView()?.el || document)?.click() },
  'ctrl+shift+i': { run: () => $('.composer-under .chip:last-child', focusedView()?.el || document)?.click() },
  'ctrl+shift+e': { run: () => $('.composer-under .chip:last-child', focusedView()?.el || document)?.click() },
};
const TERMINAL_PASS = new Set(['ctrl+n', 'ctrl+k', 'ctrl+,', 'ctrl+/', 'ctrl+`', 'ctrl+\\', 'ctrl+;', 'ctrl+tab', 'ctrl+shift+d', 'ctrl+shift+b', 'ctrl+shift+m', 'ctrl+shift+i', 'ctrl+shift+]', 'ctrl+shift+[', 'ctrl+alt+w']);
function cycleSession(dir) {
  const v = focusedView();
  const ids = cache.sessions.get().map(s => s.id);
  const i = ids.indexOf(v?.sid);
  const s = cache.sessions.get()[(i + dir + ids.length) % ids.length];
  if (s) openSession(s.root, s.id);
}
addEventListener('keydown', e => {
  if (document.querySelector('.overlay')) return;                     // modal handles its own keys
  const k = e.key.toLowerCase(), ctrl = e.ctrlKey || e.metaKey;
  const inTerm = !!document.activeElement?.closest?.('.xterm');
  const chord = (ctrl ? 'ctrl+' : '') + (e.altKey ? 'alt+' : '') + (e.shiftKey && ctrl ? 'shift+' : '') + (e.code === 'Backquote' ? '`' : e.code === 'BracketRight' ? ']' : e.code === 'BracketLeft' ? '[' : k);
  if (inTerm && !TERMINAL_PASS.has(chord)) return;                    // the shell gets everything else
  if (inTerm) e.stopPropagation();
  const binding = SHORTCUTS[chord];
  if (binding) { e.preventDefault(); binding.run(); return; }
  if (menuOpen() && /^[1-9]$/.test(e.key) && !ctrl) { e.preventDefault(); return; }  // menu handles digits
  if (e.key === 'Escape' && !inTerm) { const L = focusedView()?.sid ? liveOf(focusedView().sid) : null; if (L && busy(L)) { import('./core/store.js').then(st => st.stop(L)); } return; }
  // Y/A/T/N answer a pending permission without leaving the keyboard
  const inField = /INPUT|TEXTAREA/.test(document.activeElement?.tagName) || inTerm;
  if (!inField) {
    const L = focusedView()?.sid ? liveOf(focusedView().sid) : null;
    const p = L?.snap.get()?.pendingPermission;
    if (p && !ctrl) {
      if (k === 'y') return import('./core/store.js').then(st => st.decide(L, p.id, 'allow'));
      if (k === 't') return import('./core/store.js').then(st => st.decide(L, p.id, 'all'));
      if (k === 'a') return import('./core/store.js').then(st => st.decide(L, p.id, 'always'));
      if (k === 'n') return import('./core/store.js').then(st => st.decide(L, p.id, 'deny'));
    }
    const q = L?.snap.get()?.pendingQuestion;
    if (q && /^[1-9]$/.test(e.key)) {
      const o = q.options?.[+e.key - 1];
      if (o) return import('./core/store.js').then(st => st.answer(L, q.id, o));
    }
    if (!ctrl && e.key.length === 1 && focusedView()?._promptEl && !ui.page.get()) focusedView()._promptEl.focus();
  }
}, true);

// ---------------------------------------------------------------- boot
export async function boot() {
  readToken();
  try { await loadCatalog(); } catch (e) { console.error('message catalog:', e); }
  await reloadSettings();
  setLocale(cache.settings.get().language || 'pt-BR');
  applyTheme();
  // views mounted before the locale was known render in the default language — rebuild them now
  for (const v of ui.views.get()) v.el = null;
  ui.views.set([...ui.views.get()]);
  prefsSig.set({ ...pref() });
  await reloadSessions();
  await ensureModels();
  reloadUsage();
  cache.editors.set(await api('/api/editors', { root: '' }).catch(() => []));
  ui.lang.set((cache.settings.get().language || '').toLowerCase().startsWith('en') ? 'en' : 'pt');

  // sidebar
  $('#navNew').onclick = () => newSessionScreen();
  $('#navNew').onkeydown = e => { if (e.key === 'Enter') newSessionScreen(); };
  $('#navScheduled').onclick = () => import('./pages/scheduled.js').then(p => p.openScheduled());
  $('#navCustomize').onclick = () => import('./pages/customize.js').then(p => p.openCustomize());
  $('#btnSettings').onclick = () => import('./pages/settings.js').then(p => p.openSettings());
  mountSidebar();
  renderPanes();
  bindPanes();

  // today's spend (footer)
  effect(() => {
    const u = cache.usage.get();
    $('#usageToday').textContent = u?.today ? t('usage.today') + ' ' + money(u.today) : '';
  });

  // restore layout: sessions of the saved views
  const lay = pref('layout');
  if (lay?.views?.length) {
    const views = ui.views.get();
    ui.views.set(lay.views.map(() => ({ ...views[0], sid: '', root: '', el: null, atts: [], images: [] })));
    for (let i = 0; i < lay.views.length; i++) {
      const lv = lay.views[i];
      const s = cache.sessions.get().find(x => x.id === lv.sid);
      if (s) { setFocus(i); await openSession(s.root, s.id); }
      else if (lv.root) ui.views.get()[i].draft.root = lv.root;
    }
    setFocus(Math.min(lay.focus || 0, ui.views.get().length - 1));
  }
  // persist layout on changes (debounced by the signal write)
  effect(() => {
    ui.views.get(); ui.focus.get();
    pref('layout', {
      views: ui.views.get().map(v => ({ sid: v.sid, root: v.sid ? (liveOf(v.sid)?.root || v.root) : v.draft.root })),
      focus: ui.focus.get()
    });
  });

  if (!cache.settings.get().provider) setTimeout(() => toast(t('boot.connectProvider')), 600);
  setInterval(() => reloadSessions().catch(() => { }), 20000); // safety net; SSE drives the real updates
  // sessions created elsewhere (the scheduler, another window, the API) appear as soon as the user looks
  document.addEventListener('visibilitychange', () => { if (!document.hidden) reloadSessions().catch(() => { }); });
  window.addEventListener('focus', () => reloadSessions().catch(() => { }));
}
window.addEventListener('error', e => console.error('page error:', e.message, e.filename + ':' + e.lineno));
window.addEventListener('unhandledrejection', e => console.error('unhandled rejection:', e.reason));
