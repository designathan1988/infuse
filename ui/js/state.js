'use strict';
// state
// Application state, the reducer and the action creators. Server data is a cache reconciled from the
// API and SSE events; ui holds preferences and view configuration. Nothing assigns to S directly.

function initialState() {
  return {
    settings: {}, projects: [], sessions: [], archived: [], models: [], connections: [], editors: [],
    commands: {}, files: {}, live: {}, views: [], focus: 0, page: null,
    history: lsGet('hist', []),
    sb: lsGet('sb', { group: 'folder', sort: 'updated', closed: {} }),
    unread: lsGet('unread', []),
    hits: {}, hitsQ: '',
    viewMode: lsGet('viewMode', 'normal'), rail: lsGet('rail', true), recent: [], lang: 'pt', fold: {},
    pageArg: null, usage: null, version: '', shell: null,
  };
}

// keys that hold a map or set-like collection addressed by a sub-key
const MAP_KEYS = new Set(['commands', 'files', 'hits', 'fold', 'sb']);

function reducer(state, action) {
  switch (action.type) {
    case 'unread': {
      const has = state.unread.includes(action.id);
      if (has === action.on) return state;
      return { ...state, unread: action.on ? [...state.unread, action.id] : state.unread.filter(x => x !== action.id) };
    }
    case 'history/push': {
      const list = [...state.history, action.text];
      if (list.length > 300) list.shift();
      return { ...state, history: list };
    }
  }
  const [group, kind] = action.type.split('/');
  switch (group) {
    case 'server': return serverReducer(state, action, kind);
    case 'ui': return uiReducer(state, action, kind);
    case 'views': return viewsReducer(state, action, kind);
    case 'view': return viewReducer(state, action, kind);
    case 'live': return liveReducer(state, action, kind);
  }
  return state;
}

function serverReducer(state, action, kind) {
  return kind === 'set' ? { ...state, [action.key]: action.value } : state;
}

function uiReducer(state, action, kind) {
  switch (kind) {
    case 'set': return { ...state, [action.key]: action.value };
    case 'sub': return { ...state, [action.key]: { ...(state[action.key] || {}), [action.sub]: action.value } };
    case 'del': {
      const m = { ...(state[action.key] || {}) };
      delete m[action.sub];
      return { ...state, [action.key]: m };
    }
    case 'merge': return { ...state, [action.key]: { ...(state[action.key] || {}), ...action.value } };
  }
  return state;
}

function viewsReducer(state, action, kind) {
  switch (kind) {
    case 'set':
      return { ...state, views: action.views, focus: Math.min(state.focus, Math.max(0, action.views.length - 1)) };
    case 'add':
      return { ...state, views: [...state.views, action.view], focus: state.views.length };
    case 'remove': {
      let focus = state.focus;
      const views = state.views.filter((_, i) => i !== action.index);
      if (action.index < focus) focus--;
      if (focus >= views.length) focus = Math.max(0, views.length - 1);
      return { ...state, views, focus };
    }
  }
  return state;
}

function viewReducer(state, action, kind) {
  if (kind === 'set') return withView(state, action.id, v => ({ ...v, ...action.patch }));
  if (kind === 'draft') return withView(state, action.id, v => ({ ...v, draft: { ...v.draft, ...action.patch } }));
  return state;
}

function liveReducer(state, action, kind) {
  const current = state.live[action.id];
  switch (kind) {
    case 'set':
      return { ...state, live: { ...state.live, [action.id]: action.value } };
    case 'patch':
      if (!current) return state;
      return { ...state, live: { ...state.live, [action.id]: { ...current, ...action.patch } } };
    case 'snap':
      if (!current?.snap) return state;
      return { ...state, live: { ...state.live, [action.id]: { ...current, snap: { ...current.snap, ...action.patch } } } };
    case 'queue': {
      if (!current) return state;
      const q = [...(current.queue || [])];
      if (action.op === 'push') q.push(action.item);
      else if (action.op === 'unshift') q.unshift(action.item);
      else if (action.op === 'shift') q.shift();
      else if (action.op === 'remove') q.splice(action.index, 1);
      return { ...state, live: { ...state.live, [action.id]: { ...current, queue: q } } };
    }
    case 'event':
      if (!current) return state;
      return { ...state, live: { ...state.live, [action.id]: applyEvent(current, action.ev, action.ts) } };
    case 'delete': {
      if (!(action.id in state.live)) return state;
      const live = { ...state.live };
      delete live[action.id];
      return { ...state, live };
    }
  }
  return state;
}

function withView(state, id, fn) {
  return { ...state, views: state.views.map(v => v.id === id ? fn(v) : v) };
}

function normSnap(sn) {
  if (sn) {
    sn.messages = sn.messages || [];
    sn.turnStarts = sn.turnStarts || [];
    sn.tasks = sn.tasks || [];
    sn.checkpoints = sn.checkpoints || {};
  }
  return sn;
}

function busySnap(snap) { return !!(snap && snap.state !== 'idle' && snap.state !== 'error'); }

// applyEvent folds one SSE event into the session's cache. It only builds the next value; the view
// renders after the dispatch, reading the new state.
function applyEvent(L, ev, ts) {
  const d = ev.data;
  switch (ev.type) {
    case 'session': return applySnapshotEvent(L, d);
    case 'state': return applyStateEvent(L, d, ts);
    case 'turn': return applyTurnEvent(L, d, ts);
    case 'stream': return applyStreamEvent(L, d);
    case 'message': return applyMessageEvent(L, d);
    case 'tool_result': return applyToolResultEvent(L, d, ts);
    case 'tool_run': return { ...L, running: d };
    case 'stream_reset': return { ...L, text: '', think: '', tools: {} };
    case 'permission', 'question', 'done', 'error': return applyPendingEvent(L, ev.type, d);
    case 'tasks', 'diagnoses', 'procs', 'agents', 'mission', 'cwd': return applySnapFieldEvent(L, ev.type, d);
    case 'bans': return { ...L, bans: d };
    case 'subagent': return applySubagentEvent(L, d);
    case 'checkpoint': return applyCheckpointEvent(L, d);
    case 'report': return applyReportEvent(L, d);
  }
  return L;
}

function applySnapshotEvent(L, d) {
  const next = normSnap(d);
  return {
    ...L, snap: next, lastIn: d.lastInput || L.lastIn,
    ...(busySnap(next) ? {} : { text: '', think: '', tools: {} }),
  };
}

function applyStateEvent(L, d, ts) {
  if (!L.snap) return L;
  return {
    ...L,
    snap: { ...L.snap, state: d.state },
    ...(d.state === 'thinking' ? { text: '', think: '', started: L.started || ts } : {}),
    ...(d.state === 'idle' || d.state === 'error' ? { started: 0 } : {}),
  };
}

function applyTurnEvent(L, d, ts) {
  const snap = L.snap;
  if (!snap) return L;
  const messages = [...snap.messages, { role: 'user', content: [{ type: 'text', text: d.text }], ts }];
  return { ...L, snap: { ...snap, turn: d.turn, title: d.title, turnStarts: [...snap.turnStarts, snap.messages.length], messages } };
}

function applyStreamEvent(L, d) {
  if (d.kind === 'text') return { ...L, text: L.text + d.text };
  if (d.kind === 'thinking') return { ...L, think: L.think + d.text };
  if (d.kind === 'tool_start') return { ...L, tools: { ...L.tools, [d.id]: { name: d.name, input: '' } } };
  if (d.kind === 'tool_input') {
    const tool = L.tools[d.id] || { name: '', input: '' };
    return { ...L, tools: { ...L.tools, [d.id]: { ...tool, input: tool.input + d.text } } };
  }
  return L;
}

function applyMessageEvent(L, d) {
  if (!L.snap) return L;
  return {
    ...L,
    snap: { ...L.snap, messages: [...L.snap.messages, d.message], usage: d.totalUsage, cost: d.cost },
    lastIn: d.lastInput, lastOut: d.usage?.output || 0, text: '', think: '', tools: {},
  };
}

function applyToolResultEvent(L, d, ts) {
  if (!L.snap) return L;
  return {
    ...L, running: null,
    snap: { ...L.snap, messages: [...L.snap.messages, { role: 'user', content: [{ type: 'tool_result', tool_use_id: d.id, content: d.output, display: d.display, is_error: d.isError, ms: d.ms }], ts }] },
  };
}

function applyPendingEvent(L, type, d) {
  if (!L.snap) return L;
  if (type === 'permission') return { ...L, snap: { ...L.snap, pendingPermission: d } };
  if (type === 'question') return { ...L, snap: { ...L.snap, pendingQuestion: d } };
  if (type === 'done') return { ...L, snap: { ...L.snap, pendingPermission: null, pendingQuestion: null, cost: d.cost, usage: d.usage } };
  return { ...L, snap: { ...L.snap, error: d, pendingPermission: null, pendingQuestion: null } };
}

function applySnapFieldEvent(L, type, d) {
  if (!L.snap) return L;
  return { ...L, snap: { ...L.snap, [type]: d } };
}

function applySubagentEvent(L, d) {
  return { ...L, sub: d.state === 'done' ? null : { ...(L.sub || {}), ...d, steps: (L.sub?.steps || 0) + (d.state === 'tool' ? 1 : 0) } };
}

function applyCheckpointEvent(L, d) {
  if (!L.snap) return L;
  return { ...L, snap: { ...L.snap, checkpoints: { ...L.snap.checkpoints, [d.turn]: [...(L.snap.checkpoints[d.turn] || []), { path: d.path }] } } };
}

function applyReportEvent(L, d) {
  if (!L.snap) return L;
  const messages = L.snap.messages.map(m => {
    if (!(m.content || []).some(b => b.type === 'tool_use' && b.id === d.callId)) return m;
    return { ...m, content: m.content.map(b => b.type === 'tool_use' && b.id === d.callId ? { ...b, meta: d } : b) };
  });
  return { ...L, snap: { ...L.snap, messages } };
}

const store = createStore(reducer, initialState(), {
  persist(state, action) {
    if (action.type === 'ui/set' && ['viewMode', 'rail', 'lang'].includes(action.key)) lsSet(action.key, action.value);
    if (action.type === 'ui/sub' && action.key === 'sb') lsSet('sb', state.sb);
    if (action.type === 'ui/set' && action.key === 'sb') lsSet('sb', state.sb);
    if (action.type === 'unread') lsSet('unread', state.unread);
    if (action.type === 'history/push') lsSet('hist', state.history);
  },
});

let S = store.getState();
store.subscribe(state => { S = state; });

const dispatch = store.dispatch;
const select = store.select;
const subscribe = store.subscribe;

// actions
const Server = {
  set: (key, value) => dispatch({ type: 'server/set', key, value }),
};
const UI = {
  set: (key, value) => dispatch({ type: 'ui/set', key, value }),
  sub: (key, sub, value) => dispatch({ type: 'ui/sub', key, sub, value }),
  del: (key, sub) => dispatch({ type: 'ui/del', key, sub }),
  merge: (key, value) => dispatch({ type: 'ui/merge', key, value }),
  unread: (id, on) => dispatch({ type: 'unread', id, on }),
  history: text => dispatch({ type: 'history/push', text }),
  viewsSet: views => dispatch({ type: 'views/set', views }),
  viewsAdd: view => dispatch({ type: 'views/add', view }),
  viewsRemove: index => dispatch({ type: 'views/remove', index }),
};
const Live = {
  set: (id, value) => dispatch({ type: 'live/set', id, value }),
  patch: (id, patch) => dispatch({ type: 'live/patch', id, patch }),
  snap: (id, patch) => dispatch({ type: 'live/snap', id, patch }),
  queue: (id, op, item, index) => dispatch({ type: 'live/queue', id, op, item, index }),
  event: (id, ev, ts) => dispatch({ type: 'live/event', id, ev, ts }),
  remove: id => dispatch({ type: 'live/delete', id }),
};
const View = {
  set: (id, patch) => dispatch({ type: 'view/set', id, patch }),
  draft: (id, patch) => dispatch({ type: 'view/draft', id, patch }),
};
