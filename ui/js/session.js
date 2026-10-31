'use strict';
// session
// Live session events, attention, changes, open/split/close session.

function connect(root, id) {
  if (S.live[id]) return S.live[id];
  Live.set(id, { id, root, snap: null, text: '', think: '', tools: {}, running: null, started: 0, queue: [], lastIn: 0, lastOut: 0, ctx: null, changes: null });
  const es = new EventSource(`/api/sessions/${id}/events?root=${encodeURIComponent(root)}`);
  liveRT(id).es = es;
  es.onmessage = e => onEvent(id, JSON.parse(e.data));
  return S.live[id];
}
function closeLive(id) { liveRT(id).es?.close(); RT.live.delete(id); Live.remove(id); }
const norm = sn => { if (sn) { sn.messages = sn.messages || []; sn.turnStarts = sn.turnStarts || []; sn.tasks = sn.tasks || []; sn.checkpoints = sn.checkpoints || {}; } return sn; };
function viewsOf(L) { return S.views.filter(v => v.sid === L.id); }
function onEvent(id, ev) {
  Live.event(id, ev, Date.now());
  const L = S.live[id]; if (!L) return;
  const d = ev.data; const vs = viewsOf(L);
  const re = () => vs.forEach(v => { renderChat(v); renderComposer(v); });
  switch (ev.type) {
    case 'session': vs.forEach(renderView); break;
    case 'state':
      if (d.state === 'idle' || d.state === 'error') drainQueue(L);
      vs.forEach(v => { renderLive(v); renderComposer(v); }); renderSidebarState(L); break;
    case 'turn': vs.forEach(renderView); loadSessions(); break;
    case 'stream':
      vs.forEach(v => { if (!viewRT(v).liveRaf) viewRT(v).liveRaf = requestAnimationFrame(() => { viewRT(v).liveRaf = 0; renderLive(v); }); }); break;
    case 'message': re(); vs.forEach(renderRail); break;
    case 'tool_run': vs.forEach(renderLive); break;
    case 'stream_reset': vs.forEach(renderLive); break;
    case 'tool_result': vs.forEach(renderChat); refreshChanges(L); break;
    case 'permission': vs.forEach(renderChat); attention(L, 'infuse precisa de você', t('ui.session.aprovar.ec21f0') + d.tool + '?'); break;
    case 'question': vs.forEach(renderChat); attention(L, 'infuse tem uma pergunta', d.question); break;
    case 'done': re(); vs.forEach(renderRail); attention(L, 'infuse terminou', L.snap.title || ''); if (!S.views.some(x => x.sid === L.id) || !document.hasFocus()) setUnread(L.id, true); refreshChanges(L); loadUsage(); if (!d.reason) suggestNext(L); break;
    case 'error': re(); attention(L, 'infuse: erro', String(d)); break;
    case 'tasks': vs.forEach(renderRail); paneRefresh('tasks'); break;
    case 'diagnoses': vs.forEach(renderRail); break;
    case 'bans': vs.forEach(renderRail); break;
    case 'procs': vs.forEach(renderRail); vs.forEach(renderRunBadges); paneRefresh('procs'); break;
    case 'agents': vs.forEach(renderLive); vs.forEach(refreshAgentBlocks); vs.forEach(renderRail); vs.forEach(renderRunBadges); paneRefresh('agents'); break;
    case 'memory': paneRefresh('memory'); break;
    case 'gate': toast(t('ui.session.infuse-interveio.0ebaf9') + String(d.display || d.message || '').replace(/^\[infuse\] /, '').split('\n')[0].slice(0, 140)); break;
    case 'subagent': vs.forEach(renderLive); break;
    case 'mission': vs.forEach(renderRail); break;
    case 'report': vs.forEach(renderChat); break;
    case 'cwd': break;
    case 'checkpoint': break;
    case 'map': paneRefresh('map'); paneRefresh('files'); break;
    case 'compacted': vs.forEach(renderChat); toast('Contexto compactado'); break;
  }
}
const setUnread = (id, on) => { UI.unread(id, on); renderSidebar(); };
// the search box also looks inside every conversation (titles alone find little)
const searchTranscripts = debounce(async q => { UI.set('hitsQ', q); const r = await api('/api/sessions/search?q=' + encodeURIComponent(q), { root: '' }); if ($('#search').value.trim().toLowerCase() !== q) return; const hits = {}; (Array.isArray(r) ? r : []).forEach(x => hits[x.id] = x.snippet); UI.set('hits', hits); renderSidebar(); }, 250);
function renderSidebarState(L) { const r = $$('#sessList .srow').find(x => x.querySelector('.t') && S.sessions.find(s => s.id === L.id) && x.title.endsWith(L.root)); renderSidebar(); }
function attention(L, title, body) {
  if (S.settings.notify === false) return;
  const viewing = S.views[S.focus]?.sid === L.id && document.hasFocus();
  if (!viewing) nat('notify', t(title), t(body));
}
// Changes of a folder without git: since infuse first saw it, or since the start of the latest turn.
const chgScope = () => lsGet('chgScope', 'all');
const CHG_ST = { A: ['novo', 'ok'], D: ['excluído', 'bad'] };
function scopeSwitch() { const s = chgScope(); return `<span class="seg2">${[['all', 'Tudo'], ['turn', 'Este turno']].map(([k, l]) => `<button data-scope="${k}" class="${s === k ? 'on' : ''}" title="${esc(t(k === 'all' ? 'Desde que o infuse começou a acompanhar a pasta' : 'Desde o início do último turno'))}">${t(l)}</button>`).join('')}</span>`; }
function bindScope(el) { $$('.seg2 button', el).forEach(b => b.onclick = e => { e.stopPropagation(); lsSet('chgScope', b.dataset.scope); Object.values(S.live).forEach(L => refreshChanges(L)); paneRefresh('changes'); }); }
const chgTag = f => CHG_ST[f.status] ? `<span class="tag ${CHG_ST[f.status][1]} chst">${t(CHG_ST[f.status][0])}</span>` : '';
async function refreshChanges(L) {
  if (!L?.snap) return;
  const changes = await api('/api/changes?session=' + encodeURIComponent(L.id) + '&scope=' + chgScope(), { root: L.root, dir: L.snap.workRoot || L.root });
  Live.patch(L.id, { changes });
  viewsOf(L).forEach(v => { renderRail(v); renderChangesBadge(v); });
  paneRefresh('changes');
}

async function openSession(root, id) {
  closeMenu(); UI.set('page', null); showPage(null);
  if (!V()) UI.viewsAdd(newView());
  const v = V();
  if (S.views.some((x, i) => x.sid === id && i !== S.focus)) { UI.set('focus', S.views.findIndex(x => x.sid === id)); mountViews(); renderSidebar(); renderPanes(); return; }
  View.set(v.id, { sid: id, root }); if (S.unread.includes(id)) setUnread(id, false);
  connect(root, id);
  if (!S.live[id].snap) { const snap = norm(await api(`/api/sessions/${id}`, { root })); Live.patch(id, { snap, lastIn: snap.lastInput || 0 }); }
  if (!S.live[id].changes) refreshChanges(S.live[id]);
  UI.set('recent', [id, ...S.recent.filter(x => x !== id)].slice(0, 20));
  lsSet('lastRoot', root); saveLayout();
  mountViews(); renderSidebar(); renderPanes();
  setTimeout(() => $('.prompt', elOf(v))?.focus(), 30);
}
async function openInSplit(root, id, fromMenu) {
  const before = S.focus;
  if (S.views.length < 2) UI.viewsAdd(newView());
  const other = fromMenu ? 1 - before : 1;
  UI.set('focus', other); await openSession(root, id);
}
function exitSplit() { if (S.views.length < 2) return; UI.viewsRemove(1 - S.focus); UI.set('focus', 0); mountViews(); renderSidebar(); saveLayout(); }
function newSessionScreen(root) {
  closeMenu(); UI.set('page', null); showPage(null);
  if (!V()) UI.viewsAdd(newView());
  const v = V();
  View.set(v.id, { sid: '' }); if (root) View.draft(v.id, { root });
  const target = root || v.draft.root;
  api('/api/projectmode', { root: target }).then(r => { if (r.mode) { View.draft(v.id, { mode: r.mode }); renderComposer(freshView(v)); } });
  mountViews(); renderSidebar(); renderPanes();
  setTimeout(() => $('.prompt', elOf(v))?.focus(), 30);
}
function saveLayout() { lsSet('layout', { views: S.views.map(v => ({ sid: v.sid, root: v.sid ? S.live[v.sid]?.root : v.draft.root })), focus: S.focus }); }
