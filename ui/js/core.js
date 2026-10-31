'use strict';
// core
// Shared DOM helpers, the application state, the API client, the native bridge, theme and view selectors.

'use strict';
// infuse window: sidebar (sessions grouped by folder), session views (one, or two in split view), the
// session details rail, the composer, panes (panes.js) and full pages (pages.js). Everything the page
// shows comes from the local API; nothing here decides anything the executor enforces.

const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const h = (html) => { const t = document.createElement('template'); t.innerHTML = html.trim(); return t.content.firstElementChild; };
const debounce = (fn, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; };
const money = v => 'US$ ' + (v || 0).toFixed(v && v < 1 ? 3 : 2);
const ktok = n => n >= 1e6 ? +(n / 1e6).toFixed(n % 1e6 ? 2 : 0) + 'M' : n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k' : String(n || 0);
const baseName = p => (p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop();
const ago = ts => { if (!ts) return ''; const s = (Date.now() - ts) / 1000; if (s < 60) return t('agora'); if (s < 3600) return Math.floor(s / 60) + ' min'; if (s < 86400) return Math.floor(s / 3600) + ' h'; if (s < 7 * 86400) return Math.floor(s / 86400) + ' d'; return new Date(ts).toLocaleDateString(); };
const I = {
  branch: '<svg class="i" viewBox="0 0 24 24"><circle cx="6" cy="5" r="2"/><circle cx="6" cy="19" r="2"/><circle cx="18" cy="8" r="2"/><path d="M6 7v10M18 10c0 4-6 3-11.5 7"/></svg>',
  pin: '<svg class="i" viewBox="0 0 24 24"><path d="M9 3h6l-1 6 4 3v2H6v-2l4-3zM12 14v7"/></svg>',
  speak: '<svg class="i" viewBox="0 0 24 24"><path d="M4 9v6h4l5 4V5L8 9zM16.5 8.5a5 5 0 0 1 0 7M19 6a8.5 8.5 0 0 1 0 12"/></svg>',
  plus: '<svg class="i" viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>',
  more: '<svg class="i" viewBox="0 0 24 24"><circle cx="5" cy="12" r="1.2"/><circle cx="12" cy="12" r="1.2"/><circle cx="19" cy="12" r="1.2"/></svg>',
  vmore: '<svg class="i" viewBox="0 0 24 24"><circle cx="12" cy="5" r="1.2"/><circle cx="12" cy="12" r="1.2"/><circle cx="12" cy="19" r="1.2"/></svg>',
  agents: '<svg class="i" viewBox="0 0 24 24"><circle cx="12" cy="5.5" r="2.5"/><circle cx="5.5" cy="17.5" r="2.5"/><circle cx="18.5" cy="17.5" r="2.5"/><path d="M12 8v3.5M12 11.5 7 15.5M12 11.5l5 4"/></svg>',
  procs: '<svg class="i" viewBox="0 0 24 24"><path d="M3 12h4l2.5-6 5 12 2.5-6h4"/></svg>',
  term: '<svg class="i" viewBox="0 0 24 24"><rect x="3" y="4.5" width="18" height="15" rx="2"/><path d="m7 9.5 3 2.5-3 2.5M12.5 15h4"/></svg>',
  diff: '<svg class="i" viewBox="0 0 24 24"><path d="M8 4v10M3 9h10M11 20h10"/></svg>',
  files: '<svg class="i" viewBox="0 0 24 24"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/></svg>',
  globe: '<svg class="i" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/></svg>',
  rail: '<svg class="i" viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M15 4v16"/></svg>',
  close: '<svg class="i" viewBox="0 0 24 24"><path d="M6 6l12 12M18 6 6 18"/></svg>',
  up: '<svg viewBox="0 0 24 24"><path d="M12 19V5M5.5 11.5 12 5l6.5 6.5"/></svg>',
  stop: '<svg viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="2"/></svg>',
  caret: '<span class="car">▾</span>',
  pop: '<svg class="i" viewBox="0 0 24 24"><path d="M14 4h6v6M20 4l-8 8M18 14v4a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4"/></svg>',
  reload: '<svg class="i" viewBox="0 0 24 24"><path d="M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7"/></svg>',
  folder: '<svg class="i" viewBox="0 0 24 24"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>',
  branch: '<svg class="i" viewBox="0 0 24 24"><circle cx="6" cy="6" r="2"/><circle cx="6" cy="18" r="2"/><circle cx="18" cy="8" r="2"/><path d="M6 8v8M18 10c0 4-6 3-12 6"/></svg>',
  copy: '<svg class="i" viewBox="0 0 24 24"><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/></svg>',
  archive: '<svg class="i" viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="5" rx="1"/><path d="M5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9M10 13h4"/></svg>',
  trash: '<svg class="i" viewBox="0 0 24 24"><path d="M4 7h16M10 11v6M14 11v6M6 7l1 12a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-12M9 7V4h6v3"/></svg>',
  pc: '<svg class="i" viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/></svg>',
};

// ---------------------------------------------------------------- persisted small values
function lsGet(k, d) { try { const v = localStorage.getItem(k); return v ? JSON.parse(v) : d; } catch { return d; } }
function lsSet(k, v) { try { localStorage.setItem(k, JSON.stringify(v)); } catch { } }
const native = !!window.infuse;
// The API token arrives in the URL fragment (never sent over the network); it is kept for this tab only.
let API_TOKEN = '';
function readToken() { const m = location.hash.match(/token=([A-Za-z0-9_-]+)/); try { if (m) { sessionStorage.setItem('infuseToken', m[1]); history.replaceState(null, '', location.pathname); } API_TOKEN = m ? m[1] : (sessionStorage.getItem('infuseToken') || API_TOKEN); } catch { if (m) API_TOKEN = m[1]; } return API_TOKEN; }
readToken(); addEventListener('hashchange', readToken);

async function api(url, opts = {}) {
  const root = opts.root ?? curRoot();
  if (root && !/[?&]root=/.test(url)) url += (url.includes('?') ? '&' : '?') + 'root=' + encodeURIComponent(root);
  if (opts.dir && !/[?&]dir=/.test(url)) url += '&dir=' + encodeURIComponent(opts.dir);
  const r = await fetch(url, { method: opts.method || (opts.body ? 'POST' : 'GET'), headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: opts.body ? JSON.stringify(opts.body) : undefined });
  return r.json().catch(() => ({}));
}
const nat = async (method, ...p) => {
  if (native) return infuse.call(method, ...p);
  if (method === 'notify') { if (!document.hasFocus() && 'Notification' in window) { if (Notification.permission === 'granted') new Notification(p[0], { body: p[1] }); else Notification.requestPermission(); } return; }
  if (method === 'reveal') return api('/api/reveal', { body: { path: p[0] } });
};
let toastT; function toast(m) { const el = $('#toast'); el.textContent = t(m); el.classList.add('show'); clearTimeout(toastT); toastT = setTimeout(() => el.classList.remove('show'), 2600); }

// ---------------------------------------------------------------- theme & language
function applyTheme() { const th = S.settings.theme || 'system'; document.documentElement.classList.toggle('dark', th === 'dark' || (th === 'system' && matchMedia('(prefers-color-scheme: dark)').matches)); }
matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyTheme);

// ---------------------------------------------------------------- views
// A view shows one session (or the new-session screen when sid is empty). Split view = two views.
function V() { return S.views[S.focus] || S.views[0]; }
function curRoot() { const v = V(); if (!v) return ''; if (v.sid) return S.live[v.sid]?.root || v.root; return v.draft.root; }
function curDir() { const v = V(); const L = v?.sid && S.live[v.sid]; return L?.snap?.workRoot || curRoot(); }
function curL() { const v = V(); return v?.sid ? S.live[v.sid] : null; }
