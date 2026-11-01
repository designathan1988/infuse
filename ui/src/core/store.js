// store — the single source of truth. Two realms, both signal-backed:
//   serverCache: authoritative server data mirrored locally (settings, sessions, projects, models…),
//                refreshed by SSE events — never by blind polling.
//   live: one entry per open session; snap (server snapshot) + transient streaming buffers as signals.
//   ui: view layout, focus, panes, folds, prefs (one versioned localStorage object).
// Components only READ signals and call actions; they never write state they don't own.
import { signal, batch } from './signals.js';
import { api } from './api.js';
import { t } from './i18n.js';
import { sseUrl } from './api.js';

// ------------------------------------------------------------------ tiny event bus (store → UI notices)
const listeners = {};
export const bus = {
  on(ev, fn) { (listeners[ev] = listeners[ev] || []).push(fn); return () => { listeners[ev] = (listeners[ev] || []).filter(f => f !== fn); }; },
  emit(ev, data) { for (const fn of listeners[ev] || []) try { fn(data); } catch (e) { console.error(e); } }
};

// ------------------------------------------------------------------ versioned ui prefs (single key, migratable)
const PREFS_KEY = 'uiPrefs.v1';
const DEFAULT_PREFS = {
  sidebar: { group: 'folder', sort: 'updated', closed: {} },
  viewMode: 'normal', rail: true, unread: [], recent: [], hist: [], lastRoot: '',
  layout: null, panes: { open: [], width: 520, sizes: {} }, railFolds: {}, fold: {},
  chgScope: 'all', memScope: 'project', rulesScope: 'project', oracleHelp: null, memHelp: true,
  browserUrl: {}, autofix: {}
};
let prefs = loadPrefs();
function loadPrefs() {
  try { const v = JSON.parse(localStorage.getItem(PREFS_KEY)); if (v && v.v === 1) return { ...DEFAULT_PREFS, ...v }; } catch { }
  return structuredClone(DEFAULT_PREFS);
}
export function pref(key, val) {
  if (val === undefined) return prefs[key];
  prefs[key] = val; savePrefs(); prefsSig.set({ ...prefs });
  return val;
}
function savePrefs() { try { localStorage.setItem(PREFS_KEY, JSON.stringify(prefs)); } catch { } }
export const prefsSig = signal(prefs);

// ------------------------------------------------------------------ server cache
export const cache = {
  settings: signal({}), version: '', shell: {},
  sessions: signal([]), archived: signal([]), projects: signal([]),
  models: signal([]), editors: signal([]), usage: signal(null),
  filesByRoot: {}, commandsByRoot: {},
};
export async function reloadSessions() {
  const [list, projects] = await Promise.all([
    api('/api/sessions/all', { root: '' }).catch(() => []),
    api('/api/projects', { root: '' }).catch(() => [])
  ]);
  cache.sessions.set(Array.isArray(list) ? list : []);
  cache.projects.set(Array.isArray(projects) ? projects : []);
}
export async function reloadUsage() {
  cache.usage.set(await api('/api/usage', { root: '' }).catch(() => null));
}
export async function reloadSettings() {
  const r = await api('/api/settings/full', { root: '' }).catch(() => ({}));
  cache.settings.set(r.settings || {}); cache.version = r.version; cache.shell = r.shell || {};
}
export async function saveSettings(patch) {
  const r = await api('/api/settings', { method: 'PUT', body: patch, root: '' });
  cache.settings.set({ ...cache.settings.get(), ...patch });
  return r;
}
export async function ensureModels() {
  if (!cache.models.get().length) cache.models.set(await api('/api/models', { root: '' }).catch(() => []));
  return cache.models.get();
}
export const filesOf = async root => {
  if (!cache.filesByRoot[root]) cache.filesByRoot[root] = await api('/api/files/list', { root }).catch(() => []);
  return cache.filesByRoot[root];
};
export const commandsOf = async root => {
  if (!cache.commandsByRoot[root]) cache.commandsByRoot[root] = await api('/api/commands', { root }).catch(() => []);
  return cache.commandsByRoot[root] || [];
};
export const projectLabel = root => cache.projects.get().find(x => x.root === root)?.label || (root || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop();

// ------------------------------------------------------------------ live sessions (SSE-fed)
const liveMap = new Map();
export const liveOf = id => liveMap.get(id);
export const busy = L => {
  if (!L) return false;
  const s = L.snap.get();
  return !!s && s.state !== 'idle' && s.state !== 'error';
};

function normSnap(s) {
  if (s) { s.messages = s.messages || []; s.turnStarts = s.turnStarts || []; s.tasks = s.tasks || []; s.checkpoints = s.checkpoints || {}; }
  return s;
}

export function connect(root, id) {
  let L = liveMap.get(id);
  if (L) return L;
  L = {
    id, root,
    snap: signal(null),                       // authoritative snapshot (server-fed)
    text: signal(''), think: signal(''), tools: signal({}), running: signal(null),
    queue: [], started: 0, lastIn: 0, lastOut: 0,
    changes: signal(null), verdict: signal(null), bans: signal(undefined),
    es: null
  };
  liveMap.set(id, L);
  L.es = new EventSource(sseUrl(`/api/sessions/${id}/events`, root));
  L.es.onmessage = e => onEvent(L, JSON.parse(e.data));
  return L;
}
export function closeLive(id) {
  const L = liveMap.get(id);
  if (L?.es) L.es.close();
  liveMap.delete(id);
}
export function openViewsOf(id) { return ui.views.get().filter(v => v.sid === id); }

function onEvent(L, ev) {
  const handler = EVENT_HANDLERS[ev.type];
  if (handler) batch(() => handler(L, ev.data));
}

const setSnap = (L, patch) => L.snap.set({ ...L.snap.get(), ...patch });
const fieldHandler = key => (L, d) => setSnap(L, { [key]: d });

function onSession(L, d) {
  L.snap.set(normSnap(d));
  if (d.lastInput) L.lastIn = d.lastInput;
  if (!busy(L)) resetStream(L);
}

function onState(L, d) {
  setSnap(L, { state: d.state });
  if (d.state === 'thinking') {
    L.text.set(''); L.think.set('');
    if (!L.started) L.started = Date.now();
  }
  if (d.state === 'idle' || d.state === 'error') { L.started = 0; drainQueue(L); }
}

function onTurn(L, d) {
  const s = L.snap.get();
  setSnap(L, {
    turn: d.turn, title: d.title,
    turnStarts: [...s.turnStarts, s.messages.length],
    messages: [...s.messages, { role: 'user', content: [{ type: 'text', text: d.text }], ts: Date.now() }]
  });
  bus.emit('sessions-changed');
}

function onStream(L, d) {
  if (d.kind === 'text') L.text.set(L.text.get() + d.text);
  else if (d.kind === 'thinking') L.think.set(L.think.get() + d.text);
  else if (d.kind === 'tool_start') L.tools.set({ ...L.tools.get(), [d.id]: { name: d.name, input: '' } });
  else if (d.kind === 'tool_input') {
    const t = { ...(L.tools.get()[d.id] || { name: '', input: '' }) };
    t.input += d.text;
    L.tools.set({ ...L.tools.get(), [d.id]: t });
  }
}

function onMessage(L, d) {
  const s = L.snap.get();
  setSnap(L, {
    messages: [...s.messages, d.message],
    usage: d.totalUsage, cost: d.cost
  });
  L.lastIn = d.lastInput; L.lastOut = d.usage?.output || 0;
  resetStream(L);
}

function onToolResult(L, d) {
  L.running.set(null);
  const s = L.snap.get();
  setSnap(L, { messages: [...s.messages, { role: 'user', content: [{ type: 'tool_result', tool_use_id: d.id, content: d.output, display: d.display, is_error: d.isError, ms: d.ms }], ts: Date.now() }] });
  refreshChanges(L);
}

function onPermission(L, d) {
  setSnap(L, { pendingPermission: d });
  bus.emit('attention', { L, kind: 'permission' });
}

function onQuestion(L, d) {
  setSnap(L, { pendingQuestion: d });
  bus.emit('attention', { L, kind: 'question' });
}

function onDone(L, d) {
  setSnap(L, { pendingPermission: null, pendingQuestion: null, cost: d.cost, usage: d.usage });
  refreshChanges(L); reloadUsage();
  bus.emit('attention', { L, kind: 'done' });
  if (!d.reason) bus.emit('suggest-next', L);
}

function onError(L, d) {
  setSnap(L, { error: d, pendingPermission: null, pendingQuestion: null });
  bus.emit('attention', { L, kind: 'error' });
}

function onReport(L, d) {
  const s = L.snap.get();
  setSnap(L, {
    messages: s.messages.map(m => ({
      ...m, content: (m.content || []).map(b => b.type === 'tool_use' && b.id === d.callId ? { ...b, meta: d } : b)
    }))
  });
}

function onCheckpoint(L, d) {
  const s = L.snap.get();
  const checkpoints = { ...s.checkpoints };
  (checkpoints[d.turn] = checkpoints[d.turn] || []).push({ path: d.path });
  setSnap(L, { checkpoints });
}

function onSubagent(L, d) {
  L.sub = d.state === 'done' ? null : { ...(L.sub || {}), ...d, steps: (L.sub?.steps || 0) + (d.state === 'tool' ? 1 : 0) };
}

function resetStream(L) { L.text.set(''); L.think.set(''); L.tools.set({}); }

const EVENT_HANDLERS = {
  session: onSession, state: onState, turn: onTurn, stream: onStream, message: onMessage,
  tool_run: (L, d) => L.running.set(d),
  stream_reset: L => resetStream(L),
  tool_result: onToolResult, permission: onPermission, question: onQuestion, done: onDone,
  error: onError, tasks: fieldHandler('tasks'), diagnoses: fieldHandler('diagnoses'),
  bans: (L, d) => L.bans.set(d), procs: fieldHandler('procs'), agents: fieldHandler('agents'),
  memory: () => bus.emit('pane-refresh', 'memory'), gate: (L, d) => bus.emit('gate', d),
  subagent: onSubagent, mission: fieldHandler('mission'), report: onReport,
  cwd: fieldHandler('cwd'), checkpoint: onCheckpoint,
  map: () => { bus.emit('pane-refresh', 'map'); bus.emit('pane-refresh', 'files'); },
  compacted: () => bus.emit('compacted'),
};

async function drainQueue(L) {
  if (!L.queue.length) return;
  const next = L.queue.shift();
  await api(`/api/sessions/${L.id}/send`, { body: next, root: L.root }).catch(e => { L.queue.unshift(next); bus.emit('api-error', e); });
}

export async function refreshChanges(L) {
  if (!L?.snap.get()) return;
  const snap = L.snap.get();
  L.changes.set(await api(`/api/changes?session=${encodeURIComponent(L.id)}&scope=${pref('chgScope')}`, { root: L.root, dir: snap.workRoot || L.root }).catch(() => null));
}

// ------------------------------------------------------------------ ui state (layout, focus, panes)
function newViewDraft() {
  const st = cache.settings.get();
  const lastRoot = pref('lastRoot') || cache.projects.get()[0]?.root || '';
  return { sid: '', root: '', draft: { root: lastRoot, worktree: false, mode: '', provider: st.provider, model: st.model, effort: st.effort || 'medium' }, atts: [], images: [], hi: -1 };
}
export const ui = {
  views: signal([newViewDraft()]),
  focus: signal(0),
  page: signal(null),            // 'settings' | 'scheduled' | 'customize' | null
  pageArg: signal('general'),
  panes: signal(null),           // initialized from prefs in boot
  lang: signal('pt'),
  searchHits: signal({}), searchQ: '',
};
export const focusedView = () => ui.views.get()[ui.focus.get()] || ui.views.get()[0];
export const curL = () => { const v = focusedView(); return v?.sid ? liveOf(v.sid) : null; };
export const curRoot = () => { const v = focusedView(); if (!v) return ''; return v.sid ? (liveOf(v.sid)?.root || v.root) : v.draft.root; };
export const curDir = () => { const v = focusedView(); const L = v?.sid ? liveOf(v.sid) : null; return L?.snap.get()?.workRoot || curRoot(); };
export const ctxLimit = L => L?.snap.get()?.contextLimit || cache.settings.get().contextLimit || 1000000;

export function setFocus(i) { if (i >= 0 && i !== ui.focus.get()) ui.focus.set(i); }

export async function openSession(root, id, force = false) {
  const v = focusedView() || (ui.views.set([newViewDraft()]), ui.views.get()[0]);
  const already = ui.views.get().findIndex((x, i) => x.sid === id && i !== ui.focus.get());
  if (already >= 0 && !force) { setFocus(already); return; }
  v.sid = id; v.root = root;
  setUnread(id, false);
  const L = connect(root, id);
  if (!L.snap.get()) {
    L.snap.set(normSnap(await api(`/api/sessions/${id}`, { root }).catch(() => null)));
    L.lastIn = L.snap.get()?.lastInput || 0;
  }
  if (!L.changes.get()) refreshChanges(L);
  prefs.recent = [id, ...prefs.recent.filter(x => x !== id)].slice(0, 20);
  pref('lastRoot', root); savePrefs();
  ui.views.set([...ui.views.get()]);
  bus.emit('sessions-changed');
}
export function newSessionScreen(root) {
  const v = focusedView();
  v.sid = ''; if (root) v.draft.root = root;
  ui.views.set([...ui.views.get()]);
}
export function openInSplit(root, id, fromMenu) {
  if (ui.views.get().length < 2) ui.views.set([...ui.views.get(), newViewDraft()]);
  setFocus(fromMenu ? 1 - ui.focus.get() : 1);
  return openSession(root, id, true);
}
export function exitSplit() {
  if (ui.views.get().length < 2) return;
  const views = ui.views.get().slice();
  views.splice(1 - ui.focus.get(), 1);
  ui.views.set(views); setFocus(0);
}
export function setUnread(id, on) {
  const set = new Set(pref('unread'));
  on ? set.add(id) : set.delete(id);
  pref('unread', [...set]);
}
export const isUnread = id => pref('unread').includes(id);

// ------------------------------------------------------------------ actions (the only writers of server state via API)
export async function send(v) {
  const P = v._promptEl;
  let text = P ? P.value.trim() : v._pendingText || '';
  if (P) { P.value = ''; P.style.height = 'auto'; }
  if (!text && !v.atts.length && !v.images.length) return;
  let L = v.sid ? liveOf(v.sid) : null;
  if (!L) {
    const d = { mode: v.draft.mode, provider: v.draft.provider, model: v.draft.model, effort: v.draft.effort, worktree: v.draft.worktree, title: text.slice(0, 40) };
    if (!v.draft.root) return bus.emit('gate', { display: 'choose-folder' });
    const r = await api('/api/sessions/new', { body: d, root: v.draft.root }).catch(e => (bus.emit('api-error', e), null));
    if (!r || r.error) return;
    await openSession(v.draft.root, r.id);
    L = liveOf(r.id);
  }
  const msg = { text: text || '(anexos)', attachments: v.atts, images: v.images };
  prefs.hist = [...prefs.hist.filter(x => x !== text), text].slice(-300); savePrefs(); v.hi = -1;
  v.atts = []; v.images = [];
  if (busy(L)) { L.queue.push(msg); ui.views.set([...ui.views.get()]); return; }
  const r = await api(`/api/sessions/${L.id}/send`, { body: msg, root: L.root }).catch(e => ({ error: String(e) }));
  if (r && r.error) bus.emit('api-error', r.error);
}
export function stop(L) { api(`/api/sessions/${L.id}/stop`, { body: {}, root: L.root }).catch(() => { }); }
export async function decide(L, id, d) {
  if (d === 'all') await setSessionOpts(L, { mode: 'bypass' });
  const r = await api(`/api/sessions/${L.id}/permission`, { body: { id, decision: d === 'all' ? 'allow' : d }, root: L.root }).catch(e => ({ error: e }));
  if (!r || r.ok !== true) {
    bus.emit('api-error', r?.error instanceof Error ? r.error : new Error(t('perm.failed')));
    return;
  }
  const s = { ...L.snap.get() }; s.pendingPermission = null; L.snap.set(s);
}
export async function answer(L, id, a) {
  const r = await api(`/api/sessions/${L.id}/answer`, { body: { id, answer: a }, root: L.root }).catch(e => ({ error: e }));
  if (!r || r.ok !== true) {
    bus.emit('api-error', r?.error instanceof Error ? r.error : new Error(t('question.answerFailed')));
    return;
  }
  const s = { ...L.snap.get() }; s.pendingQuestion = null; L.snap.set(s);
}
export async function setSessionOpts(L, patch) {
  const r = await api(`/api/session/${L.id}/settings`, { body: patch, root: L.root }).catch(() => null);
  if (r && r.id) {
    const s = normSnap({ ...L.snap.get(), ...r, messages: L.snap.get().messages });
    L.snap.set(s);
    bus.emit('sessions-changed');
  }
}
