'use strict';
// composer
// Prompt box: attachments, slash commands, autocomplete, sending, queuing, vim keys.

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
  v = freshView(v);
  if (!elOf(v)) return;
  const o = vopts(v); const L = v.sid && S.live[v.sid];
  const under = $('.under', elOf(v)); under.innerHTML = '';
  const add = el => under.appendChild(el);
  if (!o.session) {
    add(h(`<span class="chip static" title="${esc(t('Ambiente: este computador'))}">${I.pc}<span class="v">${t('Local')}</span></span>`));
    const fc = h(`<span class="chip" title="${esc(o.root || '')}">${I.folder}<span class="v">${esc(o.root ? projectLabel(o.root) : t('Escolher pasta'))}</span>${I.caret}</span>`);
    fc.onclick = e => folderMenu(v, e.currentTarget); add(fc);
    const wt = h(`<span class="chip" title="${esc(t('Criar um worktree git isolado para esta sessão'))}"><span class="sw${o.worktree ? ' on' : ''}"></span><span>${t('Worktree')}</span></span>`);
    wt.onclick = () => { View.draft(v.id, { worktree: !v.draft.worktree }); renderComposer(freshView(v)); }; add(wt);
  } else {
    add(h(`<span class="chip static" title="${esc(L.snap.workRoot || o.root)}">${I.folder}<span class="v">${esc(projectLabel(o.root))}</span></span>`));
  }
  add(h('<span style="flex:1"></span>'));
  const brow = $('.brow', elOf(v)); $('.modechip', brow)?.remove();
  const mc = h(`<span class="chip modechip mode-${o.mode}" title="${esc(t('Modo de permissão (Shift+Tab ou Ctrl+Shift+M)'))}"><span class="v">${t(modeInfo[o.mode]?.[0] || o.mode)}</span>${I.caret}</span>`);
  mc.onclick = e => modeMenu(v, e.currentTarget); brow.insertBefore(mc, $('.sp', brow));
  const mo = h(`<span class="chip" title="${esc(t('Modelo e esforço (Ctrl+Shift+I)'))}"><span class="v">${esc(modelLabel(o))}</span><span class="faint">${t(effortInfo[o.effort] || '')}</span>${I.caret}</span>`);
  mo.onclick = e => modelMenu(v, e.currentTarget); add(mo);
  // send / stop, queue, context ring
  const sendBtn = $('.send', elOf(v)); const isBusy = busy(L);
  sendBtn.classList.toggle('stop', !!(isBusy && !$('.prompt', elOf(v)).value.trim()));
  sendBtn.innerHTML = sendBtn.classList.contains('stop') ? I.stop : I.up; sendBtn.title = t(sendBtn.classList.contains('stop') ? 'Parar (Esc)' : isBusy ? 'Pôr na fila (Enter)' : 'Enviar (Enter)');
  const q = $('.queue', elOf(v)); q.innerHTML = '';
  (L?.queue || []).forEach((m, i) => { const row = h(`<div class="qi"><span>⏎ ${esc(m.text)}</span><span class="faint">${t('na fila')}</span><button class="ib" style="width:20px;height:20px">${I.close}</button></div>`); $('button', row).onclick = () => { Live.queue(L.id, 'remove', null, i); renderComposer(freshView(v)); }; q.appendChild(row); });
  const ring = $('.ring', elOf(v)); const pct = L ? Math.min(100, Math.round(((L.lastIn || 0) / ctxLimit(L)) * 100)) : 0;
  const c = 2 * Math.PI * 8;
  ring.innerHTML = L ? `<svg width="18" height="18" viewBox="0 0 20 20"><circle cx="10" cy="10" r="8" stroke="var(--s3)"/><circle cx="10" cy="10" r="8" stroke="${pct > 80 ? 'var(--warn)' : 'var(--text2)'}" stroke-dasharray="${c * pct / 100} ${c}" transform="rotate(-90 10 10)"/></svg>` : '';
  ring.title = L ? `${t('Contexto')}: ${pct}% · ${ktok(L.lastIn)} / ${ktok(ctxLimit(L))}` : '';
  $('.box', elOf(v)).classList.toggle('vim', !!S.settings.vim);
  renderAtts(v);
}
function folderMenu(v, anchor) {
  const items = [{ h: 'Projetos' }, ...S.projects.map(p => ({ label: p.label || baseName(p.root), d: p.root, check: p.root === v.draft.root, f: () => { View.draft(v.id, { root: p.root }); lsSet('lastRoot', p.root); api('/api/projectmode', { root: p.root }).then(r => { View.draft(v.id, { mode: r.mode || '' }); renderView(freshView(v)); }); renderView(freshView(v)); renderPanes(); } })), { sep: 1 },
    { label: 'Abrir pasta…', d: 'Explorador', f: () => pickFolder(v) }];
  menu(anchor, items, null, { above: true });
}
// Always the Windows folder dialog: inside the native window through the bridge, in a browser through
// the local server (same machine), which opens it in front of the window in use.
async function pickFolder(v) {
  const p = native ? await nat('pickFolder') : (await api('/api/dialog/folder', { body: {}, root: '' })).path;
  if (p) { await loadSessions(); View.draft(v.id, { root: p }); lsSet('lastRoot', p); renderView(freshView(v)); renderPanes(); }
  return;
}
async function setSessionOpts(v, patch) {
  const L = v.sid && S.live[v.sid];
  if (!L) { View.draft(v.id, patch); renderComposer(freshView(v)); return; }
  const r = await api(`/api/session/${L.id}/settings`, { body: patch, root: L.root });
  if (r.id) { Live.snap(L.id, { ...r, messages: L.snap.messages }); viewsOf(L).forEach(renderView); if (patch.title) loadSessions(); }
}
function modeMenu(v, anchor) {
  const o = vopts(v);
  menu(anchor, Object.entries(modeInfo).map(([k, [l, d]]) => ({ label: l, sub: d, check: o.mode === k, f: () => { setSessionOpts(v, { mode: k }); toast(t('Modo: ') + t(l)); } })), null, { above: true, numbers: true });
}
async function modelMenu(v, anchor, effortOnly) {
  const o = vopts(v);
  const effort = [{ h: 'Esforço' }, ...Object.entries(effortInfo).map(([k, l]) => ({ label: l, check: o.effort === k, sub: { low: 'Rápido, sem raciocínio estendido', medium: 'Equilíbrio', high: 'Mais raciocínio', max: 'Raciocínio máximo, mais lento e caro' }[k], f: () => setSessionOpts(v, { effort: k }) }))];
  if (effortOnly) return menu(anchor, effort, null, { above: true, numbers: true });
  if (!S.models.length) Server.set('models', await api('/api/models', { root: '' }));
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
    { label: 'Comandos de barra', d: '/', f: () => { const P = $('.prompt', elOf(v)); if (!P.value.startsWith('/')) { P.value = '/' + P.value; } P.focus(); autocomplete(v); } },
    { sep: 1 }, { label: 'Pergunta paralela', d: 'Ctrl ;', f: () => openSide(v) },
  ], null, { above: true });
}
function insertAtCursor(v, s) { const P = $('.prompt', elOf(v)); const p = P.selectionStart; P.value = P.value.slice(0, p) + s + P.value.slice(p); P.selectionStart = P.selectionEnd = p + s.length; P.focus(); autocomplete(v); }
async function attachFiles(v) {
  // the Windows file dialog in both cases; files are attached by their real path
  const list = native ? await nat('pickFiles') : await api('/api/dialog/files', { body: {}, root: '' }); if (Array.isArray(list)) { for (const a of list) if (!viewRT(v).atts.find(x => x.path === a.path)) viewRT(v).atts.push(a); renderAtts(v); }
}
function readImage(v, f) { const r = new FileReader(); r.onload = () => { viewRT(v).images.push({ mediaType: f.type, data: r.result.split(',')[1] }); renderAtts(v); }; r.readAsDataURL(f); }
function readBrowserFile(v, f) { if (f.type.startsWith('image/')) return readImage(v, f); const r = new FileReader(); r.onload = () => { const P = $('.prompt', elOf(v)); P.value += `\n<arquivo nome="${f.name}">\n${r.result.slice(0, 200000)}\n</arquivo>`; grow(P); }; r.readAsText(f); }
function renderAtts(v) {
  v = freshView(v);
  const el = $('.atts', elOf(v)); el.innerHTML = '';
  viewRT(v).atts.forEach((a, i) => { const c = h(`<span class="chipx" title="${esc(a.path)}">📎 <span>${esc(a.name)}</span><b>×</b></span>`); $('b', c).onclick = () => { viewRT(v).atts.splice(i, 1); renderAtts(v); }; el.appendChild(c); });
  viewRT(v).images.forEach((im, i) => { const c = h(`<span class="chipx"><img src="data:${im.mediaType};base64,${im.data}"><span>${t('imagem')}</span><b>×</b></span>`); $('b', c).onclick = () => { viewRT(v).images.splice(i, 1); renderAtts(v); }; el.appendChild(c); });
}

// The gray suggestion of the next message (Tab or → accepts), asked of the fast model after each turn.
const PROMPT_PH = 'Peça algo… @ para arquivos, / para comandos';
function setGhost(v, text) { const P = elOf(v) && $('.prompt', elOf(v)); if (!P) return; P.dataset.ghost = text || ''; P.placeholder = text ? text + '   ⇥ Tab' : t(PROMPT_PH); P.classList.toggle('ghost', !!text); }
async function suggestNext(L) {
  if (S.settings.promptSuggestions === false || !L?.id) return;
  const r = await api(`/api/session/${L.id}/suggest`, { body: {}, root: L.root }); if (!r.text) return;
  viewsOf(L).forEach(v => { const P = $('.prompt', elOf(v)); if (P && !P.value && !viewRT(v).atts.length && !viewRT(v).images.length && !busy(L)) setGhost(v, r.text); });
}
async function send(v) {
  const P = $('.prompt', elOf(v)); let text = P.value.trim(); setGhost(v, '');
  if (!text && !viewRT(v).atts.length && !viewRT(v).images.length) return;
  if (text.startsWith('/')) { const handled = await slash(v, text); if (handled) { P.value = ''; grow(P); hideAC(); return; } text = P.value.trim(); }
  let L = v.sid && S.live[v.sid];
  if (!L) {
    const d = vopts(v);
    if (!d.root) { folderMenu(v, $('.under .chip:nth-child(2)', elOf(v))); toast('Escolha a pasta do projeto'); return; }
    const r = await api('/api/sessions/new', { body: { mode: v.draft.mode, provider: d.provider, model: d.model, effort: d.effort, worktree: d.worktree, title: text.slice(0, 40) }, root: d.root });
    if (r.error) return toast(r.error);
    await openSession(d.root, r.id); L = S.live[r.id];
  }
  const msg = { text: text || t('(anexos)'), attachments: viewRT(v).atts, images: viewRT(v).images };
  UI.history(text); viewRT(v).hi = -1;
  viewRT(v).atts = []; viewRT(v).images = []; P.value = ''; grow(P); hideAC();
  if (busy(L)) { Live.queue(L.id, 'push', msg); renderComposer(freshView(v)); return; }
  const r = await api(`/api/sessions/${L.id}/send`, { body: msg, root: L.root }); if (r.error) toast(r.error);
  renderComposer(v);
}
async function drainQueue(L) {
  const current = S.live[L.id];
  if (!current?.queue?.length) return;
  const next = current.queue[0];
  Live.queue(L.id, 'shift');
  viewsOf(L).forEach(renderComposer);
  const r = await api(`/api/sessions/${L.id}/send`, { body: next, root: L.root }); if (r.error) { toast(r.error); Live.queue(L.id, 'unshift', next); }
}
function stop(v) { const L = v.sid && S.live[v.sid]; if (L) api(`/api/sessions/${L.id}/stop`, { body: {}, root: L.root }); }

// slash commands: built-ins act here; custom ones expand into the prompt
async function slash(v, text) {
  const [cmd, ...rest] = text.slice(1).split(/\s+/); const arg = rest.join(' ');
  const root = vopts(v).root; const cmds = await getCommands(root);
  const custom = cmds.find(c => !c.builtin && c.name === cmd);
  if (custom) { $('.prompt', elOf(v)).value = custom.prompt.replace(/\$ARGUMENTS/g, arg); return false; }
  const L = v.sid && S.live[v.sid];
  switch (cmd) {
    case 'clear': case 'new': newSessionScreen(root); return true;
    case 'compact': if (L && L.snap.turn > 1) { const r = await api(`/api/session/${L.id}/summarize`, { body: { turn: L.snap.turn, hint: arg }, root: L.root }); toast(r.error || 'Conversa resumida'); } else toast('Nada para compactar ainda'); return true;
    case 'model': modelMenu(v, $('.under .chip:last-child', elOf(v))); return true;
    case 'effort': modelMenu(v, $('.under .chip:last-child', elOf(v)), true); return true;
    case 'mode': case 'permissions': modeMenu(v, $('.modechip', elOf(v))); return true;
    case 'plan': setSessionOpts(v, { mode: 'plan' }); if (arg) { $('.prompt', elOf(v)).value = arg; return false; } return true;
    case 'rewind': case 'undo': case 'checkpoint': rewindDialog(v); return true;
    case 'diff': togglePane('changes', true); return true;
    case 'cost': case 'usage': case 'context': contextDialog(v); return true;
    case 'btw': openSide(v, arg); return true;
    case 'config': case 'settings': openSettings(); return true;
    case 'resume': palette('sessions'); return true;
    case 'rename': if (L && arg) setSessionOpts(v, { title: arg }); else renameInline(v); return true;
    case 'review': case 'code-review': runReview(v, arg); return true;
    case 'mission': case 'missao': if (arg) $('.prompt', elOf(v)).value = arg; missionDialog(v); return true;
    case 'init': $('.prompt', elOf(v)).value = t('Analise este projeto e escreva um INFUSE.md na raiz com: como compilar, testar e rodar; a arquitetura e onde cada responsabilidade mora; convenções do código. Seja conciso.'); return false;
    case 'memory': togglePane('memory', true); return true;
    case 'terminal': togglePane('terminal', true); return true;
    case 'vim': await saveSetting({ vim: !S.settings.vim }); toast(S.settings.vim ? 'Modo vim ligado' : 'Modo vim desligado'); S.views.forEach(renderComposer); return true;
    case 'help': shortcutsDialog(); return true;
    case 'copy': if (L) { const n = parseInt(arg) || 1; const ms = L.snap.messages.filter(x => x.role === 'assistant' && msgText(x)); const m = ms[ms.length - n]; if (m) copyText(msgText(m), 'Resposta copiada'); } return true;
    case 'export': if (L) exportTranscript(L); return true;
  }
  return false;
}
async function getCommands(root) { if (!S.commands[root]) UI.sub('commands', root, await api('/api/commands', { root })); return S.commands[root] || []; }

// @ files and / commands autocomplete
let AC = null;
async function autocomplete(v) {
  const P = $('.prompt', elOf(v)); const before = P.value.slice(0, P.selectionStart);
  const m = before.match(/(^|\s)([@/])([^\s]*)$/);
  if (!m || (m[2] === '/' && before.trim() !== m[2] + m[3])) return hideAC();
  const root = vopts(v).root; const q = m[3].toLowerCase(); let items;
  if (m[2] === '@') { if (!root) return hideAC(); if (!S.files[root]) UI.sub('files', root, await api('/api/files/list', { root })); items = (S.files[root] || []).filter(f => f.toLowerCase().includes(q)).sort((a, b) => (a.toLowerCase().indexOf(q) - b.toLowerCase().indexOf(q)) || a.length - b.length).slice(0, 50).map(f => ({ v: f, d: f.endsWith('/') ? t('pasta') : '' })); }
  else items = (await getCommands(root)).filter(c => c.name.startsWith(q)).map(c => ({ v: c.name, d: c.description }));
  if (!items.length) return hideAC();
  AC = { v, kind: m[2], items, i: 0 }; renderAC();
}
function renderAC() {
  const { v, kind, items, i } = AC;
  const m = menu($('.box', elOf(v)), items.map((it, k) => ({ label: kind + it.v, d: it.d, f: () => acPick(k) })), null, { above: true });
  m.style.minWidth = '360px'; const mi = $$('.mi', m); mi.forEach((x, k) => x.classList.toggle('k', k === i)); mi[i]?.scrollIntoView({ block: 'nearest' });
}
function hideAC() { if (AC) { AC = null; closeMenu(); } }
function acKey(e) { if (!AC) return false; if (e.key === 'ArrowDown') AC.i = (AC.i + 1) % AC.items.length; else if (e.key === 'ArrowUp') AC.i = (AC.i - 1 + AC.items.length) % AC.items.length; else if (e.key === 'Tab' || e.key === 'Enter') { acPick(AC.i); e.preventDefault(); return true; } else if (e.key === 'Escape') { hideAC(); e.preventDefault(); return true; } else return false; e.preventDefault(); renderAC(); return true; }
function acPick(k) {
  const { v, kind, items } = AC; const it = items[k]; const P = $('.prompt', elOf(v)); const pos = P.selectionStart;
  const before = P.value.slice(0, pos).replace(/([@/])[^\s]*$/, kind + it.v + ' ');
  P.value = before + P.value.slice(pos); P.selectionStart = P.selectionEnd = before.length; hideAC(); P.focus(); grow(P);
  if (kind === '@' && !it.v.endsWith('/')) { const root = vopts(v).root; const abs = root.replace(/[\\/]+$/, '') + '/' + it.v; if (!viewRT(v).atts.find(a => a.path === abs)) { viewRT(v).atts.push({ path: abs, name: it.v, size: 0 }); renderAtts(v); } }
}

let lastEsc = 0;
function composerKey(v, e) {
  if (vimKey(v, e)) return;
  if (acKey(e)) return;
  const P = $('.prompt', elOf(v)); const L = v.sid && S.live[v.sid];
  if ((e.key === 'Tab' || e.key === 'ArrowRight') && !e.shiftKey && !P.value && P.dataset.ghost) { e.preventDefault(); P.value = P.dataset.ghost; setGhost(v, ''); grow(P); return; }
  if (e.key === 'Enter' && !e.shiftKey && !e.altKey) { e.preventDefault(); send(v); return; }
  if (e.key === 'Tab' && e.shiftKey) { e.preventDefault(); const order = Object.keys(modeInfo); const cur = vopts(v).mode; setSessionOpts(v, { mode: order[(order.indexOf(cur) + 1) % order.length] }); return; }
  if (e.key === 'Escape') {
    if (busy(L)) { stop(v); return; }
    const now = Date.now();
    if (now - lastEsc < 500) { lastEsc = 0; if (P.value.trim()) { UI.history(P.value); P.value = ''; grow(P); } else if (L) rewindDialog(v); return; }
    lastEsc = now; return;
  }
  if (e.key === 'ArrowUp' && (P.selectionStart === 0 || !P.value)) { const hist = S.history; if (!hist.length) return; viewRT(v).hi = Math.min((viewRT(v).hi ?? -1) + 1, hist.length - 1); P.value = hist[hist.length - 1 - viewRT(v).hi]; grow(P); e.preventDefault(); return; }
  if (e.key === 'ArrowDown' && (viewRT(v).hi ?? -1) >= 0 && P.selectionStart === P.value.length) { viewRT(v).hi--; P.value = viewRT(v).hi < 0 ? '' : S.history[S.history.length - 1 - viewRT(v).hi]; grow(P); e.preventDefault(); return; }
  setTimeout(() => renderComposerSend(v), 0);
}
function renderComposerSend(v) { const L = v.sid && S.live[v.sid]; const b = $('.send', elOf(v)); const stopMode = busy(L) && !$('.prompt', elOf(v)).value.trim(); if (b.classList.contains('stop') !== stopMode) renderComposer(v); }

// vim (minimal): Esc → NORMAL; i a A I o; h j k l w b 0 $; x dd; Enter sends
function vimKey(v, e) {
  if (!S.settings.vim) return false; const P = $('.prompt', elOf(v));
  viewRT(v).vim = viewRT(v).vim || 'INSERT';
  if (viewRT(v).vim === 'INSERT') { if (e.key === 'Escape' && !AC && P.value) { viewRT(v).vim = 'NORMAL'; P.placeholder = '-- NORMAL --'; e.preventDefault(); return true; } return false; }
  if (e.ctrlKey || e.metaKey) return false;
  e.preventDefault(); const val = P.value, p = P.selectionStart; const set = n => { P.selectionStart = P.selectionEnd = Math.max(0, Math.min(val.length, n)); };
  const ls = val.lastIndexOf('\n', p - 1) + 1, le = val.indexOf('\n', p) === -1 ? val.length : val.indexOf('\n', p);
  switch (e.key) {
    case 'h': set(p - 1); break; case 'l': set(p + 1); break; case '0': set(ls); break; case '$': set(le); break;
    case 'w': { const m = val.slice(p).match(/^\S*\s+/); set(p + (m ? m[0].length : val.length)); break; }
    case 'b': { const m = val.slice(0, p).match(/\S+\s*$/); set(p - (m ? m[0].length : p)); break; }
    case 'j': case 'k': { const lines = val.split('\n'); let acc = 0, li = 0, col = p; for (; li < lines.length; li++) { if (col <= lines[li].length) break; col -= lines[li].length + 1; } const tl = li + (e.key === 'j' ? 1 : -1); if (tl >= 0 && tl < lines.length) { for (let i = 0; i < tl; i++) acc += lines[i].length + 1; set(acc + Math.min(col, lines[tl].length)); } break; }
    case 'i': viewRT(v).vim = 'INSERT'; break; case 'a': set(p + 1); viewRT(v).vim = 'INSERT'; break; case 'A': set(le); viewRT(v).vim = 'INSERT'; break; case 'I': set(ls); viewRT(v).vim = 'INSERT'; break;
    case 'o': P.value = val.slice(0, le) + '\n' + val.slice(le); set(le + 1); viewRT(v).vim = 'INSERT'; break;
    case 'x': P.value = val.slice(0, p) + val.slice(p + 1); set(p); break;
    case 'd': if (viewRT(v).vimD) { P.value = val.slice(0, ls) + val.slice(Math.min(val.length, le + 1)); set(ls); viewRT(v).vimD = false; } else { viewRT(v).vimD = true; return true; } break;
    case 'Enter': send(v); break;
  }
  viewRT(v).vimD = false; P.placeholder = viewRT(v).vim === 'NORMAL' ? '-- NORMAL --' : t('Peça algo… @ para arquivos, / para comandos'); grow(P); return true;
}
