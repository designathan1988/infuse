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
  pc: '<svg class="i" viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/></svg>',
};

// ---------------------------------------------------------------- state
const S = {
  settings: {}, projects: [], sessions: [], archived: [], models: [], connections: [], editors: [],
  commands: {}, files: {}, live: {}, views: [], focus: 0, page: null,
  history: lsGet('hist', []), sb: lsGet('sb', { group: 'folder', sort: 'updated', closed: {} }),
  viewMode: lsGet('viewMode', 'normal'), rail: lsGet('rail', true), recent: [], lang: 'pt',
};
function lsGet(k, d) { try { const v = localStorage.getItem(k); return v ? JSON.parse(v) : d; } catch { return d; } }
function lsSet(k, v) { try { localStorage.setItem(k, JSON.stringify(v)); } catch { } }
const native = !!window.infuse;

async function api(url, opts = {}) {
  const root = opts.root ?? curRoot();
  if (root && !/[?&]root=/.test(url)) url += (url.includes('?') ? '&' : '?') + 'root=' + encodeURIComponent(root);
  if (opts.dir && !/[?&]dir=/.test(url)) url += '&dir=' + encodeURIComponent(opts.dir);
  const r = await fetch(url, { method: opts.method || (opts.body ? 'POST' : 'GET'), headers: { 'content-type': 'application/json' }, body: opts.body ? JSON.stringify(opts.body) : undefined });
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
    <div class="vbody"><div class="vchat"><div class="scroller"><div class="chat"></div></div>
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
  let list = (archivedMode ? S.archived : S.sessions).filter(s => !q || (s.title || '').toLowerCase().includes(q) || (s.root || '').toLowerCase().includes(q) || (s.branch || '').toLowerCase().includes(q));
  const sortKey = S.sb.sort; list.sort((a, b) => sortKey === 'title' ? (a.title || '').localeCompare(b.title || '') : (b[sortKey] || 0) - (a[sortKey] || 0));
  const openIds = S.views.map(v => v.sid);
  const row = s => {
    const L = S.live[s.id]; const st = L?.snap?.state || s.state;
    const r = h(`<div class="srow${openIds[S.focus] === s.id ? ' on' : ''}${openIds.includes(s.id) && openIds[S.focus] !== s.id ? ' split' : ''}" draggable="true"><span class="st ${stateClass(st)}"></span><span class="t">${esc(L?.snap?.title || s.title || t('Nova sessão'))}</span><span class="w">${ago(s.updated)}</span><button class="ib more" title="Mais">${I.more}</button></div>`);
    r.title = (s.branch ? s.branch + ' · ' : '') + s.root;
    r.onclick = e => { if (e.target.closest('.more')) return sessionRowMenu(s, e.target.closest('.more')); if (archivedMode) return; if (e.altKey) openInSplit(s.root, s.id); else openSession(s.root, s.id); };
    r.oncontextmenu = e => { e.preventDefault(); sessionRowMenu(s, null, e); };
    r.ondragstart = e => e.dataTransfer.setData('text/infuse-session', JSON.stringify({ root: s.root, id: s.id }));
    return r;
  };
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
  if (archivedMode) el.prepend(h(`<div class="srow" style="color:var(--accent)" onclick="S.sb.show='';lsSet('sb',S.sb);renderSidebar()">← ${t('Voltar às sessões')}</div>`));
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
    { label: 'Abrir em tela dividida', d: 'Alt+clique', f: () => openInSplit(s.root, s.id) },
    { label: 'Renomear', f: () => { openSession(s.root, s.id).then(() => renameInline(V())); } },
    { sep: 1 }, ...openInItems(s.root),
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
    { sep: 1 }, ...openInItems(root), { sep: 1 },
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
let openMenu = null;
function closeMenu() { if (openMenu) { openMenu.remove(); openMenu = null; } }
function menu(anchor, items, ev, opts = {}) {
  closeMenu();
  const m = h('<div class="menu"></div>'); let n = 0;
  for (const it of items) {
    if (!it) continue;
    if (it.sep) { m.appendChild(h('<div class="ms"></div>')); continue; }
    if (it.h) { m.appendChild(h(`<div class="mh">${esc(t(it.h))}</div>`)); continue; }
    if (it.html) { m.appendChild(it.html); continue; }
    n++;
    const row = h(`<div class="mi${it.danger ? ' danger' : ''}${it.sub ? ' col' : ''}">${it.check !== undefined ? `<span class="ck">${it.check ? '✓' : ''}</span>` : ''}${it.icon || ''}<span class="two"><span>${esc(t(it.label))}</span>${it.sub ? `<small>${esc(t(it.sub))}</small>` : ''}</span>${it.d ? `<span class="d">${esc(t(it.d))}</span>` : n <= 9 && opts.numbers ? `<span class="d">${n}</span>` : ''}</div>`);
    row.onclick = e => { e.stopPropagation(); if (!it.keep) closeMenu(); it.f && it.f(); };
    row.dataset.n = n;
    m.appendChild(row);
  }
  document.body.appendChild(m); openMenu = m;
  let x, y;
  if (ev) { x = ev.clientX; y = ev.clientY; } else { const r = anchor.getBoundingClientRect(); x = r.left; y = opts.above ? r.top - m.offsetHeight - 6 : r.bottom + 4; if (opts.alignRight) x = r.right - m.offsetWidth; }
  x = Math.max(6, Math.min(x, innerWidth - m.offsetWidth - 6)); y = Math.max(6, Math.min(y, innerHeight - m.offsetHeight - 6));
  m.style.left = x + 'px'; m.style.top = y + 'px';
  return m;
}
addEventListener('mousedown', e => { if (openMenu && !openMenu.contains(e.target)) closeMenu(); });

function sessionMenu(v, anchor) {
  const L = S.live[v.sid]; if (!L) return;
  const modes = [['normal', 'Normal', 'Ferramentas resumidas, texto completo'], ['thinking', 'Pensamento', 'Também mostra o raciocínio'], ['verbose', 'Detalhado', 'Cada chamada de ferramenta aberta']];
  menu(anchor, [
    { h: 'Modo de exibição' }, ...modes.map(([k, l, d]) => ({ label: l, sub: d, check: S.viewMode === k, f: () => setViewMode(k) })),
    { sep: 1 }, { label: 'Renomear', f: () => renameInline(v) },
    { label: 'Voltar no tempo…', d: 'Esc Esc', f: () => rewindDialog(v) },
    { label: 'Pergunta paralela', d: 'Ctrl ;', f: () => openSide(v) },
    { label: S.views.length > 1 ? 'Sair da tela dividida' : 'Abrir em tela dividida', f: () => S.views.length > 1 ? exitSplit() : openInSplit(L.root, L.id, true) },
    { sep: 1 }, ...openInItems(L.snap.workRoot || L.root),
    { sep: 1 }, { label: 'Arquivar', f: () => archiveSession({ id: L.id, root: L.root }) }, { label: 'Excluir', danger: 1, f: () => deleteSession({ id: L.id, root: L.root }) },
  ]);
}
function moreMenu(v, anchor) {
  menu(anchor, [
    { label: 'Tarefas', f: () => togglePane('tasks') }, { label: 'Plano', f: () => togglePane('plan') },
    { sep: 1 }, { h: 'Filtro' }, { label: 'Pronto (critérios)', f: () => togglePane('oracle') }, { label: 'Regras', f: () => togglePane('rules') }, { label: 'Jornal', f: () => togglePane('journal') }, { label: 'Vícios', f: () => togglePane('vices') }, { label: 'Mapa do projeto', f: () => togglePane('map') },
    { sep: 1 }, { label: S.rail ? 'Ocultar detalhes da sessão' : 'Mostrar detalhes da sessão', f: () => { S.rail = !S.rail; lsSet('rail', S.rail); S.views.forEach(renderView); } },
  ], null, { alignRight: true });
}
function setViewMode(k) { S.viewMode = k; lsSet('viewMode', k); S.views.forEach(renderChat); toast(t('Exibição: ') + t({ normal: 'Normal', thinking: 'Pensamento', verbose: 'Detalhado' }[k])); }

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
    case 'session': L.snap = norm(d); if (!busy(L)) { L.text = ''; L.think = ''; L.tools = {}; } vs.forEach(renderView); break;
    case 'state':
      if (L.snap) L.snap.state = d.state;
      if (d.state === 'thinking') { L.text = ''; L.think = ''; if (!L.started) L.started = Date.now(); }
      if (d.state === 'idle' || d.state === 'error') { L.started = 0; drainQueue(L); }
      vs.forEach(v => { renderLive(v); renderComposer(v); }); renderSidebarState(L); break;
    case 'turn': if (L.snap) { L.snap.turn = d.turn; L.snap.title = d.title; L.snap.turnStarts.push(L.snap.messages.length); L.snap.messages.push({ role: 'user', content: [{ type: 'text', text: d.text }] }); } vs.forEach(renderView); loadSessions(); break;
    case 'stream':
      if (d.kind === 'text') L.text += d.text; else if (d.kind === 'thinking') L.think += d.text;
      else if (d.kind === 'tool_start') L.tools[d.id] = { name: d.name, input: '' }; else if (d.kind === 'tool_input') (L.tools[d.id] = L.tools[d.id] || { name: '', input: '' }).input += d.text;
      vs.forEach(renderLive); break;
    case 'message': L.snap.messages.push(d.message); L.snap.usage = d.totalUsage; L.snap.cost = d.cost; L.lastIn = d.lastInput; L.lastOut = d.usage?.output || 0; L.text = ''; L.think = ''; L.tools = {}; re(); vs.forEach(renderRail); break;
    case 'tool_run': L.running = d; vs.forEach(renderLive); break;
    case 'tool_result': L.running = null; L.snap.messages.push({ role: 'user', content: [{ type: 'tool_result', tool_use_id: d.id, content: d.output, is_error: d.isError }] }); vs.forEach(renderChat); refreshChanges(L); break;
    case 'permission': L.snap.pendingPermission = d; vs.forEach(renderChat); attention(L, 'infuse precisa de você', t('Aprovar ') + d.tool + '?'); break;
    case 'question': L.snap.pendingQuestion = d; vs.forEach(renderChat); attention(L, 'infuse tem uma pergunta', d.question); break;
    case 'done': L.snap.pendingPermission = null; L.snap.pendingQuestion = null; L.snap.cost = d.cost; L.snap.usage = d.usage; re(); vs.forEach(renderRail); attention(L, 'infuse terminou', L.snap.title || ''); refreshChanges(L); loadUsage(); break;
    case 'error': L.snap.error = d; L.snap.pendingPermission = null; L.snap.pendingQuestion = null; re(); attention(L, 'infuse: erro', String(d)); break;
    case 'tasks': L.snap.tasks = d; vs.forEach(renderRail); paneRefresh('tasks'); break;
    case 'cwd': L.snap.cwd = d; break;
    case 'checkpoint': (L.snap.checkpoints[d.turn] = L.snap.checkpoints[d.turn] || []).push({ path: d.path }); break;
    case 'map': paneRefresh('map'); paneRefresh('files'); break;
    case 'compacted': vs.forEach(renderChat); toast('Contexto compactado'); break;
  }
}
function renderSidebarState(L) { const r = $$('#sessList .srow').find(x => x.querySelector('.t') && S.sessions.find(s => s.id === L.id) && x.title.endsWith(L.root)); renderSidebar(); }
function attention(L, title, body) {
  if (S.settings.notify === false) return;
  const viewing = S.views[S.focus]?.sid === L.id && document.hasFocus();
  if (!viewing) nat('notify', t(title), t(body));
}
async function refreshChanges(L) {
  if (!L?.snap) return;
  L.changes = await api('/api/changes', { root: L.root, dir: L.snap.workRoot || L.root });
  viewsOf(L).forEach(v => { renderRail(v); renderChangesBadge(v); });
  paneRefresh('changes');
}

async function openSession(root, id) {
  closeMenu(); S.page = null; showPage(null);
  const v = V() || (S.views[0] = newView());
  if (S.views.some((x, i) => x.sid === id && i !== S.focus)) { S.focus = S.views.findIndex(x => x.sid === id); mountViews(); renderSidebar(); renderPanes(); return; }
  v.sid = id; v.root = root;
  const L = connect(root, id);
  if (!L.snap) L.snap = norm(await api(`/api/sessions/${id}`, { root }));
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
    if (starts.has(i)) { turn++; out += `<div class="turnsep"><span class="l"></span><button class="btn sm" data-rw="${turn}" title="${esc(t('Restaurar código e/ou conversa a antes deste turno'))}">↶ ${t('voltar aqui')}</button></div>`; }
    if (m.role === 'user') {
      const texts = m.content.filter(b => b.type === 'text'), imgs = m.content.filter(b => b.type === 'image');
      if (!texts.length && !imgs.length) return;
      const main = texts[0]?.text || '';
      if (main.startsWith('[Conversa resumida]') || main.startsWith('[Contexto compactado')) { out += `<div class="marker" title="${esc(main.slice(0, 600))}">${t('Conversa resumida')}</div>`; return; }
      const attached = texts.slice(1).map(b => (b.text.match(/path="([^"]+)"/) || [])[1]).filter(Boolean);
      out += `<div class="u sel">${esc(main)}${attached.length ? `<div class="att">${attached.map(p => `<span class="chipx" data-path="${esc(p)}">📎 <span>${esc(baseName(p))}</span></span>`).join('')}</div>` : ''}${imgs.map(b => `<img src="data:${b.mediaType};base64,${b.data}">`).join('')}</div>`;
    } else {
      if (m.content.length === 2 && m.content[0]?.text === 'Entendido, continuo a partir desse estado.') return;
      out += '<div class="a">';
      for (const b of m.content) {
        if (b.type === 'text' && b.text) out += `<div class="md sel">${md(b.text)}</div>`;
        else if (b.type === 'thinking' && b.thinking && S.viewMode !== 'normal') out += `<details class="think"${S.viewMode !== 'normal' ? ' open' : ''}><summary>${t('Raciocínio')}</summary><pre>${esc(b.thinking)}</pre></details>`;
        else if (b.type === 'thinking' && b.thinking) out += `<details class="think"><summary>${t('Pensou')}</summary><pre>${esc(b.thinking)}</pre></details>`;
        else if (b.type === 'tool_use') out += toolBlock(b.id, b.name, b.input, results[b.id], false);
      }
      out += '</div>';
    }
  });
  out += '<div class="live"></div>';
  if (s.pendingPermission) out += permCard(s.pendingPermission);
  if (s.pendingQuestion) out += questionCard(s.pendingQuestion);
  if (s.error && s.state === 'error') out += `<div class="errbox">${esc(s.error)}</div>`;
  if (!msgs.length) out = `<div class="empty" style="padding-top:14vh">${t('Sessão vazia. Descreva a feature, a correção ou a dúvida.')}</div>` + out;
  const sc = $('.scroller', v.el); const atBottom = sc.scrollHeight - sc.scrollTop - sc.clientHeight < 120;
  chat.innerHTML = out; bindChat(v, chat); renderLive(v);
  if (atBottom || v._jump !== v.sid) { sc.scrollTop = sc.scrollHeight; v._jump = v.sid; }
}
function bindChat(v, root) {
  $$('[data-rw]', root).forEach(b => b.onclick = () => rewindDialog(v, +b.dataset.rw));
  $$('.tool .h', root).forEach(x => x.onclick = () => x.parentNode.classList.toggle('open'));
  $$('[data-path]', root).forEach(x => { x.onclick = () => openFileInPane(x.dataset.path); x.oncontextmenu = e => { e.preventDefault(); pathMenu(x.dataset.path, e); }; });
  $$('[data-perm]', root).forEach(b => b.onclick = () => decide(v, b.dataset.id, b.dataset.perm));
  $$('[data-ans]', root).forEach(b => b.onclick = () => answer(v, b.dataset.id, b.dataset.ans));
  const free = $('.free input', root); if (free) free.onkeydown = e => { if (e.key === 'Enter' && free.value.trim()) answer(v, free.dataset.id, free.value.trim()); };
}
function renderLive(v) {
  if (!v.el) return; const el = $('.live', v.el); const L = v.sid && S.live[v.sid]; if (!el || !L) return;
  let out = '';
  if (L.think) out += `<details class="think"${S.viewMode !== 'normal' ? ' open' : ''}><summary>${t('pensando…')}</summary><pre>${esc(L.think)}</pre></details>`;
  if (L.text) out += `<div class="a"><div class="md sel">${md(L.text)}<span class="cursor"></span></div></div>`;
  for (const [id, tl] of Object.entries(L.tools)) { let inp; try { inp = JSON.parse(tl.input); } catch { inp = { _partial: tl.input }; } out += toolBlock(id, tl.name, inp, null, L.running?.id === id); }
  const st = L.snap?.state;
  if (st === 'thinking' || st === 'tool' || st === 'compacting') out += `<div class="status"><span class="spin"></span><span>${st === 'tool' ? t('executando ') + esc(L.running?.name || '') : st === 'compacting' ? t('compactando contexto') : t('pensando')}</span><span class="faint el" data-el="${L.id}"></span></div>`;
  el.innerHTML = out; bindChat(v, el);
  const sc = $('.scroller', v.el); if (sc.scrollHeight - sc.scrollTop - sc.clientHeight < 200) sc.scrollTop = sc.scrollHeight;
}
setInterval(() => { for (const el of $$('[data-el]')) { const L = S.live[el.dataset.el]; if (L?.started) el.textContent = Math.round((Date.now() - L.started) / 1000) + 's'; } }, 500);

const toolLabel = { Bash: 'Executou', Read: 'Leu', Write: 'Escreveu', Edit: 'Editou', Glob: 'Procurou arquivos', Grep: 'Buscou', LS: 'Listou', WebFetch: 'Buscou na web', TodoWrite: 'Atualizou tarefas', AskUserQuestion: 'Perguntou' };
function toolSummary(name, inp) {
  if (!inp) return ''; if (inp._partial !== undefined) return inp._partial.slice(-90);
  switch (name) { case 'Bash': return inp.command || ''; case 'Read': case 'Write': case 'Edit': return inp.path || inp.file_path || ''; case 'Glob': return inp.pattern || ''; case 'Grep': return `${inp.pattern || ''}${inp.path ? ' · ' + inp.path : ''}`; case 'LS': return inp.path || '.'; case 'WebFetch': return inp.url || ''; case 'TodoWrite': return `${(inp.todos || inp.tasks || []).length} ${t('itens')}`; case 'AskUserQuestion': return inp.question || ''; default: return JSON.stringify(inp).slice(0, 120); }
}
function lineCounts(name, inp) {
  if (name === 'Write' && inp?.content != null) return `<span class="cnt add">+${inp.content.split('\n').length}</span>`;
  if (name === 'Edit' && inp?.old_string != null) return `<span class="cnt"><span class="add">+${(inp.new_string || '').split('\n').length}</span> <span class="del">−${inp.old_string.split('\n').length}</span></span>`;
  return '';
}
function toolBlock(id, name, input, result, running) {
  let inp = input; if (typeof inp === 'string') { try { inp = JSON.parse(inp); } catch { inp = {}; } }
  const isPath = ['Read', 'Write', 'Edit'].includes(name) && (inp?.path || inp?.file_path);
  let body = '';
  if (name === 'Bash' && inp?.command) body += `<div class="lab">${t('comando')}</div><pre>${esc(inp.command)}</pre>`;
  else if (name === 'Write' && inp?.content != null) body += diffHTML(inp.content.split('\n').map(l => '+' + l).join('\n'));
  else if (name === 'Edit' && inp?.old_string != null) body += diffHTML(inp.old_string.split('\n').map(l => '-' + l).join('\n') + '\n' + (inp.new_string || '').split('\n').map(l => '+' + l).join('\n'));
  else if (inp && Object.keys(inp).length && name !== 'TodoWrite') body += `<div class="lab">${t('entrada')}</div><pre>${esc(JSON.stringify(inp, null, 2))}</pre>`;
  if (name === 'TodoWrite' && Array.isArray(inp?.todos || inp?.tasks)) body += (inp.todos || inp.tasks).map(x => `<div>${x.status === 'done' ? '☑' : x.status === 'in_progress' ? '◐' : '☐'} ${esc(x.text || x.content || '')}</div>`).join('');
  if (result) { const o = String(result.content || ''); const k = o.indexOf('\n[infuse'); const main = k >= 0 ? o.slice(0, k) : o, inf = k >= 0 ? o.slice(k + 1) : ''; body += `<div class="lab">${t('resultado')}</div><pre>${esc(main.length > 20000 ? main.slice(0, 20000) + '\n…' : main)}</pre>` + (inf ? `<div class="lab">${t('infundido')}</div><pre class="inf">${esc(inf)}</pre>` : ''); }
  const dot = running ? 'run' : result ? (result.is_error ? 'err' : '') : 'pend';
  const open = S.viewMode === 'verbose' || result?.is_error;
  const summ = isPath ? `<span class="s"><span class="path" data-path="${esc(inp.path || inp.file_path)}">${esc(inp.path || inp.file_path)}</span></span>` : `<span class="s">${esc(toolSummary(name, inp))}</span>`;
  return `<div class="tool${open ? ' open' : ''}"><div class="h"><span class="dot ${dot}"></span><b>${esc(t(toolLabel[name] || name))}</b>${summ}${lineCounts(name, inp)}</div><div class="b">${body}</div></div>`;
}
const diffHTML = text => `<div class="diff">${text.split('\n').map(l => `<div class="${l.startsWith('+') && !l.startsWith('+++') ? 'a1' : l.startsWith('-') && !l.startsWith('---') ? 'd1' : /^(@@|---|\+\+\+|diff )/.test(l) ? 'hh' : ''}">${esc(l) || ' '}</div>`).join('')}</div>`;
function permCard(p) {
  let inp = {}; try { inp = typeof p.input === 'string' ? JSON.parse(p.input) : p.input || {}; } catch { }
  const verb = p.category === 'exec' ? 'executar um comando' : p.category === 'write' ? 'alterar um arquivo' : p.category === 'net' ? 'acessar a web' : 'usar uma ferramenta';
  const detail = p.tool === 'Bash' ? `<pre class="cmd">${esc(inp.command || '')}</pre>` : p.tool === 'Edit' ? `<div class="mono" style="margin-top:8px">${esc(inp.path)}</div>` + diffHTML((inp.old_string || '').split('\n').map(l => '-' + l).join('\n') + '\n' + (inp.new_string || '').split('\n').map(l => '+' + l).join('\n')) : p.tool === 'Write' ? `<div class="mono" style="margin-top:8px">${esc(inp.path)}</div>` + diffHTML((inp.content || '').split('\n').slice(0, 200).map(l => '+' + l).join('\n')) : p.tool === 'WebFetch' ? `<pre class="cmd">${esc(inp.url || '')}</pre>` : `<pre class="cmd">${esc(JSON.stringify(inp, null, 2))}</pre>`;
  return `<div class="pcard"><div class="hd"><b>${esc(p.tool)}</b><span class="muted">${t('quer')} ${t(verb)}</span></div>${detail}
  <div class="opts"><div class="opt" data-perm="allow" data-id="${p.id}">${t('Sim')}<span class="kbd">Y</span></div><div class="opt" data-perm="always" data-id="${p.id}">${t('Sim, e não perguntar de novo nesta sessão')}${p.tool === 'Bash' ? ` <span class="faint">(${esc((inp.command || '').trim().split(/\s+/).slice(0, 2).join(' '))})</span>` : ''}<span class="kbd">A</span></div><div class="opt" data-perm="deny" data-id="${p.id}">${t('Não, e dizer ao agente o que fazer diferente')}<span class="kbd">N</span></div></div></div>`;
}
function questionCard(q) {
  return `<div class="pcard"><div class="hd"><b>${esc(q.question)}</b></div><div class="opts">${(q.options || []).map((o, i) => `<div class="opt" data-ans="${esc(o)}" data-id="${q.id}">${esc(o)}<span class="kbd">${i + 1}</span></div>`).join('')}</div>
  <div class="free"><input data-id="${q.id}" placeholder="${esc(t('ou responda com suas palavras…'))}"></div></div>`;
}
async function decide(v, id, d) { const L = S.live[v.sid]; await api(`/api/sessions/${v.sid}/permission`, { body: { id, decision: d }, root: L.root }); if (L.snap) L.snap.pendingPermission = null; renderChat(v); if (d === 'deny') setTimeout(() => $('.prompt', v.el).focus(), 50); }
async function answer(v, id, a) { const L = S.live[v.sid]; await api(`/api/sessions/${v.sid}/answer`, { body: { id, answer: a }, root: L.root }); if (L.snap) L.snap.pendingQuestion = null; renderChat(v); }

// markdown (small, safe): code blocks, inline code (paths become links), headings, lists, quotes, tables, links
function md(src) {
  const blocks = [];
  src = src.replace(/```[\w+-]*\n([\s\S]*?)```/g, (_, c) => { blocks.push(`<pre><code>${esc(c.replace(/\n$/, ''))}</code></pre>`); return `\u0000${blocks.length - 1}\u0000`; });
  let x = esc(src);
  x = x.replace(/^\|(.+)\|\n\|[-:| ]+\|\n((?:\|.*\|\n?)*)/gm, (_, head, body) => `<table><tr>${head.split('|').map(c => `<th>${c.trim()}</th>`).join('')}</tr>${body.trim().split('\n').map(r => `<tr>${r.replace(/^\||\|$/g, '').split('|').map(c => `<td>${c.trim()}</td>`).join('')}</tr>`).join('')}</table>`);
  x = x.replace(/^### (.*)$/gm, '<h3>$1</h3>').replace(/^## (.*)$/gm, '<h2>$1</h2>').replace(/^# (.*)$/gm, '<h1>$1</h1>').replace(/^&gt; (.*)$/gm, '<blockquote>$1</blockquote>');
  x = x.replace(/`([^`\n]+)`/g, (_, c) => /^[\w.\-/\\]+\.[a-z0-9]{1,6}(:\d+)?$/i.test(c) && c.includes('.') ? `<code class="path" data-path="${c.replace(/:\d+$/, '')}">${c}</code>` : `<code>${c}</code>`);
  x = x.replace(/\*\*([^*\n]+)\*\*/g, '<b>$1</b>').replace(/(^|[^*\w])\*([^*\n]+)\*/g, '$1<i>$2</i>').replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  x = x.replace(/(?:^(?:\s*[-*] |\s*\d+\. ).*(?:\n|$))+/gm, m => { const ol = /^\s*\d/.test(m); return `<${ol ? 'ol' : 'ul'}>${m.trim().split('\n').map(l => `<li>${l.replace(/^\s*(?:[-*] |\d+\. )/, '')}</li>`).join('')}</${ol ? 'ol' : 'ul'}>`; });
  x = x.split(/\n{2,}/).map(p => /^\s*<(h\d|ul|ol|pre|table|blockquote)/.test(p) || /^\u0000\d+\u0000$/.test(p.trim()) ? p : `<p>${p.replace(/\n/g, '<br>')}</p>`).join('');
  return x.replace(/\u0000(\d+)\u0000/g, (_, i) => blocks[i]);
}

// ---------------------------------------------------------------- composer
const modeInfo = {
  default: ['Perguntar', 'Pede aprovação antes de editar arquivos ou rodar comandos'],
  acceptEdits: ['Aceitar edições', 'Edita arquivos sem perguntar; comandos ainda pedem aprovação'],
  plan: ['Plano', 'Explora e propõe um plano, sem editar nada'],
  bypass: ['Sem perguntas', 'Executa tudo sem pedir aprovação (o oráculo continua protegido)'],
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
  const mc = h(`<span class="chip mode-${o.mode}" title="${esc(t('Modo de permissão (Ctrl+Shift+M)'))}"><span class="v">${t(modeInfo[o.mode]?.[0] || o.mode)}</span>${I.caret}</span>`);
  mc.onclick = e => modeMenu(v, e.currentTarget); add(mc);
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
    { label: 'Abrir pasta…', d: native ? 'Explorador' : '', f: () => pickFolder(v) }];
  menu(anchor, items, null, { above: true });
}
async function pickFolder(v) {
  if (native) { const p = await nat('pickFolder'); if (p) { await loadSessions(); v.draft.root = p; lsSet('lastRoot', p); renderView(v); renderPanes(); } return; }
  browseFolders(v, lsGet('browsePath', ''));
}
async function browseFolders(v, path) {
  const r = await api('/api/fs?path=' + encodeURIComponent(path), { root: '' }); lsSet('browsePath', r.path || '');
  const anchor = $('.under .chip:nth-child(2)', v.el);
  const items = [{ h: r.path || t('Abrir pasta') }];
  if (r.path) items.push({ label: '↑ ' + (r.parent ? baseName(r.parent) || r.parent : t('unidades')), keep: 1, f: () => browseFolders(v, r.parent || '') });
  else r.roots.forEach(x => items.push({ label: x.name, d: x.path, keep: 1, f: () => browseFolders(v, x.path) }));
  (r.dirs || []).forEach(x => items.push({ label: x.name, keep: 1, f: () => browseFolders(v, x.path) }));
  if (r.path) items.push({ sep: 1 }, { label: 'Usar esta pasta', d: baseName(r.path), f: async () => { const res = await api('/api/projects/add', { body: { root: r.path }, root: '' }); if (res.error) return toast(res.error); await loadSessions(); v.draft.root = res.root; lsSet('lastRoot', res.root); renderView(v); renderPanes(); } });
  menu(anchor, items, null, { above: true });
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
  if (!native) { const inp = document.createElement('input'); inp.type = 'file'; inp.multiple = true; inp.onchange = () => { for (const f of inp.files) readBrowserFile(v, f); }; inp.click(); return; }
  const list = await nat('pickFiles'); if (Array.isArray(list)) { for (const a of list) if (!v.atts.find(x => x.path === a.path)) v.atts.push(a); renderAtts(v); }
}
function readImage(v, f) { const r = new FileReader(); r.onload = () => { v.images.push({ mediaType: f.type, data: r.result.split(',')[1] }); renderAtts(v); }; r.readAsDataURL(f); }
function readBrowserFile(v, f) { if (f.type.startsWith('image/')) return readImage(v, f); const r = new FileReader(); r.onload = () => { const P = $('.prompt', v.el); P.value += `\n<arquivo nome="${f.name}">\n${r.result.slice(0, 200000)}\n</arquivo>`; grow(P); }; r.readAsText(f); }
function renderAtts(v) {
  const el = $('.atts', v.el); el.innerHTML = '';
  v.atts.forEach((a, i) => { const c = h(`<span class="chipx" title="${esc(a.path)}">📎 <span>${esc(a.name)}</span><b>×</b></span>`); $('b', c).onclick = () => { v.atts.splice(i, 1); renderAtts(v); }; el.appendChild(c); });
  v.images.forEach((im, i) => { const c = h(`<span class="chipx"><img src="data:${im.mediaType};base64,${im.data}"><span>${t('imagem')}</span><b>×</b></span>`); $('b', c).onclick = () => { v.images.splice(i, 1); renderAtts(v); }; el.appendChild(c); });
}

async function send(v) {
  const P = $('.prompt', v.el); let text = P.value.trim();
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
    case 'mode': case 'permissions': modeMenu(v, $$('.under .chip', v.el).slice(-2)[0]); return true;
    case 'plan': setSessionOpts(v, { mode: 'plan' }); if (arg) { $('.prompt', v.el).value = arg; return false; } return true;
    case 'rewind': case 'undo': case 'checkpoint': rewindDialog(v); return true;
    case 'diff': togglePane('changes', true); return true;
    case 'cost': case 'usage': case 'context': contextDialog(v); return true;
    case 'btw': openSide(v, arg); return true;
    case 'config': case 'settings': openSettings(); return true;
    case 'resume': palette('sessions'); return true;
    case 'rename': if (L && arg) setSessionOpts(v, { title: arg }); else renameInline(v); return true;
    case 'review': case 'code-review': $('.prompt', v.el).value = t('Revise as alterações não commitadas deste projeto (git diff) procurando bugs de correção. Liste cada achado com arquivo:linha, o problema e a correção sugerida.') + (arg ? '\n' + arg : ''); return false;
    case 'init': $('.prompt', v.el).value = t('Analise este projeto e escreva um INFUSE.md na raiz com: como compilar, testar e rodar; a arquitetura e onde cada responsabilidade mora; convenções do código. Seja conciso.'); return false;
    case 'memory': openCustomize('instructions'); return true;
    case 'terminal': togglePane('terminal', true); return true;
    case 'vim': await saveSetting({ vim: !S.settings.vim }); toast(S.settings.vim ? 'Modo vim ligado' : 'Modo vim desligado'); S.views.forEach(renderComposer); return true;
    case 'help': shortcutsDialog(); return true;
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
  <div class="card"><div class="ch">${t('Alterações')}<span class="r mono">${c.isGit === false ? t('sem git') : `<span class="add">+${c.added || 0}</span> <span class="del">−${c.removed || 0}</span>`}</span></div>
    ${files.length ? files.slice(0, 8).map(f => `<div class="li" data-f="${esc(f.path)}"><span class="n" title="${esc(f.path)}">${esc(f.path)}</span><span class="add">+${f.added}</span><span class="del">−${f.removed}</span></div>`).join('') + (files.length > 8 ? `<div class="faint" style="padding:3px 4px">+${files.length - 8} ${t('arquivos')}</div>` : '') : `<div class="faint">${t('Nenhuma alteração')}</div>`}
    <div class="prline"></div></div>
  <div class="card ctxcard" style="cursor:pointer"><div class="ch">${t('Contexto')}<span class="r">${pct}%</span></div><div class="bar100"><i style="width:${pct}%"></i></div><div class="faint">${ktok(L.lastIn)} / ${ktok(S.settings.contextLimit || 200000)} tokens · ${money(L.snap.cost)}</div></div>
  <div class="card"><div class="ch">${t('Ambiente')}</div><div class="kv"><span>${t('Onde')}</span><span>${t('Este computador')}</span></div><div class="kv"><span>${t('Pasta')}</span><span title="${esc(L.snap.workRoot)}">${esc(baseName(L.snap.workRoot || L.root))}</span></div>${L.snap.branch ? `<div class="kv"><span>${t('Branch')}</span><span class="mono">${esc(L.snap.branch)}</span></div>` : ''}${L.snap.worktree ? `<div class="kv"><span>${t('Worktree')}</span><span>${t('isolado')}</span></div>` : ''}<div class="kv"><span>${t('Modelo')}</span><span class="mono">${esc(L.snap.model || '')}</span></div></div>
  ${tasks.length ? `<div class="card" style="cursor:pointer" data-open="tasks"><div class="ch">${t('Tarefas')}<span class="r">${tasks.filter(x => x.status === 'done').length}/${tasks.length}</span></div>${tasks.slice(0, 5).map(x => `<div style="font-size:12px;color:${x.status === 'done' ? 'var(--text3)' : 'var(--text)'}">${x.status === 'done' ? '☑' : x.status === 'in_progress' ? '◐' : '☐'} ${esc(x.text)}</div>`).join('')}</div>` : ''}
  <div class="card" style="cursor:pointer" data-open="oracle"><div class="ch">${t('Pronto')}<span class="r faint">${t('critérios do oráculo')}</span></div><div class="faint">${t('Definidos por você; o agente lê e não altera.')}</div></div>`;
  $$('[data-f]', el).forEach(x => x.onclick = () => { togglePane('changes', true); setTimeout(() => paneCall('changes', 'select', x.dataset.f), 50); });
  $('.ctxcard', el).onclick = () => contextDialog(v);
  $$('[data-open]', el).forEach(x => x.onclick = () => togglePane(x.dataset.open, true));
  renderPR(v, $('.prline', el));
}
async function renderPR(v, el) {
  const L = S.live[v.sid]; if (!L || !L.changes?.isGit || !el) return;
  if (!L.pr || Date.now() - L.prAt > 60000) { L.pr = await api('/api/pr', { root: L.root, dir: L.snap.workRoot || L.root }); L.prAt = Date.now(); }
  const pr = L.pr?.pr;
  if (!L.pr?.gh) { el.innerHTML = ''; return; }
  if (!pr) { el.innerHTML = `<div class="row" style="margin-top:6px"><button class="btn sm line" data-pr="create">${t('Criar PR')}</button></div>`; }
  else {
    const checks = pr.statusCheckRollup || []; const fail = checks.filter(c => /FAIL|ERROR/i.test(c.conclusion || c.state || '')).length, pend = checks.filter(c => !(c.conclusion || '') && !/SUCCESS/.test(c.state || '')).length;
    el.innerHTML = `<div class="row" style="margin-top:8px;font-size:12px"><a href="${esc(pr.url)}" target="_blank">#${pr.number}</a><span class="trunc grow">${esc(pr.title)}</span><span class="tag ${fail ? 'bad' : pend ? 'warn' : 'ok'}">${fail ? t('CI falhou') : pend ? t('CI rodando') : t('CI ok')}</span><button class="ib" data-pr="menu" style="width:22px;height:22px">${I.more}</button></div>`;
  }
  $$('[data-pr]', el).forEach(b => b.onclick = async e => {
    if (b.dataset.pr === 'create') { b.disabled = true; const r = await api('/api/pr', { body: { action: 'create' }, root: L.root, dir: L.snap.workRoot || L.root }); toast(r.ok ? 'PR criado' : (r.error || r.output)); L.prAt = 0; renderRail(v); }
    else menu(b, [{ label: 'Abrir no GitHub', f: () => window.open(pr.url, '_blank') }, { label: 'Mesclar quando pronto', sub: 'Ativa o auto-merge (squash) no GitHub', f: async () => { const r = await api('/api/pr', { body: { action: 'merge-auto' }, root: L.root, dir: L.snap.workRoot || L.root }); toast(r.ok ? 'Auto-merge ativado' : (r.error || r.output)); } }]);
  });
}
function renderChangesBadge(v) {
  const L = v.sid && S.live[v.sid]; const b = $('.chg', v.el); if (!b) return;
  const c = L?.changes; b.innerHTML = c?.isGit ? `<span class="add">+${c.added || 0}</span><span class="del">−${c.removed || 0}</span>` : I.diff;
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
    { label: 'Tarefas', f: () => togglePane('tasks', true) }, { label: 'Plano', f: () => togglePane('plan', true) }, { label: 'Pronto (critérios)', f: () => togglePane('oracle', true) }, { label: 'Regras', f: () => togglePane('rules', true) }, { label: 'Jornal', f: () => togglePane('journal', true) }, { label: 'Vícios', f: () => togglePane('vices', true) }, { label: 'Mapa do projeto', f: () => togglePane('map', true) },
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
addEventListener('keydown', e => {
  if (document.querySelector('.overlay')) return;
  const k = e.key.toLowerCase(), ctrl = e.ctrlKey || e.metaKey; const v = V(); const L = curL();
  const inField = /INPUT|TEXTAREA/.test(document.activeElement?.tagName) || document.activeElement?.closest?.('.xterm');
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
  if (e.ctrlKey && (e.key === '`' || e.code === 'Backquote')) { e.preventDefault(); togglePane('terminal'); return; }
  if (ctrl && e.key === '\\') { e.preventDefault(); closeFocusedPane(); return; }
  if (ctrl && e.key === ';') { e.preventDefault(); openSide(v); return; }
  if (ctrl && !e.shiftKey && k === 'o') { e.preventDefault(); const order = ['normal', 'thinking', 'verbose']; setViewMode(order[(order.indexOf(S.viewMode) + 1) % 3]); return; }
  if (ctrl && e.shiftKey && k === 'm') { e.preventDefault(); modeMenu(v, $$('.under .chip', v.el).slice(-2)[0]); return; }
  if (ctrl && e.shiftKey && k === 'i') { e.preventDefault(); modelMenu(v, $('.under .chip:last-child', v.el)); return; }
  if (ctrl && e.shiftKey && k === 'e') { e.preventDefault(); modelMenu(v, $('.under .chip:last-child', v.el), true); return; }
  if (!inField && L?.snap?.pendingPermission && !ctrl) { const id = L.snap.pendingPermission.id; if (k === 'y') { decide(v, id, 'allow'); return; } if (k === 'a') { decide(v, id, 'always'); return; } if (k === 'n') { decide(v, id, 'deny'); return; } }
  if (!inField && L?.snap?.pendingQuestion && /^[1-9]$/.test(e.key)) { const o = L.snap.pendingQuestion.options?.[+e.key - 1]; if (o) answer(v, L.snap.pendingQuestion.id, o); return; }
  if (e.key === 'Escape' && !inField && busy(L)) { stop(v); return; }
  if (!inField && !ctrl && e.key.length === 1 && v?.el && !S.page) { $('.prompt', v.el).focus(); }
});

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
