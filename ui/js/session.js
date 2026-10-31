'use strict';
// session
// Live session events, attention, changes, open/split/close session.

function connect(root, id) {
  if (S.live[id]) return S.live[id];
  const L = { id, root, snap: null, text: '', think: '', tools: {}, running: null, started: 0, queue: [], lastIn: 0, lastOut: 0, ctx: null };
  S.live[id] = L;
  L.es = new EventSource(`/api/sessions/${id}/events?root=${encodeURIComponent(root)}`);
  L.es.onmessage = e => onEvent(L, JSON.parse(e.data));
  return L;
}
function closeLive(id) { const L = S.live[id]; if (L?.es) L.es.close(); delete S.live[id]; }
const norm = sn => { if (sn) { sn.messages = sn.messages || []; sn.turnStarts = sn.turnStarts || []; sn.tasks = sn.tasks || []; sn.checkpoints = sn.checkpoints || {}; } return sn; };
function viewsOf(L) { return S.views.filter(v => v.sid === L.id); }
function onEvent(L, ev) {
  const d = ev.data; const vs = viewsOf(L);
  const re = () => vs.forEach(v => { renderChat(v); renderComposer(v); });
  switch (ev.type) {
    case 'session': L.snap = norm(d); if (d.lastInput) L.lastIn = d.lastInput; if (!busy(L)) { L.text = ''; L.think = ''; L.tools = {}; } vs.forEach(renderView); break;
    case 'state':
      if (L.snap) L.snap.state = d.state;
      if (d.state === 'thinking') { L.text = ''; L.think = ''; if (!L.started) L.started = Date.now(); }
      if (d.state === 'idle' || d.state === 'error') { L.started = 0; drainQueue(L); }
      vs.forEach(v => { renderLive(v); renderComposer(v); }); renderSidebarState(L); break;
    case 'turn': if (L.snap) { L.snap.turn = d.turn; L.snap.title = d.title; L.snap.turnStarts.push(L.snap.messages.length); L.snap.messages.push({ role: 'user', content: [{ type: 'text', text: d.text }], ts: Date.now() }); } vs.forEach(renderView); loadSessions(); break;
    case 'stream':
      if (d.kind === 'text') L.text += d.text; else if (d.kind === 'thinking') L.think += d.text;
      else if (d.kind === 'tool_start') L.tools[d.id] = { name: d.name, input: '' }; else if (d.kind === 'tool_input') (L.tools[d.id] = L.tools[d.id] || { name: '', input: '' }).input += d.text;
      vs.forEach(v => { if (!v._liveRaf) v._liveRaf = requestAnimationFrame(() => { v._liveRaf = 0; renderLive(v); }); }); break;
    case 'message': L.snap.messages.push(d.message); L.snap.usage = d.totalUsage; L.snap.cost = d.cost; L.lastIn = d.lastInput; L.lastOut = d.usage?.output || 0; L.text = ''; L.think = ''; L.tools = {}; re(); vs.forEach(renderRail); break;
    case 'tool_run': L.running = d; vs.forEach(renderLive); break;
    case 'stream_reset': L.text = ''; L.think = ''; L.tools = {}; vs.forEach(renderLive); break;
    case 'tool_result': L.running = null; L.snap.messages.push({ role: 'user', content: [{ type: 'tool_result', tool_use_id: d.id, content: d.output, display: d.display, is_error: d.isError, ms: d.ms }], ts: Date.now() }); vs.forEach(renderChat); refreshChanges(L); break;
    case 'permission': L.snap.pendingPermission = d; vs.forEach(renderChat); attention(L, 'infuse precisa de você', t('Aprovar ') + d.tool + '?'); break;
    case 'question': L.snap.pendingQuestion = d; vs.forEach(renderChat); attention(L, 'infuse tem uma pergunta', d.question); break;
    case 'done': L.snap.pendingPermission = null; L.snap.pendingQuestion = null; L.snap.cost = d.cost; L.snap.usage = d.usage; re(); vs.forEach(renderRail); attention(L, 'infuse terminou', L.snap.title || ''); if (!S.views.some(x => x.sid === L.id) || !document.hasFocus()) setUnread(L.id, true); refreshChanges(L); loadUsage(); if (!d.reason) suggestNext(L); break;
    case 'error': L.snap.error = d; L.snap.pendingPermission = null; L.snap.pendingQuestion = null; re(); attention(L, 'infuse: erro', String(d)); break;
    case 'tasks': L.snap.tasks = d; vs.forEach(renderRail); paneRefresh('tasks'); break;
    case 'diagnoses': L.snap.diagnoses = d; vs.forEach(renderRail); break;
    case 'bans': L.bans = d; vs.forEach(renderRail); break;
    case 'procs': L.snap.procs = d; vs.forEach(renderRail); vs.forEach(renderRunBadges); paneRefresh('procs'); break;
    case 'agents': L.snap.agents = d; vs.forEach(renderLive); vs.forEach(refreshAgentBlocks); vs.forEach(renderRail); vs.forEach(renderRunBadges); paneRefresh('agents'); break;
    case 'memory': paneRefresh('memory'); break;
    case 'gate': toast(t('infuse interveio: ') + String(d.display || d.message || '').replace(/^\[infuse\] /, '').split('\n')[0].slice(0, 140)); break;
    case 'subagent': L.sub = d.state === 'done' ? null : { ...(L.sub || {}), ...d, steps: (L.sub?.steps || 0) + (d.state === 'tool' ? 1 : 0) }; vs.forEach(renderLive); break;
    case 'mission': L.snap.mission = d; vs.forEach(renderRail); break;
    case 'report': { // infuse's check of a Report call: data attached to that call, drawn by reportCard
      for (const m of L.snap.messages) for (const b of m.content || []) if (b.type === 'tool_use' && b.id === d.callId) b.meta = d;
      vs.forEach(renderChat); break;
    }
    case 'cwd': L.snap.cwd = d; break;
    case 'checkpoint': (L.snap.checkpoints[d.turn] = L.snap.checkpoints[d.turn] || []).push({ path: d.path }); break;
    case 'map': paneRefresh('map'); paneRefresh('files'); break;
    case 'compacted': vs.forEach(renderChat); toast('Contexto compactado'); break;
  }
}
const setUnread = (id, on) => { if (on) S.unread.add(id); else S.unread.delete(id); lsSet('unread', [...S.unread]); renderSidebar(); };
// the search box also looks inside every conversation (titles alone find little)
const searchTranscripts = debounce(async q => { S.hitsQ = q; const r = await api('/api/sessions/search?q=' + encodeURIComponent(q), { root: '' }); if ($('#search').value.trim().toLowerCase() !== q) return; S.hits = {}; (Array.isArray(r) ? r : []).forEach(x => S.hits[x.id] = x.snippet); renderSidebar(); }, 250);
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
  L.changes = await api('/api/changes?session=' + encodeURIComponent(L.id) + '&scope=' + chgScope(), { root: L.root, dir: L.snap.workRoot || L.root });
  viewsOf(L).forEach(v => { renderRail(v); renderChangesBadge(v); });
  paneRefresh('changes');
}

async function openSession(root, id) {
  closeMenu(); S.page = null; showPage(null);
  const v = V() || (S.views[0] = newView());
  if (S.views.some((x, i) => x.sid === id && i !== S.focus)) { S.focus = S.views.findIndex(x => x.sid === id); mountViews(); renderSidebar(); renderPanes(); return; }
  v.sid = id; v.root = root; if (S.unread.has(id)) setUnread(id, false);
  const L = connect(root, id);
  if (!L.snap) { L.snap = norm(await api(`/api/sessions/${id}`, { root })); L.lastIn = L.snap.lastInput || 0; }
  if (!L.changes) refreshChanges(L);
  S.recent = [id, ...S.recent.filter(x => x !== id)].slice(0, 20);
  lsSet('lastRoot', root); saveLayout();
  mountViews(); renderSidebar(); renderPanes();
  setTimeout(() => $('.prompt', v.el)?.focus(), 30);
}
async function openInSplit(root, id, fromMenu) {
  if (S.views.length < 2) { const nv = newView(); S.views.push(nv); }
  const other = fromMenu ? 1 - S.focus : 1;
  S.focus = other; await openSession(root, id);
}
function exitSplit() { if (S.views.length < 2) return; S.views.splice(1 - S.focus, 1); S.focus = 0; mountViews(); renderSidebar(); saveLayout(); }
function newSessionScreen(root) {
  closeMenu(); S.page = null; showPage(null);
  const v = V() || (S.views[0] = newView());
  v.sid = ''; if (root) v.draft.root = root;
  api('/api/projectmode', { root: v.draft.root }).then(r => { if (r.mode) { v.draft.mode = r.mode; renderComposer(v); } });
  mountViews(); renderSidebar(); renderPanes();
  setTimeout(() => $('.prompt', v.el)?.focus(), 30);
}
function saveLayout() { lsSet('layout', { views: S.views.map(v => ({ sid: v.sid, root: v.sid ? S.live[v.sid]?.root : v.draft.root })), focus: S.focus }); }
