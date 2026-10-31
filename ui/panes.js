'use strict';
// Panes: stacked on the right, each with a header (drag to reorder, × to close), resizable between each
// other and against the chat. They follow the focused view's session (its worktree when it has one).

const PANES = {
  changes: { title: 'Alterações', mount: mountChanges },
  terminal: { title: 'Terminal', mount: mountTerminal, keep: true },
  files: { title: 'Arquivos', mount: mountFiles },
  browser: { title: 'Navegador', mount: mountBrowser, keep: true },
  tasks: { title: 'Tarefas', mount: mountTasks },
  procs: { title: 'Processos', mount: mountProcs },
  agents: { title: 'Agentes', mount: mountAgents },
  plan: { title: 'Plano', mount: mountPlan },
  oracle: { title: 'Pronto', mount: mountOracle },
  memory: { title: 'Memória', mount: mountMemory },
  rules: { title: 'Regras', mount: mountRules },
  journal: { title: 'Jornal', mount: mountJournal },
  vices: { title: 'Vícios', mount: mountVices },
  map: { title: 'Mapa', mount: mountMap },
};
const P2 = { open: [], inst: {}, focus: null, width: lsGet('paneWidth', 520), sizes: lsGet('paneSizes', {}) };

// the toolbar shows which panes are open
function markPaneButtons() { $$('.vbar [data-a]').forEach(b => b.classList.toggle('on', P2.open.includes(b.dataset.a))); }
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
  setTimeout(markPaneButtons, 0);
  const host = $('#panes'); host.classList.toggle('empty', !P2.open.length || !!S.page); // full pages hide the panes
  fitPaneWidth(); // the width the user set, within the room there is
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
  e.preventDefault(); const pa = $(`.pane[data-id="${a}"]`), pb = $(`.pane[data-id="${b}"]`); const ha = pa.offsetHeight, hb = pb.offsetHeight, y0 = e.clientY; const done = dragShield('row-resize');
  const move = ev => { const d = ev.clientY - y0; const na = Math.max(90, ha + d), nb = Math.max(90, hb - d); P2.sizes[a] = na / 100; P2.sizes[b] = nb / 100; pa.style.flex = `${P2.sizes[a]} 1 0`; pb.style.flex = `${P2.sizes[b]} 1 0`; P2.inst[a]?.fit?.(); P2.inst[b]?.fit?.(); };
  const up = () => { removeEventListener('mousemove', move); removeEventListener('mouseup', up); done(); lsSet('paneSizes', P2.sizes); };
  addEventListener('mousemove', move); addEventListener('mouseup', up);
}
// While a divider is dragged a shield covers the window: the mouse stays with the drag even over the
// browser's page (a stream or a native view), which would otherwise take the movements.
function dragShield(cursor) {
  const d = h(`<div class="dragshield" style="cursor:${cursor}"></div>`); document.body.appendChild(d); document.body.classList.add('dragging');
  return () => { d.remove(); document.body.classList.remove('dragging'); };
}
// The panes never take the conversation's room: whatever width the user set, the chat keeps at least
// CHAT_MIN px beside them (a window made narrower, a sidebar shown, a width saved on a bigger screen).
const CHAT_MIN = 380, PANE_MIN = 300;
function paneRoom() { const main = $('#main'); return Math.max(PANE_MIN, (main ? main.clientWidth : innerWidth) - CHAT_MIN); }
function fitPaneWidth() { const host = $('#panes'); if (host) host.style.width = Math.min(P2.width, paneRoom()) + 'px'; }
new ResizeObserver(() => { fitPaneWidth(); for (const id of P2.open) P2.inst[id]?.fit?.(); }).observe($('#main'));
$('#paneGrip').onmousedown = e => {
  e.preventDefault(); const x0 = e.clientX; const w0 = $('#panes').getBoundingClientRect().width; // from what is on screen
  const done = dragShield('col-resize'); let raf = 0;
  const move = ev => { P2.width = Math.round(Math.max(PANE_MIN, Math.min(paneRoom(), w0 - (ev.clientX - x0)))); cancelAnimationFrame(raf); raf = requestAnimationFrame(() => { $('#panes').style.width = P2.width + 'px'; for (const id of P2.open) P2.inst[id]?.fit?.(); }); };
  const up = () => { removeEventListener('mousemove', move); removeEventListener('mouseup', up); done(); lsSet('paneWidth', P2.width); for (const id of P2.open) P2.inst[id]?.fit?.(); };
  addEventListener('mousemove', move); addEventListener('mouseup', up);
};
addEventListener('resize', () => { for (const id of P2.open) P2.inst[id]?.fit?.(); });

// ---------------------------------------------------------------- Changes: file list, diff with line comments, commit, discard
function mountChanges(pb, extra, c) {
  pb.innerHTML = `<div class="chg2"><div class="fl"></div><div class="dv"><div class="empty">${t('Selecione um arquivo')}</div></div></div><div class="pfoot"><span class="faint ncom"></span><button class="btn sm line sendc hidden">${t('Enviar comentários')} <span class="kbd">Ctrl Enter</span></button><span class="grow"></span><input class="cmsg" placeholder="${esc(t('Mensagem do commit'))}"><button class="btn sm pri commit">${t('Commit')}</button></div>`;
  extra.innerHTML = `<span class="scopeslot"></span><button class="ib" title="${esc(t('Atualizar'))}">${I.reload}</button>`;
  const fl = $('.fl', pb), dv = $('.dv', pb); let sel = null, files = [];
  const q = () => '&session=' + encodeURIComponent(curL()?.id || '') + '&scope=' + chgScope();
  const comments = []; // {path, line, code, text}
  const updC = () => { $('.ncom', pb).textContent = comments.length ? `${comments.length} ${t('comentário(s)')}` : ''; $('.sendc', pb).classList.toggle('hidden', !comments.length); };
  const load = async () => {
    const sid = curL()?.id || '';
    const r = await api('/api/changes?session=' + encodeURIComponent(sid) + '&scope=' + chgScope(), { root: c.root, dir: c.dir });
    files = r.files || [];
    $('.scopeslot', extra).innerHTML = r.local ? scopeSwitch() : ''; bindScope(extra);
    $$('.cmsg, .commit', pb).forEach(x => x.classList.toggle('hidden', !!r.local)); // no git: nothing to commit
    fl.innerHTML = files.length ? '' : `<div class="empty">${t('Nenhuma alteração')}</div>`;
    files.forEach(f => { const it = h(`<div class="fli${f.path === sel ? ' on' : ''}${f.status === 'D' ? ' gone' : ''}" title="${esc(f.path)}"><span class="n">${esc(f.path)}</span>${chgTag(f)}<span class="add">+${f.added}</span><span class="del">−${f.removed}</span></div>`); it.onclick = () => select(f.path); it.oncontextmenu = e => { e.preventDefault(); menu(null, [{ label: 'Abrir no painel Arquivos', f: () => openFileInPane(f.path) }, ...openInItems(c.dir.replace(/[\\/]+$/, '') + '/' + f.path), { sep: 1 }, { label: 'Descartar alterações', danger: 1, f: () => discard(f.path) }], e); }; fl.appendChild(it); });
    if (sel && files.find(f => f.path === sel)) select(sel); else if (files[0]) select(files[0].path); else dv.innerHTML = `<div class="empty">${t('Nada para revisar.')}</div>`;
  };
  const discard = async p => { if (!confirm(t('Descartar as alterações em ') + p + '?')) return; await fetch(`/api/git?root=${encodeURIComponent(c.dir)}${q()}&project=${encodeURIComponent(c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ action: 'discard', path: p }) }); load(); const L = curL(); if (L) refreshChanges(L); };
  const select = async p => {
    sel = p; $$('.fli', fl).forEach(x => x.classList.toggle('on', x.title === p));
    const r = await api('/api/diff?path=' + encodeURIComponent(p) + q() + '&project=' + encodeURIComponent(c.root), { root: c.dir });
    const lines = (r.diff || '').replace(/\n$/, '').split('\n'); let nNew = 0; const out = h('<div class="rdiff"></div>');
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
  $('.ib', extra).onclick = load;
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
    // Ctrl+letter that arrives without a key code (remote desktop, some layouts, automation) is still a control character
    term.attachCustomKeyEventHandler(e => { if (e.type === 'keydown' && e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && !e.keyCode && /^[a-z]$/i.test(e.key)) { term.input(String.fromCharCode(e.key.toUpperCase().charCodeAt(0) - 64), true); e.preventDefault(); return false; } return true; });
    const r = await api('/api/term/open', { body: { cwd: cwd || c.dir, cols: term.cols, rows: term.rows, run: run || '' }, root: c.root });
    if (r.error) { term.write('\r\n' + r.error + '\r\n'); return; }
    const tb = { id: r.id, term, fit, el, name: name || baseName(r.cwd) || 'shell' + (tabs.length + 1) };
    const es = new EventSource(`/api/term/${r.id}/stream?root=${encodeURIComponent(c.root)}`); tb.es = es;
    es.onmessage = e => { const bin = atob(e.data); const bytes = new Uint8Array(bin.length); for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i); term.write(bytes); };
    es.addEventListener('exit', () => { term.write(`\r\n\x1b[2m[${t('processo encerrado')}]\x1b[0m\r\n`); es.close(); tb.dead = true; });
    es.onerror = () => { if (es.readyState === EventSource.CLOSED || tb.dead) return; fetch(`/api/term/${r.id}/resize?root=${encodeURIComponent(c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ cols: term.cols, rows: term.rows }) }).then(x => { if (x.status === 404) { es.close(); tb.dead = true; term.write(`\r\n\x1b[2m[${t('terminal encerrado')}]\x1b[0m\r\n`); } }); };
    let pending = '', timer = null;
    term.onData(d => { if (tb.dead) return; pending += d; if (!timer) timer = setTimeout(() => { const data = pending; pending = ''; timer = null; fetch(`/api/term/${r.id}/input?root=${encodeURIComponent(c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ data }) }); }, 8); });
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
    const r = await fetch(`/api/file?root=${encodeURIComponent(c.root)}&dir=${encodeURIComponent(c.dir)}`, { method: 'PUT', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ path: curPath, content: ta.value, mtime, force: !!force }) });
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
// The address bar takes an address or words to search. In the app window every tab is a native WebView2
// view placed over the pane. Anywhere else (the UI in a regular browser) every tab is a real browser
// without a window whose screen is streamed here and which receives the mouse, wheel and keyboard —
// so any site opens, with no exceptions, in both cases.
function addressOf(v) {
  v = v.trim(); if (!v) return '';
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(v) || /^(about|data|file):/i.test(v)) return v;
  if (/^(localhost|127\.0\.0\.1|\[::1\]|\d{1,3}(\.\d{1,3}){3})(:\d+)?(\/|$)/i.test(v)) return 'http://' + v;
  if (!/\s/.test(v) && /^[\w-]+(\.[\w-]+)+(:\d+)?(\/.*)?$/.test(v)) return 'https://' + v;
  return '';
}
const searchURL = q => 'https://www.google.com/search?q=' + encodeURIComponent(q);
let browserSeq = 0;
// DevTools modifier bits: Alt 1, Ctrl 2, Meta 4, Shift 8
const keyMods = e => (e.altKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.metaKey ? 4 : 0) | (e.shiftKey ? 8 : 0);
function mountBrowser(pb, extra, c) {
  const tabs = []; let cur = null; let native = !!(window.infuse && window.infuse.native); let picking = false;
  pb.innerHTML = `<div class="urlbar"><button class="ib back" title="${esc(t('Voltar'))}">‹</button><button class="ib fwd" title="${esc(t('Avançar'))}">›</button><button class="ib rl" title="${esc(t('Recarregar'))}">${I.reload}</button><input class="url" placeholder="${esc(t('Endereço ou pesquisa — ex.: localhost:3000, github.com, como usar fetch'))}"><span class="spin bload" style="display:none"></span><button class="ib pick" title="${esc(t('Selecionar um elemento da página para a mensagem (Ctrl+Shift+S)'))}">⌖</button><button class="ib ext" title="${esc(t('Abrir no navegador padrão'))}">${I.pop}</button></div><div class="frames" style="flex:1;display:flex;min-height:0;position:relative;overflow:hidden"></div>`;
  extra.innerHTML = `<span class="row tabs" style="gap:2px;min-width:0"></span><button class="ib nt" title="${esc(t('Nova aba'))}">${I.plus}</button><button class="btn sm srv">${t('Servidores')} ${I.caret}</button>`;
  const frames = $('.frames', pb), url = $('.url', pb), bar = $('.tabs', extra), spin = $('.bload', pb);
  const label = tb => tb.title || (tb.url ? tb.url.replace(/^https?:\/\/(www\.)?/, '').split(/[/?#]/)[0] : t('Nova aba'));
  const draw = () => { bar.innerHTML = ''; tabs.forEach(tb => { const el = h(`<span class="ptab${tb === cur ? ' on' : ''}" title="${esc(tb.url || '')}"><span class="trunc" style="max-width:120px">${esc(label(tb))}</span><span class="x">×</span></span>`); el.onclick = e => { if (e.target.classList.contains('x')) return closeTab(tb); show(tb); }; bar.appendChild(el); }); spin.style.display = cur?.loading ? '' : 'none'; };
  const onEv = ev => { const tb = tabs.find(x => x.id === ev.id); if (!tb) return; if (ev.url !== undefined) tb.url = ev.url; if (ev.title !== undefined) tb.title = ev.title; tb.loading = !!ev.loading; if (tb === cur && document.activeElement !== url) url.value = tb.url === 'about:blank' ? '' : (tb.url || ''); if (tb.url && tb.url !== 'about:blank') lsSet('browserUrl:' + c.root, tb.url); draw(); };

  // ---- native views (app window): kept exactly over the pane; hidden while a menu or dialog is open
  let lastPlace = '';
  const rectNow = () => { const r = frames.getBoundingClientRect(), d = window.devicePixelRatio || 1; return { x: Math.round(r.left * d), y: Math.round(r.top * d), w: Math.round(r.width * d), h: Math.round(r.height * d) }; };
  const place = force => {
    if (!native) return;
    const rect = rectNow();
    const visible = document.body.contains(frames) && frames.offsetParent !== null && rect.w > 4 && rect.h > 4 && !document.querySelector('.menu, .overlay, .dragshield') && !document.hidden;
    const key = JSON.stringify([rect, visible, cur?.id]); if (!force && key === lastPlace) return; lastPlace = key;
    tabs.forEach(tb => tb.opened && infuse.call('browser', 'place', { id: tb.id, ...rect, visible: visible && tb === cur }).catch(() => { }));
  };
  if (native) infuse.on('browser', ev => { if (document.body.contains(pb)) onEv(ev); });

  // ---- streamed tabs (any other place): the real browser's screen, our mouse and keyboard
  const size = () => ({ width: Math.max(200, Math.round(frames.clientWidth)), height: Math.max(150, Math.round(frames.clientHeight)), dpr: window.devicePixelRatio || 1 });
  const input = (tb, body) => fetch(`/api/view/input?root=${encodeURIComponent(c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ id: tb.id, ...body }) }).then(r => r.json()).catch(() => ({}));
  const stream = tb => {
    const img = tb.img = h('<img class="rview" draggable="false" alt="">'); frames.appendChild(img);
    const kb = tb.kb = h('<textarea class="rkb" aria-label="teclado da página" autocomplete="off" spellcheck="false"></textarea>'); frames.appendChild(kb); // receives the keyboard for the page
    tb.es = new EventSource(`/api/view/stream?root=${encodeURIComponent(c.root)}&id=${encodeURIComponent(tb.id)}`);
    tb.es.onmessage = e => { const d = JSON.parse(e.data); if (d.type === 'frame') { img.src = 'data:image/jpeg;base64,' + d.data; tb.vw = d.w; tb.vh = d.h; if (d.w) { img.style.width = d.w + 'px'; img.style.height = d.h + 'px'; } } else if (d.type === 'nav') onEv({ id: tb.id, url: d.url, title: d.title, loading: d.loading }); };
    const at = e => { const r = img.getBoundingClientRect(); return { x: Math.round(e.clientX - r.left), y: Math.round(e.clientY - r.top) }; }; // the page's CSS pixels
    const btn = e => ['left', 'middle', 'right'][e.button] || 'left';
    img.onmousedown = async e => {
      e.preventDefault(); setTimeout(() => kb.focus(), 0);
      if (picking) { picking = false; img.classList.remove('picking'); const r = await input(tb, { kind: 'pick', ...at(e) }); if (r.pick) insertPick(r.pick); else toast(r.error || t('Nada nesse ponto')); return; }
      input(tb, { kind: 'mouse', type: 'mousePressed', ...at(e), button: btn(e), clickCount: e.detail || 1, modifiers: keyMods(e) });
    };
    img.onmouseup = e => { if (!picking) input(tb, { kind: 'mouse', type: 'mouseReleased', ...at(e), button: btn(e), clickCount: e.detail || 1, modifiers: keyMods(e) }); };
    let moveAt = 0; img.onmousemove = e => { const now = Date.now(); if (now - moveAt < 40) return; moveAt = now; input(tb, { kind: 'mouse', type: 'mouseMoved', ...at(e), modifiers: keyMods(e) }); };
    img.addEventListener('wheel', e => { e.preventDefault(); input(tb, { kind: 'wheel', ...at(e), dx: e.deltaX, dy: e.deltaY }); }, { passive: false });
    img.oncontextmenu = e => e.preventDefault();
    kb.onkeydown = e => {
      if (e.isComposing || e.key === 'Dead' || e.key === 'Process') return; // accents and IME arrive as input
      if ((e.ctrlKey || e.metaKey) && ['l', 't', 'w'].includes(e.key.toLowerCase())) return; // the app's own shortcuts
      e.preventDefault(); e.stopPropagation();
      if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) return input(tb, { kind: 'text', text: e.key });
      const text = e.key === 'Enter' ? '\r' : '';
      input(tb, { kind: 'key', type: 'keyDown', key: e.key, code: e.code, keyCode: e.keyCode, text, modifiers: keyMods(e) }).then(() => input(tb, { kind: 'key', type: 'keyUp', key: e.key, code: e.code, keyCode: e.keyCode, modifiers: keyMods(e) }));
    };
    kb.addEventListener('paste', e => { const txt = e.clipboardData?.getData('text'); if (txt) { e.preventDefault(); input(tb, { kind: 'text', text: txt }); } });
    kb.oninput = () => { if (kb.value) { input(tb, { kind: 'text', text: kb.value }); kb.value = ''; } }; // composed characters (accents, IME)
  };
  // live resize: the page reflows while the pane is dragged — one request in flight, the latest size wins
  let resizeBusy = false, resizeAgain = false, resizeRaf = 0;
  const sendResize = async () => {
    if (resizeBusy) { resizeAgain = true; return; }
    resizeBusy = true; const sz = size();
    await Promise.all(tabs.filter(tb => tb.img).map(tb => input(tb, { kind: 'resize', ...sz })));
    resizeBusy = false; if (resizeAgain) { resizeAgain = false; sendResize(); }
  };
  const resizeStreams = () => { if (native) return; cancelAnimationFrame(resizeRaf); resizeRaf = requestAnimationFrame(sendResize); };
  const ro = new ResizeObserver(() => { place(); resizeStreams(); }); ro.observe(frames);
  const timer = setInterval(() => place(), 200);

  const show = tb => {
    cur = tb; tabs.forEach(x => { if (x.img) x.img.style.display = x === tb ? '' : 'none'; });
    url.value = tb.url && tb.url !== 'about:blank' ? tb.url : ''; draw(); place(true); if (native && tb.opened) infuse.call('browser', 'focus', { id: tb.id }).catch(() => { });
  };
  const add = u => { const tb = { id: 'b' + (++browserSeq) + '-' + Date.now(), url: u }; tabs.push(tb); if (!native) stream(tb); show(tb); if (u) go(u); else setTimeout(() => url.focus(), 30); return tb; };
  const closeTab = tb => { const i = tabs.indexOf(tb); tabs.splice(i, 1); if (tb.opened) infuse.call('browser', 'close', { id: tb.id }).catch(() => { }); if (tb.es) { tb.es.close(); input(tb, { kind: 'close' }); } tb.img?.remove(); tb.kb?.remove(); if (!tabs.length) add(''); else show(tabs[Math.min(i, tabs.length - 1)]); };
  const go = async v => {
    if (!cur) return; const typed = v.trim(); const addr = addressOf(typed) || (typed ? searchURL(typed) : ''); if (!addr) return;
    cur.url = addr; cur.loading = true; url.value = addr; draw();
    if (native) {
      try { await infuse.call('browser', cur.opened ? 'navigate' : 'open', { id: cur.id, url: addr, ...rectNow(), visible: true }); cur.opened = true; place(true); return; }
      catch (e) { native = false; tabs.forEach(x => { if (!x.img) stream(x); }); show(cur); }
    }
    const r = await api('/api/view/open', { body: { id: cur.id, url: addr, ...size() }, root: c.root });
    if (!r.ok) { cur.loading = false; draw(); toast(r.error || t('Não abriu')); }
    cur.kb?.focus();
  };
  const nav = op => { if (!cur) return; if (native && cur.opened) return infuse.call('browser', op, { id: cur.id }).catch(() => { }); if (cur.img) input(cur, { kind: op }); };
  url.onkeydown = e => { if (e.key === 'Enter') { go(url.value); url.blur(); } if (e.key === 'Escape') { url.value = cur?.url || ''; url.blur(); } };
  url.onfocus = () => url.select();
  $('.back', pb).onclick = () => nav('back'); $('.fwd', pb).onclick = () => nav('forward'); $('.rl', pb).onclick = () => nav('reload');
  $('.ext', pb).onclick = () => cur?.url && api('/api/open', { body: { url: cur.url } });
  $('.nt', extra).onclick = () => add('');
  const pick = async () => {
    if (!cur?.url) return toast(t('Abra uma página primeiro'));
    if (native && cur.opened) { toast(t('Clique num elemento da página · Esc cancela')); return infuse.call('browser', 'pick', { id: cur.id }).catch(e => toast(e.message)); }
    picking = true; cur.img?.classList.add('picking'); toast(t('Clique num elemento da página'));
  };
  $('.pick', pb).onclick = pick;
  $('.srv', extra).onclick = async e => {
    const anchor = e.currentTarget; const list = await api('/api/devservers', { root: c.root });
    const items = list.length ? list.flatMap(s => [{ label: s.name, d: `:${s.port} · ${s.running || s.listening ? t('rodando') : t('parado')}${s.approved ? '' : ' · ' + t('não aprovado')}`, f: async () => { if (!s.running && !s.listening) { let r = await api('/api/devservers', { body: { action: 'start', name: s.name }, root: c.root }); if (r.needsApproval) { if (!confirm(t('Executar este comando de launch.json? Ele roda com as suas permissões, fora do filtro.') + '\n\n' + r.command)) return; r = await api('/api/devservers', { body: { action: 'approve', name: s.name }, root: c.root }); } if (r.error) return toast(r.error); toast(t('Iniciando ') + s.name); setTimeout(() => go(s.url), 2500); } else go(s.url); } }]) : [{ label: 'Nenhum servidor configurado', sub: 'Declare em .infuse/launch.json ou .claude/launch.json', f: () => { } }];
    if (list.some(s => s.running)) items.push({ sep: 1 }, ...list.filter(s => s.running).map(s => ({ label: t('Parar ') + s.name, f: () => api('/api/devservers', { body: { action: 'stop', name: s.name }, root: c.root }) })), { label: 'Parar todos', danger: 1, f: () => api('/api/devservers', { body: { action: 'stopall' }, root: c.root }) });
    menu(anchor, items, null, { alignRight: true });
  };
  add(lsGet('browserUrl:' + c.root, ''));
  return { go: v => { if (!cur) add(''); go(v); }, pick, destroy: () => { clearInterval(timer); ro.disconnect(); tabs.forEach(tb => { if (tb.opened) infuse.call('browser', 'close', { id: tb.id }).catch(() => { }); if (tb.es) { tb.es.close(); input(tb, { kind: 'close' }); } }); } };
}

// ---------------------------------------------------------------- Tasks, Plan
function mountTasks(pb) {
  const draw = () => { const L = curL(); const ts = L?.snap?.tasks || []; pb.innerHTML = `<div class="plist">${ts.length ? `<div class="card plan" style="border:0;padding:0">${planCard(L).replace(/^<div class="card plan" data-card="plan">/, '').replace(/<\/div>$/, '')}</div>` : `<div class="empty">${t('O agente ainda não registrou o plano nesta sessão. O protocolo exige o plano antes de qualquer alteração.')}</div>`}${L?.running ? `<div class="status" style="margin-top:10px"><span class="spin"></span>${t('executando ')} ${esc(L.running.name)}</div>` : ''}</div>`; };
  draw(); return { refresh: draw };
}
// Background processes of the session: state, how long, the output as it comes, stop.
function mountProcs(pb) {
  const open = new Set(), full = {}; let timer = null;
  const load = async (L, p) => { if (full[p.id]?.bytes === p.bytes) return; const r = await api(`/api/session/${L.id}/procs`, { body: { action: 'output', id: p.id }, root: L.root }); full[p.id] = { bytes: p.bytes, text: r.output ?? '' }; draw(); };
  const draw = () => {
    const L = curL(); const ps = (L?.snap?.procs || []).slice().reverse();
    ps.filter(p => open.has(p.id) && full[p.id]?.bytes !== p.bytes).forEach(p => load(L, p));
    pb.innerHTML = ps.length ? `<div class="plist">${ps.map(p => `<div class="proc ${p.state}" data-id="${p.id}">
      <div class="row ph2"><span class="dot ${p.state === 'running' ? 'run' : p.state === 'stopped' ? '' : p.exit ? 'bad' : 'ok'}"></span><span class="grow">${p.description ? `<span class="d">${esc(p.description)}</span>` : ''}<span class="mono cmd">${esc(p.command)}</span>
      <span class="faint meta">${p.id} · ${p.state === 'running' ? t('rodando há') + ' ' + elapsed(p.started) : (p.state === 'stopped' ? t('encerrado') : t('terminou') + (p.exit != null ? ' · ' + t('código') + ' ' + p.exit : '')) + ' · ' + t('durou') + ' ' + elapsed(p.started, p.ended)}</span></span>
      ${p.state === 'running' ? `<button class="btn sm danger stop">${t('Encerrar')}</button>` : ''}</div>
      ${open.has(p.id) ? `<pre class="pout">${esc(full[p.id]?.text ?? '')}</pre>` : ''}</div>`).join('')}</div>`
      : `<div class="empty">${t('Nenhum processo em segundo plano nesta sessão. O agente inicia um com Bash run_in_background (servidor, jogo, watcher).')}</div>`;
    $$('.proc', pb).forEach(el => {
      const id = el.dataset.id;
      $('.ph2', el).onclick = e => { if (e.target.closest('.stop')) return; open.has(id) ? open.delete(id) : open.add(id); draw(); };
      const st = $('.stop', el); if (st) st.onclick = async () => { await api(`/api/session/${L.id}/procs`, { body: { action: 'stop', id }, root: L.root }); };
      const pre = $('.pout', el); if (pre) pre.scrollTop = pre.scrollHeight;
    });
  };
  draw(); timer = setInterval(() => { if ((curL()?.snap?.procs || []).some(p => p.state === 'running')) draw(); }, 1000); // elapsed time ticks
  return { refresh: draw, destroy: () => clearInterval(timer) };
}

// Subagents of the session: who is doing what, live, and what each one concluded.
function mountAgents(pb) {
  const open = new Set();
  const draw = () => {
    const L = curL(); const as = (L?.snap?.agents || []).slice().reverse();
    pb.innerHTML = as.length ? `<div class="plist">${as.map(a => `<div class="agent ${a.state}" data-id="${a.id}">
      <div class="row ah">${a.state === 'running' ? '<span class="spin sm"></span>' : `<span class="dot ${a.state === 'done' ? 'ok' : 'bad'}"></span>`}<span class="tag">${esc(t(a.kind))}</span><span class="grow tt">${esc(a.title)}</span>
      <span class="faint">${a.steps} ${t('passos')} · ${elapsed(a.started, a.ended)}</span></div>
      ${a.state === 'running' && a.tool ? `<div class="faint now mono">${esc(a.tool)} ${esc(a.summary || '')}</div>` : ''}
      ${open.has(a.id) ? `<div class="abody"><details class="atask"><summary>${t('Tarefa dada ao subagente')}</summary><div class="md sel">${md(a.task || '')}</div></details><div class="atrail">${agentTrailHTML(a, 'pane:' + a.id)}</div></div>` : ''}</div>`).join('')}</div>`
      : `<div class="empty">${t('Nenhum subagente nesta sessão. O agente delega pesquisas e revisões com a ferramenta Agent; vários trabalham ao mesmo tempo.')}</div>`;
    $$('.agent .ah', pb).forEach(h => h.onclick = () => { const id = h.parentElement.dataset.id; open.has(id) ? open.delete(id) : open.add(id); draw(); });
    bindFolds(pb);
  };
  draw(); return { refresh: draw };
}

function mountPlan(pb) {
  const draw = () => { const L = curL(); const p = L?.snap?.plan; pb.innerHTML = p ? `<div class="plist md sel">${md(p)}</div><div class="pfoot"><span class="grow faint">${t('Plano proposto no modo Plano')}</span><button class="btn sm pri">${t('Aprovar e executar')}</button></div>` : `<div class="empty">${t('Nenhum plano ainda. Use o modo Plano (Shift+Tab ou /plan) para o agente propor antes de editar.')}</div>`; const b = $('.btn', pb); if (b) b.onclick = async () => { const v = V(); await setSessionOpts(v, { mode: 'acceptEdits' }); const P = $('.prompt', v.el); P.value = t('Plano aprovado. Execute-o.'); send(v); }; };
  draw(); return { refresh: draw };
}

// ---------------------------------------------------------------- infuse: oracle, rules, journal, vices, map
// Pronto: "Pronto?" runs the criteria in a frozen copy of the project and shows what passed. Criteria
// read as sentences with the values in place; the project's own scripts become one-click suggestions.
const CRIT_KINDS = {
  command: { name: 'Um comando termina sem erro', ex: 'npm test · go test ./... · python -m pytest', help: 'Roda o comando numa cópia congelada do projeto. Passa quando o comando termina sem erro (código de saída 0). Pode exigir também um texto na saída, como "passed".', make: () => ({ kind: 'command', run: '', expect: { exit: 0 } }), title: 'Os testes passam' },
  http: { name: 'Uma página ou API responde', ex: 'http://localhost:3000/login responde 200 e mostra "Entrar"', help: 'Pede o endereço e confere a resposta: o código (200 = ok, 404 = não encontrado) e, se quiser, um texto que precisa aparecer. O app precisa estar rodando — inicie pelo Navegador › Servidores.', make: () => ({ kind: 'http', url: 'http://localhost:3000/', expect: { status: 200 } }), title: 'A página abre' },
  file: { name: 'Um arquivo existe ou contém um texto', ex: 'README.md contém "Instalação" · .env não existe', help: 'Confere um arquivo na cópia do projeto: que exista, que não exista (ex.: nenhum .env com senhas no repositório) ou que contenha um texto.', make: () => ({ kind: 'file', path: '', expect: {} }), title: 'O arquivo existe' },
  maxLines: { name: 'Nenhum arquivo passa de N linhas', ex: 'src/**/*.ts com no máximo 400 linhas', help: 'Percorre os arquivos que casam com o padrão e reprova os que passam do limite. No padrão, ** quer dizer "qualquer pasta" e * "qualquer nome": src/**/*.ts são todos os .ts dentro de src.', make: () => ({ kind: 'maxLines', glob: '**/*.ts', max: 400 }), title: 'Nenhum arquivo grande demais' },
};
const ORACLE_EXAMPLE = `# Pronto quando
- O login aceita e-mail e senha e mostra uma mensagem clara quando a senha está errada.
- Nenhuma tela quebra no celular (largura de 375px).
- Os testes que já existiam continuam passando.
`;
const oracleSpecs = {}; // root → the spec last loaded, for the titles in the verdict
const critTitle = (root, id) => (oracleSpecs[root]?.checks || []).find(k => k.id === id)?.description || id;
async function runOracleUI(c, out) {
  out.innerHTML = `<div class="vbox"><div class="status"><span class="spin"></span>${t('copiando o projeto e rodando cada critério…')}</div></div>`;
  const L = curL();
  const r = await api('/api/oracle/run', { body: { session: L?.id || '' }, root: c.root, dir: c.dir });
  const v = r.verdict || {}; drawVerdict(c, out, v, r.error); if (L) { L.verdict = v; viewsOf(L).forEach(renderRail); }
  out.dispatchEvent(new CustomEvent('verdict', { detail: v, bubbles: true }));
}
function drawVerdict(c, out, v, err) {
  if (!v || !v.run) { out.innerHTML = err ? `<div class="errbox">${esc(err)}</div>` : `<div class="vbox idle"><b>${t('Ainda não verificado')}</b><span>${t('Quando quiser conferir o trabalho, clique em Pronto?.')}</span></div>`; return; }
  const checks = v.checks || [], passed = checks.filter(k => k.passed).length;
  const head = { aprovado: [t('Pronto'), `${passed} ${t('de')} ${checks.length} ${t('critérios passaram')}`, 'ok'], reprovado: [t('Ainda não'), `${checks.length - passed} ${t('de')} ${checks.length} ${t('falharam — veja abaixo o motivo de cada um')}`, 'bad'], 'sem-criterios': [t('Nada para verificar'), t('Adicione pelo menos um critério abaixo.'), 'warn'], erro: [t('Não foi possível verificar'), v.summary || err || '', 'bad'] }[v.verdict] || [v.verdict, v.summary || '', ''];
  out.innerHTML = `<div class="vbox ${head[2]}"><b>${esc(head[0])}</b><span>${esc(head[1])}</span><span class="grow"></span><span class="faint" title="${esc(t('identificador da cópia congelada em que os critérios rodaram'))}">${new Date(v.finished || v.started).toLocaleTimeString()}</span></div>
    ${checks.map(k => `<div class="vrow"><span class="${k.passed ? 'add' : 'del'}">${k.passed ? '✓' : '✗'}</span><span class="vt">${esc(critTitle(c.root, k.id))}</span><span class="faint grow trunc" title="${esc(k.detail)}">${esc(k.detail)}</span><span class="faint">${k.millis < 1000 ? k.millis + ' ms' : (k.millis / 1000).toFixed(1) + ' s'}</span>${k.evidence ? `<button class="btn sm" data-ev="${esc(k.evidence)}">${t('ver saída')}</button>` : ''}</div>`).join('')}
    <div class="evbox"></div>`;
  $$('[data-ev]', out).forEach(b => b.onclick = async () => { const box = $('.evbox', out); if (box.dataset.f === b.dataset.ev) { box.innerHTML = ''; box.dataset.f = ''; return; } const e = await api(`/api/oracle/evidence?run=${encodeURIComponent(v.run)}&file=${encodeURIComponent(b.dataset.ev)}`, { root: c.root }); box.dataset.f = b.dataset.ev; box.innerHTML = `<pre class="evpre sel">${esc(e.content || e.error || t('(saída vazia)'))}</pre>`; });
}
function mountOracle(pb, extra, c) {
  extra.innerHTML = `<button class="ib help" title="${esc(t('Como funciona'))}">?</button>`;
  let spec = { checks: [] }, sugg = [], last = null, helpOpen = lsGet('oracleHelp', null);
  const save = debounce(() => api('/api/oracle/spec', { method: 'PUT', body: { spec }, root: c.root }).then(() => loadSugg()), 400);
  const loadSugg = async () => { sugg = await api('/api/oracle/suggest', { root: c.root }); if (!Array.isArray(sugg)) sugg = []; drawSugg(); };
  const uid = title => { const base = (title || 'criterio').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'criterio'; let id = base, n = 2; while (spec.checks.some(k => k.id === id)) id = base + '-' + n++; return id; };
  const draw = async () => {
    const [files, specR, hist] = await Promise.all([api('/api/oracle', { root: c.root }), api('/api/oracle/spec', { root: c.root }), api('/api/oracle/verdicts', { root: c.root })]);
    spec = specR.spec || { checks: [] }; spec.checks = spec.checks || []; oracleSpecs[c.root] = spec; last = hist[0];
    if (helpOpen === null) helpOpen = !spec.checks.length;
    pb.innerHTML = `<div class="plist oracle">
      <div class="ohead"><button class="btn pri run">${t('Pronto?')}</button><span class="grow">${t('Confere o trabalho por conta própria: roda cada critério numa cópia congelada do projeto e mostra o que passou e o que falhou.')}</span></div>
      <div class="howto" style="${helpOpen ? '' : 'display:none'}">
        <div class="row"><b class="grow">${t('Como funciona')}</b><button class="ib hx" title="${esc(t('Fechar'))}">${I.close}</button></div>
        <ol>
          <li><b>${t('Diga o que precisa ser verdade')}</b> ${t('quando o trabalho estiver pronto. Use uma sugestão abaixo (um clique) ou adicione um critério — por exemplo, "os testes passam" ou "a página de login abre".')}</li>
          <li><b>${t('Peça o trabalho ao agente')}</b> ${t('normalmente, na conversa. Ele lê estes critérios; nenhuma ferramenta dele consegue alterá-los.')}</li>
          <li><b>${t('Clique em Pronto?')}</b> ${t('O infuse copia o projeto do jeito que está agora, roda cada critério nessa cópia e mostra ✓ ou ✗ com a saída de cada um. O resultado vem da execução, não do que o agente diz.')}</li>
        </ol>
        <div class="kinds">${Object.entries(CRIT_KINDS).map(([k, x]) => `<div><b>${t(x.name)}</b><span class="faint">${t('ex.:')} ${esc(x.ex)}</span></div>`).join('')}</div>
      </div>
      <div class="verdict"></div>
      <div class="osect"><span class="grow">${t('O que precisa ser verdade')}</span><button class="btn sm addc">${I.plus} ${t('Adicionar critério')}</button></div>
      <div class="checks"></div>
      <div class="sugg"></div>
      <details class="odet"${spec.setup ? ' open' : ''}><summary>${t('Antes de verificar (opcional)')}</summary>
        <div class="cs">${t('Rodar')} <input class="slot mono setup" value="${esc(spec.setup || '')}" placeholder="npm ci" spellcheck="false"> ${t('na cópia, antes dos critérios.')}</div>
        <div class="faint chelp">${t('Normalmente não é preciso: a cópia já usa as dependências instaladas (node_modules, .venv). Use quando a verificação precisa de um passo antes, como instalar dependências do zero ou gerar código.')}</div>
      </details>
      <details class="odet"${files.some(f => f.name !== 'checks.json' && f.content.trim()) ? ' open' : ''}><summary>${t('Descrição em palavras (opcional)')}</summary>
        <div class="faint chelp">${t('O agente lê este texto antes de trabalhar. Escreva em linguagem comum o que significa "pronto" — o que não dá para conferir com um comando.')}</div>
        <div class="docs"></div>
      </details></div>`;
    $('.run', pb).onclick = () => runOracleUI(c, $('.verdict', pb));
    pb.addEventListener('verdict', e => { last = e.detail; drawChecks(); });
    $('.hx', pb).onclick = () => toggleHelp(false);
    $('.setup', pb).oninput = e => { spec.setup = e.target.value.trim(); save(); };
    $('.addc', pb).onclick = e => menu(e.currentTarget, [{ h: 'Que tipo de critério?' }, ...Object.entries(CRIT_KINDS).map(([k, x]) => ({ label: x.name, d: t('ex.:') + ' ' + x.ex, f: () => addCheck({ ...x.make(), description: t(x.title) }, true) }))], null, { alignRight: true });
    drawVerdict(c, $('.verdict', pb), last); drawChecks(); drawSugg(); drawDocs(files); loadSugg();
  };
  const toggleHelp = open => { helpOpen = open ?? !helpOpen; lsSet('oracleHelp', helpOpen); const hb = $('.howto', pb); if (hb) hb.style.display = helpOpen ? '' : 'none'; };
  $('.help', extra).onclick = () => toggleHelp();
  const addCheck = (k, focus) => { k.id = uid(k.description); spec.checks.push(k); save(); drawChecks(); if (focus) setTimeout(() => { const card = $$('.checks .crit', pb).pop(); (card && ($('.slot', card) || $('.ctitle', card)))?.focus(); }, 30); };
  const drawSugg = () => {
    const box = $('.sugg', pb); if (!box) return;
    if (!sugg.length) { box.innerHTML = ''; return; }
    box.innerHTML = `<div class="osect"><span class="grow">${t('Sugestões para este projeto')}</span>${sugg.length > 1 ? `<button class="btn sm all">${t('Adicionar todas')}</button>` : ''}</div>${sugg.map((s, i) => `<div class="sug"><div class="grow"><b>${esc(t(s.title))}</b><div class="faint">${esc(s.why)}</div></div><button class="btn sm" data-i="${i}">${I.plus} ${t('Adicionar')}</button></div>`).join('')}`;
    const take = s => { if (s.setup && !spec.setup) { spec.setup = s.setup; const inp = $('.setup', pb); if (inp) inp.value = s.setup; } addCheck({ ...s.check, description: t(s.title) }); };
    $$('[data-i]', box).forEach(b => b.onclick = () => { take(sugg[+b.dataset.i]); sugg.splice(+b.dataset.i, 1); drawSugg(); });
    const all = $('.all', box); if (all) all.onclick = () => { sugg.forEach(take); sugg = []; drawSugg(); };
  };
  const slot = (f, val, ph, mono = true) => `<input class="slot${mono ? ' mono' : ''}" data-f="${f}" value="${esc(val ?? '')}" placeholder="${esc(ph)}" spellcheck="false">`;
  const sentence = k => {
    const x = k.expect || (k.expect = {});
    const extra = x.contains != null || k._more ? ` ${t(k.kind === 'command' ? 'e que a saída contenha' : 'e que a resposta contenha')} ${slot('contains', x.contains, t('um texto'), false)}` : '';
    if (k.kind === 'command') return `${t('Rodar')} ${slot('run', k.run, 'npm test')} ${t('e esperar que termine sem erro')}${extra}`;
    if (k.kind === 'http') return `${t('Abrir')} ${slot('url', k.url, 'http://localhost:3000/')} ${t('e esperar a resposta')} ${slot('status', x.status || 200, '200')}${extra}`;
    if (k.kind === 'maxLines') return `${t('Nenhum arquivo')} ${slot('glob', k.glob, '**/*.ts')} ${t('passa de')} ${slot('max', k.max || 400, '400')} ${t('linhas')}`;
    const mode = x.exists === false ? 'absent' : x.contains != null ? 'contains' : 'exists';
    return `${t('O arquivo')} ${slot('path', k.path, 'README.md')} <span class="chip fmode">${t({ exists: 'existe', absent: 'não existe', contains: 'contém' }[mode])} ${I.caret}</span>${mode === 'contains' ? ' ' + slot('contains', x.contains, t('um texto'), false) : ''}`;
  };
  const drawChecks = () => {
    const box = $('.checks', pb); if (!box) return;
    const res = {}; (last?.checks || []).forEach(r => res[r.id] = r);
    box.innerHTML = spec.checks.length ? '' : `<div class="oempty">${t('Nenhum critério ainda. Adicione uma sugestão abaixo ou clique em "Adicionar critério".')}</div>`;
    spec.checks.forEach((k, i) => {
      const r = res[k.id], kind = CRIT_KINDS[k.kind] || CRIT_KINDS.command;
      const card = h(`<div class="crit"><div class="ch"><span class="cst ${r ? (r.passed ? 'ok' : 'bad') : ''}" title="${esc(r ? r.detail : t('ainda não verificado'))}">${r ? (r.passed ? '✓' : '✗') : '○'}</span><input class="ctitle" value="${esc(k.description || k.id)}" placeholder="${esc(t('nome do critério'))}" spellcheck="false"><span class="faint ckind">${esc(t(kind.name))}</span><button class="ib q" title="${esc(t('O que isto faz?'))}">?</button><button class="ib more" title="${esc(t('Mais'))}">⋯</button></div><div class="cs">${sentence(k)}</div><div class="chelp faint" style="display:none">${esc(t(kind.help))}</div>${r && !r.passed ? `<div class="cres">${esc(r.detail)}</div>` : ''}</div>`);
      $('.ctitle', card).oninput = e => { k.description = e.target.value; save(); };
      $$('.slot', card).forEach(inp => inp.oninput = () => {
        const f = inp.dataset.f, val = inp.value; k.expect = k.expect || {};
        if (f === 'contains') { if (val) k.expect.contains = val; else delete k.expect.contains; k._more = true; }
        else if (f === 'status') k.expect.status = parseInt(val) || 200;
        else if (f === 'max') k.max = parseInt(val) || 400;
        else k[f] = val.trim();
        save();
      });
      $('.q', card).onclick = () => { const hp = $('.chelp', card); hp.style.display = hp.style.display ? '' : 'none'; };
      const fm = $('.fmode', card);
      if (fm) fm.onclick = e => menu(e.currentTarget, [['exists', 'existe'], ['absent', 'não existe'], ['contains', 'contém um texto']].map(([m, l]) => ({ label: l, f: () => { k.expect = m === 'absent' ? { exists: false } : m === 'contains' ? { contains: '' } : {}; save(); drawChecks(); } })));
      $('.more', card).onclick = e => menu(e.currentTarget, [
        (k.kind === 'command' || k.kind === 'http') && k.expect?.contains == null && { label: k.kind === 'command' ? 'Exigir um texto na saída' : 'Exigir um texto na resposta', f: () => { k._more = true; k.expect.contains = ''; drawChecks(); } },
        { label: 'Duplicar', f: () => addCheck(JSON.parse(JSON.stringify({ ...k, description: (k.description || k.id) + ' ' + t('(cópia)') }))) },
        { label: 'Explicar este tipo', f: () => $('.q', card).click() },
        { sep: 1 },
        { label: 'Remover', danger: 1, f: () => { spec.checks.splice(i, 1); save(); drawChecks(); } },
      ], null, { alignRight: true });
      box.appendChild(card);
    });
  };
  const drawDocs = files => {
    const docs = $('.docs', pb);
    const prose = files.filter(f => f.name !== 'checks.json');
    (prose.length ? prose : [{ name: 'pronto.md', content: '' }]).forEach(f => {
      const d = h(`<div style="margin-bottom:10px">${prose.length > 1 ? `<div class="faint mono" style="margin-bottom:4px">${esc(f.name)}</div>` : ''}<textarea class="doc" placeholder="${esc(t('Ex.: o login aceita e-mail e senha e mostra uma mensagem clara quando a senha está errada.'))}">${esc(f.content)}</textarea><div class="row"><button class="btn sm ex">${t('Inserir exemplo')}</button><span class="faint st"></span></div></div>`);
      const ta = $('textarea', d), st = $('.st', d);
      const put = async () => { await api('/api/oracle', { method: 'PUT', body: { name: f.name, content: ta.value }, root: c.root }); st.textContent = t('salvo'); setTimeout(() => st.textContent = '', 1200); };
      ta.oninput = debounce(put, 500);
      $('.ex', d).onclick = () => { ta.value = (ta.value.trim() ? ta.value.trimEnd() + '\n\n' : '') + t(ORACLE_EXAMPLE); put(); ta.focus(); };
      docs.appendChild(d);
    });
  };
  draw(); return { refresh: () => loadSugg() };
}
// ---------------------------------------------------------------- Memory
// What the agent must always remember: typed items per session, project or every project. They go into
// every request (most important first) and survive compaction. The agent adds them with its Memory tool.
const MEM_TYPES = {
  correcao: ['Correção', 'Algo que você corrigiu e que não deve se repetir', 'use pnpm, não npm'],
  criterio: ['Critério de aceite', 'Condição combinada para o trabalho estar pronto', 'o formulário valida o e-mail antes de enviar'],
  decisao: ['Decisão', 'Uma escolha feita, e o porquê', 'CommonJS em vez de ESM: roda no Node 22 sem build'],
  beco: ['Beco sem saída', 'Algo já tentado que não funciona, e o motivo', 'vite 6 quebra o plugin de PWA; ficar no 5'],
  comando: ['Comando que funcionou', 'Como rodar, testar ou publicar', 'pnpm test -- --run'],
  arquivo: ['Arquivo', 'Um arquivo importante e o papel dele', 'src/auth.ts concentra a sessão do usuário'],
  fato: ['Fato', 'Qualquer informação útil sobre o projeto', 'a API de produção fica em api.exemplo.com'],
};
const MEM_SCOPES = { session: 'Esta sessão', project: 'Este projeto', global: 'Todos os projetos' };
const MEM_PRIO = ['', 'normal', 'importante', 'essencial'];
function mountMemory(pb, extra, c) {
  extra.innerHTML = `<button class="ib help" title="${esc(t('Como funciona'))}">?</button>`;
  const sid = c.L?.id || '';
  let data = { session: [], project: [], global: [] }, scope = lsGet('memScope', 'project'), newType = 'correcao', helpOpen = lsGet('memHelp', true);
  if (scope === 'session' && !sid) scope = 'project';
  const save = debounce(sc => api('/api/memory/items/save', { body: { scope: sc, session: sid, items: data[sc] }, root: c.root }), 300);
  const load = async () => { const r = await api('/api/memory/items' + (sid ? '?session=' + encodeURIComponent(sid) : ''), { root: c.root }); data = { session: r.session || [], project: r.project || [], global: r.global || [] }; draw(); };
  const add = (type, text) => { text = text.trim(); if (!text) return; data[scope].unshift({ id: 'm' + Date.now() + Math.floor(Math.random() * 1000), type, text, origin: 'usuario', priority: type === 'correcao' ? 2 : 1, created: Date.now() }); save(scope); draw(); };
  const draw = () => {
    const items = data[scope] || [];
    pb.innerHTML = `<div class="plist mem">
      <div class="mintro">${t('O que o agente precisa lembrar sempre. Estes itens vão em toda mensagem, mesmo depois que a conversa é resumida — os mais importantes primeiro.')}</div>
      <div class="howto" style="${helpOpen ? '' : 'display:none'}"><div class="row"><b class="grow">${t('Como funciona')}</b><button class="ib hx" title="${esc(t('Fechar'))}">${I.close}</button></div>
        <ol><li>${t('Escreva uma frase e tecle Enter. Escolha o tipo à esquerda — o tipo ajuda o agente a pesar o item.')}</li>
        <li>${t('A estrela marca o que é importante ou essencial: vai primeiro e nunca fica de fora.')}</li>
        <li>${t('O agente também registra itens sozinho (ex.: quando você o corrige). Os dele aparecem marcados "agente"; edite ou apague à vontade.')}</li>
        <li>${t('Escopo: "Esta sessão" vale só aqui; "Este projeto" para toda sessão deste projeto; "Todos os projetos" para tudo.')}</li></ol>
        <div class="kinds">${Object.entries(MEM_TYPES).map(([k, [n, d, ex]]) => `<div class="mex" data-k="${k}" data-ex="${esc(ex)}" title="${esc(t('Usar este exemplo'))}"><b>${t(n)}</b><span class="faint">${t(d)}</span><span class="mono faint">${t('ex.:')} ${esc(ex)}</span></div>`).join('')}</div></div>
      <div class="seg">${Object.entries(MEM_SCOPES).map(([k, l]) => `<button class="${k === scope ? 'on' : ''}" data-sc="${k}" ${k === 'session' && !sid ? 'disabled title="' + esc(t('Abra uma sessão')) + '"' : ''}>${t(l)} <span class="faint">${(data[k] || []).length}</span></button>`).join('')}</div>
      <div class="madd"><span class="chip mtype">${t(MEM_TYPES[newType][0])} ${I.caret}</span><input class="mnew" placeholder="${esc(t('Algo para lembrar — ex.:') + ' ' + t(MEM_TYPES[newType][2]))}" spellcheck="false"></div>
      <div class="mlist">${items.length ? '' : `<div class="oempty">${t('Nada aqui ainda. Escreva acima, ou clique num exemplo em "Como funciona" (?).')}</div>`}</div></div>`;
    const list = $('.mlist', pb);
    items.forEach((it, i) => {
      const row = h(`<div class="mitem p${it.priority || 1}"><span class="chip mt">${t((MEM_TYPES[it.type] || MEM_TYPES.fato)[0])} ${I.caret}</span><textarea class="mtext" rows="1" spellcheck="false">${esc(it.text)}</textarea><button class="ib star" title="${esc(t('Prioridade') + ': ' + t(MEM_PRIO[it.priority || 1]))}">${(it.priority || 1) > 1 ? '★' : '☆'}${(it.priority || 1) > 2 ? '<sup>+</sup>' : ''}</button><button class="ib del" title="${esc(t('Apagar'))}">${I.close}</button><div class="mmeta faint">${t(it.origin === 'agente' ? 'agente' : 'você')} · ${new Date(it.created).toLocaleString([], { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })}</div></div>`);
      const ta = $('.mtext', row); const fit = () => { ta.style.height = 'auto'; ta.style.height = ta.scrollHeight + 'px'; };
      ta.oninput = () => { it.text = ta.value; fit(); save(scope); }; setTimeout(fit, 0);
      $('.star', row).onclick = () => { it.priority = (it.priority || 1) % 3 + 1; save(scope); draw(); };
      $('.del', row).onclick = () => { items.splice(i, 1); save(scope); draw(); };
      $('.mt', row).onclick = e => menu(e.currentTarget, Object.entries(MEM_TYPES).map(([k, [n, d]]) => ({ label: n, d, check: it.type === k, f: () => { it.type = k; save(scope); draw(); } })));
      list.appendChild(row);
    });
    $$('[data-sc]', pb).forEach(b => b.onclick = () => { scope = b.dataset.sc; lsSet('memScope', scope); draw(); });
    const inp = $('.mnew', pb);
    inp.onkeydown = e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); add(newType, inp.value); setTimeout(() => $('.mnew', pb)?.focus(), 0); } };
    $('.mtype', pb).onclick = e => menu(e.currentTarget, Object.entries(MEM_TYPES).map(([k, [n, d]]) => ({ label: n, d, check: newType === k, f: () => { newType = k; const v = inp.value; draw(); const ni = $('.mnew', pb); ni.value = v; ni.focus(); } })));
    $$('.mex', pb).forEach(x => x.onclick = () => { newType = x.dataset.k; draw(); const ni = $('.mnew', pb); ni.value = x.dataset.ex; ni.focus(); ni.select(); });
    $('.hx', pb).onclick = () => { helpOpen = false; lsSet('memHelp', false); draw(); };
  };
  $('.help', extra).onclick = () => { helpOpen = !helpOpen; lsSet('memHelp', helpOpen); draw(); };
  load(); return { refresh: load };
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
