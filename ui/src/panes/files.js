// files — project tree + plain editor (gutter, tab, autosave with mtime conflict check).
import { $, $$, h, esc, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { api, tokenHeader, urlWith } from '../core/api.js';
import { pref } from '../core/store.js';
import { menu } from '../components/menu.js';
import { toast } from '../components/toast.js';

function makeState(pb, extra, c) {
  pb.innerHTML = `<div class="files-pane"><div class="tree" role="tree"></div><div class="ed">
    <div class="editor-bar"><span class="mono trunc grow fname">${t('files.noneOpen')}</span><span class="faint fstat" aria-live="polite"></span></div>
    <div class="editor-wrap"><div class="editor-gutter" aria-hidden="true"></div><textarea spellcheck="false" disabled aria-label="${esc(t('files.editorAria'))}"></textarea></div>
  </div></div>`;
  extra.innerHTML = `<button class="icon-btn" title="${esc(t('common.more'))}" aria-label="${esc(t('common.more'))}">${I.vmore}</button>`;
  return {
    pb, extra, c, autoSave: true, curPath: null, mtime: 0, dirty: false, changed: {},
    tree: pb.querySelector('.tree'), ta: pb.querySelector('textarea'), gut: pb.querySelector('.editor-gutter'),
    fname: pb.querySelector('.fname'), fstat: pb.querySelector('.fstat')
  };
}

function gutter(s) {
  const n = s.ta.value.split('\n').length;
  s.gut.textContent = Array.from({ length: n }, (_, i) => i + 1).join('\n');
  s.gut.scrollTop = s.ta.scrollTop;
}

function conflictBar(s, j) {
  s.fstat.innerHTML = '';
  const bar = h(`<span class="row"><span class="del">${t('files.changedOnDisk')}</span><button class="btn sm line">${t('files.overwrite')}</button><button class="btn sm">${t('files.discardMine')}</button></span>`);
  bar.querySelectorAll('button')[0].onclick = () => saveFile(s, true);
  bar.querySelectorAll('button')[1].onclick = () => { s.ta.value = j.content || ''; s.mtime = j.mtime || s.mtime; s.dirty = false; gutter(s); s.fstat.textContent = ''; };
  s.fstat.appendChild(bar);
}

async function saveFile(s, force) {
  if (!s.curPath || !s.dirty) return;
  s.fstat.textContent = t('files.saving');
  const r = await fetch(urlWith('/api/file', { root: s.c.root, dir: s.c.dir }), {
    method: 'PUT', headers: tokenHeader(), body: JSON.stringify({ path: s.curPath, content: s.ta.value, mtime: s.mtime, force: !!force })
  });
  const j = await r.json().catch(() => ({}));
  if (r.status === 409) return conflictBar(s, j);
  if (j.error) { s.fstat.textContent = j.error; return; }
  s.mtime = j.mtime; s.dirty = false;
  s.fstat.textContent = t('files.saved');
  setTimeout(() => { if (!s.dirty) s.fstat.textContent = ''; }, 1200);
}

function bindEditor(s) {
  const autosave = debounce(() => s.autoSave && saveFile(s), 800);
  s.ta.onscroll = () => { s.gut.scrollTop = s.ta.scrollTop; };
  s.ta.oninput = () => { s.dirty = true; s.fstat.textContent = s.autoSave ? '' : t('files.unsaved'); gutter(s); autosave(); };
  s.ta.onkeydown = e => {
    if (e.key === 's' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); saveFile(s); }
    if (e.key === 'Tab') {
      e.preventDefault();
      const p = s.ta.selectionStart;
      s.ta.value = s.ta.value.slice(0, p) + '\t' + s.ta.value.slice(s.ta.selectionEnd);
      s.ta.selectionStart = s.ta.selectionEnd = p + 1;
      s.ta.oninput();
    }
  };
}

function openInItems(path) {
  return [
    { label: t('view.showInExplorer'), f: () => api('/api/open', { body: { path, app: 'explorer' }, root: '' }) },
    { label: t('files.copyPath'), f: () => navigator.clipboard.writeText(path) },
  ];
}

function extraMenu(s) {
  return [
    { label: t('files.autoSave'), check: s.autoSave, f: () => { s.autoSave = !s.autoSave; } },
    { label: t('files.save'), d: 'Ctrl S', f: () => saveFile(s) },
    { sep: 1 },
    ...(s.curPath ? openInItems(s.c.dir.replace(/[\\/]+$/, '') + '/' + s.curPath) : []),
  ];
}

async function openFile(s, p) {
  if (s.dirty) await saveFile(s);
  const r = await api('/api/file?path=' + encodeURIComponent(p), { root: s.c.root, dir: s.c.dir }).catch(() => ({}));
  s.curPath = p;
  s.fname.textContent = p; s.fname.title = r.abs || p; s.fstat.textContent = '';
  $$('.tree-node', s.tree).forEach(x => x.classList.toggle('on', x.dataset.p === p));
  if (r.error || r.binary) { s.ta.value = r.error || t('files.binaryTooBig'); s.ta.disabled = true; gutter(s); return; }
  s.ta.value = r.content; s.mtime = r.mtime; s.dirty = false; s.ta.disabled = !!r.readonly; gutter(s);
}

function treeNode(s, n, depth) {
  if (!n.dir) {
    const el = h(`<div class="tree-node${n.path === s.curPath ? ' on' : ''}" data-p="${esc(n.path)}" title="${esc(n.path)}" role="treeitem" tabindex="0"><span class="trunc">${esc(n.name)}</span>${s.changed[n.path] ? `<span class="m">${esc(s.changed[n.path])}</span>` : ''}</div>`);
    el.onclick = () => openFile(s, n.path);
    el.oncontextmenu = e => { e.preventDefault(); import('../components/view.js').then(() => { }); pathMenu(s, n.path, e); };
    return el;
  }
  const wrap = h(`<div class="${depth > 0 ? 'tree-closed' : ''}" role="group"><div class="tree-node" role="treeitem" tabindex="0" aria-expanded="false"><span aria-hidden="true">${depth > 0 ? '▸' : ''}</span><span class="trunc">${esc(n.name)}</span></div><div class="tree-children"></div></div>`);
  const head = wrap.querySelector('.tree-node');
  head.onclick = () => {
    wrap.classList.toggle('tree-closed');
    head.setAttribute('aria-expanded', String(!wrap.classList.contains('tree-closed')));
    head.querySelector('span').textContent = wrap.classList.contains('tree-closed') ? '▸' : '▾';
  };
  for (const ch of n.children || []) wrap.querySelector('.tree-children').appendChild(treeNode(s, ch, depth + 1));
  return wrap;
}

async function loadTree(s) {
  const [tr, ch] = await Promise.all([
    api('/api/files/tree', { root: s.c.dir }).catch(() => null),
    api('/api/changes', { root: s.c.root, dir: s.c.dir }).catch(() => ({}))
  ]);
  if (!tr) return;
  s.changed = {};
  for (const f of ch.files || []) s.changed[f.path] = f.status;
  s.tree.innerHTML = '';
  s.tree.appendChild(treeNode(s, tr, 0));
}

function pathMenu(s, path, ev) {
  menu(null, [
    { label: t('files.openInPane'), f: () => openFile(s, path) },
    { sep: 1 }, ...openInItems(s.c.dir.replace(/[\\/]+$/, '') + '/' + path),
  ], ev);
}

function wireFiles(s) {
  bindEditor(s);
  s.extra.querySelector('button').onclick = e => menu(e.currentTarget, extraMenu(s), null, { alignRight: true });
}

export function mount(pb, extra, c) {
  const s = makeState(pb, extra, c);
  wireFiles(s);
  loadTree(s);
  return { open: p => openFile(s, p), refresh: debounce(() => loadTree(s), 500), destroy: () => { if (s.dirty) saveFile(s); } };
}
