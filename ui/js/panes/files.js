'use strict';
// files
// Files pane: tree and editor with auto save.

// ---------------------------------------------------------------- Files: tree + editor with auto save
function mountFiles(pb, extra, c) {
  pb.innerHTML = `<div class="fp"><div class="tree"></div><div class="ed"><div class="edbar"><span class="mono trunc grow fname">${t('ui.pane.files.nenhum-arquivo-aberto.d1fa6f')}</span><span class="faint fstat"></span></div><div class="edwrap"><div class="gutter"></div><textarea spellcheck="false" disabled></textarea></div></div></div>`;
  let autoSave = lsGet('autoSave', true);
  extra.innerHTML = `<button class="ib" title="${esc(t('ui.page.connectors.mais.4f728e'))}">${I.vmore}</button>`;
  const tree = $('.tree', pb), ta = $('textarea', pb), gut = $('.gutter', pb), fname = $('.fname', pb), fstat = $('.fstat', pb);
  let curPath = null, mtime = 0, dirty = false, changed = {};
  const gutter = () => { const n = ta.value.split('\n').length; gut.textContent = Array.from({ length: n }, (_, i) => i + 1).join('\n'); gut.scrollTop = ta.scrollTop; };
  ta.onscroll = () => gut.scrollTop = ta.scrollTop;
  const save = async (force) => {
    if (!curPath || !dirty) return; fstat.textContent = t('ui.pane.files.salvando.47d334');
    const r = await fetch(`/api/file?root=${encodeURIComponent(c.root)}&dir=${encodeURIComponent(c.dir)}`, { method: 'PUT', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ path: curPath, content: ta.value, mtime, force: !!force }) });
    const j = await r.json();
    if (r.status === 409) { fstat.innerHTML = ''; const bar = h(`<span class="row"><span class="del">${t('ui.pane.files.o-arquivo-mudou-no.c2a9a1')}</span><button class="btn sm line">${t('ui.pane.files.sobrescrever.a72fc1')}</button><button class="btn sm">${t('ui.pane.files.descartar-minhas-edicoes.4e2cdc')}</button></span>`); fstat.appendChild(bar); $$('button', bar)[0].onclick = () => save(true); $$('button', bar)[1].onclick = () => { ta.value = j.content; mtime = j.mtime; dirty = false; gutter(); fstat.textContent = ''; }; return; }
    if (j.error) { fstat.textContent = j.error; return; }
    mtime = j.mtime; dirty = false; fstat.textContent = t('ui.page.customize.salvo.7af5c9'); setTimeout(() => { if (!dirty) fstat.textContent = ''; }, 1200);
  };
  const autosave = debounce(() => autoSave && save(), 800);
  ta.oninput = () => { dirty = true; fstat.textContent = autoSave ? '' : t('ui.pane.files.nao-salvo.6ba8e9'); gutter(); autosave(); };
  ta.onkeydown = e => { if (e.key === 's' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); save(); } if (e.key === 'Tab') { e.preventDefault(); const p = ta.selectionStart; ta.value = ta.value.slice(0, p) + '\t' + ta.value.slice(ta.selectionEnd); ta.selectionStart = ta.selectionEnd = p + 1; ta.oninput(); } };
  $('button', extra).onclick = e => menu(e.currentTarget, [{ label: 'Salvar automaticamente', check: autoSave, f: () => { autoSave = !autoSave; lsSet('autoSave', autoSave); } }, { label: 'Salvar', d: 'Ctrl S', f: () => save() }, { sep: 1 }, ...(curPath ? openInItems(c.dir.replace(/[\\/]+$/, '') + '/' + curPath) : [])], null, { alignRight: true });
  const open = async p => {
    if (dirty) await save();
    const r = await api('/api/file?path=' + encodeURIComponent(p), { root: c.root, dir: c.dir });
    curPath = p; fname.textContent = p; fname.title = r.abs || p; fstat.textContent = '';
    $$('.tn', tree).forEach(x => x.classList.toggle('on', x.dataset.p === p));
    if (r.error || r.binary) { ta.value = r.error || t('ui.pane.files.arquivo-binario-ou-grande.a3467b'); ta.disabled = true; gutter(); return; }
    ta.value = r.content; mtime = r.mtime; dirty = false; ta.disabled = !!r.readonly; gutter();
  };
  const loadTree = async () => {
    const [tr, ch] = await Promise.all([api('/api/files/tree', { root: c.dir }), api('/api/changes', { root: c.root, dir: c.dir })]);
    changed = {}; for (const f of ch.files || []) changed[f.path] = f.status;
    const rec = (n, depth) => {
      if (!n.dir) { const el = h(`<div class="tn${n.path === curPath ? ' on' : ''}" data-p="${esc(n.path)}" title="${esc(n.path)}"><span class="trunc">${esc(n.name)}</span>${changed[n.path] ? `<span class="m">${esc(changed[n.path])}</span>` : ''}</div>`); el.onclick = () => open(n.path); el.oncontextmenu = e => { e.preventDefault(); pathMenu(n.path, e); }; return el; }
      const wrap = h(`<div class="${depth > 0 ? 'tclosed' : ''}"><div class="tn"><span>${depth > 0 ? '▸' : ''}</span><span class="trunc">${esc(n.name)}</span></div><div class="tc"></div></div>`);
      $('.tn', wrap).onclick = () => { wrap.classList.toggle('tclosed'); $('.tn span', wrap).textContent = wrap.classList.contains('tclosed') ? '▸' : '▾'; };
      for (const ch of n.children || []) $('.tc', wrap).appendChild(rec(ch, depth + 1));
      return wrap;
    };
    tree.innerHTML = ''; tree.appendChild(rec(tr, 0));
  };
  loadTree();
  return { open, refresh: debounce(loadTree, 500), destroy: () => { if (dirty) save(); } };
}
