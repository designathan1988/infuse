'use strict';
// overlays
// Rewind/context/palette/shortcuts/side-chat dialogs and the global keyboard.

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
  const dlg = h(`<div class="dlg"><div class="dh">${t('ui.overlays.voltar-no-tempo.1ebfc5')}</div><div class="faint u-p018px8px">${t('ui.overlays.escolha-o-ponto-a.d4464c')}</div><div class="db"></div></div>`);
  const close = overlay(dlg); const db = $('.db', dlg); let k = 0;
  const list = () => {
    db.innerHTML = ''; turns.forEach((tu, i) => { const it = h(`<div class="it${i === k ? ' k' : ''}"><span class="faint">${tu.n}</span><span class="trunc grow">${esc(tu.text.split('\n')[0] || '—')}</span><span class="d">${tu.files ? tu.files + ' ' + t('ui.overlays.arquivo-s.8e3594') : t('ui.overlays.sem-alteracoes-de-codigo.141d8f')}</span></div>`); it.onclick = () => choose(tu); db.appendChild(it); });
  };
  const choose = tu => {
    db.innerHTML = `<div class="faint u-p6px12px">${t('ui.overlays.turno.bf9e3d')} ${tu.n}: ${esc(tu.text.split('\n')[0].slice(0, 90))}</div>`;
    const opts = [
      tu.files && { l: 'Restaurar código e conversa', f: () => run({ turn: tu.n, code: true, conversation: true }) },
      { l: 'Restaurar conversa', d: 'mantém o código atual', f: () => run({ turn: tu.n, code: false, conversation: true }) },
      tu.files && { l: 'Restaurar código', d: 'mantém a conversa', f: () => run({ turn: tu.n, code: true, conversation: false }) },
      tu.n > 1 && { l: 'Resumir até aqui', d: 'compacta o que veio antes', f: () => summarize(tu.n) },
      { l: 'Deixa pra lá', f: () => list() },
    ].filter(Boolean);
    opts.forEach((o, i) => { const it = h(`<div class="it">${t(o.l)}<span class="d">${o.d ? t(o.d) + ' · ' : ''}${i + 1}</span></div>`); it.onclick = o.f; db.appendChild(it); });
    const ctx = h(`<input class="ctx" placeholder="${esc(t('ui.overlays.contexto-para-o-resumo.15dd5f'))}">`); if (tu.n > 1) db.appendChild(ctx);
    dlg.onkeydown = e => { const n = +e.key; if (n >= 1 && n <= opts.length && document.activeElement !== ctx) { e.preventDefault(); opts[n - 1].f(); } };
    const summarize = async n => { db.innerHTML = `<div class="status u-p14px"><span class="spin"></span>${t('ui.overlays.resumindo.1db6f1')}</div>`; const r = await api(`/api/session/${L.id}/summarize`, { body: { turn: n, hint: ctx.value }, root: L.root }); close(); if (r.error) return toast(r.error); L.snap = norm(r.session); viewsOf(L).forEach(renderView); toast('Conversa resumida'); };
  };
  const run = async body => {
    const r = await api(`/api/session/${L.id}/rewind`, { body, root: L.root }); close();
    if (r.error) return toast(r.error);
    Live.patch(L.id, { snap: norm(r.session) }); viewsOf(L).forEach(renderView); refreshChanges(L);
    if (body.conversation && r.prompt) { const P = $('.prompt', elOf(v)); P.value = r.prompt; grow(P); P.focus(); }
    toast(body.code && body.conversation ? 'Código e conversa restaurados' : body.code ? 'Código restaurado' : 'Conversa restaurada');
  };
  dlg.tabIndex = 0; dlg.onkeydown = e => { if (e.key === 'ArrowDown') { k = Math.min(k + 1, turns.length - 1); list(); } else if (e.key === 'ArrowUp') { k = Math.max(0, k - 1); list(); } else if (e.key === 'Enter') choose(turns[k]); };
  if (preselect) choose(turns.find(x => x.n === preselect) || turns[0]); else list(); dlg.focus();
}
async function contextDialog(v) {
  const L = v.sid && S.live[v.sid]; if (!L) return;
  const r = await api(`/api/session/${L.id}/context`, { root: L.root }); const u = L.snap.usage || {};
  const dlg = h(`<div class="dlg"><div class="dh">${t('ui.overlays.contexto-e-custo.6e6abf')}</div><div class="db u-p018px16px"></div></div>`); overlay(dlg);
  const pct = x => Math.round(x / (r.limit || 200000) * 100);
  $('.db', dlg).innerHTML = `<div class="bar100 chart">${r.categories.map((c, i) => `<i style="--w:${pct(c.tokens)}%" class="c${i % 6}"></i>`).join('')}</div>
  <table class="tbl">${r.categories.map((c, i) => `<tr><td><span class="chartdot c${i % 6}"></span> ${t(c.name)}</td><td class="mono">${ktok(c.tokens)}</td><td class="faint">${pct(c.tokens)}%</td></tr>`).join('')}
  <tr><td><b>${t('ui.overlays.total-estimado.fe8c12')}</b></td><td class="mono"><b>${ktok(r.total)}</b></td><td class="faint">${t('ui.chat.de.600ccd')} ${ktok(r.limit)}</td></tr></table>
  <h4 class="u-m16px06px-fs12px">${t('ui.overlays.esta-sessao.6d8bb1')}</h4><div class="kv"><span>${t('ui.overlays.entrada.a9df7a')}</span><span class="mono">${ktok((u.input || 0) + (u.cacheRead || 0) + (u.cacheWrite || 0))} tokens</span></div><div class="kv"><span>${t('ui.overlays.saida.3b08da')}</span><span class="mono">${ktok(u.output)} tokens</span></div><div class="kv"><span>${t('ui.overlays.custo.19a010')}</span><span class="mono">${money(L.snap.cost)}</span></div>
  <div class="faint u-mt8px">${t('ui.overlays.compactacao-automatica-em-75.f808fb')}</div>`;
}
function palette(kind) {
  const all = paletteItems(kind);
  const dlg = h(`<div class="dlg"><input class="q" placeholder="${esc(t(kind === 'sessions' ? 'Buscar sessão…' : 'Digite um comando ou busque uma sessão…'))}"><div class="db u-p6px"></div></div>`);
  const close = overlay(dlg); const q = $('input', dlg), db = $('.db', dlg); let k = 0, list = [];
  const draw = () => { const s = q.value.toLowerCase(); list = all.filter(x => t(x.label).toLowerCase().includes(s) || (x.d || '').toLowerCase().includes(s)).slice(0, 80); k = Math.min(k, Math.max(0, list.length - 1)); db.innerHTML = ''; list.forEach((x, i) => { const it = h(`<div class="it${i === k ? ' k' : ''}"><span class="trunc">${esc(t(x.label))}</span><span class="d">${esc(x.d || '')}</span></div>`); it.onclick = () => { close(); x.f(); }; db.appendChild(it); }); db.children[k]?.scrollIntoView({ block: 'nearest' }); };
  q.oninput = () => { k = 0; draw(); }; q.onkeydown = e => { if (e.key === 'ArrowDown') { k = Math.min(k + 1, list.length - 1); draw(); e.preventDefault(); } else if (e.key === 'ArrowUp') { k = Math.max(0, k - 1); draw(); e.preventDefault(); } else if (e.key === 'Enter' && list[k]) { close(); list[k].f(); } };
  draw(); q.focus();
}
function paletteItems(kind) {
  const sess = S.sessions.map(s => ({ label: s.title || t('ui.sidebar.nova-sessao.a7ef08'), d: projectLabel(s.root) + ' · ' + ago(s.updated), f: () => openSession(s.root, s.id) }));
  if (kind === 'sessions') return sess;
  const v = V();
  return [
    { label: 'Nova sessão', d: 'Ctrl N', f: () => newSessionScreen() }, { label: 'Agendadas', f: () => openScheduled() }, { label: 'Personalizar', f: () => openCustomize() }, { label: 'Configurações', d: 'Ctrl ,', f: () => openSettings() },
    { label: 'Provedores e chaves', f: () => openSettings('providers') },
    { label: 'Voltar no tempo…', d: 'Esc Esc', f: () => rewindDialog(v) }, { label: 'Pergunta paralela', d: 'Ctrl ;', f: () => openSide(v) },
    { label: 'Alterações', d: 'Ctrl Shift D', f: () => togglePane('changes', true) }, { label: 'Terminal', d: 'Ctrl `', f: () => togglePane('terminal', true) }, { label: 'Arquivos', f: () => togglePane('files', true) }, { label: 'Navegador', d: 'Ctrl Shift B', f: () => togglePane('browser', true) },
    { label: 'Tarefas', f: () => togglePane('tasks', true) }, { label: 'Plano', f: () => togglePane('plan', true) }, { label: 'Processos', f: () => togglePane('procs', true) }, { label: 'Agentes', f: () => togglePane('agents', true) }, { label: 'Pronto (critérios)', f: () => togglePane('oracle', true) }, { label: 'Memória', f: () => togglePane('memory', true) }, { label: 'Regras', f: () => togglePane('rules', true) }, { label: 'Jornal', f: () => togglePane('journal', true) }, { label: 'Vícios', f: () => togglePane('vices', true) }, { label: 'Mapa do projeto', f: () => togglePane('map', true) },
    ...Object.entries(modeInfo).map(([k, [l]]) => ({ label: t('ui.composer.modo.3d33be') + t(l), f: () => setSessionOpts(v, { mode: k }) })),
    { label: 'Exibição: Normal', f: () => setViewMode('normal') }, { label: 'Exibição: Pensamento', f: () => setViewMode('thinking') }, { label: 'Exibição: Detalhado', d: 'Ctrl O', f: () => setViewMode('verbose') },
    { label: 'Barra lateral', d: 'Ctrl B', f: () => document.body.classList.toggle('nosb') }, { label: 'Atalhos de teclado', d: 'Ctrl /', f: shortcutsDialog },
    ...sess,
  ];
}
function shortcutsDialog() {
  const rows = [['Nova sessão', 'Ctrl N'], ['Paleta de comandos', 'Ctrl K'], ['Trocar de sessão', 'Ctrl Tab'], ['Próxima / anterior na lista', 'Ctrl Shift ] / ['], ['Fechar sessão', 'Ctrl W'], ['Parar o agente', 'Esc'], ['Voltar no tempo', 'Esc Esc'], ['Ciclar modo de permissão', 'Shift Tab'],
    ['Menu de modo', 'Ctrl Shift M'], ['Menu de modelo', 'Ctrl Shift I'], ['Menu de esforço', 'Ctrl Shift E'], ['Alterações', 'Ctrl Shift D'], ['Navegador', 'Ctrl Shift B'], ['Terminal', 'Ctrl `'], ['Fechar painel em foco', 'Ctrl \\'], ['Sair da tela dividida', 'Ctrl Alt W'],
    ['Pergunta paralela', 'Ctrl ;'], ['Ciclar exibição', 'Ctrl O'], ['Barra lateral', 'Ctrl B'], ['Aprovar / sempre / negar', 'Y / A / N'], ['Histórico de prompts', '↑ / ↓'], ['Item do menu aberto', '1–9'], ['Enviar comentários do diff', 'Ctrl Enter'], ['Abrir sessão lado a lado', 'Alt + clique']];
  const dlg = h(`<div class="dlg"><div class="dh">${t('ui.overlays.atalhos-de-teclado.287af6')}</div><div class="db"><div class="sc">${rows.map(([a, b]) => `<span>${t(a)}</span><span class="kbd">${b}</span>`).join('')}</div></div></div>`); overlay(dlg);
}
function openSide(v, first) {
  const L = v.sid && S.live[v.sid]; if (!L) return toast('Abra uma sessão primeiro');
  if (viewRT(v).sideEl) { viewRT(v).sideEl.remove(); viewRT(v).sideEl = null; if (!first) return; }
  const el = h(`<div class="side"><div class="sh">${t('ui.sidebar.pergunta-paralela.aad6c9')}<span class="faint u-fw400">${t('ui.overlays.nao-entra-no-historico.64e4e8')}</span><span class="grow"></span><button class="ib">${I.close}</button></div><div class="sb"></div><textarea rows="1" placeholder="${esc(t('ui.overlays.pergunte-sobre-a-sessao.22723f'))}"></textarea></div>`);
  viewRT(v).sideEl = el; elOf(v).appendChild(el); const sb = $('.sb', el), ta = $('textarea', el);
  $('.sh button', el).onclick = () => { el.remove(); viewRT(v).sideEl = null; };
  const ask = async q => { sb.appendChild(h(`<div class="sq">${esc(q)}</div>`)); const w = h(`<div class="status"><span class="spin"></span></div>`); sb.appendChild(w); sb.scrollTop = sb.scrollHeight; const r = await api(`/api/session/${L.id}/btw`, { body: { question: q }, root: L.root }); w.replaceWith(h(`<div class="md sel">${r.ok ? md(r.answer) : `<span class="del">${esc(r.error)}</span>`}</div>`)); sb.scrollTop = sb.scrollHeight; };
  ta.onkeydown = e => { e.stopPropagation(); if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); const q = ta.value.trim(); if (q) { ta.value = ''; ask(q); } } if (e.key === 'Escape') { el.remove(); viewRT(v).sideEl = null; } };
  ta.focus(); if (first) ask(first);
}
function pathMenu(path, ev) {
  const root = curDir(); const abs = /^([a-zA-Z]:|\/)/.test(path) ? path : root.replace(/[\\/]+$/, '') + (root.includes('\\') ? '\\' : '/') + path.replace(/\//g, root.includes('\\') ? '\\' : '/');
  const rel = abs.startsWith(root) ? abs.slice(root.length).replace(/^[\\/]/, '').replace(/\\/g, '/') : path;
  menu(null, [
    { label: 'Abrir no painel Arquivos', f: () => openFileInPane(rel) },
    { label: 'Anexar como contexto', f: () => { const v = V(); if (!viewRT(v).atts.find(a => a.path === abs)) viewRT(v).atts.push({ path: abs, name: baseName(abs), size: 0 }); renderAtts(v); } },
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
  if (ctrl && e.shiftKey && k === 'a') { e.preventDefault(); togglePane('agents'); return; }
  if (ctrl && e.shiftKey && k === 'p') { e.preventDefault(); togglePane('procs'); return; }
  if (ctrl && e.shiftKey && k === 's') { e.preventDefault(); togglePane('browser', true); setTimeout(() => paneCall('browser', 'pick'), 100); return; }
  if (e.ctrlKey && (e.key === '`' || e.code === 'Backquote')) { e.preventDefault(); togglePane('terminal'); return; }
  if (ctrl && e.key === '\\') { e.preventDefault(); closeFocusedPane(); return; }
  if (ctrl && e.key === ';') { e.preventDefault(); openSide(v); return; }
  if (ctrl && !e.shiftKey && k === 'o') { e.preventDefault(); const order = ['normal', 'thinking', 'verbose']; setViewMode(order[(order.indexOf(S.viewMode) + 1) % 3]); return; }
  if (ctrl && e.shiftKey && k === 'm') { e.preventDefault(); modeMenu(v, $('.modechip', elOf(v))); return; }
  if (ctrl && e.shiftKey && k === 'i') { e.preventDefault(); modelMenu(v, $('.under .chip:last-child', elOf(v))); return; }
  if (ctrl && e.shiftKey && k === 'e') { e.preventDefault(); modelMenu(v, $('.under .chip:last-child', elOf(v)), true); return; }
  if (!inField && L?.snap?.pendingPermission && !ctrl) { const id = L.snap.pendingPermission.id; if (k === 'y') { decide(v, id, 'allow'); return; } if (k === 't') { decide(v, id, 'all'); return; } if (k === 'a') { decide(v, id, 'always'); return; } if (k === 'n') { decide(v, id, 'deny'); return; } }
  if (!inField && L?.snap?.pendingQuestion && /^[1-9]$/.test(e.key)) { const o = L.snap.pendingQuestion.options?.[+e.key - 1]; if (o) answer(v, L.snap.pendingQuestion.id, o); return; }
  if (e.key === 'Escape' && !inField && busy(L)) { stop(v); return; }
  if (!inField && !ctrl && e.key.length === 1 && v?.el && !S.page) { $('.prompt', elOf(v)).focus(); }
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
    for (const a of r.attachments || []) if (!viewRT(v).atts.find(x => x.path === a.path)) viewRT(v).atts.push(a);
    renderAtts(v);
  } else { files.forEach(f => readBrowserFile(v, f)); if ([...e.dataTransfer.items].some(i => i.webkitGetAsEntry?.()?.isDirectory)) toast('Para abrir uma pasta use o seletor de pasta abaixo da caixa de texto'); }
});
