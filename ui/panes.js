'use strict';
// Panes: stacked on the right, each with a header (drag to reorder, × to close), resizable between each
// other and against the chat. They follow the focused view's session (its worktree when it has one).

const PANES = {
  changes: { title: 'Alterações', mount: mountChanges },
  terminal: { title: 'Terminal', mount: mountTerminal, keep: true },
  files: { title: 'Arquivos', mount: mountFiles },
  browser: { title: 'Navegador', mount: mountBrowser, keep: true },
  tasks: { title: 'Tarefas', mount: mountTasks },
  plan: { title: 'Plano', mount: mountPlan },
  oracle: { title: 'Pronto', mount: mountOracle },
  rules: { title: 'Regras', mount: mountRules },
  journal: { title: 'Jornal', mount: mountJournal },
  vices: { title: 'Vícios', mount: mountVices },
  map: { title: 'Mapa', mount: mountMap },
};
const P2 = { open: [], inst: {}, focus: null, width: lsGet('paneWidth', 520), sizes: lsGet('paneSizes', {}) };

function togglePane(id, forceOpen) {
  if (P2.open.includes(id) && !forceOpen) { closePane(id); return; }
  if (!P2.open.includes(id)) P2.open.push(id);
  P2.focus = id; lsSet('panes', P2.open); renderPanes();
}
function closePane(id) { P2.open = P2.open.filter(x => x !== id); const inst = P2.inst[id]; if (inst?.destroy) inst.destroy(); delete P2.inst[id]; if (P2.focus === id) P2.focus = P2.open[P2.open.length - 1] || null; lsSet('panes', P2.open); renderPanes(); }
function closeFocusedPane() { if (P2.focus) closePane(P2.focus); }
function restorePanes() { P2.open = lsGet('panes', []).filter(id => PANES[id]); renderPanes(); }
function paneRefresh(id) { const inst = P2.inst[id]; if (inst?.refresh) inst.refresh(); }
function paneCall(id, fn, ...a) { const inst = P2.inst[id]; if (inst?.[fn]) inst[fn](...a); }
function openFileInPane(path) { togglePane('files', true); setTimeout(() => paneCall('files', 'open', path), 30); }
function paneCtx() { return { root: curRoot(), dir: curDir(), L: curL() }; }

function renderPanes() {
  const host = $('#panes'); host.classList.toggle('empty', !P2.open.length || !!S.page); // full pages hide the panes host.style.width = P2.width + 'px';
  const ctxKey = curRoot() + '|' + curDir();
  // remove panes no longer open
  $$('.pane', host).forEach(p => { if (!P2.open.includes(p.dataset.id)) p.remove(); });
  $$('.vsplit', host).forEach(x => x.remove());
  P2.open.forEach((id, i) => {
    let p = $(`.pane[data-id="${id}"]`, host);
    if (!p) {
      p = h(`<div class="pane" data-id="${id}"><div class="ph" draggable="true"><span class="ttl">${t(PANES[id].title)}</span><span class="extra row" style="gap:2px;min-width:0"></span><span class="sp"></span><button class="ib x" title="${esc(t('Fechar (Ctrl+\\)'))}">${I.close}</button></div><div class="pb"></div></div>`);
      $('.x', p).onclick = () => closePane(id);
      p.addEventListener('mousedown', () => { P2.focus = id; $$('.pane', host).forEach(x => x.classList.toggle('focus', x.dataset.id === id)); });
      const ph = $('.ph', p);
      ph.ondragstart = e => e.dataTransfer.setData('text/infuse-pane', id);
      p.ondragover = e => { if (e.dataTransfer.types.includes('text/infuse-pane')) { e.preventDefault(); p.classList.add('dragover'); } };
      p.ondragleave = () => p.classList.remove('dragover');
      p.ondrop = e => { p.classList.remove('dragover'); const from = e.dataTransfer.getData('text/infuse-pane'); if (!from || from === id) return; const a = P2.open.indexOf(from), b = P2.open.indexOf(id); P2.open.splice(a, 1); P2.open.splice(b, 0, from); lsSet('panes', P2.open); renderPanes(); };
    }
    host.appendChild(p);
    p.style.flex = `${P2.sizes[id] || 1} 1 0`;
    p.classList.toggle('focus', P2.focus === id);
    const inst = P2.inst[id];
    if (!inst || inst.ctxKey !== ctxKey) {
      if (inst && PANES[id].keep && inst.root === curRoot() && inst.openAt) { inst.openAt(curDir()); inst.ctxKey = ctxKey; } // same project, other worktree: a new tab there
      else if (!inst || !PANES[id].keep || inst.root !== curRoot()) {
        if (inst?.destroy) inst.destroy();
        $('.pb', p).innerHTML = ''; $('.extra', p).innerHTML = '';
        const c = paneCtx();
        if (!c.root) { $('.pb', p).innerHTML = `<div class="empty">${t('Escolha um projeto.')}</div>`; P2.inst[id] = { ctxKey }; }
        else { P2.inst[id] = PANES[id].mount($('.pb', p), $('.extra', p), c) || {}; P2.inst[id].ctxKey = ctxKey; P2.inst[id].root = c.root; }
      } else inst.ctxKey = ctxKey;
    }
    if (i < P2.open.length - 1) { const sp = h('<div class="vsplit"></div>'); host.appendChild(sp); sp.onmousedown = e => dragSplit(e, id, P2.open[i + 1]); }
  });
  for (const id of P2.open) P2.inst[id]?.fit?.();
  S.views.forEach(v => { if (!v.el) return; $$('.vbar [data-a]', v.el).forEach(b => b.classList.toggle('on', P2.open.includes(b.dataset.a))); $('.chg', v.el)?.classList.toggle('on', P2.open.includes('changes')); });
}
function dragSplit(e, a, b) {
  e.preventDefault(); const pa = $(`.pane[data-id="${a}"]`), pb = $(`.pane[data-id="${b}"]`); const ha = pa.offsetHeight, hb = pb.offsetHeight, y0 = e.clientY;
  const move = ev => { const d = ev.clientY - y0; const na = Math.max(90, ha + d), nb = Math.max(90, hb - d); P2.sizes[a] = na / 100; P2.sizes[b] = nb / 100; pa.style.flex = `${P2.sizes[a]} 1 0`; pb.style.flex = `${P2.sizes[b]} 1 0`; P2.inst[a]?.fit?.(); P2.inst[b]?.fit?.(); };
  const up = () => { removeEventListener('mousemove', move); removeEventListener('mouseup', up); lsSet('paneSizes', P2.sizes); };
  addEventListener('mousemove', move); addEventListener('mouseup', up);
}
$('#paneGrip').onmousedown = e => {
  e.preventDefault(); const w0 = P2.width, x0 = e.clientX;
  const move = ev => { P2.width = Math.max(320, Math.min(innerWidth - 480, w0 - (ev.clientX - x0))); $('#panes').style.width = P2.width + 'px'; for (const id of P2.open) P2.inst[id]?.fit?.(); };
  const up = () => { removeEventListener('mousemove', move); removeEventListener('mouseup', up); lsSet('paneWidth', P2.width); };
  addEventListener('mousemove', move); addEventListener('mouseup', up);
};
addEventListener('resize', () => { for (const id of P2.open) P2.inst[id]?.fit?.(); });

// ---------------------------------------------------------------- Changes: file list, diff with line comments, commit, discard
function mountChanges(pb, extra, c) {
  pb.innerHTML = `<div class="chg2"><div class="fl"></div><div class="dv"><div class="empty">${t('Selecione um arquivo')}</div></div></div><div class="pfoot"><span class="faint ncom"></span><button class="btn sm line sendc hidden">${t('Enviar comentários')} <span class="kbd">Ctrl Enter</span></button><span class="grow"></span><input class="cmsg" placeholder="${esc(t('Mensagem do commit'))}"><button class="btn sm pri commit">${t('Commit')}</button></div>`;
  extra.innerHTML = `<button class="ib" title="${esc(t('Atualizar'))}">${I.reload}</button>`;
  const fl = $('.fl', pb), dv = $('.dv', pb); let sel = null, files = [];
  const comments = []; // {path, line, code, text}
  const updC = () => { $('.ncom', pb).textContent = comments.length ? `${comments.length} ${t('comentário(s)')}` : ''; $('.sendc', pb).classList.toggle('hidden', !comments.length); };
  const load = async () => {
    const r = await api('/api/changes', { root: c.root, dir: c.dir });
    if (r.isGit === false) { fl.innerHTML = ''; dv.innerHTML = `<div class="empty">${t('Esta pasta não é um repositório git.')}</div>`; return; }
    files = r.files || [];
    fl.innerHTML = files.length ? '' : `<div class="empty">${t('Nenhuma alteração')}</div>`;
    files.forEach(f => { const it = h(`<div class="fli${f.path === sel ? ' on' : ''}" title="${esc(f.path)}"><span class="n">${esc(f.path)}</span><span class="add">+${f.added}</span><span class="del">−${f.removed}</span></div>`); it.onclick = () => select(f.path); it.oncontextmenu = e => { e.preventDefault(); menu(null, [{ label: 'Abrir no painel Arquivos', f: () => openFileInPane(f.path) }, ...openInItems(c.dir.replace(/[\\/]+$/, '') + '/' + f.path), { sep: 1 }, { label: 'Descartar alterações', danger: 1, f: () => discard(f.path) }], e); }; fl.appendChild(it); });
    if (sel && files.find(f => f.path === sel)) select(sel); else if (files[0]) select(files[0].path); else dv.innerHTML = `<div class="empty">${t('Nada para revisar.')}</div>`;
  };
  const discard = async p => { if (!confirm(t('Descartar as alterações em ') + p + '?')) return; await api('/api/git', { body: { action: 'discard', path: p }, root: c.root, dir: c.dir }); load(); const L = curL(); if (L) refreshChanges(L); };
  const select = async p => {
    sel = p; $$('.fli', fl).forEach(x => x.classList.toggle('on', x.title === p));
    const r = await api('/api/diff?path=' + encodeURIComponent(p), { root: c.dir });
    const lines = (r.diff || '').split('\n'); let nNew = 0; const out = h('<div class="rdiff"></div>');
    out.appendChild(h(`<div class="row" style="padding:6px 10px;border-bottom:1px solid var(--border);font-family:var(--sans)"><span class="mono trunc grow">${esc(p)}</span><button class="btn sm danger">${t('Descartar')}</button></div>`));
    $('.btn', out).onclick = () => discard(p);
    for (const l of lines) {
      if (/^(diff |index |--- |\+\+\+ )/.test(l)) continue;
      const m = l.match(/^@@ -\d+(?:,\d+)? \+(\d+)/); if (m) { nNew = +m[1]; out.appendChild(h(`<div class="ln hh"><span class="no"></span><span class="tx">${esc(l)}</span></div>`)); continue; }
      const cls = l.startsWith('+') ? 'a1' : l.startsWith('-') ? 'd1' : ''; const lineNo = l.startsWith('-') ? '' : nNew;
      const row = h(`<div class="ln ${cls}"><span class="no">${lineNo}</span><span class="tx">${esc(l) || ' '}</span>${lineNo ? '<span class="cadd" title="Comentar">+</span>' : ''}</div>`);
      if (lineNo) $('.cadd', row).onclick = () => addComment(row, p, lineNo, l.slice(1));
      out.appendChild(row);
      for (const cm of comments.filter(x => x.path === p && x.line === lineNo && lineNo)) out.appendChild(noteEl(cm));
      if (!l.startsWith('-')) nNew++;
    }
    dv.innerHTML = ''; dv.appendChild(out);
  };
  const noteEl = cm => { const n = h(`<div class="cnote"><span class="grow">${esc(cm.text)}</span><button class="ib" style="width:20px;height:20px">${I.close}</button></div>`); $('button', n).onclick = () => { comments.splice(comments.indexOf(cm), 1); n.remove(); updC(); }; return n; };
  const addComment = (row, path, line, code) => {
    const box = h(`<div class="cbox"><textarea placeholder="${esc(t('Comentário para o agente… Enter adiciona, Ctrl+Enter envia todos'))}"></textarea></div>`); row.after(box); const ta = $('textarea', box); ta.focus();
    ta.onkeydown = e => { e.stopPropagation(); if (e.key === 'Escape') box.remove(); if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); if (ta.value.trim()) { comments.push({ path, line, code, text: ta.value.trim() }); } box.remove(); sendComments(); return; } if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); if (ta.value.trim()) { const cm = { path, line, code, text: ta.value.trim() }; comments.push(cm); box.replaceWith(noteEl(cm)); updC(); } } };
  };
  const sendComments = async () => {
    const L = curL(); if (!L) return toast('Abra a sessão para enviar os comentários');
    if (!comments.length) return; const r = await api(`/api/session/${L.id}/comments`, { body: { comments }, root: L.root });
    if (r.error) return toast(r.error); comments.length = 0; updC(); toast('Comentários enviados ao agente'); select(sel);
  };
  $('.sendc', pb).onclick = sendComments;
  pb.addEventListener('keydown', e => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && comments.length) { e.preventDefault(); sendComments(); } });
  const commit = async () => { const msg = $('.cmsg', pb).value.trim(); if (!msg) return toast('Escreva a mensagem do commit'); const r = await api('/api/git', { body: { action: 'commit', message: msg }, root: c.dir }); toast(r.ok ? 'Commit feito' : t('Falhou: ') + (r.output || r.error)); if (r.ok) { $('.cmsg', pb).value = ''; load(); const L = curL(); if (L) refreshChanges(L); } };
  $('.commit', pb).onclick = commit; $('.cmsg', pb).onkeydown = e => { if (e.key === 'Enter') commit(); };
  $('button', extra).onclick = load;
  load();
  return { refresh: debounce(load, 300), select };
}

// ---------------------------------------------------------------- Terminal: real shells (ConPTY / pty) in tabs
function mountTerminal(pb, extra, c) {
  const tabs = []; let cur = null;
  const host = h('<div class="termhost"></div>'); pb.appendChild(host);
  const bar = h('<span class="row" style="gap:2px;min-width:0"></span>'); extra.appendChild(bar);
  const addBtn = h(`<button class="ib" title="${esc(t('Novo terminal'))}">${I.plus}</button>`); extra.appendChild(addBtn);
  const dark = () => document.documentElement.classList.contains('dark');
  const theme = () => { const cs = getComputedStyle(document.documentElement); return { background: cs.getPropertyValue('--surface').trim(), foreground: cs.getPropertyValue('--text').trim(), cursor: cs.getPropertyValue('--text2').trim(), selectionBackground: dark() ? '#3a4a6a' : '#cfdcf8' }; };
  const drawTabs = () => { bar.innerHTML = ''; tabs.forEach((tb, i) => { const el = h(`<span class="ptab${tb === cur ? ' on' : ''}">${esc(tb.name)}<span class="x">×</span></span>`); el.onclick = e => { if (e.target.classList.contains('x')) return closeTab(tb); show(tb); }; bar.appendChild(el); }); };
  const show = tb => { tabs.forEach(x => x.el.style.display = x === tb ? '' : 'none'); cur = tb; drawTabs(); setTimeout(() => { tb.fit.fit(); tb.term.focus(); }, 10); };
  const closeTab = tb => { tb.dead = true; api(`/api/term/${tb.id}/close`, { body: {}, root: c.root }); tb.es?.close(); tb.term.dispose(); tb.el.remove(); tabs.splice(tabs.indexOf(tb), 1); if (cur === tb) cur = tabs[tabs.length - 1] || null; if (cur) show(cur); else drawTabs(); };
  const open = async (cwd, run, name) => {
    if (!window.Terminal) { host.innerHTML = `<div class="empty">xterm.js não carregou</div>`; return; }
    const el = h('<div></div>'); host.appendChild(el);
    const term = new Terminal({ fontFamily: getComputedStyle(document.body).getPropertyValue('--mono'), fontSize: 13, cursorBlink: true, theme: theme(), allowProposedApi: true, scrollback: 5000 });
    const fit = new FitAddon.FitAddon(); term.loadAddon(fit); term.open(el); fit.fit();
    const r = await api('/api/term/open', { body: { cwd: cwd || c.dir, cols: term.cols, rows: term.rows, run: run || '' }, root: c.root });
    if (r.error) { term.write('\r\n' + r.error + '\r\n'); return; }
    const tb = { id: r.id, term, fit, el, name: name || baseName(r.cwd) || 'shell' + (tabs.length + 1) };
    const es = new EventSource(`/api/term/${r.id}/stream?root=${encodeURIComponent(c.root)}`); tb.es = es;
    es.onmessage = e => { const bin = atob(e.data); const bytes = new Uint8Array(bin.length); for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i); term.write(bytes); };
    es.addEventListener('exit', () => { term.write(`\r\n\x1b[2m[${t('processo encerrado')}]\x1b[0m\r\n`); es.close(); tb.dead = true; });
    es.onerror = () => { if (es.readyState === EventSource.CLOSED || tb.dead) return; fetch(`/api/term/${r.id}/resize?root=${encodeURIComponent(c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': '1' }, body: JSON.stringify({ cols: term.cols, rows: term.rows }) }).then(x => { if (x.status === 404) { es.close(); tb.dead = true; term.write(`\r\n\x1b[2m[${t('terminal encerrado')}]\x1b[0m\r\n`); } }); };
    let pending = '', timer = null;
    term.onData(d => { if (tb.dead) return; pending += d; if (!timer) timer = setTimeout(() => { const data = pending; pending = ''; timer = null; fetch(`/api/term/${r.id}/input?root=${encodeURIComponent(c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': '1' }, body: JSON.stringify({ data }) }); }, 8); });
    term.onResize(({ cols, rows }) => api(`/api/term/${r.id}/resize`, { body: { cols, rows }, root: c.root }));
    tabs.push(tb); show(tb);
  };
  addBtn.onclick = () => open();
  open();
  return { fit: () => cur?.fit.fit(), destroy: () => tabs.slice().forEach(closeTab), openAt: (cwd, run, name) => open(cwd, run, name) };
}

// ---------------------------------------------------------------- Files: tree + editor with auto save
function mountFiles(pb, extra, c) {
  pb.innerHTML = `<div class="fp"><div class="tree"></div><div class="ed"><div class="edbar"><span class="mono trunc grow fname">${t('Nenhum arquivo aberto')}</span><span class="faint fstat"></span></div><div class="edwrap"><div class="gutter"></div><textarea spellcheck="false" disabled></textarea></div></div></div>`;
  let autoSave = lsGet('autoSave', true);
  extra.innerHTML = `<button class="ib" title="${esc(t('Mais'))}">${I.vmore}</button>`;
  const tree = $('.tree', pb), ta = $('textarea', pb), gut = $('.gutter', pb), fname = $('.fname', pb), fstat = $('.fstat', pb);
  let curPath = null, mtime = 0, dirty = false, changed = {};
  const gutter = () => { const n = ta.value.split('\n').length; gut.textContent = Array.from({ length: n }, (_, i) => i + 1).join('\n'); gut.scrollTop = ta.scrollTop; };
  ta.onscroll = () => gut.scrollTop = ta.scrollTop;
  const save = async (force) => {
    if (!curPath || !dirty) return; fstat.textContent = t('salvando…');
    const r = await fetch(`/api/file?root=${encodeURIComponent(c.root)}&dir=${encodeURIComponent(c.dir)}`, { method: 'PUT', headers: { 'content-type': 'application/json', 'X-Infuse': '1' }, body: JSON.stringify({ path: curPath, content: ta.value, mtime, force: !!force }) });
    const j = await r.json();
    if (r.status === 409) { fstat.innerHTML = ''; const bar = h(`<span class="row"><span class="del">${t('O arquivo mudou no disco.')}</span><button class="btn sm line">${t('Sobrescrever')}</button><button class="btn sm">${t('Descartar minhas edições')}</button></span>`); fstat.appendChild(bar); $$('button', bar)[0].onclick = () => save(true); $$('button', bar)[1].onclick = () => { ta.value = j.content; mtime = j.mtime; dirty = false; gutter(); fstat.textContent = ''; }; return; }
    if (j.error) { fstat.textContent = j.error; return; }
    mtime = j.mtime; dirty = false; fstat.textContent = t('salvo'); setTimeout(() => { if (!dirty) fstat.textContent = ''; }, 1200);
  };
  const autosave = debounce(() => autoSave && save(), 800);
  ta.oninput = () => { dirty = true; fstat.textContent = autoSave ? '' : t('não salvo'); gutter(); autosave(); };
  ta.onkeydown = e => { if (e.key === 's' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); save(); } if (e.key === 'Tab') { e.preventDefault(); const p = ta.selectionStart; ta.value = ta.value.slice(0, p) + '\t' + ta.value.slice(ta.selectionEnd); ta.selectionStart = ta.selectionEnd = p + 1; ta.oninput(); } };
  $('button', extra).onclick = e => menu(e.currentTarget, [{ label: 'Salvar automaticamente', check: autoSave, f: () => { autoSave = !autoSave; lsSet('autoSave', autoSave); } }, { label: 'Salvar', d: 'Ctrl S', f: () => save() }, { sep: 1 }, ...(curPath ? openInItems(c.dir.replace(/[\\/]+$/, '') + '/' + curPath) : [])], null, { alignRight: true });
  const open = async p => {
    if (dirty) await save();
    const r = await api('/api/file?path=' + encodeURIComponent(p), { root: c.root, dir: c.dir });
    curPath = p; fname.textContent = p; fname.title = r.abs || p; fstat.textContent = '';
    $$('.tn', tree).forEach(x => x.classList.toggle('on', x.dataset.p === p));
    if (r.error || r.binary) { ta.value = r.error || t('Arquivo binário ou grande demais para o editor.'); ta.disabled = true; gutter(); return; }
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

// ---------------------------------------------------------------- Browser: tabs, URL bar, dev servers
function mountBrowser(pb, extra, c) {
  const tabs = []; let cur = null;
  pb.innerHTML = `<div class="urlbar"><button class="ib back" title="${esc(t('Voltar'))}">‹</button><button class="ib rl" title="${esc(t('Recarregar'))}">${I.reload}</button><input class="url" placeholder="http://localhost:3000"><button class="ib ext" title="${esc(t('Abrir no navegador padrão'))}">${I.pop}</button></div><div class="frames" style="flex:1;display:flex;min-height:0"></div>`;
  extra.innerHTML = `<span class="row tabs" style="gap:2px;min-width:0"></span><button class="ib nt" title="${esc(t('Nova aba'))}">${I.plus}</button><button class="btn sm srv">${t('Servidores')} ${I.caret}</button>`;
  const frames = $('.frames', pb), url = $('.url', pb), bar = $('.tabs', extra);
  const draw = () => { bar.innerHTML = ''; tabs.forEach(tb => { const el = h(`<span class="ptab${tb === cur ? ' on' : ''}"><span class="trunc" style="max-width:110px">${esc(tb.title || tb.url || t('Nova aba'))}</span><span class="x">×</span></span>`); el.onclick = e => { if (e.target.classList.contains('x')) { tb.fr.remove(); tabs.splice(tabs.indexOf(tb), 1); cur = tabs[tabs.length - 1]; if (!cur) add(''); else show(cur); return; } show(tb); }; bar.appendChild(el); }); };
  const show = tb => { tabs.forEach(x => x.fr.style.display = x === tb ? '' : 'none'); cur = tb; url.value = tb.url || ''; draw(); };
  const add = u => { const fr = h('<iframe class="bframe" sandbox="allow-scripts allow-forms allow-same-origin allow-popups allow-modals"></iframe>'); frames.appendChild(fr); const tb = { fr, url: u }; fr.onload = () => { try { tb.title = fr.contentDocument?.title; } catch { tb.title = ''; } draw(); }; if (u) fr.src = u; tabs.push(tb); show(tb); };
  const go = u => { if (!u) return; if (!/^https?:/.test(u)) u = 'http://' + u; cur.url = u; cur.fr.src = u; url.value = u; draw(); };
  url.onkeydown = e => { if (e.key === 'Enter') go(url.value.trim()); };
  $('.rl', pb).onclick = () => { if (cur?.url) cur.fr.src = cur.url; };
  $('.back', pb).onclick = () => { try { cur.fr.contentWindow.history.back(); } catch { } };
  $('.ext', pb).onclick = () => cur?.url && window.open(cur.url, '_blank');
  $('.nt', extra).onclick = () => add('');
  $('.srv', extra).onclick = async e => {
    const anchor = e.currentTarget; const list = await api('/api/devservers', { root: c.root });
    const items = list.length ? list.flatMap(s => [{ label: s.name, d: `:${s.port} · ${s.running || s.listening ? t('rodando') : t('parado')}${s.approved ? '' : ' · ' + t('não aprovado')}`, f: async () => { if (!s.running && !s.listening) { let r = await api('/api/devservers', { body: { action: 'start', name: s.name }, root: c.root }); if (r.needsApproval) { if (!confirm(t('Executar este comando de launch.json? Ele roda com as suas permissões, fora do filtro.') + '\n\n' + r.command)) return; r = await api('/api/devservers', { body: { action: 'approve', name: s.name }, root: c.root }); } if (r.error) return toast(r.error); toast(t('Iniciando ') + s.name); setTimeout(() => go(s.url), 2500); } else go(s.url); } }]) : [{ label: 'Nenhum servidor configurado', sub: 'Declare em .infuse/launch.json ou .claude/launch.json', f: () => { } }];
    if (list.some(s => s.running)) items.push({ sep: 1 }, ...list.filter(s => s.running).map(s => ({ label: t('Parar ') + s.name, f: () => api('/api/devservers', { body: { action: 'stop', name: s.name }, root: c.root }) })), { label: 'Parar todos', danger: 1, f: () => api('/api/devservers', { body: { action: 'stopall' }, root: c.root }) });
    menu(anchor, items, null, { alignRight: true });
  };
  add(lsGet('browserUrl:' + c.root, ''));
  url.addEventListener('change', () => lsSet('browserUrl:' + c.root, url.value));
  return { go };
}

// ---------------------------------------------------------------- Tasks, Plan
function mountTasks(pb) {
  const draw = () => { const L = curL(); const ts = L?.snap?.tasks || []; pb.innerHTML = `<div class="plist">${ts.length ? ts.map(x => `<div class="row" style="padding:5px 0;border-bottom:1px solid var(--border)"><span>${x.status === 'done' ? '☑' : x.status === 'in_progress' ? '◐' : '☐'}</span><span class="grow" style="${x.status === 'done' ? 'color:var(--text3);text-decoration:line-through' : ''}">${esc(x.text)}</span><span class="tag ${x.status === 'done' ? 'ok' : x.status === 'in_progress' ? 'warn' : ''}">${t(x.status === 'done' ? 'feito' : x.status === 'in_progress' ? 'agora' : 'depois')}</span></div>`).join('') : `<div class="empty">${t('O agente ainda não listou tarefas nesta sessão.')}</div>`}${L?.running ? `<div class="status" style="margin-top:10px"><span class="spin"></span>${t('executando ')} ${esc(L.running.name)}</div>` : ''}</div>`; };
  draw(); return { refresh: draw };
}
function mountPlan(pb) {
  const draw = () => { const L = curL(); const p = L?.snap?.plan; pb.innerHTML = p ? `<div class="plist md sel">${md(p)}</div><div class="pfoot"><span class="grow faint">${t('Plano proposto no modo Plano')}</span><button class="btn sm pri">${t('Aprovar e executar')}</button></div>` : `<div class="empty">${t('Nenhum plano ainda. Use o modo Plano (Shift+Tab ou /plan) para o agente propor antes de editar.')}</div>`; const b = $('.btn', pb); if (b) b.onclick = async () => { const v = V(); await setSessionOpts(v, { mode: 'acceptEdits' }); const P = $('.prompt', v.el); P.value = t('Plano aprovado. Execute-o.'); send(v); }; };
  draw(); return { refresh: draw };
}

// ---------------------------------------------------------------- infuse: oracle, rules, journal, vices, map
function mountOracle(pb, extra, c) {
  const draw = async () => {
    const files = await api('/api/oracle', { root: c.root });
    pb.innerHTML = `<div class="plist"><div class="faint" style="margin-bottom:10px">${t('Critérios de pronto. O agente lê; não consegue alterar por nenhuma ferramenta (bloqueado e registrado no jornal).')}</div><div class="docs"></div><input class="nf" style="margin-top:10px;padding:6px 10px;border-radius:7px;background:var(--s2);width:100%" placeholder="${esc(t('novo arquivo de critérios (ex.: performance.md) e Enter'))}"></div>`;
    const docs = $('.docs', pb);
    (files.length ? files : [{ name: 'pronto.md', content: '' }]).forEach(f => {
      const d = h(`<div style="margin-bottom:12px"><div class="row" style="margin-bottom:4px"><span class="mono">.infuse/oracle/${esc(f.name)}</span><span class="faint st"></span></div><textarea class="doc" placeholder="${esc(t('Ex.: a página /login autentica com e-mail e senha; o teste e2e login.spec passa; nenhum arquivo acima de 400 linhas.'))}">${esc(f.content)}</textarea></div>`);
      const ta = $('textarea', d), st = $('.st', d);
      ta.oninput = debounce(async () => { await api('/api/oracle', { method: 'PUT', body: { name: f.name, content: ta.value }, root: c.root }); st.textContent = t('salvo'); setTimeout(() => st.textContent = '', 1200); }, 500);
      docs.appendChild(d);
    });
    $('.nf', pb).onkeydown = async e => { if (e.key === 'Enter' && e.target.value.trim()) { await api('/api/oracle', { method: 'PUT', body: { name: e.target.value.trim(), content: '' }, root: c.root }); draw(); } };
  };
  draw(); return {};
}
function mountRules(pb, extra, c) {
  const draw = async () => {
    const scope = lsGet('rulesScope', 'project'); const r = await api(`/api/rules?scope=${scope}`, { root: c.root }); const st = { rules: r.rules || [], disable: r.disable || [] };
    const actOf = x => x.replace != null ? 'replace' : x.say ? 'say' : x.infuse ? 'infuse' : x.infuseFile ? 'infuseFile' : x.vice ? 'vice' : 'say';
    pb.innerHTML = `<div class="plist"><div class="row" style="margin-bottom:8px"><span class="chip sc">${t(scope === 'project' ? 'este projeto' : 'todos os projetos')} ${I.caret}</span><span class="faint mono trunc grow">${esc(r.file)}</span></div>
      <label class="row" style="margin-bottom:8px;cursor:pointer"><span class="sw ${st.disable.includes('heredoc') ? 'on' : ''} hd"></span>${t('desligar a transformação de heredoc')}</label>
      <div class="rule faint" style="font-size:10.5px;text-transform:uppercase"><span></span><span>${t('quando')}</span><span>${t('casa com (regex)')}</span><span>${t('ação')}</span><span>${t('valor')}</span><span></span></div><div class="rows"></div>
      <button class="btn sm add" style="margin-top:6px">${I.plus} ${t('regra')}</button>
      <div style="margin-top:16px" class="faint">${t('Testar um comando')}</div><input class="tc mono" style="width:100%;padding:6px 10px;border-radius:7px;background:var(--s2);margin-top:4px" placeholder="cat > a.ts <<EOF …"><pre class="tout" style="display:none;white-space:pre-wrap;font-family:var(--mono);font-size:12px;background:var(--s2);border-radius:7px;padding:8px;margin-top:6px"></pre></div>`;
    const rows = $('.rows', pb);
    const save = debounce(async () => { const res = await api(`/api/rules?scope=${scope}`, { method: 'PUT', body: { rules: st.rules, disable: st.disable }, root: c.root }); if (res.error) toast(res.error); }, 400);
    const drawRows = () => { rows.innerHTML = ''; st.rules.forEach((x, i) => { const row = h(`<div class="rule"><span class="sw ${x.enabled === false ? '' : 'on'}" data-k="en"></span><select data-k="when">${['command', 'write', 'list', 'always'].map(w => `<option ${x.when === w ? 'selected' : ''}>${w}</option>`).join('')}</select><input data-k="match" class="mono" value="${esc(x.match || '')}"><select data-k="act">${['say', 'infuse', 'infuseFile', 'replace', 'vice'].map(a => `<option ${actOf(x) === a ? 'selected' : ''}>${a}</option>`).join('')}</select><input data-k="val" value="${esc(x.replace ?? x.say ?? x.infuse ?? x.infuseFile ?? x.vice ?? '')}"><button class="ib" data-k="del" style="width:22px;height:22px">${I.close}</button></div>`);
      row.oninput = row.onchange = () => { x.when = $('[data-k=when]', row).value; x.match = $('[data-k=match]', row).value; const act = $('[data-k=act]', row).value, val = $('[data-k=val]', row).value; delete x.replace; delete x.say; delete x.infuse; delete x.infuseFile; delete x.vice; x[act] = val; save(); };
      $('[data-k=en]', row).onclick = e => { x.enabled = x.enabled === false; e.target.classList.toggle('on', x.enabled !== false); save(); };
      $('[data-k=del]', row).onclick = () => { st.rules.splice(i, 1); drawRows(); save(); }; rows.appendChild(row); }); };
    drawRows();
    $('.add', pb).onclick = () => { st.rules.push({ id: 'r' + Date.now(), when: 'command', match: '', say: '' }); drawRows(); save(); };
    $('.hd', pb).onclick = e => { const on = !e.target.classList.contains('on'); e.target.classList.toggle('on', on); st.disable = on ? ['heredoc'] : []; save(); };
    $('.sc', pb).onclick = e => menu(e.currentTarget, [{ label: 'este projeto', check: scope === 'project', f: () => { lsSet('rulesScope', 'project'); draw(); } }, { label: 'todos os projetos', check: scope === 'global', f: () => { lsSet('rulesScope', 'global'); draw(); } }]);
    $('.tc', pb).onkeydown = async e => { if (e.key !== 'Enter') return; const res = await api('/api/rules/test', { body: { command: e.target.value }, root: c.root }); const o = $('.tout', pb); o.style.display = ''; o.textContent = `${t('COMANDO FINAL')}:\n${res.finalCommand}\n\n${t('O AGENTE RECEBE')}:\n${(res.lines || []).join('\n') || t('(apenas a saída normal)')}`; };
  };
  draw(); return {};
}
function mountJournal(pb, extra, c) { const draw = async () => { const l = await api('/api/journal', { root: c.root }); pb.innerHTML = `<div class="plist">${l.length ? `<table><tr><th>${t('quando')}</th><th>${t('tipo')}</th><th>${t('o quê')}</th></tr>${l.map(e => `<tr><td class="faint" style="white-space:nowrap">${new Date(e.ts).toLocaleString()}</td><td><span class="tag ${e.kind === 'blocked' ? 'bad' : ''}">${esc(e.kind)}</span></td><td class="mono sel" style="word-break:break-all">${esc(e.path || e.command || '')}${e.detail ? ` <span class="faint">${esc(e.detail)}</span>` : ''}</td></tr>`).join('')}</table>` : `<div class="empty">${t('Nenhuma escrita passou pelo filtro ainda.')}</div>`}</div>`; }; draw(); return { refresh: debounce(draw, 800) }; }
function mountVices(pb, extra, c) { const draw = async () => { const l = await api('/api/lessons', { root: c.root }); pb.innerHTML = `<div class="plist">${l.length ? `<table><tr><th>${t('quando')}</th><th>${t('vício')}</th><th>${t('detalhe')}</th></tr>${l.map(e => `<tr><td class="faint" style="white-space:nowrap">${new Date(e.ts).toLocaleString()}</td><td><span class="tag warn">${esc(e.vice)}</span></td><td class="sel">${esc(e.detail)}</td></tr>`).join('')}</table>` : `<div class="empty">${t('Nenhum vício observado.')}</div>`}</div>`; }; draw(); return { refresh: draw }; }
function mountMap(pb, extra, c) {
  extra.innerHTML = `<button class="ib" title="${esc(t('Regerar'))}">${I.reload}</button>`;
  const draw = async (refresh) => { const m = await api('/api/map' + (refresh ? '?refresh=1' : ''), { root: c.dir }); const dirs = Object.entries(m.dirs || {}).sort(); pb.innerHTML = `<div class="plist"><div class="faint" style="margin-bottom:8px">${Object.keys(m.files || {}).length} ${t('arquivos')} · ${new Date(m.generatedAt).toLocaleString()}</div>${(m.hotspots || []).length ? `<div class="card" style="margin-bottom:10px"><b>${t('Arquivos grandes demais')}</b>${m.hotspots.map(x => `<div class="mono path" data-path="${esc(x.path)}">${esc(x.path)} — ${x.lines} ${t('linhas')}</div>`).join('')}</div>` : ''}<table><tr><th>${t('pasta')}</th><th>${t('arquivos')}</th><th>${t('linhas')}</th></tr>${dirs.map(([p, d]) => `<tr><td class="mono">${esc(p)}</td><td>${(d.files || []).length}</td><td>${d.lines || 0}</td></tr>`).join('')}</table></div>`; $$('[data-path]', pb).forEach(x => x.onclick = () => openFileInPane(x.dataset.path)); };
  $('button', extra).onclick = () => draw(true); draw(); return { refresh: debounce(() => draw(), 800) };
}

// Agent CLIs (Claude Code, Codex, Gemini) run in a terminal tab inside the window, in the session's folder.
async function runAppInTerminal(id, install) {
  const r = await api('/api/apps/command', { body: { id, install: install ? '1' : '' }, root: '' });
  if (!r.ok) return toast(r.error);
  showPage(null); togglePane('terminal', true);
  setTimeout(() => paneCall('terminal', 'openAt', curDir(), r.command, r.name), 80);
}
