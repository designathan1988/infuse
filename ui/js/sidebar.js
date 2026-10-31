'use strict';
// sidebar
// Views (split view, layout) and the session sidebar with its menus.

function newView() {
  const d = S.settings;
  const lastRoot = lsGet('lastRoot', '') || (S.projects[0]?.root || '');
  return { id: newViewId(), sid: '', root: '', draft: { root: S.projects.find(p => p.root === lastRoot) ? lastRoot : (S.projects[0]?.root || ''), worktree: false, mode: '', provider: d.provider, model: d.model, effort: d.effort || 'medium' } };
}

function mountViews() {
  const host = $('#views'); host.innerHTML = ''; host.classList.toggle('split', S.views.length > 1);
  S.views.forEach((v, i) => { if (!elOf(v)) buildView(v); elOf(v).classList.toggle('focus', i === S.focus); host.appendChild(elOf(v)); renderView(v); });
}

function buildView(v) {
  const el = h(`<section class="view">
    <div class="vbar"><span class="title" tabindex="0" title="Clique para renomear"></span><button class="ib caret" title="Menu da sessão">${I.caret}</button><span class="branch"></span><span class="sp"></span>
      <span class="r row" style="gap:2px"><button class="chg" data-a="changes" title="Alterações (Ctrl+Shift+D)"></button>
      <button class="ib" data-a="terminal" title="Terminal (Ctrl+\`)">${I.term}</button><button class="ib" data-a="files" title="Arquivos">${I.files}</button><button class="ib" data-a="browser" title="Navegador (Ctrl+Shift+B)">${I.globe}</button>
      <button class="ib cnt" data-a="agents" title="Agentes: os subagentes da sessão, ao vivo (Ctrl+Shift+A)">${I.agents}<i></i></button><button class="ib cnt" data-a="procs" title="Processos em segundo plano (Ctrl+Shift+P)">${I.procs}<i></i></button>
      <button class="ib" data-a="rail" title="Mostrar/ocultar detalhes da sessão">${I.rail}</button><button class="ib" data-a="more" title="Mais">${I.vmore}</button></span></div>
    <div class="vbody"><div class="vchat"><div class="toc"><div class="ticks"></div><div class="toclist"></div></div><div class="scroller"><div class="chat"></div></div><button class="jumpdown" title="Ir para o fim da conversa">${I.down || '↓'}</button>
      <div class="composer"><div class="hometitle"><b>O que vamos construir?</b><span class="hsub"></span></div><div class="queue"></div>
        <div class="box"><div class="atts"></div><textarea class="prompt" rows="1" placeholder="Peça algo… @ para arquivos, / para comandos"></textarea>
          <div class="brow"><button class="ib plusbtn" title="Anexar e mais">${I.plus}</button><span class="sp"></span><span class="ring" title="Uso do contexto"></span><button class="send" title="Enviar (Enter)">${I.up}</button></div></div>
        <div class="under"></div></div></div>
      <aside class="rail"></aside></div></section>`);
  setViewEl(v, el);
  el.addEventListener('mousedown', () => { const i = S.views.indexOf(v); if (i >= 0 && i !== S.focus) { UI.set('focus', i); $$('.view').forEach((x, j) => x.classList.toggle('focus', j === i)); renderSidebar(); renderPanes(); } }, true);
  const title = $('.title', el);
  title.onclick = () => renameInline(v);
  $('.caret', el).onclick = e => sessionMenu(v, e.currentTarget);
  $$('[data-a]', el).forEach(b => b.onclick = e => {
    const a = b.dataset.a;
    if (a === 'rail') { UI.set('rail', !S.rail); S.views.forEach(renderView); }
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
  v = freshView(v);
  const el = elOf(v); if (!el) return;
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
  Server.set('sessions', Array.isArray(list) ? list : []); Server.set('projects', Array.isArray(projects) ? projects : []);
  renderSidebar();
}
function projectLabel(root) { const p = S.projects.find(x => x.root === root); return p?.label || baseName(root); }
function renderSidebar() {
  const q = $('#search').value.trim().toLowerCase();
  const el = $('#sessList'); el.innerHTML = '';
  $('#navScheduled').classList.toggle('on', S.page === 'scheduled'); $('#navCustomize').classList.toggle('on', S.page === 'customize');
  const archivedMode = S.sb.show === 'archived';
  $('#sbHeadLabel').textContent = t(archivedMode ? 'Arquivadas' : 'Sessões');
  if (q.length >= 2 && S.hitsQ !== q) searchTranscripts(q); else if (!q) { UI.set('hits', {}); UI.set('hitsQ', ''); }
  let list = (archivedMode ? S.archived : S.sessions).filter(s => !q || S.hits[s.id] || (s.title || '').toLowerCase().includes(q) || (s.root || '').toLowerCase().includes(q) || (s.branch || '').toLowerCase().includes(q));
  const sortKey = S.sb.sort; list.sort((a, b) => sortKey === 'title' ? (a.title || '').localeCompare(b.title || '') : (b[sortKey] || 0) - (a[sortKey] || 0));
  const openIds = S.views.map(v => v.sid);
  const row = s => {
    const L = S.live[s.id]; const st = L?.snap?.state || s.state;
    const hit = q && S.hits[s.id] && S.hits[s.id] !== s.title ? S.hits[s.id] : '';
    const r = h(`<div class="srow${openIds[S.focus] === s.id ? ' on' : ''}${openIds.includes(s.id) && openIds[S.focus] !== s.id ? ' split' : ''}${S.unread.includes(s.id) ? ' unread' : ''}${hit ? ' hit' : ''}" draggable="true"><span class="st ${stateClass(st)}"></span><span class="t"><span class="tt">${s.pinned ? '<span class="pin">📌</span>' : ''}${esc(L?.snap?.title || s.title || t('Nova sessão'))}</span>${hit ? `<span class="snip">${esc(hit)}</span>` : ''}</span><span class="w">${ago(s.updated)}</span><button class="ib more act" data-act="${archivedMode ? 'delete' : 'archive'}" title="${esc(t(archivedMode ? 'Excluir' : 'Arquivar'))}">${archivedMode ? I.trash : I.archive}</button><button class="ib more" title="Mais">${I.more}</button></div>`);
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
    $('.back', bar).onclick = () => { setSb('show', ''); };
    if ($('.delall', bar)) $('.delall', bar).onclick = async () => { if (!confirm(t('Excluir todas as sessões arquivadas e seus checkpoints? Isto não pode ser desfeito.'))) return; for (const x of list) await api(`/api/sessions/${x.id}`, { method: 'DELETE', root: x.root }); Server.set('archived', await api('/api/sessions/all?archived=1', { root: '' })); renderSidebar(); toast('Sessões excluídas'); };
    el.prepend(bar);
  }
}
$('#search').oninput = renderSidebar;
$('#btnGroup').onclick = e => menu(e.currentTarget, [
  { h: 'Agrupar por' }, { label: 'Pasta', check: S.sb.group === 'folder', f: () => setSb('group', 'folder') }, { label: 'Nenhum', check: S.sb.group === 'none', f: () => setSb('group', 'none') },
  { h: 'Ordenar por' }, { label: 'Atividade recente', check: S.sb.sort === 'updated', f: () => setSb('sort', 'updated') }, { label: 'Criação', check: S.sb.sort === 'created', f: () => setSb('sort', 'created') }, { label: 'Título', check: S.sb.sort === 'title', f: () => setSb('sort', 'title') },
  { sep: 1 }, { label: 'Ver arquivadas', f: async () => { Server.set('archived', await api('/api/sessions/all?archived=1', { root: '' })); setSb('show', 'archived'); } },
]);
function setSb(k, v) { UI.sub('sb', k, v); renderSidebar(); }

function sessionRowMenu(s, anchor, ev) {
  const items = s.archived ? [
    { label: 'Desarquivar', f: async () => { await api(`/api/session/${s.id}/unarchive`, { body: {}, root: s.root }); Server.set('archived', await api('/api/sessions/all?archived=1', { root: '' })); await loadSessions(); } },
    { label: 'Excluir', danger: 1, f: () => deleteSession(s) },
  ] : [
    { label: 'Abrir', f: () => openSession(s.root, s.id) },
    { label: 'Abrir em', items: [{ label: 'Tela dividida', d: 'Alt+clique', f: () => openInSplit(s.root, s.id) }, { sep: 1 }, ...openInItems(s.root)] },
    { label: 'Renomear', f: () => { openSession(s.root, s.id).then(() => renameInline(V())); } },
    { label: s.pinned ? 'Desafixar' : 'Fixar no topo', f: async () => { await api(`/api/session/${s.id}/pin`, { body: { pinned: !s.pinned }, root: s.root }); await loadSessions(); } },
    { label: S.unread.includes(s.id) ? 'Marcar como lida' : 'Marcar como não lida', f: () => setUnread(s.id, !S.unread.includes(s.id)) },
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
  S.views.forEach(v => { if (v.sid === s.id) { View.set(v.id, { sid: '' }); View.draft(v.id, { root: s.root }); } });
  closeLive(s.id); await loadSessions(); mountViews(); renderPanes(); toast('Sessão arquivada');
}
async function deleteSession(s) {
  if (!confirm(t('Excluir esta sessão e seus checkpoints? Isto não pode ser desfeito.'))) return;
  await api(`/api/sessions/${s.id}`, { method: 'DELETE', root: s.root });
  S.views.forEach(v => { if (v.sid === s.id) { View.set(v.id, { sid: '' }); View.draft(v.id, { root: s.root }); } });
  closeLive(s.id); await loadSessions(); if (S.sb.show === 'archived') Server.set('archived', await api('/api/sessions/all?archived=1', { root: '' })); renderSidebar(); mountViews(); renderPanes();
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
  inlineEdit($('.title', elOf(v)), L.snap.title, val => setSessionOpts(v, { title: val }));
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
  const modes = VIEW_MODES;
  // grouped like the desktop app: a few direct actions, the rest one level down
  menu(anchor, [
    { label: 'Exibição', items: [
      ...modes.map(([k, l, d]) => ({ label: l, sub: d, check: S.viewMode === k, f: () => setViewMode(k) })),
      { sep: 1 }, { label: 'Expandir tudo', f: () => foldAll(v, true) }, { label: 'Recolher tudo', f: () => foldAll(v, false) },
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
    { label: 'Tarefas', f: () => togglePane('tasks') }, { label: 'Plano', f: () => togglePane('plan') }, { label: 'Processos', f: () => togglePane('procs') }, { label: 'Agentes', f: () => togglePane('agents') },
    { label: 'Filtro', items: [{ label: 'Pronto (critérios)', f: () => togglePane('oracle') }, { label: 'Memória', f: () => togglePane('memory') }, { label: 'Regras', f: () => togglePane('rules') }, { label: 'Jornal', f: () => togglePane('journal') }, { label: 'Vícios', f: () => togglePane('vices') }, { label: 'Mapa do projeto', f: () => togglePane('map') }] },
    { sep: 1 }, { label: S.rail ? 'Ocultar detalhes da sessão' : 'Mostrar detalhes da sessão', f: () => { UI.set('rail', !S.rail); S.views.forEach(renderView); } },
  ], null, { alignRight: true });
}
function foldAll(v, open) { const patch = {}; $$('[data-fold]', elOf(v)).forEach(x => { patch[x.dataset.fold] = open; x.parentNode.classList.toggle('open', open); }); UI.merge('fold', patch); }
const VIEW_MODES = [['normal', 'Normal', 'Texto completo; raciocínio e ferramentas em uma linha cada'], ['thinking', 'Pensamento', 'Também abre o raciocínio do modelo'], ['verbose', 'Detalhado', 'Abre tudo: raciocínio, cada ferramenta com entrada e saída, cada passo dos subagentes']];
function setViewMode(k) {
  UI.set('fold', {}); UI.set('viewMode', k); S.views.forEach(renderChat);
  const L = curL(); const thinks = (L?.snap?.messages || []).some(m => (m.content || []).some(b => b.type === 'thinking'));
  toast(t('Exibição: ') + t({ normal: 'Normal', thinking: 'Pensamento', verbose: 'Detalhado' }[k]) + (k !== 'normal' && L && !thinks ? ' · ' + t('esta sessão ainda não tem raciocínio: o esforço Baixo desliga o modo de pensamento') : ''));
}

// ---------------------------------------------------------------- live sessions (SSE)
