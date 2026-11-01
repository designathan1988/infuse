// sidebar — sessions grouped by project, search (titles + transcript contents), unread dots,
// pin/archive/delete, drag-to-reorder projects. One effect renders the whole list keyed by row.
import { $, $$, h, esc, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { ago } from '../core/fmt.js';
import { I } from '../core/icons.js';
import { effect, signal } from '../core/signals.js';
import { api } from '../core/api.js';
import { ui, pref, cache, liveOf, projectLabel, openSession, openInSplit, reloadSessions, setUnread, isUnread, newSessionScreen } from '../core/store.js';
import { menu } from './menu.js';
import { toast } from './toast.js';
import { archiveSession, deleteSession, renameSession, setPinned, unarchiveSession, closeSessionViews } from './session-actions.js';
import { openInItems } from './open-in.js';

const sbState = { showArchived: false, hits: signal({}), hitsQ: '' };
const stateClass = st => !st || st === 'idle' ? '' : st === 'error' ? 'err' : st.startsWith('waiting') ? 'wait' : 'busy';

export function mountSidebar() {
  const ctx = { host: $('#sessList'), searchInput: $('#search') };
  ctx.render = () => renderSidebar(ctx);
  const search = debounce(q => searchTranscripts(ctx, q), 250);
  ctx.searchInput.oninput = () => onSearch(ctx, search);
  $('#btnGroup').onclick = e => groupMenu(ctx, e.currentTarget);
  effect(() => renderSidebar(ctx));
  return { render: ctx.render };
}

async function searchTranscripts(ctx, q) {
  sbState.hitsQ = q;
  const r = await api('/api/sessions/search?q=' + encodeURIComponent(q), { root: '' }).catch(() => []);
  if (ctx.searchInput.value.trim().toLowerCase() !== q) return;
  const hits = {};
  (Array.isArray(r) ? r : []).forEach(x => hits[x.id] = x.snippet);
  sbState.hits.set(hits);
}

function onSearch(ctx, search) {
  const q = ctx.searchInput.value.trim().toLowerCase();
  if (q.length >= 2 && sbState.hitsQ !== q) search(q);
  else if (!q) { sbState.hits.set({}); sbState.hitsQ = ''; }
  ctx.render();
}

function setSb(ctx, k, val) {
  pref('sidebar', { ...pref('sidebar'), [k]: val });
  ctx.render();
}

function groupMenu(ctx, anchor) {
  menu(anchor, [
    { h: t('sidebar.groupBy') },
    { label: t('common.folder'), check: pref('sidebar').group === 'folder', f: () => setSb(ctx, 'group', 'folder') },
    { label: t('sidebar.none'), check: pref('sidebar').group === 'none', f: () => setSb(ctx, 'group', 'none') },
    { h: t('sidebar.sortBy') },
    { label: t('sidebar.sortActivity'), check: pref('sidebar').sort === 'updated', f: () => setSb(ctx, 'sort', 'updated') },
    { label: t('sidebar.sortCreated'), check: pref('sidebar').sort === 'created', f: () => setSb(ctx, 'sort', 'created') },
    { label: t('sidebar.sortTitle'), check: pref('sidebar').sort === 'title', f: () => setSb(ctx, 'sort', 'title') },
    { sep: 1 },
    {
      label: t('sidebar.viewArchived'), f: async () => {
        cache.archived.set(await api('/api/sessions/all?archived=1', { root: '' }).catch(() => []));
        sbState.showArchived = true; ctx.render();
      }
    },
  ]);
}

function sessionRow(ctx, s) {
  const L = liveOf(s.id);
  const st = L?.snap.get()?.state || s.state;
  const hit = sbState.hitsQ && sbState.hits.get()[s.id] && sbState.hits.get()[s.id] !== s.title ? sbState.hits.get()[s.id] : '';
  const openIds = ui.views.get().map(v => v.sid);
  const focusIdx = ui.focus.get();
  const archived = sbState.showArchived;
  const row = h(`<div class="session-row${openIds[focusIdx] === s.id ? ' on' : ''}${openIds.includes(s.id) && openIds[focusIdx] !== s.id ? ' split-view' : ''}${isUnread(s.id) ? ' unread' : ''}${hit ? ' hit' : ''}" draggable="true" tabindex="0" role="button" aria-label="${esc(s.title || t('sidebar.newSession'))}">
    <span class="status-dot ${stateClass(st)}" aria-hidden="true"></span>
    <span class="t"><span class="tt">${s.pinned ? '<span class="pin-flag">📌</span>' : ''}${esc(L?.snap.get()?.title || s.title || t('sidebar.newSession'))}</span>${hit ? `<span class="snip">${esc(hit)}</span>` : ''}</span>
    <span class="when">${ago(s.updated)}</span>
    <button class="icon-btn more-btn act" data-act="${archived ? 'delete' : 'archive'}" aria-label="${esc(t(archived ? 'sidebar.delete' : 'sidebar.archive'))}">${archived ? I.trash : I.archive}</button>
    <button class="icon-btn more-btn" aria-label="${esc(t('common.more'))}">${I.more}</button>
  </div>`);
  row.title = (s.branch ? s.branch + ' · ' : '') + s.root;
  row.onclick = e => {
    const act = e.target.closest('[data-act]');
    if (act) { e.stopPropagation(); return act.dataset.act === 'delete' ? deleteRow(ctx, s) : archiveRow(ctx, s); }
    if (e.target.closest('.more-btn')) return rowMenu(ctx, s, e.target.closest('.more-btn'), null, row);
    if (sbState.showArchived) return;
    if (e.altKey) openInSplit(s.root, s.id); else openSession(s.root, s.id);
  };
  row.onkeydown = e => { if (e.key === 'Enter') openSession(s.root, s.id); };
  row.oncontextmenu = e => { e.preventDefault(); rowMenu(ctx, s, null, e, row); };
  row.ondragstart = e => e.dataTransfer.setData('text/infuse-session', JSON.stringify({ root: s.root, id: s.id }));
  return row;
}

function rowMenu(ctx, s, anchor, ev, row) {
  const items = s.archived ? [
    { label: t('sidebar.unarchive'), f: () => unarchiveSession(s) },
    { label: t('sidebar.delete'), danger: 1, f: () => deleteRow(ctx, s) },
  ] : [
    { label: t('common.open'), f: () => openSession(s.root, s.id) },
    { label: t('sidebar.openIn'), items: [{ label: t('view.splitView'), d: t('sidebar.altClick'), f: () => openInSplit(s.root, s.id) }, { sep: 1 }, ...openInItems(s.root)] },
    { label: t('action.rename'), f: () => renameRow(s, row) },
    { label: s.pinned ? t('sidebar.unpin') : t('sidebar.pinTop'), f: () => setPinned(s, !s.pinned) },
    { label: isUnread(s.id) ? t('sidebar.markRead') : t('sidebar.markUnread'), f: () => setUnread(s.id, !isUnread(s.id)) },
    { sep: 1 },
    { label: t('sidebar.archive'), f: () => archiveRow(ctx, s) },
    { label: t('sidebar.delete'), danger: 1, f: () => deleteRow(ctx, s) },
  ];
  menu(anchor, items, ev);
}

const archiveRow = (ctx, s) => archiveSession({ id: s.id, root: s.root });
const deleteRow = (ctx, s) => deleteSession({ id: s.id, root: s.root });

function renameRow(s, row) {
  const span = row?.querySelector('.tt');
  if (!span) return;
  const cur = s.title || '';
  const inp = document.createElement('input');
  inp.className = 'rename-input'; inp.value = cur;
  span.replaceWith(inp); inp.focus(); inp.select();
  let done = false;
  const fin = ok => {
    if (done) return; done = true;
    const val = inp.value.trim();
    inp.replaceWith(span);
    if (!ok || !val || val === cur) return;
    renameSession({ id: s.id, root: s.root }, val);
  };
  inp.onkeydown = e => { e.stopPropagation(); if (e.key === 'Enter') fin(true); if (e.key === 'Escape') fin(false); };
  inp.onblur = () => fin(true);
  inp.onclick = e => e.stopPropagation();
}

function renderSidebar(ctx) {
  ctx.host.innerHTML = '';
  const q = ctx.searchInput.value.trim().toLowerCase();
  const sb = pref('sidebar');
  $('#sbHeadLabel').textContent = t(sbState.showArchived ? 'sidebar.archived' : 'sidebar.sessions');
  $('#navScheduled').classList.toggle('on', ui.page.get() === 'scheduled');
  $('#navCustomize').classList.toggle('on', ui.page.get() === 'customize');
  let list = (sbState.showArchived ? cache.archived.get() : cache.sessions.get())
    .filter(s => !q || sbState.hits.get()[s.id] || (s.title || '').toLowerCase().includes(q) || (s.root || '').toLowerCase().includes(q) || (s.branch || '').toLowerCase().includes(q));
  const sortKey = sb.sort;
  list = [...list].sort((a, b) => sortKey === 'title' ? (a.title || '').localeCompare(b.title || '') : (b[sortKey] || 0) - (a[sortKey] || 0));
  const pinned = sbState.showArchived ? [] : list.filter(s => s.pinned);
  if (pinned.length) { renderPinned(ctx, pinned); list = list.filter(s => !s.pinned); }
  if (sb.group === 'folder' && !sbState.showArchived) renderFolders(ctx, list, q, sb);
  else list.forEach(s => ctx.host.appendChild(sessionRow(ctx, s)));
  if (!cache.projects.get().length && !list.length) ctx.host.appendChild(h(`<div class="empty">${t('sidebar.empty1')}<br>${t('sidebar.empty2')}</div>`));
  if (sbState.showArchived) renderArchivedBar(ctx, list);
}

function renderPinned(ctx, pinned) {
  const g = h(`<div class="side-group"><div class="group-head"><span class="trunc">${t('sidebar.pinned')}</span></div></div>`);
  pinned.forEach(s => g.appendChild(sessionRow(ctx, s)));
  ctx.host.appendChild(g);
}

// Same folder, one group: slash style and (on Windows) letter case must not split the sidebar.
const rootKey = r => String(r || '').replace(/[\\/]+/g, '/').toLowerCase().replace(/\/+$/, '');

function renderFolders(ctx, list, q, sb) {
  const roots = cache.projects.get().map(p => p.root);
  const keys = roots.map(rootKey);
  for (const s of list) if (!keys.includes(rootKey(s.root))) { roots.push(s.root); keys.push(rootKey(s.root)); }
  for (const root of roots) {
    const items = list.filter(s => rootKey(s.root) === rootKey(root));
    if (q && !items.length && !root.toLowerCase().includes(q)) continue;
    ctx.host.appendChild(renderGroup(ctx, root, items, q, sb));
  }
}

function renderGroup(ctx, root, items, q, sb) {
  const g = h(`<div class="side-group${sb.closed[root] && !q ? ' closed' : ''}" draggable="true"><div class="group-head" tabindex="0" role="button" aria-label="${esc(projectLabel(root))}"><span class="chev" aria-hidden="true">▼</span><span class="trunc">${esc(projectLabel(root))}</span><span class="acts"><button class="icon-btn" data-a="new" aria-label="${esc(t('sidebar.newInFolder'))}">${I.plus}</button><button class="icon-btn" data-a="menu" aria-label="${esc(t('common.more'))}">${I.more}</button></span></div></div>`);
  g.title = root;
  bindGroupHead(ctx, g, root, sb);
  bindGroupDrag(ctx, g, root);
  items.forEach(s => g.appendChild(sessionRow(ctx, s)));
  if (!items.length) g.appendChild(h(`<div class="session-row session-row-empty">${t('sidebar.noSessions')}</div>`));
  return g;
}

function bindGroupHead(ctx, g, root, sb) {
  const head = g.querySelector('.group-head');
  head.onclick = e => {
    const a = e.target.closest('[data-a]')?.dataset.a;
    if (a === 'new') return newSessionScreen(root);
    if (a === 'menu') return projectMenu(ctx, root, e.target.closest('button'));
    const closed = { ...sb.closed }; closed[root] = !closed[root];
    setSb(ctx, 'closed', closed);
  };
  head.onkeydown = e => { if (e.key === 'Enter') e.target.closest('.group-head').click(); };
}

function bindGroupDrag(ctx, g, root) {
  g.oncontextmenu = e => { e.preventDefault(); projectMenu(ctx, root, null, e); };
  g.ondragstart = e => { if (e.target === g) e.dataTransfer.setData('text/infuse-project', root); };
  g.ondragover = e => { if (e.dataTransfer.types.includes('text/infuse-project')) { e.preventDefault(); g.classList.add('drop-target'); } };
  g.ondragleave = () => g.classList.remove('drop-target');
  g.ondrop = e => {
    g.classList.remove('drop-target');
    const from = e.dataTransfer.getData('text/infuse-project');
    if (!from || from === root) return;
    e.preventDefault();
    const l = cache.projects.get().slice();
    const i = l.findIndex(x => x.root === from), j = l.findIndex(x => x.root === root);
    if (i < 0 || j < 0) return;
    const [m] = l.splice(i, 1); l.splice(j, 0, m);
    api('/api/projects', { method: 'PUT', body: { projects: l }, root: '' }).then(reloadSessions);
  };
}

function renderArchivedBar(ctx, list) {
  const bar = h(`<div class="row archived-bar"><button class="btn sm back">← ${t('sidebar.backToSessions')}</button><span class="grow"></span>${list.length ? `<button class="btn sm danger delall">${t('sidebar.deleteAll')}</button>` : ''}</div>`);
  bar.querySelector('.back').onclick = () => { sbState.showArchived = false; ctx.render(); };
  bar.querySelector('.delall')?.addEventListener('click', async () => {
    if (!confirm(t('sidebar.deleteAllConfirm'))) return;
    for (const x of list) { await api(`/api/sessions/${x.id}`, { method: 'DELETE', root: x.root }).catch(() => { }); closeSessionViews(x.id, x.root); }
    cache.archived.set(await api('/api/sessions/all?archived=1', { root: '' }).catch(() => []));
    ctx.render();
    toast(t('sidebar.deleted'));
  });
  ctx.host.prepend(bar);
}

function projectMenu(ctx, root, anchor, ev) {
  menu(anchor, [
    { label: t('sidebar.newInFolder'), f: () => newSessionScreen(root) },
    { label: t('sidebar.renameProject'), f: () => renameProject(ctx, root) },
    { label: t('view.openIn'), items: openInItems(root) },
    { sep: 1 },
    { label: t('sidebar.removeFromList'), danger: 1, f: () => api('/api/projects/remove', { body: { root }, root: '' }).then(reloadSessions) },
  ], ev);
}

function renameProject(ctx, root) {
  const g = $$('.side-group').find(x => x.title === root);
  const span = g && g.querySelector('.group-head .trunc');
  if (!span) return;
  const inp = document.createElement('input');
  inp.value = projectLabel(root); inp.className = 'rename-input';
  span.replaceWith(inp); inp.focus(); inp.select();
  const fin = ok => {
    const val = inp.value.trim();
    inp.replaceWith(span);
    if (ok && val) api('/api/projects', { method: 'PUT', body: { projects: cache.projects.get().map(p => p.root === root ? { ...p, label: val } : p) }, root: '' }).then(reloadSessions);
  };
  inp.onkeydown = e => { e.stopPropagation(); if (e.key === 'Enter') fin(true); if (e.key === 'Escape') fin(false); };
  inp.onblur = () => fin(true);
}
