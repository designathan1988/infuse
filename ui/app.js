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
const ktok = n => n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k' : String(n || 0);
const baseName = p => (p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop();
const ago = ts => { if (!ts) return ''; const s = (Date.now() - ts) / 1000; if (s < 60) return t('agora'); if (s < 3600) return Math.floor(s / 60) + ' min'; if (s < 86400) return Math.floor(s / 3600) + ' h'; if (s < 7 * 86400) return Math.floor(s / 86400) + ' d'; return new Date(ts).toLocaleDateString(); };
const I = {
  branch: '<svg class="i" viewBox="0 0 24 24"><circle cx="6" cy="5" r="2"/><circle cx="6" cy="19" r="2"/><circle cx="18" cy="8" r="2"/><path d="M6 7v10M18 10c0 4-6 3-11.5 7"/></svg>',
  pin: '<svg class="i" viewBox="0 0 24 24"><path d="M9 3h6l-1 6 4 3v2H6v-2l4-3zM12 14v7"/></svg>',
  speak: '<svg class="i" viewBox="0 0 24 24"><path d="M4 9v6h4l5 4V5L8 9zM16.5 8.5a5 5 0 0 1 0 7M19 6a8.5 8.5 0 0 1 0 12"/></svg>',
  plus: '<svg class="i" viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>',
  more: '<svg class="i" viewBox="0 0 24 24"><circle cx="5" cy="12" r="1.2"/><circle cx="12" cy="12" r="1.2"/><circle cx="19" cy="12" r="1.2"/></svg>',
  vmore: '<svg class="i" viewBox="0 0 24 24"><circle cx="12" cy="5" r="1.2"/><circle cx="12" cy="12" r="1.2"/><circle cx="12" cy="19" r="1.2"/></svg>',
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

// ---------------------------------------------------------------- state
const S = {
  settings: {}, projects: [], sessions: [], archived: [], models: [], connections: [], editors: [],
  commands: {}, files: {}, live: {}, views: [], focus: 0, page: null,
  history: lsGet('hist', []), sb: lsGet('sb', { group: 'folder', sort: 'updated', closed: {} }), unread: new Set(lsGet('unread', [])), hits: {}, hitsQ: '',
  viewMode: lsGet('viewMode', 'normal'), rail: lsGet('rail', true), recent: [], lang: 'pt', fold: {},
};
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

function newView() {
  const d = S.settings;
  const lastRoot = lsGet('lastRoot', '') || (S.projects[0]?.root || '');
  return { sid: '', root: '', draft: { root: S.projects.find(p => p.root === lastRoot) ? lastRoot : (S.projects[0]?.root || ''), worktree: false, mode: '', provider: d.provider, model: d.model, effort: d.effort || 'medium' }, el: null, atts: [], images: [], sideOpen: false, side: [] };
}

function mountViews() {
  const host = $('#views'); host.innerHTML = ''; host.classList.toggle('split', S.views.length > 1);
  S.views.forEach((v, i) => { if (!v.el) buildView(v); v.el.classList.toggle('focus', i === S.focus); host.appendChild(v.el); renderView(v); });
}

function buildView(v) {
  const el = h(`<section class="view">
    <div class="vbar"><span class="title" tabindex="0" title="Clique para renomear"></span><button class="ib caret" title="Menu da sessão">${I.caret}</button><span class="branch"></span><span class="sp"></span>
      <span class="r row" style="gap:2px"><button class="chg" data-a="changes" title="Alterações (Ctrl+Shift+D)"></button>
      <button class="ib" data-a="terminal" title="Terminal (Ctrl+\`)">${I.term}</button><button class="ib" data-a="files" title="Arquivos">${I.files}</button><button class="ib" data-a="browser" title="Navegador (Ctrl+Shift+B)">${I.globe}</button>
      <button class="ib" data-a="rail" title="Mostrar/ocultar detalhes da sessão">${I.rail}</button><button class="ib" data-a="more" title="Mais">${I.vmore}</button></span></div>
    <div class="vbody"><div class="vchat"><div class="toc"><div class="ticks"></div><div class="toclist"></div></div><div class="scroller"><div class="chat"></div></div><button class="jumpdown" title="Ir para o fim da conversa">${I.down || '↓'}</button>
      <div class="composer"><div class="hometitle"><b>O que vamos construir?</b><span class="hsub"></span></div><div class="queue"></div>
        <div class="box"><div class="atts"></div><textarea class="prompt" rows="1" placeholder="Peça algo… @ para arquivos, / para comandos"></textarea>
          <div class="brow"><button class="ib plusbtn" title="Anexar e mais">${I.plus}</button><span class="sp"></span><span class="ring" title="Uso do contexto"></span><button class="send" title="Enviar (Enter)">${I.up}</button></div></div>
        <div class="under"></div></div></div>
      <aside class="rail"></aside></div></section>`);
  v.el = el;
  el.addEventListener('mousedown', () => { const i = S.views.indexOf(v); if (i >= 0 && i !== S.focus) { S.focus = i; $$('.view').forEach((x, j) => x.classList.toggle('focus', j === i)); renderSidebar(); renderPanes(); } }, true);
  const title = $('.title', el);
  title.onclick = () => renameInline(v);
  $('.caret', el).onclick = e => sessionMenu(v, e.currentTarget);
  $$('[data-a]', el).forEach(b => b.onclick = e => {
    const a = b.dataset.a;
    if (a === 'rail') { S.rail = !S.rail; lsSet('rail', S.rail); S.views.forEach(renderView); }
    else if (a === 'more') moreMenu(v, e.currentTarget);
    else togglePane(a);
  });
  const P = $('.prompt', el);
  P.oninput = () => { grow(P); autocomplete(v); };
  P.onkeydown = e => composerKey(v, e);
  P.onpaste = e => { for (const it of e.clipboardData.items) if (it.type.startsWith('image/')) { const f = it.getAsFile(); readImage(v, f); e.preventDefault(); } };
  $('.send', el).onclick = () => { const L = v.sid && S.live[v.sid]; if (L && busy(L) && !P.value.trim()) stop(v); else send(v); };
  $('.plusbtn', el).onclick = e => plusMenu(v, e.currentTarget);
  $('.ring', el).onclick = () => contextDialog(v);
}

const busy = L => !!(L?.snap && L.snap.state !== 'idle' && L.snap.state !== 'error');
function grow(P) { P.style.height = 'auto'; P.style.height = Math.min(P.scrollHeight, 300) + 'px'; }

function renderView(v) {
  const el = v.el; if (!el) return;
  const L = v.sid ? S.live[v.sid] : null;
  el.classList.toggle('home', !L);
  el.classList.toggle('norail', !S.rail);
  if (L?.snap) {
    $('.title', el).textContent = L.snap.title || t('Nova sessão');
    $('.branch', el).innerHTML = L.snap.branch ? `${L.snap.worktree ? '⑂ ' : ''}${esc(L.snap.branch)}` : '';
    $('.branch', el).title = L.snap.worktree ? t('Worktree: ') + L.snap.worktree : '';
  } else {
    $('.hsub', el).textContent = v.draft.root ? v.draft.root : t('Escolha a pasta do projeto abaixo da caixa de texto.');
  }
  renderChat(v); renderComposer(v); renderRail(v); renderChangesBadge(v);
}

// ---------------------------------------------------------------- sidebar
function stateClass(st) { return !st || st === 'idle' ? '' : st === 'error' ? 'err' : st.startsWith('waiting') ? 'wait' : 'busy'; }
async function loadSessions() {
  const [list, projects] = await Promise.all([api('/api/sessions/all', { root: '' }), api('/api/projects', { root: '' })]);
  S.sessions = Array.isArray(list) ? list : []; S.projects = Array.isArray(projects) ? projects : [];
  renderSidebar();
}
function projectLabel(root) { const p = S.projects.find(x => x.root === root); return p?.label || baseName(root); }
function renderSidebar() {
  const q = $('#search').value.trim().toLowerCase();
  const el = $('#sessList'); el.innerHTML = '';
  $('#navScheduled').classList.toggle('on', S.page === 'scheduled'); $('#navCustomize').classList.toggle('on', S.page === 'customize');
  const archivedMode = S.sb.show === 'archived';
  $('#sbHeadLabel').textContent = t(archivedMode ? 'Arquivadas' : 'Sessões');
  if (q.length >= 2 && S.hitsQ !== q) searchTranscripts(q); else if (!q) { S.hits = {}; S.hitsQ = ''; }
  let list = (archivedMode ? S.archived : S.sessions).filter(s => !q || S.hits[s.id] || (s.title || '').toLowerCase().includes(q) || (s.root || '').toLowerCase().includes(q) || (s.branch || '').toLowerCase().includes(q));
  const sortKey = S.sb.sort; list.sort((a, b) => sortKey === 'title' ? (a.title || '').localeCompare(b.title || '') : (b[sortKey] || 0) - (a[sortKey] || 0));
  const openIds = S.views.map(v => v.sid);
  const row = s => {
    const L = S.live[s.id]; const st = L?.snap?.state || s.state;
    const hit = q && S.hits[s.id] && S.hits[s.id] !== s.title ? S.hits[s.id] : '';
    const r = h(`<div class="srow${openIds[S.focus] === s.id ? ' on' : ''}${openIds.includes(s.id) && openIds[S.focus] !== s.id ? ' split' : ''}${S.unread.has(s.id) ? ' unread' : ''}${hit ? ' hit' : ''}" draggable="true"><span class="st ${stateClass(st)}"></span><span class="t"><span class="tt">${s.pinned ? '<span class="pin">📌</span>' : ''}${esc(L?.snap?.title || s.title || t('Nova sessão'))}</span>${hit ? `<span class="snip">${esc(hit)}</span>` : ''}</span><span class="w">${ago(s.updated)}</span><button class="ib more act" data-act="${archivedMode ? 'delete' : 'archive'}" title="${esc(t(archivedMode ? 'Excluir' : 'Arquivar'))}">${archivedMode ? I.trash : I.archive}</button><button class="ib more" title="Mais">${I.more}</button></div>`);
    r.title = (s.branch ? s.branch + ' · ' : '') + s.root;
    r.onclick = e => { const act = e.target.closest('[data-act]'); if (act) { e.stopPropagation(); return act.dataset.act === 'delete' ? deleteSession(s) : archiveSession(s); } if (e.target.closest('.more')) return sessionRowMenu(s, e.target.closest('.more')); if (archivedMode) return; if (e.altKey) openInSplit(s.root, s.id); else openSession(s.root, s.id); };
    r.oncontextmenu = e => { e.preventDefault(); sessionRowMenu(s, null, e); };
    r.ondragstart = e => e.dataTransfer.setData('text/infuse-session', JSON.stringify({ root: s.root, id: s.id }));
    return r;
  };
  const pinned = archivedMode ? [] : list.filter(s => s.pinned);
  if (pinned.length) { const g = h(`<div class="grp pinned"><div class="gh"><span class="trunc">${t('Fixadas')}</span></div></div>`); pinned.forEach(s => g.appendChild(row(s))); el.appendChild(g); list = list.filter(s => !s.pinned); }
  if (S.sb.group === 'folder' && !archivedMode) {
    const roots = S.projects.map(p => p.root);
    for (const s of list) if (!roots.includes(s.root)) roots.push(s.root);
    for (const root of roots) {
      const items = list.filter(s => s.root === root);
      if (q && !items.length && !root.toLowerCase().includes(q)) continue;
      const g = h(`<div class="grp${S.sb.closed[root] && !q ? ' closed' : ''}" draggable="true"><div class="gh"><span class="chev">▼</span><span class="trunc">${esc(projectLabel(root))}</span><span class="acts"><button class="ib" data-a="new" title="Nova sessão nesta pasta">${I.plus}</button><button class="ib" data-a="menu" title="Mais">${I.more}</button></span></div></div>`);
      g.title = root;
      $('.gh', g).onclick = e => {
        const a = e.target.closest('[data-a]')?.dataset.a;
        if (a === 'new') return newSessionScreen(root);
        if (a === 'menu') return projectMenu(root, e.target.closest('button'));
        S.sb.closed[root] = !S.sb.closed[root]; lsSet('sb', S.sb); renderSidebar();
      };
      $('.gh', g).oncontextmenu = e => { e.preventDefault(); projectMenu(root, null, e); };
      g.ondragstart = e => { if (e.target === g) e.dataTransfer.setData('text/infuse-project', root); };
      g.ondragover = e => { if (e.dataTransfer.types.includes('text/infuse-project')) { e.preventDefault(); g.classList.add('drop'); } };
      g.ondragleave = () => g.classList.remove('drop');
      g.ondrop = e => { g.classList.remove('drop'); const from = e.dataTransfer.getData('text/infuse-project'); if (!from || from === root) return; e.preventDefault(); const l = S.projects.slice(); const i = l.findIndex(x => x.root === from), j = l.findIndex(x => x.root === root); if (i < 0 || j < 0) return; const [m] = l.splice(i, 1); l.splice(j, 0, m); api('/api/projects', { method: 'PUT', body: { projects: l }, root: '' }).then(loadSessions); };
      items.forEach(s => g.appendChild(row(s)));
      if (!items.length) g.appendChild(h(`<div class="srow" style="color:var(--text3);font-size:12px;cursor:default">${t('Nenhuma sessão')}</div>`));
      el.appendChild(g);
    }
  } else list.forEach(s => el.appendChild(row(s)));
  if (!S.projects.length && !list.length) el.appendChild(h(`<div class="empty">${t('Nenhuma sessão ainda.')}<br>${t('Clique em Nova sessão e escolha a pasta do projeto.')}</div>`));
  if (archivedMode) {
    const bar = h(`<div class="row" style="padding:2px 4px 6px"><button class="btn sm back">← ${t('Voltar às sessões')}</button><span class="grow"></span>${list.length ? `<button class="btn sm danger delall">${t('Excluir todas')}</button>` : ''}</div>`);
    $('.back', bar).onclick = () => { S.sb.show = ''; lsSet('sb', S.sb); renderSidebar(); };
    if ($('.delall', bar)) $('.delall', bar).onclick = async () => { if (!confirm(t('Excluir todas as sessões arquivadas e seus checkpoints? Isto não pode ser desfeito.'))) return; for (const x of list) await api(`/api/sessions/${x.id}`, { method: 'DELETE', root: x.root }); S.archived = await api('/api/sessions/all?archived=1', { root: '' }); renderSidebar(); toast('Sessões excluídas'); };
    el.prepend(bar);
  }
}
$('#search').oninput = renderSidebar;
$('#btnGroup').onclick = e => menu(e.currentTarget, [
  { h: 'Agrupar por' }, { label: 'Pasta', check: S.sb.group === 'folder', f: () => setSb('group', 'folder') }, { label: 'Nenhum', check: S.sb.group === 'none', f: () => setSb('group', 'none') },
  { h: 'Ordenar por' }, { label: 'Atividade recente', check: S.sb.sort === 'updated', f: () => setSb('sort', 'updated') }, { label: 'Criação', check: S.sb.sort === 'created', f: () => setSb('sort', 'created') }, { label: 'Título', check: S.sb.sort === 'title', f: () => setSb('sort', 'title') },
  { sep: 1 }, { label: 'Ver arquivadas', f: async () => { S.archived = await api('/api/sessions/all?archived=1', { root: '' }); setSb('show', 'archived'); } },
]);
function setSb(k, v) { S.sb[k] = v; lsSet('sb', S.sb); renderSidebar(); }

function sessionRowMenu(s, anchor, ev) {
  const items = s.archived ? [
    { label: 'Desarquivar', f: async () => { await api(`/api/session/${s.id}/unarchive`, { body: {}, root: s.root }); S.archived = await api('/api/sessions/all?archived=1', { root: '' }); await loadSessions(); } },
    { label: 'Excluir', danger: 1, f: () => deleteSession(s) },
  ] : [
    { label: 'Abrir', f: () => openSession(s.root, s.id) },
    { label: 'Abrir em', items: [{ label: 'Tela dividida', d: 'Alt+clique', f: () => openInSplit(s.root, s.id) }, { sep: 1 }, ...openInItems(s.root)] },
    { label: 'Renomear', f: () => { openSession(s.root, s.id).then(() => renameInline(V())); } },
    { label: s.pinned ? 'Desafixar' : 'Fixar no topo', f: async () => { await api(`/api/session/${s.id}/pin`, { body: { pinned: !s.pinned }, root: s.root }); await loadSessions(); } },
    { label: S.unread.has(s.id) ? 'Marcar como lida' : 'Marcar como não lida', f: () => setUnread(s.id, !S.unread.has(s.id)) },
    { sep: 1 }, { label: 'Arquivar', f: () => archiveSession(s) }, { label: 'Excluir', danger: 1, f: () => deleteSession(s) },
  ];
  menu(anchor, items, ev);
}
function openInItems(path) {
  return [{ label: native ? 'Mostrar no Explorador' : 'Mostrar na pasta', f: () => api('/api/open', { body: { path, app: 'explorer' }, root: '' }) },
    ...S.editors.map(e => ({ label: t('Abrir no ') + e.name, f: () => api('/api/open', { body: { path, app: e.id }, root: '' }) }))];
}
function projectMenu(root, anchor, ev) {
  menu(anchor, [
    { label: 'Nova sessão nesta pasta', f: () => newSessionScreen(root) },
    { label: 'Renomear projeto', f: () => { const g = $$('.grp').find(x => x.title === root); const span = g && $('.gh .trunc', g); inlineEdit(span, projectLabel(root), v => api('/api/projects', { method: 'PUT', body: { projects: S.projects.map(p => p.root === root ? { ...p, label: v } : p) }, root: '' }).then(loadSessions)); } },
    { label: 'Abrir em', items: openInItems(root) }, { sep: 1 },
    { label: 'Remover da lista', danger: 1, f: () => api('/api/projects/remove', { body: { root }, root: '' }).then(loadSessions) },
  ], ev);
}
async function archiveSession(s, force) {
  const r = await api(`/api/session/${s.id}/archive`, { body: { force: !!force }, root: s.root });
  if (r.dirty) { if (confirm(t('O worktree desta sessão tem alterações não commitadas. Arquivar mesmo assim?'))) return archiveSession(s, true); return; }
  S.views.forEach(v => { if (v.sid === s.id) { v.sid = ''; v.draft.root = s.root; } });
  closeLive(s.id); await loadSessions(); mountViews(); renderPanes(); toast('Sessão arquivada');
}
async function deleteSession(s) {
  if (!confirm(t('Excluir esta sessão e seus checkpoints? Isto não pode ser desfeito.'))) return;
  await api(`/api/sessions/${s.id}`, { method: 'DELETE', root: s.root });
  S.views.forEach(v => { if (v.sid === s.id) { v.sid = ''; v.draft.root = s.root; } });
  closeLive(s.id); await loadSessions(); if (S.sb.show === 'archived') S.archived = await api('/api/sessions/all?archived=1', { root: '' }); renderSidebar(); mountViews(); renderPanes();
}

// ---------------------------------------------------------------- inline editing
function inlineEdit(el, current, apply) {
  if (!el) return; const inp = document.createElement('input'); inp.value = current || ''; inp.className = el.className; inp.style.minWidth = '180px';
  el.replaceWith(inp); inp.focus(); inp.select();
  let done = false; const fin = ok => { if (done) return; done = true; const val = inp.value.trim(); inp.replaceWith(el); if (ok && val && val !== current) apply(val); };
  inp.onkeydown = e => { e.stopPropagation(); if (e.key === 'Enter') fin(true); if (e.key === 'Escape') fin(false); }; inp.onblur = () => fin(true); inp.onclick = e => e.stopPropagation();
}
function renameInline(v) {
  const L = v.sid && S.live[v.sid]; if (!L) return;
  inlineEdit($('.title', v.el), L.snap.title, val => setSessionOpts(v, { title: val }));
}

// ---------------------------------------------------------------- menus
// Menus nest: an item with `items` opens a submenu beside it (hover or click); openMenu is the deepest.
let openMenu = null; const menuStack = [];
function closeMenu(level = 0) { while (menuStack.length > level) menuStack.pop().remove(); openMenu = menuStack[menuStack.length - 1] || null; }
function menu(anchor, items, ev, opts = {}) {
  const level = opts.level || 0;
  closeMenu(level);
  // A menu is head (optional fixed title) + scrolling body + foot (items marked foot:true, always visible).
  const m = h('<div class="menu"><div class="mhead"></div><div class="mbody"></div><div class="mfoot"></div></div>'); let n = 0;
  const head = $('.mhead', m), body = $('.mbody', m), foot = $('.mfoot', m);
  for (const it of items) {
    if (!it) continue;
    const dest = it.foot ? foot : body;
    if (it.sep) { if (!it.foot) body.appendChild(h('<div class="ms"></div>')); continue; }
    if (it.h) { (it.head ? head : dest).appendChild(h(`<div class="mh${it.plain ? ' plain' : ''}" title="${esc(it.h)}">${esc(t(it.h))}</div>`)); continue; }
    if (it.html) { dest.appendChild(it.html); continue; }
    n++;
    const row = h(`<div class="mi${it.danger ? ' danger' : ''}${it.sub ? ' col' : ''}${it.items ? ' hassub' : ''}">${it.check !== undefined ? `<span class="ck">${it.check ? '✓' : ''}</span>` : ''}${it.icon || ''}<span class="two"><span>${esc(t(it.label))}</span>${it.sub ? `<small>${esc(t(it.sub))}</small>` : ''}</span>${it.items ? '<span class="d">›</span>' : it.d ? `<span class="d">${esc(t(it.d))}</span>` : n <= 9 && opts.numbers ? `<span class="d">${n}</span>` : ''}</div>`);
    if (it.items) {
      const open = () => { if (row.classList.contains('k') && menuStack[level + 1]) return; $$('.mi', m).forEach(x => x.classList.toggle('k', x === row)); menu(row, it.items, null, { level: level + 1, side: true }); };
      row.onmouseenter = () => { clearTimeout(m._t); m._t = setTimeout(open, 120); };
      row.onclick = e => { e.stopPropagation(); open(); };
    } else {
      row.onmouseenter = () => { clearTimeout(m._t); m._t = setTimeout(() => { $$('.mi', m).forEach(x => x.classList.remove('k')); closeMenu(level + 1); }, 120); };
      row.onclick = e => { e.stopPropagation(); if (!it.keep) closeMenu(); it.f && it.f(); };
    }
    row.dataset.n = n;
    dest.appendChild(row);
  }
  document.body.appendChild(m); menuStack.push(m); openMenu = m;
  let x, y, above = !!opts.above;
  if (ev) { x = ev.clientX; y = ev.clientY; m.style.maxHeight = (innerHeight - 12) + 'px'; }
  else if (opts.side) {
    // submenu: to the right of its row (to the left when there is no room), top aligned
    const r = anchor.getBoundingClientRect(); m.style.maxHeight = (innerHeight - 12) + 'px';
    x = r.right + 2 + m.offsetWidth > innerWidth ? r.left - m.offsetWidth - 2 : r.right + 2; y = r.top - 5;
  }
  else {
    // fit the menu in the space actually available beside the anchor; open the other way if that side is roomier
    const r = anchor.getBoundingClientRect(); const spaceAbove = r.top - 12, spaceBelow = innerHeight - r.bottom - 12;
    if (above && spaceAbove < Math.min(m.offsetHeight, 260) && spaceBelow > spaceAbove) above = false;
    if (!above && spaceBelow < Math.min(m.offsetHeight, 260) && spaceAbove > spaceBelow) above = true;
    m.style.maxHeight = Math.max(160, above ? spaceAbove : spaceBelow) + 'px';
    x = opts.alignRight ? r.right - m.offsetWidth : r.left; y = above ? r.top - m.offsetHeight - 6 : r.bottom + 4;
  }
  x = Math.max(6, Math.min(x, innerWidth - m.offsetWidth - 6)); y = Math.max(6, Math.min(y, innerHeight - m.offsetHeight - 6));
  m.style.left = x + 'px'; m.style.top = y + 'px';
  return m;
}
addEventListener('mousedown', e => { if (menuStack.length && !menuStack.some(m => m.contains(e.target))) closeMenu(); });

function sessionMenu(v, anchor) {
  const L = S.live[v.sid]; if (!L) return;
  const modes = [['normal', 'Normal', 'Ferramentas resumidas, texto completo'], ['thinking', 'Pensamento', 'Também mostra o raciocínio'], ['verbose', 'Detalhado', 'Cada chamada de ferramenta aberta']];
  // grouped like the desktop app: a few direct actions, the rest one level down
  menu(anchor, [
    { label: 'Exibição', items: [
      ...modes.map(([k, l, d]) => ({ label: l, sub: d, check: S.viewMode === k, f: () => setViewMode(k) })),
      { sep: 1 }, { label: 'Expandir tudo', f: () => foldAll(v, true) }, { label: 'Recolher tudo', f: () => foldAll(v, false) },
      { label: S.settings.showThinking === false ? 'Mostrar raciocínio' : 'Ocultar raciocínio', f: () => { saveSetting({ showThinking: S.settings.showThinking === false }); for (const k of Object.keys(S.fold)) if (k.includes(':')) delete S.fold[k]; S.views.forEach(renderChat); } },
    ] },
    { label: 'Copiar', items: [
      { label: 'Revisão independente', sub: 'Um revisor com contexto limpo confere as alterações', f: () => runReview(v) },
      { label: 'Iniciar missão…', sub: 'Trabalhar sozinho até os critérios passarem', f: () => missionDialog(v) },
      { label: 'Copiar conversa', sub: 'Markdown, com ferramentas e raciocínio', f: () => copyText(transcriptMarkdown(L), 'Conversa copiada') },
      { label: 'Copiar última resposta', f: () => { const m = [...L.snap.messages].reverse().find(x => x.role === 'assistant' && msgText(x)); if (m) copyText(msgText(m), 'Resposta copiada'); } },
      { label: 'Exportar conversa…', sub: 'Salvar como arquivo .md', f: () => exportTranscript(L) },
    ] },
    { label: 'Abrir em', items: [
      { label: S.views.length > 1 ? 'Sair da tela dividida' : 'Tela dividida', f: () => S.views.length > 1 ? exitSplit() : openInSplit(L.root, L.id, true) },
      { sep: 1 }, ...openInItems(L.snap.workRoot || L.root),
    ] },
    { sep: 1 }, { label: 'Renomear', f: () => renameInline(v) },
    { label: 'Voltar no tempo…', d: 'Esc Esc', f: () => rewindDialog(v) },
    { label: 'Pergunta paralela', d: 'Ctrl ;', f: () => openSide(v) },
    { sep: 1 }, { label: 'Arquivar', f: () => archiveSession({ id: L.id, root: L.root }) }, { label: 'Excluir', danger: 1, f: () => deleteSession({ id: L.id, root: L.root }) },
  ]);
}
function moreMenu(v, anchor) {
  menu(anchor, [
    { label: 'Tarefas', f: () => togglePane('tasks') }, { label: 'Plano', f: () => togglePane('plan') },
    { label: 'Filtro', items: [{ label: 'Pronto (critérios)', f: () => togglePane('oracle') }, { label: 'Memória', f: () => togglePane('memory') }, { label: 'Regras', f: () => togglePane('rules') }, { label: 'Jornal', f: () => togglePane('journal') }, { label: 'Vícios', f: () => togglePane('vices') }, { label: 'Mapa do projeto', f: () => togglePane('map') }] },
    { sep: 1 }, { label: S.rail ? 'Ocultar detalhes da sessão' : 'Mostrar detalhes da sessão', f: () => { S.rail = !S.rail; lsSet('rail', S.rail); S.views.forEach(renderView); } },
  ], null, { alignRight: true });
}
function foldAll(v, open) { $$('[data-fold]', v.el).forEach(x => { S.fold[x.dataset.fold] = open; x.parentNode.classList.toggle('open', open); }); }
function setViewMode(k) { S.fold = {}; S.viewMode = k; lsSet('viewMode', k); S.views.forEach(renderChat); toast(t('Exibição: ') + t({ normal: 'Normal', thinking: 'Pensamento', verbose: 'Detalhado' }[k])); }

// ---------------------------------------------------------------- live sessions (SSE)
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
    case 'tool_result': L.running = null; L.snap.messages.push({ role: 'user', content: [{ type: 'tool_result', tool_use_id: d.id, content: d.output, is_error: d.isError, ms: d.ms }], ts: Date.now() }); vs.forEach(renderChat); refreshChanges(L); break;
    case 'permission': L.snap.pendingPermission = d; vs.forEach(renderChat); attention(L, 'infuse precisa de você', t('Aprovar ') + d.tool + '?'); break;
    case 'question': L.snap.pendingQuestion = d; vs.forEach(renderChat); attention(L, 'infuse tem uma pergunta', d.question); break;
    case 'done': L.snap.pendingPermission = null; L.snap.pendingQuestion = null; L.snap.cost = d.cost; L.snap.usage = d.usage; re(); vs.forEach(renderRail); attention(L, 'infuse terminou', L.snap.title || ''); if (!S.views.some(x => x.sid === L.id) || !document.hasFocus()) setUnread(L.id, true); refreshChanges(L); loadUsage(); if (!d.reason) suggestNext(L); break;
    case 'error': L.snap.error = d; L.snap.pendingPermission = null; L.snap.pendingQuestion = null; re(); attention(L, 'infuse: erro', String(d)); break;
    case 'tasks': L.snap.tasks = d; vs.forEach(renderRail); paneRefresh('tasks'); break;
    case 'memory': paneRefresh('memory'); break;
    case 'gate': toast(t('infuse interveio: ') + String(d.message || '').replace(/^\[infuse\] /, '').split('\n')[0].slice(0, 140)); break;
    case 'subagent': L.sub = d.state === 'done' ? null : { ...(L.sub || {}), ...d, steps: (L.sub?.steps || 0) + (d.state === 'tool' ? 1 : 0) }; vs.forEach(renderLive); break;
    case 'mission': L.snap.mission = d; vs.forEach(renderRail); break;
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

// ---------------------------------------------------------------- transcript
function renderChat(v) {
  if (!v.el) return;
  const chat = $('.chat', v.el); const L = v.sid && S.live[v.sid];
  if (!L?.snap) { chat.innerHTML = ''; return; }
  const s = L.snap, msgs = s.messages, results = {};
  for (const m of msgs) for (const b of m.content || []) if (b.type === 'tool_result') results[b.tool_use_id] = b;
  const starts = new Set(s.turnStarts); let turn = 0; let out = '';
  msgs.forEach((m, i) => {
    if (starts.has(i)) { turn++; out += `<div class="turnsep" data-turn="${turn}"><span class="l"></span><button class="btn sm" data-rw="${turn}" title="${esc(t('Restaurar código e/ou conversa a antes deste turno'))}">↶ ${t('voltar aqui')}</button></div>`; }
    if (m.role === 'user') {
      if (m.content.some(b => b.type === 'tool_result')) return; // tool results (and images tools attached) are shown in their tool blocks
      const texts = m.content.filter(b => b.type === 'text'), imgs = m.content.filter(b => b.type === 'image');
      if (!texts.length && !imgs.length) return;
      const main = texts[0]?.text || '';
      if (main.startsWith('[Revisão independente]')) { out += reviewBlock(main.slice(22).trim(), i); return; }
      if (main.startsWith('[Conversa resumida]') || main.startsWith('[Contexto compactado')) { out += `<div class="marker" title="${esc(main.slice(0, 600))}">${t('Conversa resumida')}</div>`; return; }
      const attached = texts.slice(1).map(b => (b.text.match(/path="([^"]+)"/) || [])[1]).filter(Boolean);
      const mission = main.startsWith('[Missão]') ? main.slice(main.indexOf('Objetivo: ') + 10) : null;
      out += `<div class="umsg${(s.pinnedMsgs || []).includes(i) ? ' pinned' : ''}" data-mi="${i}"><div class="u sel">${mission != null ? `<span class="tag warn" style="margin-right:6px">${t('Missão')}</span>${esc(mission)}` : esc(main)}${attached.length ? `<div class="att">${attached.map(p => `<span class="chipx" data-path="${esc(p)}">📎 <span>${esc(baseName(p))}</span></span>`).join('')}</div>` : ''}${imgs.map(b => `<img src="data:${b.mediaType};base64,${b.data}">`).join('')}</div>${msgBar(s, m, i, true)}</div>`;
    } else {
      if (m.content.length === 2 && m.content[0]?.text === 'Entendido, continuo a partir desse estado.') return;
      if (msgText(m) === 'Revisão recebida.') return;
      let inner = '';
      for (const [bi, b] of m.content.entries()) {
        if (b.type === 'text' && b.text?.trim()) inner += `<div class="md sel">${md(b.text.trim())}</div>`;
        else if (b.type === 'thinking' && b.thinking?.trim()) inner += thinkBlock(`${v.sid}:${i}:${bi}`, b.thinking, false, b.ms);
        else if (b.type === 'tool_use') inner += toolBlock(b.id, b.name, b.input, results[b.id], false);
      }
      if (inner) out += `<div class="a${(s.pinnedMsgs || []).includes(i) ? ' pinned' : ''}" data-mi="${i}">${inner}${m.content.some(b => b.type === 'text' && b.text?.trim()) ? msgFoot(m, i) : ''}</div>`; // a message with nothing visible leaves no gap
    }
    const next = i + 1;
    if (turn && (starts.has(next) || (next === msgs.length && !['thinking', 'tool', 'waiting_permission', 'waiting_user', 'compacting'].includes(s.state)))) out += turnSummary(s, msgs, s.turnStarts[turn - 1], i);
  });
  out += '<div class="live"></div>';
  if (s.pendingPermission) out += permCard(s.pendingPermission);
  if (s.pendingQuestion) out += questionCard(s.pendingQuestion);
  if (s.error && s.state === 'error') out += `<div class="errbox">${esc(s.error)}</div>`;
  if (!msgs.length) out = `<div class="empty" style="padding-top:14vh">${t('Sessão vazia. Descreva a feature, a correção ou a dúvida.')}</div>` + out;
  const sc = $('.scroller', v.el); if (v._jump !== v.sid) v.stick = true;
  chat.innerHTML = out; bindChat(v, chat); renderLive(v);
  if (v.stick !== false) toBottom(v); v._jump = v.sid; renderToc(v); updateJump(v);
}
const fmtClock = ts => new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
function fmtDur(ms) { if (ms == null) return ''; if (ms < 1000) return ms + ' ms'; const s = ms / 1000; if (s < 60) return (s < 10 ? s.toFixed(1) : Math.round(s)) + ' s'; const m = Math.floor(s / 60); return `${m}m ${Math.round(s % 60)}s`; }
const fmtTok = n => n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k' : String(n || 0);
// Under each answer: when it arrived, how long the model took, which model, how many tokens; copy on hover.
// Date and time of a message: "hoje, 14:32", "ontem, 09:10", "31 out, 18:02"; the full date and how long ago on hover.
function fmtWhen(ts) {
  const d = new Date(ts), now = new Date(), day = 86400000;
  const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const hm = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  if (ts >= midnight) return t('hoje') + ', ' + hm;
  if (ts >= midnight - day) return t('ontem') + ', ' + hm;
  return d.toLocaleDateString([], { day: '2-digit', month: 'short', ...(d.getFullYear() !== now.getFullYear() ? { year: 'numeric' } : {}) }) + ', ' + hm;
}
function agoText(ts) { const s = Math.round((Date.now() - ts) / 1000); if (s < 60) return t('agora há pouco'); const m = Math.round(s / 60); if (m < 60) return `${t('há')} ${m} min`; const h = Math.round(m / 60); if (h < 24) return `${t('há')} ${h} h`; return `${t('há')} ${Math.round(h / 24)} ${t('dias')}`; }
// The actions under a message, as in Claude Code: copy, (edit,) branch from here, pin, read aloud — and when.
function msgBar(s, m, i, user) {
  const pinned = (s.pinnedMsgs || []).includes(i);
  const when = m.ts ? `<span class="when" title="${esc(new Date(m.ts).toLocaleString() + ' · ' + agoText(m.ts))}">${fmtWhen(m.ts)}</span>` : '';
  return `<div class="mbar${user ? ' u' : ''}"><span class="acts"><button class="ib" data-copy-msg="${i}" title="${esc(t('Copiar'))}">${I.copy}</button>${user && turnOf(s, i) ? `<button class="ib" data-rw="${turnOf(s, i)}" title="${esc(t('Editar e refazer a partir daqui'))}">↶</button>` : ''}<button class="ib" data-fork="${i}" title="${esc(t('Nova sessão a partir daqui'))}">${I.branch}</button><button class="ib${pinned ? ' on' : ''}" data-pinmsg="${i}" title="${esc(t(pinned ? 'Desafixar' : 'Fixar mensagem'))}">${I.pin}</button><button class="ib" data-speak="${i}" title="${esc(t('Ler em voz alta'))}">${I.speak}</button></span>${when}</div>`;
}
function msgFoot(m, i) {
  const meta = [m.ts && `<span title="${esc(new Date(m.ts).toLocaleString() + ' · ' + agoText(m.ts))}">${fmtWhen(m.ts)}</span>`, m.ms && `<span title="${esc(t('tempo de resposta do modelo'))}">${fmtDur(m.ms)}</span>`, m.model && `<span>${esc(m.model)}</span>`, m.tokens && `<span>${fmtTok(m.tokens)} tokens</span>`].filter(Boolean).join('<span class="dotsep">·</span>');
  const s = curL()?.snap || {}; const pinned = (s.pinnedMsgs || []).includes(i);
  return `<div class="afoot"><span class="meta">${meta}</span><span class="acts"><button class="ib" data-copy-msg="${i}" title="${esc(t('Copiar resposta'))}">${I.copy}</button><button class="ib" data-fork="${i}" title="${esc(t('Nova sessão a partir daqui'))}">${I.branch}</button><button class="ib${pinned ? ' on' : ''}" data-pinmsg="${i}" title="${esc(t(pinned ? 'Desafixar' : 'Fixar mensagem'))}">${I.pin}</button><button class="ib" data-speak="${i}" title="${esc(t('Ler em voz alta'))}">${I.speak}</button></span></div>`;
}
// End of a turn: how long the agent worked, tools used, tokens; copy the turn's answers.
function turnSummary(s, msgs, from, to) {
  if (from == null) return '';
  const first = msgs[from], last = msgs[to]; let tools = 0, tokens = 0, texts = 0;
  for (let k = from; k <= to; k++) { const m = msgs[k]; if (m.role !== 'assistant') continue; tokens += m.tokens || 0; for (const b of m.content || []) { if (b.type === 'tool_use') tools++; if (b.type === 'text' && b.text?.trim()) texts++; } }
  if (!texts && !tools) return '';
  const dur = first?.ts && last?.ts ? last.ts - first.ts : null;
  const bits = [dur != null && `${t('Trabalhou por')} ${fmtDur(dur)}`, tools && `${tools} ${t(tools === 1 ? 'ferramenta' : 'ferramentas')}`, tokens && `${fmtTok(tokens)} tokens`].filter(Boolean).join('<span class="dotsep">·</span>');
  return `<div class="turnsum"><span class="ok">✓</span><span class="meta">${bits}</span><span class="grow"></span>${texts ? `<button class="btn sm ghost" data-copy-turn="${from}:${to}" title="${esc(t('Copiar todas as respostas deste turno'))}">${I.copy} ${t('Copiar')}</button>` : ''}</div>`;
}
// Independent review: a reviewer with a clean context reads the diff, the plan and the criteria.
function reviewBlock(text, i) {
  return `<div class="report review"><div class="rh"><b>${t('Revisão independente')}</b><span class="faint">${t('um revisor com contexto limpo leu o diff, o plano e os critérios')}</span><span class="grow"></span><button class="ib" data-copy-msg="${i}" title="${esc(t('Copiar'))}">${I.copy}</button></div><div class="md sel">${md(text)}</div><div class="rnext"><span class="grow faint">${t('O agente vê estes achados na próxima mensagem.')}</span><button class="btn sm pri" data-fixreview>${t('Pedir correções')}</button></div></div>`;
}
// The plan, live: each task with its state (done, now, next, blocked) and how it will be proven.
const PLAN_ICON = { done: ['✓', 'ok'], in_progress: ['', 'now'], blocked: ['✗', 'bad'], pending: ['', ''], stopped: ['‖', 'stop'] };
function planCard(L) {
  const tasks = L.snap.tasks || []; if (!tasks.length) return '';
  const done = tasks.filter(x => x.status === 'done').length, pct = Math.round(done / tasks.length * 100);
  const prev = L._planPrev || {}; L._planPrev = Object.fromEntries(tasks.map(x => [x.id, x.status]));
  // only a running agent has a task "now"; once it stops, an unfinished task shows as stopped
  const live = busy(L), open = tasks.filter(x => x.status !== 'done' && x.status !== 'blocked').length;
  const head = !live && open ? `<span class="r warn">${t('parado')} · ${done} ${t('de')} ${tasks.length}</span>` : `<span class="r">${done} ${t('de')} ${tasks.length}</span>`;
  return `<div class="card plan${live ? '' : ' idle'}" data-card="plan"><div class="ch">${t('Plano')}${head}</div><div class="bar100 pbar"><i style="width:${pct}%"></i></div>
    ${tasks.map(x => { const st = x.status === 'in_progress' && !live ? 'stopped' : x.status; const [ic, cls] = PLAN_ICON[st] || PLAN_ICON.pending; const just = x.status === 'done' && prev[x.id] && prev[x.id] !== 'done'; return `<div class="ptask ${st}${just ? ' just' : ''}"><span class="rst ${cls}">${ic}</span><span class="grow"><span class="tt">${esc(x.text)}</span>${x.proof ? `<span class="pf">${t('comprovação:')} <code>${esc(x.proof)}</code></span>` : ''}${x.note ? `<span class="pn">${esc(t(x.note))}</span>` : ''}</span></div>`; }).join('')}</div>`;
}
// Every card of the right column folds with its caret; the choice is remembered.
function railFolds(v, el) {
  const folds = lsGet('railFolds', {});
  $$('.card[data-card]', el).forEach(card => {
    const key = card.dataset.card, ch = $('.ch', card); if (!ch || $('.fold', ch)) return;
    ch.insertAdjacentHTML('afterbegin', `<span class="fold" title="${esc(t('Recolher ou expandir'))}">›</span>`);
    card.classList.toggle('folded', !!folds[key]);
    ch.addEventListener('click', e => { if (e.target.closest('button, a, .tag')) return; e.stopPropagation(); folds[key] = !folds[key]; lsSet('railFolds', folds); card.classList.toggle('folded', !!folds[key]); });
  });
}
// Missions: the agent keeps working until the Done? criteria pass, within a budget.
const MSTATE = { running: ['warn', 'em andamento'], aprovada: ['ok', 'aprovada'], orcamento: ['bad', 'orçamento esgotado'], impasse: ['bad', 'impasse'], parada: ['', 'parada'] };
function missionCard(L) {
  const m = L.snap.mission;
  if (!m) return `<div class="card"><div class="ch">${t('Missão')}</div><div class="faint" style="margin-bottom:6px">${t('O agente trabalha sozinho, turno após turno, até os critérios do Pronto? passarem — dentro de um orçamento.')}</div><button class="btn sm line mnew">${t('Iniciar missão')}</button></div>`;
  const spent = Math.max(0, (L.snap.cost || 0) - (m.startCost || 0)), turns = (m.steps || []).length + (m.state === 'running' ? 1 : 0);
  const [cls, label] = MSTATE[m.state] || ['', m.state]; const pct = Math.min(100, Math.round(spent / (m.budgetUSD || 1) * 100));
  const last = (m.steps || []).slice(-1)[0];
  return `<div class="card mission" data-card="mission"><div class="ch">${t('Missão')}<span class="r"><span class="tag ${cls}">${t(label)}</span></span></div>
    <div class="faint trunc" title="${esc(m.goal)}">${esc(m.goal)}</div>
    <div class="bar100" style="margin:6px 0 3px"><i style="width:${pct}%"></i></div><div class="faint">${t('turno')} ${Math.min(turns, m.maxTurns)}/${m.maxTurns} · ${money(spent)} ${t('de')} ${money(m.budgetUSD)}</div>
    ${m.noPrice && m.state === 'running' ? `<div class="faint" style="margin-top:4px">${t('Modelo sem preço na tabela: o orçamento em dólares não é medido, vale o limite de turnos (Configurações › Modelos e custos).')}</div>` : ''}
    ${m.reason ? `<div style="margin-top:6px">${esc(t(m.reason))}</div>` : ''}
    ${m.impasse ? `<div class="mimp"><b>${t('Impasse')}:</b> ${esc(m.impasse.reason)}${(m.impasse.tried || []).length ? `<div>${t('Tentou')}: ${m.impasse.tried.map(esc).join('; ')}</div>` : ''}${m.impasse.evidence ? `<div class="mono">${esc(m.impasse.evidence)}</div>` : ''}<div><b>${t('Precisa de')}:</b> ${esc(m.impasse.needs)}</div></div>` : ''}
    ${last?.auditor ? `<div class="maud" title="${esc(t('Auditor de fluxo: conselho do modelo rápido, não um veredito'))}">${esc(last.auditor)}</div>` : ''}
    ${(m.steps || []).length ? `<details class="odet"><summary>${t('Linha do tempo')} (${m.steps.length})</summary>${m.steps.map(st => `<div class="mstep"><span class="tag ${{ aprovado: 'ok', reprovado: 'bad' }[st.verdict] || 'warn'}">${esc(t(st.verdict || '—'))}</span><span class="faint">${t('turno')} ${st.turn} · ${new Date(st.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} · ${money(st.cost)}</span><div class="mono faint">${esc(st.summary || '')}</div>${st.auditor ? `<div class="maud">${esc(st.auditor)}</div>` : ''}</div>`).join('')}</details>` : ''}
    <div class="row" style="margin-top:6px">${m.state === 'running' ? `<button class="btn sm mstop">${t('Parar missão')}</button>` : `<button class="btn sm line mnew">${t('Nova missão')}</button>`}</div></div>`;
}
function missionDialog(v) {
  const L = v.sid && S.live[v.sid]; const P = $('.prompt', v.el);
  const d = h(`<div class="dlg" style="width:min(560px,92vw)"><h3 style="margin:0 0 4px">${t('Iniciar missão')}</h3><div class="faint" style="margin-bottom:10px">${t('O agente trabalha sozinho até os critérios do Pronto? passarem. Depois de cada turno o infuse roda os critérios numa cópia do projeto; se falharem, o agente recebe os motivos e continua. Ele pode declarar impasse quando não houver caminho.')}</div>
    <textarea class="doc mg" style="min-height:110px" placeholder="${esc(t('O objetivo. Ex.: a página de login autentica com e-mail e senha e mostra erro claro quando a senha está errada.'))}">${esc(P?.value || '')}</textarea>
    <div class="cs" style="padding:8px 0 0">${t('Gastar no máximo')} US$ <input class="slot mono mb" value="2.00"> ${t('e')} <input class="slot mono mt" value="10"> ${t('turnos')}.</div>
    <div class="mcrit faint" style="margin-top:6px"></div>
    <div class="row" style="margin-top:12px;justify-content:flex-end"><button class="btn cancel">${t('Cancelar')}</button><button class="btn pri go">${t('Iniciar')}</button></div></div>`);
  const close = overlay(d);
  api('/api/oracle/spec', { root: L?.root || curRoot() }).then(r => { const n = (r.spec?.checks || []).length; $('.mcrit', d).innerHTML = n ? `✓ ${n} ${t(n === 1 ? 'critério executável definido no Pronto?' : 'critérios executáveis definidos no Pronto?')}` : `<span class="del">${t('Nenhum critério executável no Pronto?: sem eles o infuse não tem como confirmar o fim.')}</span> <a href="#" class="defc">${t('Definir critérios')}</a>`; const dc = $('.defc', d); if (dc) dc.onclick = e => { e.preventDefault(); close(); togglePane('oracle', true); }; });
  $('.cancel', d).onclick = close;
  $('.go', d).onclick = async () => {
    const goal = $('.mg', d).value.trim(); if (!goal) return toast(t('Descreva o objetivo'));
    let L2 = L; if (!L2) { P.value = goal; await send(v); return close(); } // no session yet: a normal start
    const r = await api(`/api/session/${L2.id}/mission`, { body: { goal, budget: parseFloat($('.mb', d).value) || 2, maxTurns: parseInt($('.mt', d).value) || 10 }, root: L2.root });
    if (!r.ok) return toast(r.error); if (P) P.value = ''; close();
  };
  setTimeout(() => $('.mg', d).focus(), 30);
}
async function runReview(v, focus) {
  const L = v.sid && S.live[v.sid]; if (!L) return toast(t('Abra uma sessão para revisar'));
  if (busy(L)) return toast(t('Espere o agente terminar para revisar'));
  const st = $('.live', v.el); if (st) st.innerHTML = `<div class="status"><span class="spin"></span>${t('revisor independente lendo as alterações…')}</div>`;
  const r = await api(`/api/session/${L.id}/review`, { body: { focus: focus || '' }, root: L.root });
  if (!r.ok) { toast(r.error || t('Revisão falhou')); renderChat(v); }
}
// The element the user picked in the browser pane goes into the message being written.
if (window.infuse?.on) infuse.on('browserPick', p => insertPick(p));
function insertPick(p) {
  if (!p) return; const v = V(); if (!v?.el) return; const P = $('.prompt', v.el);
  const st = p.style ? `\n${t('Estilo')}: ${p.style.font} · ${t('cor')} ${p.style.color} · ${t('fundo')} ${p.style.background} · margin ${p.style.margin} · padding ${p.style.padding}` : '';
  P.value = (P.value.trim() ? P.value.trimEnd() + '\n\n' : '') + `[${t('Elemento selecionado em')} ${p.url}]\n${t('Seletor')}: ${p.selector}\n${t('Posição')}: ${p.rect.x},${p.rect.y} · ${p.rect.w}×${p.rect.h}${st}${p.text ? `\n${t('Texto')}: ${p.text}` : ''}\n\`\`\`html\n${p.html}\n\`\`\`\n`;
  grow(P); P.focus(); P.setSelectionRange(P.value.length, P.value.length); toast(t('Elemento adicionado à mensagem'));
}
// Scrolling: the conversation follows the end only while the user is there; scrolling up stops it.
function toBottom(v) { const sc = v.el && $('.scroller', v.el); if (!sc) return; v._prog = true; sc.scrollTop = sc.scrollHeight; }
function updateJump(v) {
  const sc = v.el && $('.scroller', v.el), b = v.el && $('.jumpdown', v.el); if (!sc || !b) return;
  const far = sc.scrollHeight - sc.scrollTop - sc.clientHeight > 120; const L = v.sid && S.live[v.sid];
  b.classList.toggle('show', far); b.classList.toggle('live', far && busy(L));
  b.style.bottom = ($('.composer', v.el)?.offsetHeight || 120) + 14 + 'px';
}
function bindScroll(v) {
  const sc = $('.scroller', v.el); if (!sc || sc._bound) return; sc._bound = true;
  sc.addEventListener('wheel', e => { if (e.deltaY < 0) v.stick = false; }, { passive: true });
  sc.addEventListener('scroll', () => {
    const dist = sc.scrollHeight - sc.scrollTop - sc.clientHeight;
    if (v._prog) v._prog = false; else if (dist > 24) v.stick = false;
    if (dist < 8) v.stick = true;
    updateJump(v); markToc(v);
  }, { passive: true });
  $('.jumpdown', v.el).onclick = () => { v.stick = true; sc.scrollTo({ top: sc.scrollHeight, behavior: 'smooth' }); };
}
// Chapters: one per turn, titled by what the user asked; hover lists them, a click goes to the exact point.
function chapterTitle(m) {
  let tx = msgText(m).trim();
  if (tx.startsWith('[Missão]')) { const k = tx.indexOf('Objetivo: '); tx = (k >= 0 ? t('Missão') + ': ' + tx.slice(k + 10) : t('Missão')); }
  else if (tx.startsWith('[Missão — ')) tx = tx.slice(1, tx.indexOf(']'));
  return tx.split('\n')[0].slice(0, 90) || t('(sem texto)');
}
function renderToc(v) {
  const L = v.sid && S.live[v.sid], toc = v.el && $('.toc', v.el); if (!toc) return; bindScroll(v);
  const s = L?.snap; const starts = s?.turnStarts || [];
  if (starts.length < 2) { toc.classList.add('hidden'); return; }
  toc.classList.remove('hidden');
  const items = starts.map((idx, k) => ({ turn: k + 1, title: chapterTitle(s.messages[idx] || { content: [] }), ts: s.messages[idx]?.ts }));
  $('.ticks', toc).innerHTML = items.slice(-14).map(x => `<i data-t="${x.turn}"></i>`).join('');
  $('.toclist', toc).innerHTML = `<div class="th">${t('Capítulos')}</div>` + items.map(x => `<div class="ti" data-t="${x.turn}"><span class="n">${x.turn}</span><span class="tt">${esc(x.title)}</span>${x.ts ? `<span class="tm">${fmtClock(x.ts)}</span>` : ''}</div>`).join('');
  $$('[data-t]', toc).forEach(el => el.onclick = () => { const sep = $(`.turnsep[data-turn="${el.dataset.t}"]`, v.el); if (sep) { v.stick = false; sep.scrollIntoView({ behavior: 'smooth', block: 'start' }); } });
  markToc(v);
}
function markToc(v) {
  const sc = $('.scroller', v.el), toc = $('.toc', v.el); if (!sc || !toc) return;
  const top = sc.getBoundingClientRect().top + 40; let cur = 1;
  for (const sep of $$('.turnsep', sc)) { if (sep.getBoundingClientRect().top <= top) cur = +sep.dataset.turn; else break; }
  $$('[data-t]', toc).forEach(el => el.classList.toggle('on', +el.dataset.t === cur));
}
// Read aloud with the system's voices; a second click stops.
function speakMsg(btn, text) {
  if (!('speechSynthesis' in window)) return toast(t('Este navegador não tem leitura em voz alta'));
  if (speechSynthesis.speaking) { speechSynthesis.cancel(); $$('[data-speak].on').forEach(x => x.classList.remove('on')); return; }
  const clean = text.replace(/```[\s\S]*?```/g, ' ').replace(/[*_`#>]/g, '').slice(0, 20000);
  const u = new SpeechSynthesisUtterance(clean); u.lang = S.lang === 'en' ? 'en-US' : 'pt-BR'; u.rate = 1.05;
  u.onend = u.onerror = () => btn.classList.remove('on'); btn.classList.add('on'); speechSynthesis.speak(u);
}
function turnOf(s, msgIndex) { const k = s.turnStarts.indexOf(msgIndex); return k >= 0 ? k + 1 : 0; }
function msgText(m) { return (m.content || []).filter(b => b.type === 'text').map(b => b.text).join('\n\n').trim(); }
function copyText(txt, what) { navigator.clipboard.writeText(txt).then(() => toast(t(what || 'Copiado')), () => toast('Não foi possível copiar')); }
// The whole conversation as Markdown: prompts, answers, and each tool call with its input and result.
function transcriptMarkdown(L) {
  const s = L.snap; const res = {}; for (const m of s.messages) for (const b of m.content || []) if (b.type === 'tool_result') res[b.tool_use_id] = b;
  let out = `# ${s.title || 'Sessão'}\n\n_${projectLabel(L.root)} · ${s.model || ''} · ${new Date().toLocaleString()}_\n\n`;
  for (const m of s.messages) {
    if (m.role === 'user') { const tx = msgText(m); if (tx) out += `## ${t('Você')}\n\n${tx}\n\n`; continue; }
    out += `## ${t('Agente')}\n\n`;
    for (const b of m.content) {
      if (b.type === 'thinking' && b.thinking) out += `<details><summary>${t('Raciocínio')}</summary>\n\n${b.thinking}\n\n</details>\n\n`;
      if (b.type === 'text' && b.text?.trim()) out += b.text.trim() + '\n\n';
      if (b.type === 'tool_use') { out += `**${b.name}**\n\n\`\`\`json\n${JSON.stringify(b.input, null, 2)}\n\`\`\`\n\n`; const r = res[b.id]; if (r) out += `\`\`\`\n${String(r.content || '').slice(0, 8000)}\n\`\`\`\n\n`; }
    }
  }
  return out;
}
function exportTranscript(L) {
  const blob = new Blob([transcriptMarkdown(L)], { type: 'text/markdown' }); const a = document.createElement('a');
  a.href = URL.createObjectURL(blob); a.download = (L.snap.title || 'sessao').replace(/[^\w\- ]+/g, '').trim().slice(0, 60) + '.md'; a.click(); setTimeout(() => URL.revokeObjectURL(a.href), 2000);
}
function bindChat(v, root) {
  const L = v.sid && S.live[v.sid];
  $$('[data-copy-msg]', root).forEach(b => b.onclick = e => { e.stopPropagation(); const m = L.snap.messages[+b.dataset.copyMsg]; copyText(msgText(m), 'Mensagem copiada'); });
  $$('[data-copy-turn]', root).forEach(b => b.onclick = e => { e.stopPropagation(); const [a, z] = b.dataset.copyTurn.split(':').map(Number); copyText(L.snap.messages.slice(a + 1, z + 1).filter(m => m.role === 'assistant').map(msgText).filter(Boolean).join('\n\n'), 'Respostas copiadas'); });
  $$('.codecopy', root).forEach(b => b.onclick = () => copyText(b.nextElementSibling.innerText, 'Código copiado'));
  $$('[data-url]', root).forEach(a => a.onclick = e => { e.preventDefault(); togglePane('browser', true); setTimeout(() => paneCall('browser', 'go', a.dataset.url), 300); });
  $$('[data-fork]', root).forEach(b => b.onclick = async e => { e.stopPropagation(); const r = await api(`/api/session/${L.id}/fork`, { body: { upTo: +b.dataset.fork }, root: L.root }); if (!r.ok) return toast(r.error); await loadSessions(); await openSession(r.root, r.id); toast(t('Nova sessão a partir desta mensagem')); });
  $$('[data-pinmsg]', root).forEach(b => b.onclick = async e => { e.stopPropagation(); const r = await api(`/api/session/${L.id}/pinmsg`, { body: { index: +b.dataset.pinmsg }, root: L.root }); L.snap.pinnedMsgs = r.pinned || []; renderChat(v); renderRail(v); });
  $$('[data-speak]', root).forEach(b => b.onclick = e => { e.stopPropagation(); speakMsg(b, msgText(L.snap.messages[+b.dataset.speak])); });
  $$('[data-fixreview]', root).forEach(b => b.onclick = () => { const P = $('.prompt', v.el); P.value = t('Corrija os achados da revisão independente acima. Para cada um, diga o que mudou ou por que não procede.'); send(v); });
  $$('[data-next]', root).forEach(b => b.onclick = () => { const P = $('.prompt', v.el); P.value = b.dataset.next; grow(P); P.focus(); });
  $$('[data-copy-think]', root).forEach(b => b.onclick = e => { e.stopPropagation(); copyText(b.closest('.tool').querySelector('.thinktext')?.innerText || '', 'Raciocínio copiado'); });
  $$('[data-copy-tool]', root).forEach(b => b.onclick = e => { e.stopPropagation(); copyText(b.closest('.tool').querySelector(b.dataset.copyTool === 'out' ? '.res' : '.inp')?.innerText || '', 'Copiado'); });
  $$('[data-rw]', root).forEach(b => b.onclick = () => rewindDialog(v, +b.dataset.rw));
  $$('[data-fold]', root).forEach(x => x.onclick = e => { if (e.target.closest('[data-path]')) return; const box = x.parentNode; const open = !box.classList.contains('open'); box.classList.toggle('open', open); S.fold[x.dataset.fold] = open; });
  $$('[data-path]', root).forEach(x => { x.onclick = () => openFileInPane(x.dataset.path); x.oncontextmenu = e => { e.preventDefault(); pathMenu(x.dataset.path, e); }; });
  $$('[data-perm]', root).forEach(b => b.onclick = () => decide(v, b.dataset.id, b.dataset.perm));
  $$('[data-ans]', root).forEach(b => b.onclick = () => answer(v, b.dataset.id, b.dataset.ans));
  const free = $('.free input', root); if (free) free.onkeydown = e => { if (e.key === 'Enter' && free.value.trim()) answer(v, free.dataset.id, free.value.trim()); };
}
// The tool input while it streams: complete fields and the string being written so far, read from the
// incomplete JSON (so the code shows as the model writes it).
function partialJSON(src) {
  try { return JSON.parse(src); } catch { }
  const out = { _streaming: true };
  const re = /"([A-Za-z_]+)"\s*:\s*/g; let m;
  while ((m = re.exec(src))) {
    const key = m[1]; let i = re.lastIndex;
    if (src[i] === '"') { // a string, possibly unfinished
      let s = '', j = i + 1, done = false;
      for (; j < src.length; j++) {
        const ch = src[j];
        if (ch === '\\') { const nx = src[j + 1]; if (nx === undefined) break; const map = { n: '\n', t: '\t', r: '\r', '"': '"', '\\': '\\', '/': '/', b: '\b', f: '\f' }; if (nx === 'u') { const hex = src.slice(j + 2, j + 6); if (hex.length < 4) break; s += String.fromCharCode(parseInt(hex, 16)); j += 5; } else { s += map[nx] ?? nx; j++; } continue; }
        if (ch === '"') { done = true; break; }
        s += ch;
      }
      out[key] = s; if (!done) out._open = key; re.lastIndex = j + 1;
    } else { const mm = /^(-?\d+(?:\.\d+)?|true|false|null)/.exec(src.slice(i)); if (mm) out[key] = JSON.parse(mm[1]); }
  }
  return out;
}
// Code being written: numbered lines, following the end while it grows.
function liveCodeHTML(text, cls) {
  const lines = String(text || '').split('\n');
  return `<div class="livecode ${cls || ''}">${lines.map((l, k) => `<div class="lc"><span class="no">${k + 1}</span><span class="tx">${esc(l) || ' '}</span></div>`).join('')}<span class="cursor"></span></div>`;
}
function renderLive(v) {
  if (!v.el) return; const el = $('.live', v.el); const L = v.sid && S.live[v.sid]; if (!el || !L) return;
  let out = '';
  if (L.think) out += thinkBlock(`${L.id}:live`, L.think, true);
  if (L.text) out += `<div class="a"><div class="md sel">${md(L.text)}<span class="cursor"></span></div></div>`;
  for (const [id, tl] of Object.entries(L.tools)) { const inp = partialJSON(tl.input); out += toolBlock(id, tl.name, inp, null, L.running?.id === id); }
  if (L.sub && busy(L)) out += `<div class="status sub"><span class="spin"></span><span>${t('subagente')} ${esc(t(L.sub.kind || ''))}${L.sub.title ? ' · ' + esc(L.sub.title) : ''} · ${L.sub.steps || 0} ${t('passos')}${L.sub.tool ? ' · ' + esc(L.sub.tool) + ' ' + esc((L.sub.summary || '').slice(0, 60)) : ''}</span></div>`;
  const st = L.snap?.state;
  if (st === 'thinking' || st === 'tool' || st === 'compacting') out += `<div class="status"><span class="spin"></span><span>${st === 'tool' ? t('executando ') + esc(L.running?.name || '') : st === 'compacting' ? t('compactando contexto') : t('pensando')}</span><span class="faint el" data-el="${L.id}"></span></div>`;
  el.innerHTML = out; bindChat(v, el);
  $$('.livecode', el).forEach(c => { c.scrollTop = c.scrollHeight; });
  if (v.stick !== false) toBottom(v); updateJump(v);
}
setInterval(() => { for (const el of $$('[data-el]')) { const L = S.live[el.dataset.el]; if (L?.started) el.textContent = Math.round((Date.now() - L.started) / 1000) + 's'; } }, 500);

const toolLabel = { Bash: 'Executou', Read: 'Leu', Write: 'Escreveu', Edit: 'Editou', Glob: 'Procurou arquivos', Grep: 'Buscou', LS: 'Listou', WebFetch: 'Buscou na web', TodoWrite: 'Atualizou tarefas', AskUserQuestion: 'Perguntou', MultiEdit: 'Editou', WebSearch: 'Pesquisou na web', Browser: 'Navegador', Skill: 'Skill', Agent: 'Subagente', Memory: 'Memória', Contract: 'Critérios do pedido', Report: 'Relatório' };
function toolSummary(name, inp) {
  if (!inp) return ''; if (inp._partial !== undefined) return inp._partial.slice(-90);
  switch (name) { case 'Bash': return inp.command || ''; case 'Read': case 'Write': case 'Edit': case 'MultiEdit': return inp.path || inp.file_path || ''; case 'WebSearch': return inp.query || ''; case 'Browser': return [inp.action, inp.url || inp.selector || inp.key || (inp.width ? inp.width + 'x' + inp.height : '') || ''].filter(Boolean).join(' · '); case 'Skill': return inp.name || ''; case 'Agent': return `${inp.kind || ''} · ${inp.description || (inp.prompt || '').slice(0, 80)}`; case 'Memory': return `${inp.action || ''}${inp.text ? ' · ' + inp.text : ''}`; case 'Contract': return inp.goal || ''; case 'Glob': return inp.pattern || ''; case 'Grep': return `${inp.pattern || ''}${inp.path ? ' · ' + inp.path : ''}`; case 'LS': return inp.path || '.'; case 'WebFetch': return inp.url || ''; case 'TodoWrite': return `${(inp.todos || inp.tasks || []).length} ${t('itens')}`; case 'AskUserQuestion': return inp.question || ''; default: return JSON.stringify(inp).slice(0, 120); }
}
function lineCounts(name, inp) {
  if (name === 'Write' && inp?.content != null) return `<span class="cnt add">+${inp.content.split('\n').length}</span>`;
  if (name === 'Edit' && inp?.old_string != null) return `<span class="cnt"><span class="add">+${(inp.new_string || '').split('\n').length}</span> <span class="del">−${inp.old_string.split('\n').length}</span></span>`;
  return '';
}
// relPath shows a path relative to the session folder (absolute paths are long and repeat the project).
function relPath(p) {
  const root = (curDir() || '').replace(/[\\/]+$/, ''); if (!root || !p) return p;
  const norm = s => s.replace(/\\/g, '/').toLowerCase(); const np = norm(p), nr = norm(root) + '/';
  return np.startsWith(nr) ? p.replace(/\\/g, '/').slice(nr.length) : p;
}
// Reasoning is part of the transcript: shown open by default (Configurações or Ctrl+O hide it), with a caret.
function thinkBlock(key, text, live, ms) {
  const open = S.fold[key] ?? (S.viewMode !== 'normal' || S.settings.showThinking !== false);
  const words = text.trim().split(/\s+/).length;
  return `<div class="tool think2${open ? ' open' : ''}"><div class="h" data-fold="${esc(key)}"><span class="chev">›</span><b>${live ? t('Pensando…') : ms ? `${t('Pensou por')} ${fmtDur(ms)}` : t('Raciocínio')}</b><span class="s faint">${words} ${t('palavras')}</span><button class="ib mini" data-copy-think title="${esc(t('Copiar raciocínio'))}">${I.copy}</button></div><div class="b"><pre class="thinktext">${esc(text)}</pre></div></div>`;
}
// The agent's report, with infuse's check of each claim (from the tool result, one line per claim, in order).
const CLAIM = { confirmado: ['✓', 'ok', 'confirmado pela execução'], parcial: ['◐', 'warn', 'parcial'], 'nao-confirmado': ['✗', 'bad', 'não confirmado'], 'sem-evidencia': ['?', '', 'sem evidência'] };
function reportCard(inp, result) {
  const lines = String(result?.content || '').split('\n').filter(l => l.startsWith('- ['));
  const rows = (inp.claims || []).map((c, k) => { const m = (lines[k] || '').match(/^- \[([a-z-]+)\] .* — (.*)$/); const st = m ? m[1] : ''; const [ic, cls, lab] = CLAIM[st] || ['…', '', 'conferindo']; return `<div class="rrow"><span class="rst ${cls}" title="${esc(t(lab))}">${ic}</span><span class="grow"><span>${esc(c.text)}</span>${c.ref ? ` <code>${esc(c.ref)}</code>` : ''}${m ? `<span class="faint rev">${esc(m[2])}</span>` : ''}</span></div>`; }).join('');
  const bad = lines.filter(l => l.startsWith('- [nao-confirmado]')).length;
  const srcs = (String(result?.content || '').split('Fontes consultadas neste turno:\n')[1] || '').split('\n').filter(l => l.startsWith('- ')).map(l => l.slice(2));
  const srcHTML = srcs.length ? `<details class="odet"><summary>${t('Fontes consultadas')} (${srcs.length})</summary>${srcs.map(x => /^https?:/.test(x) ? `<div class="mono" style="font-size:11.5px"><a href="#" data-url="${esc(x)}">${esc(x)}</a></div>` : `<div class="faint" style="font-size:11.5px">${esc(x)}</div>`).join('')}</details>` : '';
  return `<div class="report"><div class="rh"><b>${t('Relatório')}</b><span class="faint">${t(result ? 'conferido pelo infuse com o que de fato rodou' : 'conferindo…')}</span>${bad ? `<span class="tag bad">${bad} ${t('não confirmada(s)')}</span>` : ''}</div>${inp.summary ? `<div class="md sel">${md(inp.summary)}</div>` : ''}${rows}${srcHTML}${inp.next ? `<div class="rnext"><span class="faint">${t('Próximo passo')}</span><span class="grow">${esc(inp.next)}</span><button class="btn sm" data-next="${esc(inp.next)}">${t('Usar')}</button></div>` : ''}</div>`;
}
function toolBlock(id, name, input, result, running) {
  let inp = input; if (typeof inp === 'string') { try { inp = JSON.parse(inp); } catch { inp = {}; } }
  if (name === 'Report' && inp && Array.isArray(inp.claims)) return reportCard(inp, result);
  if (inp?._streaming && ['Write', 'Edit', 'MultiEdit', 'Bash'].includes(name)) { // being written right now
    const p = inp.path || inp.file_path || '';
    let body = '', what = '';
    if (name === 'Write') { body = liveCodeHTML(inp.content); what = `${String(inp.content || '').split('\n').length} ${t('linhas')}`; }
    else if (name === 'Edit') { body = (inp.old_string != null ? liveCodeHTML(inp.old_string, 'old') : '') + (inp.new_string != null ? liveCodeHTML(inp.new_string, 'new') : ''); what = t('editando'); }
    else if (name === 'MultiEdit') { body = liveCodeHTML(inp._open === 'old_string' ? inp.old_string : inp.new_string, inp._open === 'old_string' ? 'old' : 'new'); what = t('editando'); }
    else { body = liveCodeHTML(inp.command, 'cmd'); what = t('montando o comando'); }
    const open = S.fold[id] ?? true;
    return `<div class="tool live${open ? ' open' : ''}"><div class="h" data-fold="${esc(id)}"><span class="chev">›</span><span class="dot run"></span><b>${esc(t({ Write: 'Escrevendo', Edit: 'Editando', MultiEdit: 'Editando', Bash: 'Preparando comando' }[name]))}</b><span class="s mono">${esc(p ? relPath(p) : '')}</span><span class="tdur faint">${esc(what)}</span></div><div class="b">${body}</div></div>`;
  }
  const shot = name === 'Browser' && result && String(result.content || '').match(/\.infuse\/shots\/([\w.-]+\.png)/); // the agent's screenshot, shown in place
  const isPath = ['Read', 'Write', 'Edit', 'MultiEdit'].includes(name) && (inp?.path || inp?.file_path);
  let body = '';
  if (name === 'Bash' && inp?.command) body += `<div class="lab">${t('comando')}<button class="ib mini" data-copy-tool="in" title="${esc(t('Copiar comando'))}">${I.copy}</button></div><pre class="inp">${esc(inp.command)}</pre>`;
  else if (name === 'Write' && inp?.content != null) body += diffHTML(inp.content.split('\n').map(l => '+' + l).join('\n'));
  else if (name === 'MultiEdit' && Array.isArray(inp?.edits)) body += inp.edits.map(e => diffHTML((e.old_string || '').split('\n').map(l => '-' + l).join('\n') + '\n' + (e.new_string || '').split('\n').map(l => '+' + l).join('\n'))).join('');
  else if (name === 'Edit' && inp?.old_string != null) body += diffHTML(inp.old_string.split('\n').map(l => '-' + l).join('\n') + '\n' + (inp.new_string || '').split('\n').map(l => '+' + l).join('\n'));
  else if (inp && Object.keys(inp).length && name !== 'TodoWrite') body += `<div class="lab">${t('entrada')}</div><pre>${esc(JSON.stringify(inp, null, 2))}</pre>`;
  if (name === 'TodoWrite' && Array.isArray(inp?.todos || inp?.tasks)) body += (inp.todos || inp.tasks).map(x => `<div>${x.status === 'done' ? '☑' : x.status === 'in_progress' ? '◐' : '☐'} ${esc(x.text || x.content || '')}</div>`).join('');
  if (result) { const o = String(result.content || ''); const k = o.indexOf('\n[infuse'); const main = k >= 0 ? o.slice(0, k) : o, inf = k >= 0 ? o.slice(k + 1) : ''; body += `<div class="lab">${t('resultado')}<button class="ib mini" data-copy-tool="out" title="${esc(t('Copiar resultado'))}">${I.copy}</button></div><pre class="res">${esc(main.length > 20000 ? main.slice(0, 20000) + '\n…' : main)}</pre>` + (inf ? `<div class="lab">${t('infundido')}</div><pre class="inf">${esc(inf)}</pre>` : ''); }
  const dot = running ? 'run' : result ? (result.is_error ? 'err' : '') : 'pend';
  const open = S.fold[id] ?? (S.viewMode === 'verbose' || !!result?.is_error);
  const fullPath = isPath ? (inp.path || inp.file_path) : '';
  const shown = isPath ? relPath(fullPath) : '';
  const summ = isPath ? `<span class="s"><span class="path" data-path="${esc(shown)}" title="${esc(fullPath)}">${esc(shown)}</span></span>` : `<span class="s">${esc(toolSummary(name, inp))}</span>`;
  const mcp = name.startsWith('mcp__') ? name.slice(5).split('__') : null; // connector tools: "Conector server · tool"
  if (shot) body = `<img class="shot" src="/api/shot?name=${encodeURIComponent(shot[1])}&root=${encodeURIComponent(curRoot())}" alt="">` + body;
  return `<div class="tool${open || shot ? ' open' : ''}"><div class="h" data-fold="${esc(id)}"><span class="chev">›</span><span class="dot ${dot}"></span><b>${mcp ? `${esc(t('Conector'))} ${esc(mcp[0])}` : esc(t(toolLabel[name] || name))}</b>${mcp ? `<span class="s mono">${esc(mcp[1] || '')}</span>` : ''}${summ}${lineCounts(name, inp)}${result?.ms != null ? `<span class="tdur faint" title="${esc(t('tempo de execução'))}">${fmtDur(result.ms)}</span>` : ''}</div><div class="b">${body}</div></div>`;
}
const diffHTML = text => `<div class="diff">${text.split('\n').map(l => `<div class="${l.startsWith('+') && !l.startsWith('+++') ? 'a1' : l.startsWith('-') && !l.startsWith('---') ? 'd1' : /^(@@|---|\+\+\+|diff )/.test(l) ? 'hh' : ''}">${esc(l) || ' '}</div>`).join('')}</div>`;
function permCard(p) {
  let inp = {}; try { inp = typeof p.input === 'string' ? JSON.parse(p.input) : p.input || {}; } catch { }
  const verb = p.category === 'exec' ? 'executar um comando' : p.category === 'write' ? 'alterar um arquivo' : p.category === 'net' ? 'acessar a web' : p.category === 'mcp' ? 'usar um conector (roda fora do filtro)' : p.category === 'memory' ? 'registrar na memória' : 'usar uma ferramenta';
  const detail = p.tool === 'Bash' ? `<pre class="cmd">${esc(inp.command || '')}</pre>` : p.tool === 'Edit' ? `<div class="mono" style="margin-top:8px">${esc(inp.path)}</div>` + diffHTML((inp.old_string || '').split('\n').map(l => '-' + l).join('\n') + '\n' + (inp.new_string || '').split('\n').map(l => '+' + l).join('\n')) : p.tool === 'Write' ? `<div class="mono" style="margin-top:8px">${esc(inp.path)}</div>` + diffHTML((inp.content || '').split('\n').slice(0, 200).map(l => '+' + l).join('\n')) : p.tool === 'WebFetch' ? `<pre class="cmd">${esc(inp.url || '')}</pre>` : `<pre class="cmd">${esc(JSON.stringify(inp, null, 2))}</pre>`;
  return `<div class="pcard"><div class="hd"><b>${esc(p.tool)}</b><span class="muted">${t('quer')} ${t(verb)}</span></div>${p.reason ? `<div class="sentinel">${t('Sentinela:')} ${esc(t(p.reason))}. ${t('O infuse pergunta mesmo no modo "aceitar tudo": enfraquecer o teste faz ele passar sem resolver o problema.')}</div>` : ''}${detail}
  <div class="opts"><div class="opt" data-perm="allow" data-id="${p.id}">${t('Sim')}<span class="kbd">Y</span></div><div class="opt" data-perm="always" data-id="${p.id}">${t('Sim, e não perguntar de novo nesta sessão')}${p.tool === 'Bash' ? ` <span class="faint">(${esc((inp.command || '').trim().split(/\s+/).slice(0, 2).join(' '))})</span>` : ''}<span class="kbd">A</span></div><div class="opt" data-perm="all" data-id="${p.id}">${t('Sim, e aceitar tudo daqui em diante')}<span class="kbd">T</span></div><div class="opt" data-perm="deny" data-id="${p.id}">${t('Não, e dizer ao agente o que fazer diferente')}<span class="kbd">N</span></div></div></div>`;
}
function questionCard(q) {
  return `<div class="pcard"><div class="hd"><b>${esc(q.question)}</b></div><div class="opts">${(q.options || []).map((o, i) => `<div class="opt" data-ans="${esc(o)}" data-id="${q.id}">${esc(o)}<span class="kbd">${i + 1}</span></div>`).join('')}</div>
  <div class="free"><input data-id="${q.id}" placeholder="${esc(t('ou responda com suas palavras…'))}"></div></div>`;
}
async function decide(v, id, d) {
  if (d === 'all') { await setSessionOpts(v, { mode: 'bypass' }); d = 'allow'; toast(t('Modo: ') + t('Aceitar tudo')); }
  const L = S.live[v.sid]; await api(`/api/sessions/${v.sid}/permission`, { body: { id, decision: d }, root: L.root }); if (L.snap) L.snap.pendingPermission = null; renderChat(v); if (d === 'deny') setTimeout(() => $('.prompt', v.el).focus(), 50); }
async function answer(v, id, a) { const L = S.live[v.sid]; await api(`/api/sessions/${v.sid}/answer`, { body: { id, answer: a }, root: L.root }); if (L.snap) L.snap.pendingQuestion = null; renderChat(v); }

// markdown (small, safe): code blocks, inline code (paths become links), headings, lists, quotes, tables, links
function md(src) {
  const blocks = [];
  src = src.replace(/```[\w+-]*\n([\s\S]*?)```/g, (_, c) => { blocks.push(`<div class="codewrap"><button class="ib codecopy" title="Copiar">${I.copy}</button><pre><code>${esc(c.replace(/\n$/, ''))}</code></pre></div>`); return `\u0000${blocks.length - 1}\u0000`; });
  let x = esc(src);
  x = x.replace(/^\|(.+)\|\n\|[-:| ]+\|\n((?:\|.*\|\n?)*)/gm, (_, head, body) => `<table><tr>${head.split('|').map(c => `<th>${c.trim()}</th>`).join('')}</tr>${body.trim().split('\n').map(r => `<tr>${r.replace(/^\||\|$/g, '').split('|').map(c => `<td>${c.trim()}</td>`).join('')}</tr>`).join('')}</table>`);
  x = x.replace(/^### (.*)$/gm, '<h3>$1</h3>').replace(/^## (.*)$/gm, '<h2>$1</h2>').replace(/^# (.*)$/gm, '<h1>$1</h1>').replace(/^&gt; (.*)$/gm, '<blockquote>$1</blockquote>');
  x = x.replace(/`([^`\n]+)`/g, (_, c) => /^[\w.\-/\\]+\.[a-z0-9]{1,6}(:\d+)?$/i.test(c) && c.includes('.') ? `<code class="path" data-path="${c.replace(/:\d+$/, '')}">${c}</code>` : `<code>${c}</code>`);
  x = x.replace(/\*\*([^*\n]+)\*\*/g, '<b>$1</b>').replace(/(^|[^*\w])\*([^*\n]+)\*/g, '$1<i>$2</i>').replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  x = x.replace(/(?:^(?:\s*[-*] |\s*\d+\. ).*(?:\n|$))+/gm, m => { const ol = /^\s*\d/.test(m), start = ol ? parseInt(m.trim(), 10) : 1; return `<${ol ? 'ol' : 'ul'}${ol && start > 1 ? ` start="${start}"` : ''}>${m.trim().split('\n').map(l => `<li>${l.replace(/^\s*(?:[-*] |\d+\. )/, '')}</li>`).join('')}</${ol ? 'ol' : 'ul'}>`; });
  x = x.split(/\n{2,}/).map(p => /^\s*<(h\d|ul|ol|pre|table|blockquote)/.test(p) || /^\u0000\d+\u0000$/.test(p.trim()) ? p : `<p>${p.replace(/\n/g, '<br>')}</p>`).join('');
  return x.replace(/\u0000(\d+)\u0000/g, (_, i) => blocks[i]);
}

// ---------------------------------------------------------------- composer
const modeInfo = {
  default: ['Perguntar', 'Pede aprovação antes de editar arquivos ou rodar comandos'],
  acceptEdits: ['Aceitar edições', 'Edita arquivos sem perguntar; comandos ainda pedem aprovação'],
  plan: ['Plano', 'Explora e propõe um plano, sem editar nada'],
  bypass: ['Aceitar tudo', 'Edita e executa sem pedir aprovação (o oráculo continua protegido)'],
};
const effortInfo = { low: 'Baixo', medium: 'Médio', high: 'Alto', max: 'Máximo' };
function vopts(v) {
  const L = v.sid && S.live[v.sid];
  if (L?.snap) return { root: L.root, mode: L.snap.mode, provider: L.snap.provider, model: L.snap.model, effort: L.snap.effort, worktree: L.snap.worktree, branch: L.snap.branch, session: true };
  const d = v.draft; return { root: d.root, mode: d.mode || S.settings.mode || 'default', provider: d.provider || S.settings.provider, model: d.model || S.settings.model, effort: d.effort || S.settings.effort, worktree: d.worktree, session: false };
}
function modelLabel(o) { return o.model || t('Escolher modelo'); }
function renderComposer(v) {
  if (!v.el) return;
  const o = vopts(v); const L = v.sid && S.live[v.sid];
  const under = $('.under', v.el); under.innerHTML = '';
  const add = el => under.appendChild(el);
  if (!o.session) {
    add(h(`<span class="chip static" title="${esc(t('Ambiente: este computador'))}">${I.pc}<span class="v">${t('Local')}</span></span>`));
    const fc = h(`<span class="chip" title="${esc(o.root || '')}">${I.folder}<span class="v">${esc(o.root ? projectLabel(o.root) : t('Escolher pasta'))}</span>${I.caret}</span>`);
    fc.onclick = e => folderMenu(v, e.currentTarget); add(fc);
    const wt = h(`<span class="chip" title="${esc(t('Criar um worktree git isolado para esta sessão'))}"><span class="sw${o.worktree ? ' on' : ''}"></span><span>${t('Worktree')}</span></span>`);
    wt.onclick = () => { v.draft.worktree = !v.draft.worktree; renderComposer(v); }; add(wt);
  } else {
    add(h(`<span class="chip static" title="${esc(L.snap.workRoot || o.root)}">${I.folder}<span class="v">${esc(projectLabel(o.root))}</span></span>`));
  }
  add(h('<span style="flex:1"></span>'));
  const brow = $('.brow', v.el); $('.modechip', brow)?.remove();
  const mc = h(`<span class="chip modechip mode-${o.mode}" title="${esc(t('Modo de permissão (Shift+Tab ou Ctrl+Shift+M)'))}"><span class="v">${t(modeInfo[o.mode]?.[0] || o.mode)}</span>${I.caret}</span>`);
  mc.onclick = e => modeMenu(v, e.currentTarget); brow.insertBefore(mc, $('.sp', brow));
  const mo = h(`<span class="chip" title="${esc(t('Modelo e esforço (Ctrl+Shift+I)'))}"><span class="v">${esc(modelLabel(o))}</span><span class="faint">${t(effortInfo[o.effort] || '')}</span>${I.caret}</span>`);
  mo.onclick = e => modelMenu(v, e.currentTarget); add(mo);
  // send / stop, queue, context ring
  const sendBtn = $('.send', v.el); const isBusy = busy(L);
  sendBtn.classList.toggle('stop', !!(isBusy && !$('.prompt', v.el).value.trim()));
  sendBtn.innerHTML = sendBtn.classList.contains('stop') ? I.stop : I.up; sendBtn.title = t(sendBtn.classList.contains('stop') ? 'Parar (Esc)' : isBusy ? 'Pôr na fila (Enter)' : 'Enviar (Enter)');
  const q = $('.queue', v.el); q.innerHTML = '';
  (L?.queue || []).forEach((m, i) => { const row = h(`<div class="qi"><span>⏎ ${esc(m.text)}</span><span class="faint">${t('na fila')}</span><button class="ib" style="width:20px;height:20px">${I.close}</button></div>`); $('button', row).onclick = () => { L.queue.splice(i, 1); renderComposer(v); }; q.appendChild(row); });
  const ring = $('.ring', v.el); const pct = L ? Math.min(100, Math.round(((L.lastIn || 0) / (S.settings.contextLimit || 200000)) * 100)) : 0;
  const c = 2 * Math.PI * 8;
  ring.innerHTML = L ? `<svg width="18" height="18" viewBox="0 0 20 20"><circle cx="10" cy="10" r="8" stroke="var(--s3)"/><circle cx="10" cy="10" r="8" stroke="${pct > 80 ? 'var(--warn)' : 'var(--text2)'}" stroke-dasharray="${c * pct / 100} ${c}" transform="rotate(-90 10 10)"/></svg>` : '';
  ring.title = L ? `${t('Contexto')}: ${pct}% · ${ktok(L.lastIn)} / ${ktok(S.settings.contextLimit || 200000)}` : '';
  $('.box', v.el).classList.toggle('vim', !!S.settings.vim);
  renderAtts(v);
}
function folderMenu(v, anchor) {
  const items = [{ h: 'Projetos' }, ...S.projects.map(p => ({ label: p.label || baseName(p.root), d: p.root, check: p.root === v.draft.root, f: () => { v.draft.root = p.root; lsSet('lastRoot', p.root); api('/api/projectmode', { root: p.root }).then(r => { v.draft.mode = r.mode || ''; renderView(v); }); renderView(v); renderPanes(); } })), { sep: 1 },
    { label: 'Abrir pasta…', d: 'Explorador', f: () => pickFolder(v) }];
  menu(anchor, items, null, { above: true });
}
// Always the Windows folder dialog: inside the native window through the bridge, in a browser through
// the local server (same machine), which opens it in front of the window in use.
async function pickFolder(v) {
  const p = native ? await nat('pickFolder') : (await api('/api/dialog/folder', { body: {}, root: '' })).path;
  if (p) { await loadSessions(); v.draft.root = p; lsSet('lastRoot', p); renderView(v); renderPanes(); }
  return;
}
async function setSessionOpts(v, patch) {
  const L = v.sid && S.live[v.sid];
  if (!L) { Object.assign(v.draft, patch); renderComposer(v); return; }
  const r = await api(`/api/session/${L.id}/settings`, { body: patch, root: L.root });
  if (r.id) { L.snap = norm({ ...L.snap, ...r, messages: L.snap.messages }); viewsOf(L).forEach(renderView); if (patch.title) loadSessions(); }
}
function modeMenu(v, anchor) {
  const o = vopts(v);
  menu(anchor, Object.entries(modeInfo).map(([k, [l, d]]) => ({ label: l, sub: d, check: o.mode === k, f: () => { setSessionOpts(v, { mode: k }); toast(t('Modo: ') + t(l)); } })), null, { above: true, numbers: true });
}
async function modelMenu(v, anchor, effortOnly) {
  const o = vopts(v);
  const effort = [{ h: 'Esforço' }, ...Object.entries(effortInfo).map(([k, l]) => ({ label: l, check: o.effort === k, sub: { low: 'Rápido, sem raciocínio estendido', medium: 'Equilíbrio', high: 'Mais raciocínio', max: 'Raciocínio máximo, mais lento e caro' }[k], f: () => setSessionOpts(v, { effort: k }) }))];
  if (effortOnly) return menu(anchor, effort, null, { above: true, numbers: true });
  if (!S.models.length) S.models = await api('/api/models', { root: '' });
  const byProv = {}; for (const m of S.models) (byProv[m.provider] = byProv[m.provider] || { label: m.label, items: [] }).items.push(m);
  const items = [];
  for (const [pid, g] of Object.entries(byProv)) { items.push({ h: g.label }); g.items.slice(0, 40).forEach(m => items.push({ label: m.model, check: o.model === m.model && o.provider === pid, f: () => setSessionOpts(v, { provider: pid, model: m.model }) })); }
  if (!items.length) items.push({ label: 'Conectar um provedor…', f: () => openSettings('providers') });
  items.push({ sep: 1 }, ...effort, { sep: 1 }, { label: 'Gerenciar provedores…', f: () => openSettings('providers') });
  menu(anchor, items, null, { above: true });
}
function plusMenu(v, anchor) {
  menu(anchor, [
    { label: 'Adicionar arquivos ou fotos', f: () => attachFiles(v) },
    { label: 'Mencionar arquivo', d: '@', f: () => insertAtCursor(v, '@') },
    { label: 'Comandos de barra', d: '/', f: () => { const P = $('.prompt', v.el); if (!P.value.startsWith('/')) { P.value = '/' + P.value; } P.focus(); autocomplete(v); } },
    { sep: 1 }, { label: 'Pergunta paralela', d: 'Ctrl ;', f: () => openSide(v) },
  ], null, { above: true });
}
function insertAtCursor(v, s) { const P = $('.prompt', v.el); const p = P.selectionStart; P.value = P.value.slice(0, p) + s + P.value.slice(p); P.selectionStart = P.selectionEnd = p + s.length; P.focus(); autocomplete(v); }
async function attachFiles(v) {
  // the Windows file dialog in both cases; files are attached by their real path
  const list = native ? await nat('pickFiles') : await api('/api/dialog/files', { body: {}, root: '' }); if (Array.isArray(list)) { for (const a of list) if (!v.atts.find(x => x.path === a.path)) v.atts.push(a); renderAtts(v); }
}
function readImage(v, f) { const r = new FileReader(); r.onload = () => { v.images.push({ mediaType: f.type, data: r.result.split(',')[1] }); renderAtts(v); }; r.readAsDataURL(f); }
function readBrowserFile(v, f) { if (f.type.startsWith('image/')) return readImage(v, f); const r = new FileReader(); r.onload = () => { const P = $('.prompt', v.el); P.value += `\n<arquivo nome="${f.name}">\n${r.result.slice(0, 200000)}\n</arquivo>`; grow(P); }; r.readAsText(f); }
function renderAtts(v) {
  const el = $('.atts', v.el); el.innerHTML = '';
  v.atts.forEach((a, i) => { const c = h(`<span class="chipx" title="${esc(a.path)}">📎 <span>${esc(a.name)}</span><b>×</b></span>`); $('b', c).onclick = () => { v.atts.splice(i, 1); renderAtts(v); }; el.appendChild(c); });
  v.images.forEach((im, i) => { const c = h(`<span class="chipx"><img src="data:${im.mediaType};base64,${im.data}"><span>${t('imagem')}</span><b>×</b></span>`); $('b', c).onclick = () => { v.images.splice(i, 1); renderAtts(v); }; el.appendChild(c); });
}

// The gray suggestion of the next message (Tab or → accepts), asked of the fast model after each turn.
const PROMPT_PH = 'Peça algo… @ para arquivos, / para comandos';
function setGhost(v, text) { const P = v.el && $('.prompt', v.el); if (!P) return; P.dataset.ghost = text || ''; P.placeholder = text ? text + '   ⇥ Tab' : t(PROMPT_PH); P.classList.toggle('ghost', !!text); }
async function suggestNext(L) {
  if (S.settings.promptSuggestions === false || !L?.id) return;
  const r = await api(`/api/session/${L.id}/suggest`, { body: {}, root: L.root }); if (!r.text) return;
  viewsOf(L).forEach(v => { const P = $('.prompt', v.el); if (P && !P.value && !v.atts.length && !v.images.length && !busy(L)) setGhost(v, r.text); });
}
async function send(v) {
  const P = $('.prompt', v.el); let text = P.value.trim(); setGhost(v, '');
  if (!text && !v.atts.length && !v.images.length) return;
  if (text.startsWith('/')) { const handled = await slash(v, text); if (handled) { P.value = ''; grow(P); hideAC(); return; } text = P.value.trim(); }
  let L = v.sid && S.live[v.sid];
  if (!L) {
    const d = vopts(v);
    if (!d.root) { folderMenu(v, $('.under .chip:nth-child(2)', v.el)); toast('Escolha a pasta do projeto'); return; }
    const r = await api('/api/sessions/new', { body: { mode: v.draft.mode, provider: d.provider, model: d.model, effort: d.effort, worktree: d.worktree, title: text.slice(0, 40) }, root: d.root });
    if (r.error) return toast(r.error);
    await openSession(d.root, r.id); L = S.live[r.id];
  }
  const msg = { text: text || t('(anexos)'), attachments: v.atts, images: v.images };
  S.history.push(text); if (S.history.length > 300) S.history.shift(); lsSet('hist', S.history); v.hi = -1;
  v.atts = []; v.images = []; P.value = ''; grow(P); hideAC();
  if (busy(L)) { L.queue.push(msg); renderComposer(v); return; }
  const r = await api(`/api/sessions/${L.id}/send`, { body: msg, root: L.root }); if (r.error) toast(r.error);
  renderComposer(v);
}
async function drainQueue(L) {
  if (!L.queue.length) return; const next = L.queue.shift();
  viewsOf(L).forEach(renderComposer);
  const r = await api(`/api/sessions/${L.id}/send`, { body: next, root: L.root }); if (r.error) { toast(r.error); L.queue.unshift(next); }
}
function stop(v) { const L = v.sid && S.live[v.sid]; if (L) api(`/api/sessions/${L.id}/stop`, { body: {}, root: L.root }); }

// slash commands: built-ins act here; custom ones expand into the prompt
async function slash(v, text) {
  const [cmd, ...rest] = text.slice(1).split(/\s+/); const arg = rest.join(' ');
  const root = vopts(v).root; const cmds = await getCommands(root);
  const custom = cmds.find(c => !c.builtin && c.name === cmd);
  if (custom) { $('.prompt', v.el).value = custom.prompt.replace(/\$ARGUMENTS/g, arg); return false; }
  const L = v.sid && S.live[v.sid];
  switch (cmd) {
    case 'clear': case 'new': newSessionScreen(root); return true;
    case 'compact': if (L && L.snap.turn > 1) { const r = await api(`/api/session/${L.id}/summarize`, { body: { turn: L.snap.turn, hint: arg }, root: L.root }); toast(r.error || 'Conversa resumida'); } else toast('Nada para compactar ainda'); return true;
    case 'model': modelMenu(v, $('.under .chip:last-child', v.el)); return true;
    case 'effort': modelMenu(v, $('.under .chip:last-child', v.el), true); return true;
    case 'mode': case 'permissions': modeMenu(v, $('.modechip', v.el)); return true;
    case 'plan': setSessionOpts(v, { mode: 'plan' }); if (arg) { $('.prompt', v.el).value = arg; return false; } return true;
    case 'rewind': case 'undo': case 'checkpoint': rewindDialog(v); return true;
    case 'diff': togglePane('changes', true); return true;
    case 'cost': case 'usage': case 'context': contextDialog(v); return true;
    case 'btw': openSide(v, arg); return true;
    case 'config': case 'settings': openSettings(); return true;
    case 'resume': palette('sessions'); return true;
    case 'rename': if (L && arg) setSessionOpts(v, { title: arg }); else renameInline(v); return true;
    case 'review': case 'code-review': runReview(v, arg); return true;
    case 'mission': case 'missao': if (arg) $('.prompt', v.el).value = arg; missionDialog(v); return true;
    case 'init': $('.prompt', v.el).value = t('Analise este projeto e escreva um INFUSE.md na raiz com: como compilar, testar e rodar; a arquitetura e onde cada responsabilidade mora; convenções do código. Seja conciso.'); return false;
    case 'memory': togglePane('memory', true); return true;
    case 'terminal': togglePane('terminal', true); return true;
    case 'vim': await saveSetting({ vim: !S.settings.vim }); toast(S.settings.vim ? 'Modo vim ligado' : 'Modo vim desligado'); S.views.forEach(renderComposer); return true;
    case 'help': shortcutsDialog(); return true;
    case 'copy': if (L) { const n = parseInt(arg) || 1; const ms = L.snap.messages.filter(x => x.role === 'assistant' && msgText(x)); const m = ms[ms.length - n]; if (m) copyText(msgText(m), 'Resposta copiada'); } return true;
    case 'export': if (L) exportTranscript(L); return true;
  }
  return false;
}
async function getCommands(root) { if (!S.commands[root]) S.commands[root] = await api('/api/commands', { root }); return S.commands[root] || []; }

// @ files and / commands autocomplete
let AC = null;
async function autocomplete(v) {
  const P = $('.prompt', v.el); const before = P.value.slice(0, P.selectionStart);
  const m = before.match(/(^|\s)([@/])([^\s]*)$/);
  if (!m || (m[2] === '/' && before.trim() !== m[2] + m[3])) return hideAC();
  const root = vopts(v).root; const q = m[3].toLowerCase(); let items;
  if (m[2] === '@') { if (!root) return hideAC(); if (!S.files[root]) S.files[root] = await api('/api/files/list', { root }); items = (S.files[root] || []).filter(f => f.toLowerCase().includes(q)).sort((a, b) => (a.toLowerCase().indexOf(q) - b.toLowerCase().indexOf(q)) || a.length - b.length).slice(0, 50).map(f => ({ v: f, d: f.endsWith('/') ? t('pasta') : '' })); }
  else items = (await getCommands(root)).filter(c => c.name.startsWith(q)).map(c => ({ v: c.name, d: c.description }));
  if (!items.length) return hideAC();
  AC = { v, kind: m[2], items, i: 0 }; renderAC();
}
function renderAC() {
  const { v, kind, items, i } = AC;
  const m = menu($('.box', v.el), items.map((it, k) => ({ label: kind + it.v, d: it.d, f: () => acPick(k) })), null, { above: true });
  m.style.minWidth = '360px'; const mi = $$('.mi', m); mi.forEach((x, k) => x.classList.toggle('k', k === i)); mi[i]?.scrollIntoView({ block: 'nearest' });
}
function hideAC() { if (AC) { AC = null; closeMenu(); } }
function acKey(e) { if (!AC) return false; if (e.key === 'ArrowDown') AC.i = (AC.i + 1) % AC.items.length; else if (e.key === 'ArrowUp') AC.i = (AC.i - 1 + AC.items.length) % AC.items.length; else if (e.key === 'Tab' || e.key === 'Enter') { acPick(AC.i); e.preventDefault(); return true; } else if (e.key === 'Escape') { hideAC(); e.preventDefault(); return true; } else return false; e.preventDefault(); renderAC(); return true; }
function acPick(k) {
  const { v, kind, items } = AC; const it = items[k]; const P = $('.prompt', v.el); const pos = P.selectionStart;
  const before = P.value.slice(0, pos).replace(/([@/])[^\s]*$/, kind + it.v + ' ');
  P.value = before + P.value.slice(pos); P.selectionStart = P.selectionEnd = before.length; hideAC(); P.focus(); grow(P);
  if (kind === '@' && !it.v.endsWith('/')) { const root = vopts(v).root; const abs = root.replace(/[\\/]+$/, '') + '/' + it.v; if (!v.atts.find(a => a.path === abs)) { v.atts.push({ path: abs, name: it.v, size: 0 }); renderAtts(v); } }
}

let lastEsc = 0;
function composerKey(v, e) {
  if (vimKey(v, e)) return;
  if (acKey(e)) return;
  const P = $('.prompt', v.el); const L = v.sid && S.live[v.sid];
  if ((e.key === 'Tab' || e.key === 'ArrowRight') && !e.shiftKey && !P.value && P.dataset.ghost) { e.preventDefault(); P.value = P.dataset.ghost; setGhost(v, ''); grow(P); return; }
  if (e.key === 'Enter' && !e.shiftKey && !e.altKey) { e.preventDefault(); send(v); return; }
  if (e.key === 'Tab' && e.shiftKey) { e.preventDefault(); const order = Object.keys(modeInfo); const cur = vopts(v).mode; setSessionOpts(v, { mode: order[(order.indexOf(cur) + 1) % order.length] }); return; }
  if (e.key === 'Escape') {
    if (busy(L)) { stop(v); return; }
    const now = Date.now();
    if (now - lastEsc < 500) { lastEsc = 0; if (P.value.trim()) { S.history.push(P.value); lsSet('hist', S.history); P.value = ''; grow(P); } else if (L) rewindDialog(v); return; }
    lastEsc = now; return;
  }
  if (e.key === 'ArrowUp' && (P.selectionStart === 0 || !P.value)) { const hist = S.history; if (!hist.length) return; v.hi = Math.min((v.hi ?? -1) + 1, hist.length - 1); P.value = hist[hist.length - 1 - v.hi]; grow(P); e.preventDefault(); return; }
  if (e.key === 'ArrowDown' && (v.hi ?? -1) >= 0 && P.selectionStart === P.value.length) { v.hi--; P.value = v.hi < 0 ? '' : S.history[S.history.length - 1 - v.hi]; grow(P); e.preventDefault(); return; }
  setTimeout(() => renderComposerSend(v), 0);
}
function renderComposerSend(v) { const L = v.sid && S.live[v.sid]; const b = $('.send', v.el); const stopMode = busy(L) && !$('.prompt', v.el).value.trim(); if (b.classList.contains('stop') !== stopMode) renderComposer(v); }

// vim (minimal): Esc → NORMAL; i a A I o; h j k l w b 0 $; x dd; Enter sends
function vimKey(v, e) {
  if (!S.settings.vim) return false; const P = $('.prompt', v.el);
  v.vim = v.vim || 'INSERT';
  if (v.vim === 'INSERT') { if (e.key === 'Escape' && !AC && P.value) { v.vim = 'NORMAL'; P.placeholder = '-- NORMAL --'; e.preventDefault(); return true; } return false; }
  if (e.ctrlKey || e.metaKey) return false;
  e.preventDefault(); const val = P.value, p = P.selectionStart; const set = n => { P.selectionStart = P.selectionEnd = Math.max(0, Math.min(val.length, n)); };
  const ls = val.lastIndexOf('\n', p - 1) + 1, le = val.indexOf('\n', p) === -1 ? val.length : val.indexOf('\n', p);
  switch (e.key) {
    case 'h': set(p - 1); break; case 'l': set(p + 1); break; case '0': set(ls); break; case '$': set(le); break;
    case 'w': { const m = val.slice(p).match(/^\S*\s+/); set(p + (m ? m[0].length : val.length)); break; }
    case 'b': { const m = val.slice(0, p).match(/\S+\s*$/); set(p - (m ? m[0].length : p)); break; }
    case 'j': case 'k': { const lines = val.split('\n'); let acc = 0, li = 0, col = p; for (; li < lines.length; li++) { if (col <= lines[li].length) break; col -= lines[li].length + 1; } const tl = li + (e.key === 'j' ? 1 : -1); if (tl >= 0 && tl < lines.length) { for (let i = 0; i < tl; i++) acc += lines[i].length + 1; set(acc + Math.min(col, lines[tl].length)); } break; }
    case 'i': v.vim = 'INSERT'; break; case 'a': set(p + 1); v.vim = 'INSERT'; break; case 'A': set(le); v.vim = 'INSERT'; break; case 'I': set(ls); v.vim = 'INSERT'; break;
    case 'o': P.value = val.slice(0, le) + '\n' + val.slice(le); set(le + 1); v.vim = 'INSERT'; break;
    case 'x': P.value = val.slice(0, p) + val.slice(p + 1); set(p); break;
    case 'd': if (v.vimD) { P.value = val.slice(0, ls) + val.slice(Math.min(val.length, le + 1)); set(ls); v.vimD = false; } else { v.vimD = true; return true; } break;
    case 'Enter': send(v); break;
  }
  v.vimD = false; P.placeholder = v.vim === 'NORMAL' ? '-- NORMAL --' : t('Peça algo… @ para arquivos, / para comandos'); grow(P); return true;
}

// ---------------------------------------------------------------- rail (session details)
function renderRail(v) {
  if (!v.el) return; const el = $('.rail', v.el); const L = v.sid && S.live[v.sid]; if (!L?.snap) { el.innerHTML = ''; return; }
  const c = L.changes || {}; const files = c.files || [];
  const pct = Math.min(100, Math.round(((L.lastIn || 0) / (S.settings.contextLimit || 200000)) * 100));
  const tasks = L.snap.tasks || [];
  el.innerHTML = `
  ${planCard(L)}
  <div class="card" data-card="changes"><div class="ch">${t('Alterações')}<span class="r mono">${c.isGit === false && !c.local ? t('sem git') : `<span class="add">+${c.added || 0}</span> <span class="del">−${c.removed || 0}</span>`}</span></div>${c.local ? `<div class="segrow">${scopeSwitch()}</div>` : ''}
    ${files.length ? files.slice(0, 8).map(f => `<div class="li${f.status === 'D' ? ' gone' : ''}" data-f="${esc(f.path)}"><span class="n" title="${esc(f.path)}">${esc(f.path)}</span>${chgTag(f)}<span class="add">+${f.added}</span><span class="del">−${f.removed}</span></div>`).join('') + (files.length > 8 ? `<div class="faint" style="padding:3px 4px">+${files.length - 8} ${t('arquivos')}</div>` : '') : `<div class="faint">${t('Nenhuma alteração')}</div>`}
    <div class="prline"></div></div>
  <div class="card ctxcard" data-card="context" style="cursor:pointer"><div class="ch">${t('Contexto')}<span class="r">${pct}%</span></div><div class="bar100"><i style="width:${pct}%"></i></div><div class="faint">${ktok(L.lastIn)} / ${ktok(S.settings.contextLimit || 200000)} tokens · ${money(L.snap.cost)}</div></div>
  <div class="card" data-card="env"><div class="ch">${t('Ambiente')}</div><div class="kv"><span>${t('Onde')}</span><span>${t('Este computador')}</span></div><div class="kv"><span>${t('Pasta')}</span><span title="${esc(L.snap.workRoot)}">${esc(baseName(L.snap.workRoot || L.root))}</span></div>${L.snap.branch ? `<div class="kv"><span>${t('Branch')}</span><span class="mono">${esc(L.snap.branch)}</span></div>` : ''}${L.snap.worktree ? `<div class="kv"><span>${t('Worktree')}</span><span>${t('isolado')}</span></div>` : ''}<div class="kv"><span>${t('Modelo')}</span><span class="mono">${esc(L.snap.model || '')}</span></div></div>
  ${(L.snap.pinnedMsgs || []).length ? `<div class="card pins" data-card="pins"><div class="ch">${t('Mensagens fixadas')}<span class="r">${L.snap.pinnedMsgs.length}</span></div>${L.snap.pinnedMsgs.map(i => `<div class="li pinli" data-goto="${i}"><span class="n">${esc(msgText(L.snap.messages[i] || { content: [] }).split('\n')[0].slice(0, 80) || '…')}</span></div>`).join('')}</div>` : ''}
  ${missionCard(L)}
  <div class="card" data-card="pronto"><div class="ch">${t('Pronto')}<span class="r">${L.verdict ? `<span class="tag ${{ aprovado: 'ok', reprovado: 'bad' }[L.verdict.verdict] || 'warn'}">${esc(t(L.verdict.verdict))}</span>` : `<span class="faint">${t('não verificado')}</span>`}</span></div>
    <div class="faint" style="margin-bottom:6px">${L.verdict ? esc(L.verdict.summary) : t('Só a execução dos critérios decide; a palavra do agente não conta.')}</div>
    <div class="row"><button class="btn sm line oraclego">${t('Pronto?')}</button><button class="btn sm" data-open="oracle">${t('Critérios')}</button><button class="btn sm revgo" title="${esc(t('Um revisor com contexto limpo lê o diff, o plano e os critérios e aponta problemas'))}">${t('Revisar')}</button></div></div>`;
  $('.revgo', el).onclick = () => runReview(v);
  railFolds(v, el);
  bindScope(el);
  $$('[data-goto]', el).forEach(x => x.onclick = () => { const m = $(`[data-mi="${x.dataset.goto}"]`, v.el); if (m) { v.stick = false; m.scrollIntoView({ behavior: 'smooth', block: 'center' }); m.classList.add('flash'); setTimeout(() => m.classList.remove('flash'), 1400); } });
  const ms = $('.mstop', el); if (ms) ms.onclick = () => api(`/api/session/${L.id}/mission`, { body: { action: 'stop' }, root: L.root });
  const mn = $('.mnew', el); if (mn) mn.onclick = () => missionDialog(v);
  $('.oraclego', el).onclick = () => { togglePane('oracle', true); setTimeout(() => { const out = $('.pane[data-id="oracle"] .verdict'); if (out) runOracleUI({ root: L.root, dir: L.snap.workRoot || L.root }, out); }, 400); };
  $$('[data-f]', el).forEach(x => x.onclick = () => { togglePane('changes', true); setTimeout(() => paneCall('changes', 'select', x.dataset.f), 50); });
  $('.ctxcard', el).onclick = () => contextDialog(v);
  $$('[data-open]', el).forEach(x => x.onclick = () => togglePane(x.dataset.open, true));
  renderPR(v, $('.prline', el));
}
// Auto-fix: the failing CI run's log goes to the agent (by click, or on its own when turned on).
async function fixCI(v, L, pr) {
  const r = await api('/api/pr', { body: { action: 'failed-logs' }, root: L.root, dir: L.snap.workRoot || L.root });
  if (!r.ok) return toast(r.error || r.output || t('Não foi possível ler o log do CI'));
  const P = $('.prompt', v.el); P.value = `${t('O CI do PR')} #${pr.number} ${t('falhou. Leia o log abaixo, encontre a causa, corrija, rode localmente o que falhou e faça o commit. Não altere testes para passar.')}\n\n\`\`\`\n${r.output}\n\`\`\``; send(v);
}
setInterval(async () => { // checks PRs of sessions with auto-fix on, every 2 minutes
  for (const v of S.views) {
    const L = v.sid && S.live[v.sid]; if (!L) continue;
    if (L.autofix === undefined) L.autofix = lsGet('autofix:' + L.id, false);
    if (!L.autofix || busy(L)) continue;
    L.prAt = 0; await renderPR(v, $('.prline', v.el) || document.createElement('div'));
    const pr = L.pr?.pr; if (!pr) continue;
    const failed = (pr.statusCheckRollup || []).filter(c => /FAIL|ERROR/i.test(c.conclusion || c.state || ''));
    const sig = pr.number + ':' + failed.map(c => c.name + c.completedAt).join('|');
    if (failed.length && L.autofixSig !== sig) { L.autofixSig = sig; toast(t('CI falhou: enviando o log ao agente')); fixCI(v, L, pr); }
  }
}, 120000);
async function renderPR(v, el) {
  const L = S.live[v.sid]; if (!L || !L.changes?.isGit || !el) return;
  if (!L.pr || Date.now() - L.prAt > 60000) { L.pr = await api('/api/pr', { root: L.root, dir: L.snap.workRoot || L.root }); L.prAt = Date.now(); }
  const pr = L.pr?.pr;
  if (!L.pr?.gh) { // without the GitHub CLI: say so and offer to install it, instead of hiding PRs
    el.innerHTML = `<div class="row" style="margin-top:6px;font-size:12px"><span class="faint grow">${t('Pull requests precisam do GitHub CLI (gh).')}</span><button class="btn sm" data-gh="install">${t('Instalar')}</button></div>`;
    $('[data-gh]', el).onclick = () => { togglePane('terminal', true); setTimeout(() => paneCall('terminal', 'openAt', L.snap.workRoot || L.root, 'winget install --id GitHub.cli -e && gh auth login', 'GitHub CLI'), 300); };
    return;
  }
  if (!pr) { el.innerHTML = `<div class="row" style="margin-top:6px"><button class="btn sm line" data-pr="create">${t('Criar PR')}</button></div>`; }
  else {
    var checks = pr.statusCheckRollup || []; var fail = checks.filter(c => /FAIL|ERROR/i.test(c.conclusion || c.state || '')).length, pend = checks.filter(c => !(c.conclusion || '') && !/SUCCESS/.test(c.state || '')).length;
    el.innerHTML = `<div class="row" style="margin-top:8px;font-size:12px"><a href="${esc(pr.url)}" target="_blank">#${pr.number}</a><span class="trunc grow">${esc(pr.title)}</span><span class="tag ${fail ? 'bad' : pend ? 'warn' : 'ok'}">${fail ? t('CI falhou') : pend ? t('CI rodando') : t('CI ok')}</span><button class="ib" data-pr="menu" style="width:22px;height:22px">${I.more}</button></div>`;
  }
  $$('[data-pr]', el).forEach(b => b.onclick = async e => {
    if (b.dataset.pr === 'create') { b.disabled = true; const r = await api('/api/pr', { body: { action: 'create' }, root: L.root, dir: L.snap.workRoot || L.root }); toast(r.ok ? 'PR criado' : (r.error || r.output)); L.prAt = 0; renderRail(v); }
    else menu(b, [{ label: 'Abrir no GitHub', f: () => { togglePane('browser', true); setTimeout(() => paneCall('browser', 'go', pr.url), 300); } }, fail ? { label: 'Corrigir o CI agora', sub: 'O agente lê o log da falha e corrige', f: () => fixCI(v, L, pr) } : null, { label: L.autofix ? 'Desligar correção automática do CI' : 'Corrigir o CI automaticamente', sub: 'Quando o CI falhar e a sessão estiver parada, o agente recebe o log e corrige', check: !!L.autofix, f: () => { L.autofix = !L.autofix; lsSet('autofix:' + L.id, L.autofix); toast(t(L.autofix ? 'Correção automática do CI ligada' : 'Correção automática do CI desligada')); } }, { label: 'Mesclar quando pronto', sub: 'Ativa o auto-merge (squash) no GitHub', f: async () => { const r = await api('/api/pr', { body: { action: 'merge-auto' }, root: L.root, dir: L.snap.workRoot || L.root }); toast(r.ok ? 'Auto-merge ativado' : (r.error || r.output)); } }]);
  });
}
function renderChangesBadge(v) {
  const L = v.sid && S.live[v.sid]; const b = $('.chg', v.el); if (!b) return;
  const c = L?.changes; b.innerHTML = (c?.isGit || c?.local) && (c.added || c.removed) ? `<span class="add">+${c.added || 0}</span><span class="del">−${c.removed || 0}</span>` : I.diff;
}

// ---------------------------------------------------------------- dialogs: rewind, context, palette, shortcuts, side chat
function overlay(inner, onClose) {
  const o = h('<div class="overlay"></div>'); o.appendChild(inner); document.body.appendChild(o);
  const close = () => { o.remove(); document.removeEventListener('keydown', key, true); onClose && onClose(); };
  const key = e => { if (e.key === 'Escape') { e.stopPropagation(); e.preventDefault(); close(); } };
  document.addEventListener('keydown', key, true); o.onmousedown = e => { if (e.target === o) close(); };
  return close;
}
function rewindDialog(v, preselect) {
  const L = v.sid && S.live[v.sid]; if (!L?.snap?.turn) return toast('Nada para voltar ainda');
  if (busy(L)) return toast('Pare o agente antes de voltar');
  const s = L.snap; const turns = s.turnStarts.map((idx, i) => ({ n: i + 1, text: (s.messages[idx]?.content || []).find(b => b.type === 'text')?.text || '', files: (s.checkpoints[i + 1] || []).length })).reverse();
  const dlg = h(`<div class="dlg"><div class="dh">${t('Voltar no tempo')}</div><div class="faint" style="padding:0 18px 8px">${t('Escolha o ponto: a mensagem e tudo depois dela.')}</div><div class="db"></div></div>`);
  const close = overlay(dlg); const db = $('.db', dlg); let k = 0;
  const list = () => {
    db.innerHTML = ''; turns.forEach((tu, i) => { const it = h(`<div class="it${i === k ? ' k' : ''}"><span class="faint">${tu.n}</span><span class="trunc grow">${esc(tu.text.split('\n')[0] || '—')}</span><span class="d">${tu.files ? tu.files + ' ' + t('arquivo(s)') : t('sem alterações de código')}</span></div>`); it.onclick = () => choose(tu); db.appendChild(it); });
  };
  const choose = tu => {
    db.innerHTML = `<div class="faint" style="padding:6px 12px">${t('Turno')} ${tu.n}: ${esc(tu.text.split('\n')[0].slice(0, 90))}</div>`;
    const opts = [
      tu.files && { l: 'Restaurar código e conversa', f: () => run({ turn: tu.n, code: true, conversation: true }) },
      { l: 'Restaurar conversa', d: 'mantém o código atual', f: () => run({ turn: tu.n, code: false, conversation: true }) },
      tu.files && { l: 'Restaurar código', d: 'mantém a conversa', f: () => run({ turn: tu.n, code: true, conversation: false }) },
      tu.n > 1 && { l: 'Resumir até aqui', d: 'compacta o que veio antes', f: () => summarize(tu.n) },
      { l: 'Deixa pra lá', f: () => list() },
    ].filter(Boolean);
    opts.forEach((o, i) => { const it = h(`<div class="it">${t(o.l)}<span class="d">${o.d ? t(o.d) + ' · ' : ''}${i + 1}</span></div>`); it.onclick = o.f; db.appendChild(it); });
    const ctx = h(`<input class="ctx" placeholder="${esc(t('contexto para o resumo (opcional)'))}">`); if (tu.n > 1) db.appendChild(ctx);
    dlg.onkeydown = e => { const n = +e.key; if (n >= 1 && n <= opts.length && document.activeElement !== ctx) { e.preventDefault(); opts[n - 1].f(); } };
    const summarize = async n => { db.innerHTML = `<div class="status" style="padding:14px"><span class="spin"></span>${t('resumindo…')}</div>`; const r = await api(`/api/session/${L.id}/summarize`, { body: { turn: n, hint: ctx.value }, root: L.root }); close(); if (r.error) return toast(r.error); L.snap = norm(r.session); viewsOf(L).forEach(renderView); toast('Conversa resumida'); };
  };
  const run = async body => {
    const r = await api(`/api/session/${L.id}/rewind`, { body, root: L.root }); close();
    if (r.error) return toast(r.error);
    L.snap = norm(r.session); viewsOf(L).forEach(renderView); refreshChanges(L);
    if (body.conversation && r.prompt) { const P = $('.prompt', v.el); P.value = r.prompt; grow(P); P.focus(); }
    toast(body.code && body.conversation ? 'Código e conversa restaurados' : body.code ? 'Código restaurado' : 'Conversa restaurada');
  };
  dlg.tabIndex = 0; dlg.onkeydown = e => { if (e.key === 'ArrowDown') { k = Math.min(k + 1, turns.length - 1); list(); } else if (e.key === 'ArrowUp') { k = Math.max(0, k - 1); list(); } else if (e.key === 'Enter') choose(turns[k]); };
  if (preselect) choose(turns.find(x => x.n === preselect) || turns[0]); else list(); dlg.focus();
}
async function contextDialog(v) {
  const L = v.sid && S.live[v.sid]; if (!L) return;
  const r = await api(`/api/session/${L.id}/context`, { root: L.root }); const u = L.snap.usage || {};
  const dlg = h(`<div class="dlg"><div class="dh">${t('Contexto e custo')}</div><div class="db" style="padding:0 18px 16px"></div></div>`); overlay(dlg);
  const pct = x => Math.round(x / (r.limit || 200000) * 100);
  $('.db', dlg).innerHTML = `<div class="bar100" style="height:8px;display:flex">${r.categories.map((c, i) => `<i style="width:${pct(c.tokens)}%;background:hsl(${210 + i * 25},${i % 2 ? 35 : 45}%,${55 + (i % 3) * 7}%)"></i>`).join('')}</div>
  <table class="tbl">${r.categories.map((c, i) => `<tr><td><span style="display:inline-block;width:8px;height:8px;border-radius:2px;background:hsl(${210 + i * 25},${i % 2 ? 35 : 45}%,${55 + (i % 3) * 7}%)"></span> ${t(c.name)}</td><td class="mono">${ktok(c.tokens)}</td><td class="faint">${pct(c.tokens)}%</td></tr>`).join('')}
  <tr><td><b>${t('Total estimado')}</b></td><td class="mono"><b>${ktok(r.total)}</b></td><td class="faint">${t('de')} ${ktok(r.limit)}</td></tr></table>
  <h4 style="margin:16px 0 6px;font-size:12px">${t('Esta sessão')}</h4><div class="kv"><span>${t('Entrada')}</span><span class="mono">${ktok((u.input || 0) + (u.cacheRead || 0) + (u.cacheWrite || 0))} tokens</span></div><div class="kv"><span>${t('Saída')}</span><span class="mono">${ktok(u.output)} tokens</span></div><div class="kv"><span>${t('Custo')}</span><span class="mono">${money(L.snap.cost)}</span></div>
  <div class="faint" style="margin-top:8px">${t('Compactação automática em 75% do limite. Custos são estimativas pela tabela de preços (Configurações > Modelos e custos).')}</div>`;
}
function palette(kind) {
  const all = paletteItems(kind);
  const dlg = h(`<div class="dlg"><input class="q" placeholder="${esc(t(kind === 'sessions' ? 'Buscar sessão…' : 'Digite um comando ou busque uma sessão…'))}"><div class="db" style="padding:6px"></div></div>`);
  const close = overlay(dlg); const q = $('input', dlg), db = $('.db', dlg); let k = 0, list = [];
  const draw = () => { const s = q.value.toLowerCase(); list = all.filter(x => t(x.label).toLowerCase().includes(s) || (x.d || '').toLowerCase().includes(s)).slice(0, 80); k = Math.min(k, Math.max(0, list.length - 1)); db.innerHTML = ''; list.forEach((x, i) => { const it = h(`<div class="it${i === k ? ' k' : ''}"><span class="trunc">${esc(t(x.label))}</span><span class="d">${esc(x.d || '')}</span></div>`); it.onclick = () => { close(); x.f(); }; db.appendChild(it); }); db.children[k]?.scrollIntoView({ block: 'nearest' }); };
  q.oninput = () => { k = 0; draw(); }; q.onkeydown = e => { if (e.key === 'ArrowDown') { k = Math.min(k + 1, list.length - 1); draw(); e.preventDefault(); } else if (e.key === 'ArrowUp') { k = Math.max(0, k - 1); draw(); e.preventDefault(); } else if (e.key === 'Enter' && list[k]) { close(); list[k].f(); } };
  draw(); q.focus();
}
function paletteItems(kind) {
  const sess = S.sessions.map(s => ({ label: s.title || t('Nova sessão'), d: projectLabel(s.root) + ' · ' + ago(s.updated), f: () => openSession(s.root, s.id) }));
  if (kind === 'sessions') return sess;
  const v = V();
  return [
    { label: 'Nova sessão', d: 'Ctrl N', f: () => newSessionScreen() }, { label: 'Agendadas', f: () => openScheduled() }, { label: 'Personalizar', f: () => openCustomize() }, { label: 'Configurações', d: 'Ctrl ,', f: () => openSettings() },
    { label: 'Provedores e chaves', f: () => openSettings('providers') },
    { label: 'Voltar no tempo…', d: 'Esc Esc', f: () => rewindDialog(v) }, { label: 'Pergunta paralela', d: 'Ctrl ;', f: () => openSide(v) },
    { label: 'Alterações', d: 'Ctrl Shift D', f: () => togglePane('changes', true) }, { label: 'Terminal', d: 'Ctrl `', f: () => togglePane('terminal', true) }, { label: 'Arquivos', f: () => togglePane('files', true) }, { label: 'Navegador', d: 'Ctrl Shift B', f: () => togglePane('browser', true) },
    { label: 'Tarefas', f: () => togglePane('tasks', true) }, { label: 'Plano', f: () => togglePane('plan', true) }, { label: 'Pronto (critérios)', f: () => togglePane('oracle', true) }, { label: 'Memória', f: () => togglePane('memory', true) }, { label: 'Regras', f: () => togglePane('rules', true) }, { label: 'Jornal', f: () => togglePane('journal', true) }, { label: 'Vícios', f: () => togglePane('vices', true) }, { label: 'Mapa do projeto', f: () => togglePane('map', true) },
    ...Object.entries(modeInfo).map(([k, [l]]) => ({ label: t('Modo: ') + t(l), f: () => setSessionOpts(v, { mode: k }) })),
    { label: 'Exibição: Normal', f: () => setViewMode('normal') }, { label: 'Exibição: Pensamento', f: () => setViewMode('thinking') }, { label: 'Exibição: Detalhado', d: 'Ctrl O', f: () => setViewMode('verbose') },
    { label: 'Barra lateral', d: 'Ctrl B', f: () => document.body.classList.toggle('nosb') }, { label: 'Atalhos de teclado', d: 'Ctrl /', f: shortcutsDialog },
    ...sess,
  ];
}
function shortcutsDialog() {
  const rows = [['Nova sessão', 'Ctrl N'], ['Paleta de comandos', 'Ctrl K'], ['Trocar de sessão', 'Ctrl Tab'], ['Próxima / anterior na lista', 'Ctrl Shift ] / ['], ['Fechar sessão', 'Ctrl W'], ['Parar o agente', 'Esc'], ['Voltar no tempo', 'Esc Esc'], ['Ciclar modo de permissão', 'Shift Tab'],
    ['Menu de modo', 'Ctrl Shift M'], ['Menu de modelo', 'Ctrl Shift I'], ['Menu de esforço', 'Ctrl Shift E'], ['Alterações', 'Ctrl Shift D'], ['Navegador', 'Ctrl Shift B'], ['Terminal', 'Ctrl `'], ['Fechar painel em foco', 'Ctrl \\'], ['Sair da tela dividida', 'Ctrl Alt W'],
    ['Pergunta paralela', 'Ctrl ;'], ['Ciclar exibição', 'Ctrl O'], ['Barra lateral', 'Ctrl B'], ['Aprovar / sempre / negar', 'Y / A / N'], ['Histórico de prompts', '↑ / ↓'], ['Item do menu aberto', '1–9'], ['Enviar comentários do diff', 'Ctrl Enter'], ['Abrir sessão lado a lado', 'Alt + clique']];
  const dlg = h(`<div class="dlg"><div class="dh">${t('Atalhos de teclado')}</div><div class="db"><div class="sc">${rows.map(([a, b]) => `<span>${t(a)}</span><span class="kbd">${b}</span>`).join('')}</div></div></div>`); overlay(dlg);
}
function openSide(v, first) {
  const L = v.sid && S.live[v.sid]; if (!L) return toast('Abra uma sessão primeiro');
  if (v.sideEl) { v.sideEl.remove(); v.sideEl = null; if (!first) return; }
  const el = h(`<div class="side"><div class="sh">${t('Pergunta paralela')}<span class="faint" style="font-weight:400">${t('não entra no histórico')}</span><span class="grow"></span><button class="ib">${I.close}</button></div><div class="sb"></div><textarea rows="1" placeholder="${esc(t('Pergunte sobre a sessão… Enter envia'))}"></textarea></div>`);
  v.sideEl = el; v.el.appendChild(el); const sb = $('.sb', el), ta = $('textarea', el);
  $('.sh button', el).onclick = () => { el.remove(); v.sideEl = null; };
  const ask = async q => { sb.appendChild(h(`<div class="sq">${esc(q)}</div>`)); const w = h(`<div class="status"><span class="spin"></span></div>`); sb.appendChild(w); sb.scrollTop = sb.scrollHeight; const r = await api(`/api/session/${L.id}/btw`, { body: { question: q }, root: L.root }); w.replaceWith(h(`<div class="md sel">${r.ok ? md(r.answer) : `<span class="del">${esc(r.error)}</span>`}</div>`)); sb.scrollTop = sb.scrollHeight; };
  ta.onkeydown = e => { e.stopPropagation(); if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); const q = ta.value.trim(); if (q) { ta.value = ''; ask(q); } } if (e.key === 'Escape') { el.remove(); v.sideEl = null; } };
  ta.focus(); if (first) ask(first);
}
function pathMenu(path, ev) {
  const root = curDir(); const abs = /^([a-zA-Z]:|\/)/.test(path) ? path : root.replace(/[\\/]+$/, '') + (root.includes('\\') ? '\\' : '/') + path.replace(/\//g, root.includes('\\') ? '\\' : '/');
  const rel = abs.startsWith(root) ? abs.slice(root.length).replace(/^[\\/]/, '').replace(/\\/g, '/') : path;
  menu(null, [
    { label: 'Abrir no painel Arquivos', f: () => openFileInPane(rel) },
    { label: 'Anexar como contexto', f: () => { const v = V(); if (!v.atts.find(a => a.path === abs)) v.atts.push({ path: abs, name: baseName(abs), size: 0 }); renderAtts(v); } },
    { sep: 1 }, ...openInItems(abs), { sep: 1 },
    { label: 'Copiar caminho absoluto', f: () => navigator.clipboard.writeText(abs) }, { label: 'Copiar caminho relativo', f: () => navigator.clipboard.writeText(rel) }, { label: 'Copiar nome do arquivo', f: () => navigator.clipboard.writeText(baseName(abs)) },
  ], ev);
}

// ---------------------------------------------------------------- keyboard
// Capture phase: app shortcuts win over the terminal (xterm swallows keys). Inside the terminal only the
// app-wide chords below are taken; everything else (Ctrl+C, Ctrl+W, Ctrl+B…) goes to the shell.
const APP_CHORDS = new Set(['ctrl+shift+s', 'ctrl+n', 'ctrl+k', 'ctrl+,', 'ctrl+/', 'ctrl+`', 'ctrl+\\', 'ctrl+;', 'ctrl+tab', 'ctrl+shift+tab', 'ctrl+shift+d', 'ctrl+shift+b', 'ctrl+shift+m', 'ctrl+shift+i', 'ctrl+shift+e', 'ctrl+shift+]', 'ctrl+shift+[', 'ctrl+alt+w']);
addEventListener('keydown', e => {
  if (document.querySelector('.overlay')) return;
  const k = e.key.toLowerCase(), ctrl = e.ctrlKey || e.metaKey; const v = V(); const L = curL();
  const inTerm = !!document.activeElement?.closest?.('.xterm');
  const chord = (ctrl ? 'ctrl+' : '') + (e.altKey ? 'alt+' : '') + (e.shiftKey && ctrl ? 'shift+' : '') + (e.code === 'Backquote' ? '`' : e.code === 'BracketRight' ? ']' : e.code === 'BracketLeft' ? '[' : k);
  if (inTerm && !APP_CHORDS.has(chord)) return;
  if (inTerm) e.stopPropagation();
  const inField = /INPUT|TEXTAREA/.test(document.activeElement?.tagName) || inTerm;
  if (openMenu && !AC && /^[1-9]$/.test(e.key) && !ctrl) { const it = $$('.mi', openMenu)[+e.key - 1]; if (it) { e.preventDefault(); it.click(); } return; }
  if (openMenu && e.key === 'Escape') { closeMenu(); e.preventDefault(); return; }
  if (ctrl && !e.shiftKey && k === 'k') { e.preventDefault(); palette(); return; }
  if (ctrl && k === 'n') { e.preventDefault(); newSessionScreen(); return; }
  if (ctrl && k === ',') { e.preventDefault(); openSettings(); return; }
  if (ctrl && k === '/') { e.preventDefault(); shortcutsDialog(); return; }
  if (ctrl && !e.shiftKey && k === 'b') { e.preventDefault(); document.body.classList.toggle('nosb'); return; }
  if (ctrl && k === 'w' && !e.altKey) { e.preventDefault(); if (S.views.length > 1) exitSplit(); else if (v?.sid) newSessionScreen(curRoot()); return; }
  if (ctrl && e.altKey && k === 'w') { e.preventDefault(); exitSplit(); return; }
  if (e.ctrlKey && e.key === 'Tab') { e.preventDefault(); const ids = S.recent.filter(id => S.sessions.find(s => s.id === id)); if (ids.length > 1) { const s = S.sessions.find(x => x.id === ids[e.shiftKey ? ids.length - 1 : 1]); if (s) openSession(s.root, s.id); } return; }
  if (ctrl && e.shiftKey && (e.key === ']' || e.key === '[' || e.code === 'BracketRight' || e.code === 'BracketLeft')) { e.preventDefault(); const order = $$('#sessList .srow').map(r => r); const ids = S.sessions.map(s => s.id); const i = ids.indexOf(v?.sid); const s = S.sessions[(i + ((e.code === 'BracketRight' || e.key === ']') ? 1 : -1) + ids.length) % ids.length]; if (s) openSession(s.root, s.id); return; }
  if (ctrl && e.shiftKey && k === 'd') { e.preventDefault(); togglePane('changes'); return; }
  if (ctrl && e.shiftKey && k === 'b') { e.preventDefault(); togglePane('browser'); return; }
  if (ctrl && e.shiftKey && k === 's') { e.preventDefault(); togglePane('browser', true); setTimeout(() => paneCall('browser', 'pick'), 100); return; }
  if (e.ctrlKey && (e.key === '`' || e.code === 'Backquote')) { e.preventDefault(); togglePane('terminal'); return; }
  if (ctrl && e.key === '\\') { e.preventDefault(); closeFocusedPane(); return; }
  if (ctrl && e.key === ';') { e.preventDefault(); openSide(v); return; }
  if (ctrl && !e.shiftKey && k === 'o') { e.preventDefault(); const order = ['normal', 'thinking', 'verbose']; setViewMode(order[(order.indexOf(S.viewMode) + 1) % 3]); return; }
  if (ctrl && e.shiftKey && k === 'm') { e.preventDefault(); modeMenu(v, $('.modechip', v.el)); return; }
  if (ctrl && e.shiftKey && k === 'i') { e.preventDefault(); modelMenu(v, $('.under .chip:last-child', v.el)); return; }
  if (ctrl && e.shiftKey && k === 'e') { e.preventDefault(); modelMenu(v, $('.under .chip:last-child', v.el), true); return; }
  if (!inField && L?.snap?.pendingPermission && !ctrl) { const id = L.snap.pendingPermission.id; if (k === 'y') { decide(v, id, 'allow'); return; } if (k === 't') { decide(v, id, 'all'); return; } if (k === 'a') { decide(v, id, 'always'); return; } if (k === 'n') { decide(v, id, 'deny'); return; } }
  if (!inField && L?.snap?.pendingQuestion && /^[1-9]$/.test(e.key)) { const o = L.snap.pendingQuestion.options?.[+e.key - 1]; if (o) answer(v, L.snap.pendingQuestion.id, o); return; }
  if (e.key === 'Escape' && !inField && busy(L)) { stop(v); return; }
  if (!inField && !ctrl && e.key.length === 1 && v?.el && !S.page) { $('.prompt', v.el).focus(); }
}, true);

// drag & drop: folders become projects, files attachments (native window reads real paths)
let dragDepth = 0;
addEventListener('dragenter', e => { if (e.dataTransfer.types.includes('Files')) { dragDepth++; $('#drop').classList.add('show'); } });
addEventListener('dragleave', () => { if (--dragDepth <= 0) { dragDepth = 0; $('#drop').classList.remove('show'); } });
addEventListener('dragover', e => { if (e.dataTransfer.types.includes('Files')) e.preventDefault(); });
addEventListener('drop', async e => {
  dragDepth = 0; $('#drop').classList.remove('show');
  if (!e.dataTransfer.files.length) return; e.preventDefault(); const v = V(); const files = [...e.dataTransfer.files];
  if (native) {
    const r = await infuse.drop(files);
    if (r.projects?.length) { await loadSessions(); newSessionScreen(r.projects[0]); }
    for (const a of r.attachments || []) if (!v.atts.find(x => x.path === a.path)) v.atts.push(a);
    renderAtts(v);
  } else { files.forEach(f => readBrowserFile(v, f)); if ([...e.dataTransfer.items].some(i => i.webkitGetAsEntry?.()?.isDirectory)) toast('Para abrir uma pasta use o seletor de pasta abaixo da caixa de texto'); }
});

// ---------------------------------------------------------------- settings helpers & boot
async function saveSetting(patch) { const r = await api('/api/settings', { method: 'PUT', body: patch, root: '' }); if (r.error) toast(r.error); Object.assign(S.settings, patch); return r; }
async function loadSettings() { const r = await api('/api/settings/full', { root: '' }); S.settings = r.settings || {}; S.version = r.version; S.shell = r.shell; }
async function loadUsage() { const u = await api('/api/usage', { root: '' }); S.usage = u; $('#usageToday').textContent = u.today ? t('hoje ') + money(u.today) : ''; }

async function boot() {
  await loadSettings(); applyTheme();
  S.lang = (S.settings.language || '').toLowerCase().startsWith('en') ? 'en' : 'pt'; startTranslation();
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
