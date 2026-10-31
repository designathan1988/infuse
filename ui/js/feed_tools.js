'use strict';
// Tool, report and agent blocks.

const toolLabel = { Bash: 'Executou', Read: 'Leu', Write: 'Escreveu', Edit: 'Editou', Glob: 'Procurou arquivos', Grep: 'Buscou', LS: 'Listou', WebFetch: 'Buscou na web', TodoWrite: 'Atualizou tarefas', AskUserQuestion: 'Perguntou', MultiEdit: 'Editou', WebSearch: 'Pesquisou na web', Browser: 'Navegador', Skill: 'Skill', Agent: 'Subagente', Memory: 'Memória', Contract: 'Critérios do pedido', Report: 'Relatório' };
function toolSummary(name, inp) {
  if (!inp) return ''; if (inp._partial !== undefined) return inp._partial.slice(-90);
  switch (name) { case 'Bash': return inp.command || ''; case 'Read': case 'Write': case 'Edit': case 'MultiEdit': return inp.path || inp.file_path || ''; case 'WebSearch': return inp.query || ''; case 'Browser': return [inp.action, inp.url || inp.selector || inp.key || (inp.width ? inp.width + 'x' + inp.height : '') || ''].filter(Boolean).join(' · '); case 'Skill': return inp.name || ''; case 'Agent': return `${inp.kind || ''} · ${inp.description || (inp.prompt || '').slice(0, 80)}`; case 'Memory': return `${inp.action || ''}${inp.text ? ' · ' + inp.text : ''}`; case 'Contract': return inp.goal || ''; case 'Glob': return inp.pattern || ''; case 'Grep': return `${inp.pattern || ''}${inp.path ? ' · ' + inp.path : ''}`; case 'LS': return inp.path || '.'; case 'WebFetch': return inp.url || ''; case 'TodoWrite': return `${(inp.todos || inp.tasks || []).length} ${t('itens')}`; case 'AskUserQuestion': return inp.question || ''; default: return JSON.stringify(inp).slice(0, 120); }
}
function lineCounts(name, inp) {
  if (name === 'Write' && inp?.content != null) return `<span class="cnt add">+${inp.content.split('\n').length}</span>`;
  if (name === 'Edit' && inp?.old_string != null) return `<span class="cnt"><span class="add">+${(inp.new_string || '').split('\n').length}</span> <span class="del">−${inp.old_string.split('\n').length}</span></span>`;
  return '';
}
// relPath shows a path relative to the session folder (absolute paths are long and repeat the project).
function relPath(p) {
  const root = (curDir() || '').replace(/[\\/]+$/, ''); if (!root || !p) return p;
  const norm = s => s.replace(/\\/g, '/').toLowerCase(); const np = norm(p), nr = norm(root) + '/';
  return np.startsWith(nr) ? p.replace(/\\/g, '/').slice(nr.length) : p;
}
// Reasoning is part of the transcript: shown open by default (Configurações or Ctrl+O hide it), with a caret.
function thinkBlock(key, text, live, ms) {
  const open = S.fold[key] ?? (S.viewMode !== 'normal'); // Pensamento and Detalhado open the reasoning; Normal keeps it on one line
  const words = text.trim().split(/\s+/).length;
  return `<div class="tool think2${open ? ' open' : ''}"><div class="h" data-fold="${esc(key)}"><span class="chev">›</span><b>${live ? t('Pensando…') : ms ? `${t('Pensou por')} ${fmtDur(ms)}` : t('Raciocínio')}</b><span class="s faint">${words} ${t('palavras')}</span><button class="ib mini" data-copy-think title="${esc(t('Copiar raciocínio'))}">${I.copy}</button></div><div class="b"><pre class="thinktext">${esc(text)}</pre></div></div>`;
}
// The agent's report, with infuse's check of each claim: the data infuse attached to the Report call
// (status code and evidence of each claim, in order, and the sources), never the text of its result.
const CLAIM = { confirmado: ['✓', 'ok', 'confirmado pela execução'], parcial: ['◐', 'warn', 'parcial'], 'nao-confirmado': ['✗', 'bad', 'não confirmado'], 'sem-evidencia': ['?', '', 'sem evidência'] };
const SRC = { search: 'pesquisa', doc: 'documentação fixada' };
function reportCard(inp, result, chk) {
  const checked = chk?.claims || [];
  const rows = (inp.claims || []).map((c, k) => { const v = checked[k]; const [ic, cls, lab] = CLAIM[v?.status] || ['…', '', 'conferindo']; return `<div class="rrow"><span class="rst ${cls}" title="${esc(t(lab))}">${ic}</span><span class="grow"><span>${esc(c.text)}</span>${c.ref ? ` <code>${esc(c.ref)}</code>` : ''}${v?.display ? `<span class="faint rev">${esc(v.display)}</span>` : ''}</span></div>`; }).join('');
  const bad = chk?.unconfirmed || 0;
  const srcs = chk?.sources || [];
  const srcHTML = srcs.length ? `<details class="odet"><summary>${t('Fontes consultadas')} (${srcs.length})</summary>${srcs.map(x => x.kind === 'page' ? `<div class="mono" style="font-size:11.5px"><a href="#" data-url="${esc(x.ref)}">${esc(x.ref)}</a></div>` : `<div class="faint" style="font-size:11.5px">${esc(t(SRC[x.kind] || x.kind))}: ${esc(x.ref)}</div>`).join('')}</details>` : '';
  return `<div class="report"><div class="rh"><b>${t('Relatório')}</b><span class="faint">${t(chk ? 'conferido pelo infuse com o que de fato rodou' : 'conferindo…')}</span>${bad ? `<span class="tag bad">${bad} ${t('não confirmada(s)')}</span>` : ''}</div>${inp.summary ? `<div class="md sel">${md(inp.summary)}</div>` : ''}${rows}${srcHTML}${inp.next ? `<div class="rnext"><span class="faint">${t('Próximo passo')}</span><span class="grow">${esc(inp.next)}</span><button class="btn sm" data-next="${esc(inp.next)}">${t('Usar')}</button></div>` : ''}</div>`;
}
// A subagent in the conversation, as Claude Code shows it: its steps appear one by one while it works
// (what it says, each tool with what it touched, running/ok/error and the time), each step opens to its
// output, and its conclusion closes the block.
function findAgentRun(inp) {
  const title = inp.description || '', head = String(inp.prompt || '').slice(0, 200);
  let best = null;
  for (const L of Object.values(S.live)) for (const a of (L.snap?.agents || [])) {
    if (a.kind !== (inp.kind || 'general') && inp.kind) continue;
    if (head && (a.task || '').includes(head)) best = a; else if (!best && title && a.title === title) best = a;
  }
  return best;
}
function agentTrailHTML(a, bid) {
  if (!a) return `<div class="astep faint"><span class="spin sm"></span>${t('iniciando o subagente…')}</div>`;
  const steps = (a.trail || []).map((st, i) => {
    if (st.kind === 'text') return `<div class="astep txt md sel">${md(st.text || '')}</div>`;
    let inp = {}; try { inp = JSON.parse(st.input || '{}'); } catch { }
    const k = `${bid}:s${i}`, open = S.fold[k] ?? (S.viewMode === 'verbose');
    return `<div class="tool astep${open ? ' open' : ''}"><div class="h" data-fold="${esc(k)}"><span class="chev">›</span><span class="dot ${st.state === 'running' ? 'run' : st.state === 'error' ? 'err' : ''}"></span><b>${esc(t(toolLabel[st.tool] || st.tool))}</b><span class="s">${esc(toolSummary(st.tool, inp))}</span>${st.state !== 'running' && st.ms != null ? `<span class="tdur faint">${fmtDur(st.ms)}</span>` : ''}</div><div class="b">${st.output ? `<pre class="res">${esc(st.display || st.output)}</pre>` : `<div class="faint">${t('sem saída')}</div>`}</div></div>`;
  }).join('');
  const tail = a.state === 'running' ? `<div class="astep faint"><span class="spin sm"></span>${t('trabalhando…')}</div>`
    : a.result ? `<div class="astep concl"><div class="lab">${t('Conclusão')}</div><div class="md sel">${md(a.result)}</div></div>` : '';
  return steps + tail;
}
function agentHead(a) {
  if (!a) return '';
  return `${a.steps} ${t('passos')} · ${a.state === 'running' ? elapsed(a.started) : fmtDur((a.ended || Date.now()) - a.started)}`;
}
function agentBlock(id, inp, result) {
  const a = findAgentRun(inp), running = a ? a.state === 'running' : !result;
  const open = S.fold[id] ?? true; // the steps stay in view, while it works and after; the caret folds them
  const dot = running ? 'run' : result?.is_error || a?.state === 'failed' ? 'err' : '';
  return `<div class="tool agentblk${open ? ' open' : ''}" data-agentblk="${esc(id)}"><div class="h" data-fold="${esc(id)}"><span class="chev">›</span><span class="dot ${dot}"></span><b>${esc(t('Subagente'))}</b><span class="tag">${esc(t(inp.kind || 'general'))}</span><span class="s">${esc(inp.description || String(inp.prompt || '').slice(0, 80))}</span><span class="tdur faint asteps">${agentHead(a)}</span></div>
    <div class="b"><details class="atask"${S.viewMode === 'verbose' ? ' open' : ''}><summary>${t('Tarefa dada ao subagente')}</summary><div class="md sel">${md(String(inp.prompt || ''))}</div></details><div class="atrail">${agentTrailHTML(a, id)}</div></div></div>`;
}
// a step of a subagent changed: only its blocks are redrawn, the conversation stays as it is
function refreshAgentBlocks(v) {
  if (!v.el) return;
  $$('[data-agentblk]', v.el).forEach(blk => {
    const L = v.sid && S.live[v.sid]; if (!L) return;
    const id = blk.dataset.agentblk;
    const tu = (L.snap.messages || []).flatMap(m => m.content || []).find(b => b.type === 'tool_use' && b.id === id);
    if (!tu) return;
    const a = findAgentRun(tu.input || {}); if (!a) return;
    const tr = $('.atrail', blk), hd = $('.asteps', blk);
    const shown = blk.classList.contains('open');
    if (tr) { tr.innerHTML = agentTrailHTML(a, id); bindFolds(tr); }
    if (hd) hd.textContent = agentHead(a);
    const dot = $('.h .dot', blk); if (dot) dot.className = 'dot ' + (a.state === 'running' ? 'run' : a.state === 'failed' ? 'err' : '');
    if (shown) stickToEnd(v);
  });
}
function bindFolds(root) { $$('[data-fold]', root).forEach(x => x.onclick = e => { if (e.target.closest('[data-path]')) return; const box = x.parentNode; const open = !box.classList.contains('open'); box.classList.toggle('open', open); S.fold[x.dataset.fold] = open; }); }
function stickToEnd(v) { if (v.stick !== false) toBottom(v); }
function toolBlock(id, name, input, result, running, meta) {
  let inp = input; if (typeof inp === 'string') { try { inp = JSON.parse(inp); } catch { inp = {}; } }
  if (name === 'Report' && inp && Array.isArray(inp.claims)) return reportCard(inp, result, meta);
  if (inp?._streaming && ['Write', 'Edit', 'MultiEdit', 'Bash'].includes(name)) { // being written right now
    const p = inp.path || inp.file_path || '';
    let body = '', what = '';
    if (name === 'Write') { body = liveCodeHTML(inp.content); what = `${String(inp.content || '').split('\n').length} ${t('linhas')}`; }
    else if (name === 'Edit') { body = (inp.old_string != null ? liveCodeHTML(inp.old_string, 'old') : '') + (inp.new_string != null ? liveCodeHTML(inp.new_string, 'new') : ''); what = t('editando'); }
    else if (name === 'MultiEdit') { body = liveCodeHTML(inp._open === 'old_string' ? inp.old_string : inp.new_string, inp._open === 'old_string' ? 'old' : 'new'); what = t('editando'); }
    else { body = liveCodeHTML(inp.command, 'cmd'); what = t('montando o comando'); }
    const open = S.fold[id] ?? (S.viewMode === 'verbose'); // the same default as the finished block: no open-then-close
    return `<div class="tool live${open ? ' open' : ''}"><div class="h" data-fold="${esc(id)}"><span class="chev">›</span><span class="dot run"></span><b>${esc(t({ Write: 'Escrevendo', Edit: 'Editando', MultiEdit: 'Editando', Bash: 'Preparando comando' }[name]))}</b><span class="s mono">${esc(p ? relPath(p) : '')}</span><span class="tdur faint">${esc(what)}</span></div><div class="b">${body}</div></div>`;
  }
  if (name === 'Agent' && inp && !inp._streaming) return agentBlock(id, inp, result);
  const shot = name === 'Browser' && result && String(result.content || '').match(/\.infuse\/shots\/([\w.-]+\.(?:png|jpe?g))/); // the agent's screenshot, shown in place
  const isPath = ['Read', 'Write', 'Edit', 'MultiEdit'].includes(name) && (inp?.path || inp?.file_path);
  let body = '';
  if (name === 'Bash' && inp?.command) body += `<div class="lab">${t('comando')}<button class="ib mini" data-copy-tool="in" title="${esc(t('Copiar comando'))}">${I.copy}</button></div><pre class="inp">${esc(inp.command)}</pre>`;
  else if (name === 'Write' && inp?.content != null) body += diffHTML(inp.content.split('\n').map(l => '+' + l).join('\n'));
  else if (name === 'MultiEdit' && Array.isArray(inp?.edits)) body += inp.edits.map(e => diffHTML((e.old_string || '').split('\n').map(l => '-' + l).join('\n') + '\n' + (e.new_string || '').split('\n').map(l => '+' + l).join('\n'))).join('');
  else if (name === 'Edit' && inp?.old_string != null) body += diffHTML(inp.old_string.split('\n').map(l => '-' + l).join('\n') + '\n' + (inp.new_string || '').split('\n').map(l => '+' + l).join('\n'));
  else if (inp && Object.keys(inp).length && name !== 'TodoWrite') body += `<div class="lab">${t('entrada')}</div><pre>${esc(JSON.stringify(inp, null, 2))}</pre>`;
  if (name === 'TodoWrite' && Array.isArray(inp?.todos || inp?.tasks)) body += (inp.todos || inp.tasks).map(x => `<div>${x.status === 'done' ? '☑' : x.status === 'in_progress' ? '◐' : '☐'} ${esc(x.text || x.content || '')}</div>`).join('');
  if (result) { const main = String(result.display || result.content || ''), inf = ''; /* the whole result: infuse's notes are already in the user's language */ body += `<div class="lab">${t('resultado')}<button class="ib mini" data-copy-tool="out" title="${esc(t('Copiar resultado'))}">${I.copy}</button></div><pre class="res">${esc(main.length > 20000 ? main.slice(0, 20000) + '\n…' : main)}</pre>` + (inf ? `<div class="lab">${t('infundido')}</div><pre class="inf">${esc(inf)}</pre>` : ''); }
  const dot = running ? 'run' : result ? (result.is_error ? 'err' : '') : 'pend';
  const open = S.fold[id] ?? (S.viewMode === 'verbose'); // an error does not open the block by itself: its first line shows in the header
  const errLine = result?.is_error ? String(result.display || result.content || '').split('\n').find(l => l.trim()) || '' : '';
  const fullPath = isPath ? (inp.path || inp.file_path) : '';
  const shown = isPath ? relPath(fullPath) : '';
  const summ = isPath ? `<span class="s"><span class="path" data-path="${esc(shown)}" title="${esc(fullPath)}">${esc(shown)}</span></span>` : `<span class="s">${esc(toolSummary(name, inp))}</span>`;
  const mcp = name.startsWith('mcp__') ? name.slice(5).split('__') : null; // connector tools: "Conector server · tool"
  if (shot) body = `<img class="shot" src="/api/shot?name=${encodeURIComponent(shot[1])}&root=${encodeURIComponent(curRoot())}" alt="">` + body;
  return `<div class="tool${open || shot ? ' open' : ''}"><div class="h" data-fold="${esc(id)}"><span class="chev">›</span><span class="dot ${dot}"></span><b>${mcp ? `${esc(t('Conector'))} ${esc(mcp[0])}` : esc(t(toolLabel[name] || name))}</b>${mcp ? `<span class="s mono">${esc(mcp[1] || '')}</span>` : ''}${summ}${errLine ? `<span class="terr" title="${esc(errLine)}">${esc(errLine.replace(/^\[infuse\]\s*/, '').slice(0, 120))}</span>` : ''}${lineCounts(name, inp)}${result?.ms != null ? `<span class="tdur faint" title="${esc(t('tempo de execução'))}">${fmtDur(result.ms)}</span>` : ''}</div><div class="b">${body}</div></div>`;
}
const diffHTML = text => `<div class="diff">${text.split('\n').map(l => `<div class="${l.startsWith('+') && !l.startsWith('+++') ? 'a1' : l.startsWith('-') && !l.startsWith('---') ? 'd1' : /^(@@|---|\+\+\+|diff )/.test(l) ? 'hh' : ''}">${esc(l) || ' '}</div>`).join('')}</div>`;
