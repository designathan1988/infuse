'use strict';
// core
// Pane registry and layout manager (loaded after the panes it references).

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
  S.views.forEach(v => { if (!elOf(v)) return; $$('.vbar [data-a]', elOf(v)).forEach(b => b.classList.toggle('on', P2.open.includes(b.dataset.a))); $('.chg', elOf(v))?.classList.toggle('on', P2.open.includes('changes')); });
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
