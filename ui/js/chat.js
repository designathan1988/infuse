'use strict';
// chat
// Transcript rendering: messages, turn summaries, rail cards, live stream, tools, agents, markdown.

// ---------------------------------------------------------------- transcript
function renderChat(v) {
  if (!v.el) return;
  const chat = $('.chat', v.el); const L = v.sid && S.live[v.sid];
  if (!L?.snap) { chat.innerHTML = ''; return; }
  const s = L.snap, msgs = s.messages, results = {};
  for (const m of msgs) for (const b of m.content || []) if (b.type === 'tool_result') results[b.tool_use_id] = b;
  const starts = new Set(s.turnStarts); let turn = 0; let out = ''; const parts = []; const cut = key => { if (out) parts.push([key, out]); out = ''; };
  msgs.forEach((m, i) => {
    if (starts.has(i)) { turn++; cut('pre:' + i); out += `<div class="turnsep" data-turn="${turn}"><span class="l"></span><button class="btn sm" data-rw="${turn}" title="${esc(t('Restaurar código e/ou conversa a antes deste turno'))}">↶ ${t('voltar aqui')}</button></div>`; }
    if (m.role === 'user') {
      if (m.content.some(b => b.type === 'tool_result')) return; // tool results (and images tools attached) are shown in their tool blocks
      const texts = m.content.filter(b => b.type === 'text'), imgs = m.content.filter(b => b.type === 'image');
      if (!texts.length && !imgs.length) return;
      // what an infuse message is comes from its kind and its data, never from its words
      const main = texts[0]?.display || texts[0]?.text || '';
      if (m.kind === 'review') { out += reviewBlock(texts.slice(1).map(b => b.text).join('\n\n').trim(), i); return; }
      if (m.kind === 'compaction') { out += `<div class="marker" title="${esc(main.slice(0, 600))}">${t('Conversa resumida')}</div>`; return; }
      const attached = texts.slice(1).map(b => (b.text.match(/path="([^"]+)"/) || [])[1]).filter(Boolean);
      const mission = missionOf(m);
      const body = mission ? `<span class="tag warn" style="margin-right:6px">${esc(missionTag(mission))}</span>${esc(mission.turn ? main : mission.goal)}` : esc(main);
      out += `<div class="umsg${(s.pinnedMsgs || []).includes(i) ? ' pinned' : ''}" data-mi="${i}"><div class="u sel">${body}${attached.length ? `<div class="att">${attached.map(p => `<span class="chipx" data-path="${esc(p)}">📎 <span>${esc(baseName(p))}</span></span>`).join('')}</div>` : ''}${imgs.map(b => `<img src="data:${b.mediaType};base64,${b.data}">`).join('')}</div>${msgBar(s, m, i, true)}</div>`;
    } else {
      if (m.kind === 'review_ack' || m.kind === 'compaction') return;
      let inner = '';
      for (const [bi, b] of m.content.entries()) {
        if (b.type === 'text' && b.text?.trim()) inner += `<div class="md sel">${md(b.text.trim())}</div>`;
        else if (b.type === 'thinking' && b.thinking?.trim()) inner += thinkBlock(`${v.sid}:${i}:${bi}`, b.thinking, false, b.ms);
        else if (b.type === 'tool_use') inner += toolBlock(b.id, b.name, b.input, results[b.id], false, b.meta);
      }
      const toolsOnly = !m.content.some(b => b.type === 'text' && b.text?.trim()); // calls with no words: they continue the group above
      if (inner) out += `<div class="a${toolsOnly ? ' toolsonly' : ''}${(s.pinnedMsgs || []).includes(i) ? ' pinned' : ''}" data-mi="${i}">${inner}${m.content.some(b => b.type === 'text' && b.text?.trim()) ? msgFoot(m, i) : ''}</div>`; // a message with nothing visible leaves no gap
    }
    cut('m:' + i);
    const next = i + 1;
    if (turn && (starts.has(next) || (next === msgs.length && !['thinking', 'tool', 'waiting_permission', 'waiting_user', 'compacting'].includes(s.state)))) { out += turnSummary(s, msgs, s.turnStarts[turn - 1], i); cut('sum:' + i); }
  });
  cut('tail');
  if (!msgs.length) parts.unshift(['empty', `<div class="empty" style="padding-top:14vh">${t('Sessão vazia. Descreva a feature, a correção ou a dúvida.')}</div>`]);
  parts.push(['live', '<div class="live"></div>']); // kept as it is: renderLive patches its inside
  if (s.pendingPermission) parts.push(['perm', permCard(s.pendingPermission)]);
  if (s.pendingQuestion) parts.push(['question', questionCard(s.pendingQuestion)]);
  if (s.error && s.state === 'error') parts.push(['error', `<div class="errbox">${esc(s.error)}</div>`]);
  const sc = $('.scroller', v.el); if (v._jump !== v.sid) v.stick = true;
  if (chat.dataset.sid !== v.sid) { chat.innerHTML = ''; chat.dataset.sid = v.sid; } // another session: start clean
  patchParts(v, chat, parts); renderLive(v);
  if (v.stick !== false) toBottom(v); v._jump = v.sid; renderToc(v); updateJump(v);
}
const fmtClock = ts => new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
function fmtDur(ms) { if (ms == null) return ''; if (ms < 1000) return ms + ' ms'; const s = ms / 1000; if (s < 60) return (s < 10 ? s.toFixed(1) : Math.round(s)) + ' s'; const m = Math.floor(s / 60); return `${m}m ${Math.round(s % 60)}s`; }
const fmtTok = n => n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k' : String(n || 0);
// Under each answer: when it arrived, how long the model took, which model, how many tokens; copy on hover.
// Date and time of a message: "hoje, 14:32", "ontem, 09:10", "31 out, 18:02"; the full date and how long ago on hover.
function fmtWhen(ts) {
  const d = new Date(ts), now = new Date(), day = 86400000;
  const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const hm = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  if (ts >= midnight) return t('hoje') + ', ' + hm;
  if (ts >= midnight - day) return t('ontem') + ', ' + hm;
  return d.toLocaleDateString([], { day: '2-digit', month: 'short', ...(d.getFullYear() !== now.getFullYear() ? { year: 'numeric' } : {}) }) + ', ' + hm;
}
function agoText(ts) { const s = Math.round((Date.now() - ts) / 1000); if (s < 60) return t('agora há pouco'); const m = Math.round(s / 60); if (m < 60) return `${t('há')} ${m} min`; const h = Math.round(m / 60); if (h < 24) return `${t('há')} ${h} h`; return `${t('há')} ${Math.round(h / 24)} ${t('dias')}`; }
// The actions under a message, as in Claude Code: copy, (edit,) branch from here, pin, read aloud — and when.
function msgBar(s, m, i, user) {
  const pinned = (s.pinnedMsgs || []).includes(i);
  const when = m.ts ? `<span class="when" title="${esc(new Date(m.ts).toLocaleString() + ' · ' + agoText(m.ts))}">${fmtWhen(m.ts)}</span>` : '';
  return `<div class="mbar${user ? ' u' : ''}"><span class="acts"><button class="ib" data-copy-msg="${i}" title="${esc(t('Copiar'))}">${I.copy}</button>${user && turnOf(s, i) ? `<button class="ib" data-rw="${turnOf(s, i)}" title="${esc(t('Editar e refazer a partir daqui'))}">↶</button>` : ''}<button class="ib" data-fork="${i}" title="${esc(t('Nova sessão a partir daqui'))}">${I.branch}</button><button class="ib${pinned ? ' on' : ''}" data-pinmsg="${i}" title="${esc(t(pinned ? 'Desafixar' : 'Fixar mensagem'))}">${I.pin}</button><button class="ib" data-speak="${i}" title="${esc(t('Ler em voz alta'))}">${I.speak}</button></span>${when}</div>`;
}
function msgFoot(m, i) {
  const meta = [m.ts && `<span title="${esc(new Date(m.ts).toLocaleString() + ' · ' + agoText(m.ts))}">${fmtWhen(m.ts)}</span>`, m.ms && `<span title="${esc(t('tempo de resposta do modelo'))}">${fmtDur(m.ms)}</span>`, m.model && `<span>${esc(m.model)}</span>`, m.tokens && `<span>${fmtTok(m.tokens)} tokens</span>`].filter(Boolean).join('<span class="dotsep">·</span>');
  const s = curL()?.snap || {}; const pinned = (s.pinnedMsgs || []).includes(i);
  return `<div class="afoot"><span class="meta">${meta}</span><span class="acts"><button class="ib" data-copy-msg="${i}" title="${esc(t('Copiar resposta'))}">${I.copy}</button><button class="ib" data-fork="${i}" title="${esc(t('Nova sessão a partir daqui'))}">${I.branch}</button><button class="ib${pinned ? ' on' : ''}" data-pinmsg="${i}" title="${esc(t(pinned ? 'Desafixar' : 'Fixar mensagem'))}">${I.pin}</button><button class="ib" data-speak="${i}" title="${esc(t('Ler em voz alta'))}">${I.speak}</button></span></div>`;
}
// End of a turn: how long the agent worked, tools used, tokens; copy the turn's answers.
function turnSummary(s, msgs, from, to) {
  if (from == null) return '';
  const first = msgs[from], last = msgs[to]; let tools = 0, tokens = 0, texts = 0;
  for (let k = from; k <= to; k++) { const m = msgs[k]; if (m.role !== 'assistant') continue; tokens += m.tokens || 0; for (const b of m.content || []) { if (b.type === 'tool_use') tools++; if (b.type === 'text' && b.text?.trim()) texts++; } }
  if (!texts && !tools) return '';
  const dur = first?.ts && last?.ts ? last.ts - first.ts : null;
  const bits = [dur != null && `${t('Trabalhou por')} ${fmtDur(dur)}`, tools && `${tools} ${t(tools === 1 ? 'ferramenta' : 'ferramentas')}`, tokens && `${fmtTok(tokens)} tokens`].filter(Boolean).join('<span class="dotsep">·</span>');
  return `<div class="turnsum"><span class="ok">✓</span><span class="meta">${bits}</span><span class="grow"></span>${texts ? `<button class="btn sm ghost" data-copy-turn="${from}:${to}" title="${esc(t('Copiar todas as respostas deste turno'))}">${I.copy} ${t('Copiar')}</button>` : ''}</div>`;
}
// A mission's message (kind "mission") carries its data on the text block: the goal, and for the turns
// infuse starts after a verdict, the turn, the limits and the verdict.
function missionOf(m) { if (m?.kind !== 'mission') return null; const b = (m.content || []).find(x => x.type === 'text' && x.meta); return b ? b.meta : { goal: '' }; }
function missionTag(n) { return n.turn ? `${t('Missão')} · ${t('turno')} ${n.turn}/${n.maxTurns}` : t('Missão'); }
// Independent review: a reviewer with a clean context reads the diff, the plan and the criteria.
function reviewBlock(text, i) {
  return `<div class="report review"><div class="rh"><b>${t('Revisão independente')}</b><span class="faint">${t('um revisor com contexto limpo leu o diff, o plano e os critérios')}</span><span class="grow"></span><button class="ib" data-copy-msg="${i}" title="${esc(t('Copiar'))}">${I.copy}</button></div><div class="md sel">${md(text)}</div><div class="rnext"><span class="grow faint">${t('O agente vê estes achados na próxima mensagem.')}</span><button class="btn sm pri" data-fixreview>${t('Pedir correções')}</button></div></div>`;
}
// What runs now: background processes and subagents of the session, each opening its pane.
const elapsed = (a, b) => { const s = Math.max(0, Math.round(((b || Date.now()) - a) / 1000)); return s < 60 ? s + 's' : s < 3600 ? Math.floor(s / 60) + 'min ' + (s % 60) + 's' : Math.floor(s / 3600) + 'h ' + Math.floor(s % 3600 / 60) + 'min'; };
function runningCard(L) {
  const ps = L.snap.procs || [], ag = L.snap.agents || [];
  const live = ps.filter(p => p.state === 'running'), working = ag.filter(a => a.state === 'running');
  if (!ps.length && !working.length) return '';
  return `<div class="card running" data-card="running"><div class="ch">${t('Em execução')}<span class="r">${live.length + working.length}</span></div>
    ${live.map(p => `<div class="li" data-pane="procs"><span class="dot run"></span><span class="n mono" title="${esc(p.command)}">${esc(p.description || p.command)}</span><span class="faint">${elapsed(p.started)}</span></div>`).join('')}
    ${working.map(a => `<div class="li" data-pane="agents"><span class="spin sm"></span><span class="n" title="${esc(a.task)}">${esc(a.title)}</span><span class="faint">${a.steps} ${t('passos')}</span></div>`).join('')}
    ${ps.length > live.length ? `<div class="li faint" data-pane="procs">${ps.length - live.length} ${t('processo(s) encerrado(s)')}</div>` : ''}</div>`;
}

// What the user forbade in this session, in the user's own words; each one is enforced by infuse until
// the user lifts it here.
function bansCard(L) {
  if (L.bans === undefined) { L.bans = []; api(`/api/session/${L.id}/bans`, { root: L.root }).then(r => { L.bans = r.bans || []; viewsOf(L).forEach(renderRail); }); }
  const bs = L.bans || []; if (!bs.length) return '';
  const on = bs.filter(b => !b.off).length;
  return `<div class="card bans" data-card="bans"><div class="ch">${t('Proibições')}<span class="r">${on}</span></div>
    ${bs.map((b, i) => `<div class="ban${b.off ? ' off' : ''}"><q>${esc(b.quote)}</q><span class="faint">${esc(t({ tool: 'ferramenta', command: 'comando', path: 'caminho', action: 'ação' }[b.kind] || b.kind))}: <code>${esc(b.value)}</code> · ${t('turno')} ${b.turn}</span><button class="btn sm line" data-ban="${i}" data-off="${b.off ? '0' : '1'}">${t(b.off ? 'Restaurar' : 'Retirar')}</button></div>`).join('')}</div>`;
}

// The diagnosis the agent had to register before acting: the cause, the approach and the evidence,
// each quote checked by infuse against a file the agent read to the end.
function diagnosisCard(L) {
  const ds = L.snap.diagnoses || []; if (!ds.length) return '';
  const d = ds[ds.length - 1], turn = L.snap.turn;
  const tag = d.turn !== turn ? `<span class="tag">${t('turno')} ${d.turn}</span>` : d.continuing ? `<span class="tag">${t('continuação')}</span>` : d.redo_text ? `<span class="tag warn" title="${esc(d.redo_text)}">${t('refeito')}</span>` : `<span class="tag ok">${t('conferido')}</span>`;
  const row = (k, v) => `<div class="dg"><div class="dk">${t(k)}</div><div class="dv md sel">${md(v || '')}</div></div>`;
  return `<div class="card diag" data-card="diagnosis"><div class="ch">${t('Diagnóstico')}<span class="r">${tag}</span></div>
    ${row('Problema', d.problem)}${row('Causa raiz', d.root_cause)}${row('Abordagem', d.approach)}${row('Por que trata a causa', d.why_cause)}
    <div class="dk">${t('Evidências')} <span class="faint">(${(d.evidence || []).length}, ${t('conferidas no arquivo')})</span></div>
    ${(d.evidence || []).map(e => `<div class="dev"><span class="path" data-path="${esc(e.file)}">${esc(e.file)}</span><q>${esc(e.quote)}</q><span class="faint">${esc(e.supports || '')}</span></div>`).join('')}
    ${ds.length > 1 ? `<div class="faint dhist">${ds.length - 1} ${t('diagnóstico(s) anterior(es) nesta sessão')}</div>` : ''}</div>`;
}

// The plan, live: each task with its state (done, now, next, blocked) and how it will be proven.
const PLAN_ICON = { done: ['✓', 'ok'], in_progress: ['', 'now'], blocked: ['✗', 'bad'], pending: ['', ''], stopped: ['‖', 'stop'] };
function planCard(L) {
  const tasks = L.snap.tasks || []; if (!tasks.length) return '';
  const done = tasks.filter(x => x.status === 'done').length, pct = Math.round(done / tasks.length * 100);
  const prev = L._planPrev || {}; L._planPrev = Object.fromEntries(tasks.map(x => [x.id, x.status]));
  // only a running agent has a task "now"; once it stops, an unfinished task shows as stopped
  const live = busy(L), open = tasks.filter(x => x.status !== 'done' && x.status !== 'blocked').length;
  const head = !live && open ? `<span class="r warn">${t('parado')} · ${done} ${t('de')} ${tasks.length}</span>` : `<span class="r">${done} ${t('de')} ${tasks.length}</span>`;
  return `<div class="card plan${live ? '' : ' idle'}" data-card="plan"><div class="ch">${t('Plano')}${head}</div><div class="bar100 pbar"><i style="width:${pct}%"></i></div>
    ${tasks.map(x => { const st = x.status === 'in_progress' && !live ? 'stopped' : x.status; const [ic, cls] = PLAN_ICON[st] || PLAN_ICON.pending; const just = x.status === 'done' && prev[x.id] && prev[x.id] !== 'done'; return `<div class="ptask ${st}${just ? ' just' : ''}"><span class="rst ${cls}">${ic}</span><span class="grow"><span class="tt">${esc(x.text)}</span>${x.proof ? `<span class="pf">${t('comprovação:')} <code>${esc(x.proof)}</code></span>` : ''}${x.note ? `<span class="pn">${esc(t(x.note))}</span>` : ''}</span></div>`; }).join('')}</div>`;
}
// Every card of the right column folds with its caret; the choice is remembered.
function railFolds(v, el) {
  const folds = lsGet('railFolds', {});
  $$('.card[data-card]', el).forEach(card => {
    const key = card.dataset.card, ch = $('.ch', card); if (!ch || $('.fold', ch)) return;
    ch.insertAdjacentHTML('afterbegin', `<span class="fold" title="${esc(t('Recolher ou expandir'))}">›</span>`);
    card.classList.toggle('folded', !!folds[key]);
    ch.addEventListener('click', e => { if (e.target.closest('button, a, .tag')) return; e.stopPropagation(); folds[key] = !folds[key]; lsSet('railFolds', folds); card.classList.toggle('folded', !!folds[key]); });
  });
}
// Missions: the agent keeps working until the Done? criteria pass, within a budget.
const MSTATE = { running: ['warn', 'em andamento'], aprovada: ['ok', 'aprovada'], orcamento: ['bad', 'orçamento esgotado'], impasse: ['bad', 'impasse'], parada: ['', 'parada'] };
function missionCard(L) {
  const m = L.snap.mission;
  if (!m) return `<div class="card"><div class="ch">${t('Missão')}</div><div class="faint" style="margin-bottom:6px">${t('O agente trabalha sozinho, turno após turno, até os critérios do Pronto? passarem — dentro de um orçamento.')}</div><button class="btn sm line mnew">${t('Iniciar missão')}</button></div>`;
  const spent = Math.max(0, (L.snap.cost || 0) - (m.startCost || 0)), turns = (m.steps || []).length + (m.state === 'running' ? 1 : 0);
  const [cls, label] = MSTATE[m.state] || ['', m.state]; const pct = Math.min(100, Math.round(spent / (m.budgetUSD || 1) * 100));
  const last = (m.steps || []).slice(-1)[0];
  return `<div class="card mission" data-card="mission"><div class="ch">${t('Missão')}<span class="r"><span class="tag ${cls}">${t(label)}</span></span></div>
    <div class="faint trunc" title="${esc(m.goal)}">${esc(m.goal)}</div>
    <div class="bar100" style="margin:6px 0 3px"><i style="width:${pct}%"></i></div><div class="faint">${t('turno')} ${Math.min(turns, m.maxTurns)}/${m.maxTurns} · ${money(spent)} ${t('de')} ${money(m.budgetUSD)}</div>
    ${m.noPrice && m.state === 'running' ? `<div class="faint" style="margin-top:4px">${t('Modelo sem preço na tabela: o orçamento em dólares não é medido, vale o limite de turnos (Configurações › Modelos e custos).')}</div>` : ''}
    ${m.reason ? `<div style="margin-top:6px">${esc(m.reason)}</div>` : ''}
    ${m.impasse ? `<div class="mimp"><b>${t('Impasse')}:</b> ${esc(m.impasse.reason)}${(m.impasse.tried || []).length ? `<div>${t('Tentou')}: ${m.impasse.tried.map(esc).join('; ')}</div>` : ''}${m.impasse.evidence ? `<div class="mono">${esc(m.impasse.evidence)}</div>` : ''}<div><b>${t('Precisa de')}:</b> ${esc(m.impasse.needs)}</div></div>` : ''}
    ${last?.auditor ? `<div class="maud" title="${esc(t('Auditor de fluxo: conselho do modelo rápido, não um veredito'))}">${esc(last.auditor)}</div>` : ''}
    ${(m.steps || []).length ? `<details class="odet"><summary>${t('Linha do tempo')} (${m.steps.length})</summary>${m.steps.map(st => `<div class="mstep"><span class="tag ${{ aprovado: 'ok', reprovado: 'bad' }[st.verdict] || 'warn'}">${esc(t(st.verdict || '—'))}</span><span class="faint">${t('turno')} ${st.turn} · ${new Date(st.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} · ${money(st.cost)}</span><div class="mono faint">${esc(st.summary || '')}</div>${st.auditor ? `<div class="maud">${esc(st.auditor)}</div>` : ''}</div>`).join('')}</details>` : ''}
    <div class="row" style="margin-top:6px">${m.state === 'running' ? `<button class="btn sm mstop">${t('Parar missão')}</button>` : `<button class="btn sm line mnew">${t('Nova missão')}</button>`}</div></div>`;
}
function missionDialog(v) {
  const L = v.sid && S.live[v.sid]; const P = $('.prompt', v.el);
  const d = h(`<div class="dlg" style="width:min(560px,92vw)"><h3 style="margin:0 0 4px">${t('Iniciar missão')}</h3><div class="faint" style="margin-bottom:10px">${t('O agente trabalha sozinho até os critérios do Pronto? passarem. Depois de cada turno o infuse roda os critérios numa cópia do projeto; se falharem, o agente recebe os motivos e continua. Ele pode declarar impasse quando não houver caminho.')}</div>
    <textarea class="doc mg" style="min-height:110px" placeholder="${esc(t('O objetivo. Ex.: a página de login autentica com e-mail e senha e mostra erro claro quando a senha está errada.'))}">${esc(P?.value || '')}</textarea>
    <div class="cs" style="padding:8px 0 0">${t('Gastar no máximo')} US$ <input class="slot mono mb" value="2.00"> ${t('e')} <input class="slot mono mt" value="10"> ${t('turnos')}.</div>
    <div class="mcrit faint" style="margin-top:6px"></div>
    <div class="row" style="margin-top:12px;justify-content:flex-end"><button class="btn cancel">${t('Cancelar')}</button><button class="btn pri go">${t('Iniciar')}</button></div></div>`);
  const close = overlay(d);
  api('/api/oracle/spec', { root: L?.root || curRoot() }).then(r => { const n = (r.spec?.checks || []).length; $('.mcrit', d).innerHTML = n ? `✓ ${n} ${t(n === 1 ? 'critério executável definido no Pronto?' : 'critérios executáveis definidos no Pronto?')}` : `<span class="del">${t('Nenhum critério executável no Pronto?: sem eles o infuse não tem como confirmar o fim.')}</span> <a href="#" class="defc">${t('Definir critérios')}</a>`; const dc = $('.defc', d); if (dc) dc.onclick = e => { e.preventDefault(); close(); togglePane('oracle', true); }; });
  $('.cancel', d).onclick = close;
  $('.go', d).onclick = async () => {
    const goal = $('.mg', d).value.trim(); if (!goal) return toast(t('Descreva o objetivo'));
    let L2 = L; if (!L2) { P.value = goal; await send(v); return close(); } // no session yet: a normal start
    const r = await api(`/api/session/${L2.id}/mission`, { body: { goal, budget: parseFloat($('.mb', d).value) || 2, maxTurns: parseInt($('.mt', d).value) || 10 }, root: L2.root });
    if (!r.ok) return toast(r.error); if (P) P.value = ''; close();
  };
  setTimeout(() => $('.mg', d).focus(), 30);
}
async function runReview(v, focus) {
  const L = v.sid && S.live[v.sid]; if (!L) return toast(t('Abra uma sessão para revisar'));
  if (busy(L)) return toast(t('Espere o agente terminar para revisar'));
  const st = $('.live', v.el); if (st) st.innerHTML = `<div class="status"><span class="spin"></span>${t('revisor independente lendo as alterações…')}</div>`;
  const r = await api(`/api/session/${L.id}/review`, { body: { focus: focus || '' }, root: L.root });
  if (!r.ok) { toast(r.error || t('Revisão falhou')); renderChat(v); }
}
// The element the user picked in the browser pane goes into the message being written.
if (window.infuse?.on) infuse.on('browserPick', p => insertPick(p));
function insertPick(p) {
  if (!p) return; const v = V(); if (!v?.el) return; const P = $('.prompt', v.el);
  const st = p.style ? `\n${t('Estilo')}: ${p.style.font} · ${t('cor')} ${p.style.color} · ${t('fundo')} ${p.style.background} · margin ${p.style.margin} · padding ${p.style.padding}` : '';
  P.value = (P.value.trim() ? P.value.trimEnd() + '\n\n' : '') + `[${t('Elemento selecionado em')} ${p.url}]\n${t('Seletor')}: ${p.selector}\n${t('Posição')}: ${p.rect.x},${p.rect.y} · ${p.rect.w}×${p.rect.h}${st}${p.text ? `\n${t('Texto')}: ${p.text}` : ''}\n\`\`\`html\n${p.html}\n\`\`\`\n`;
  grow(P); P.focus(); P.setSelectionRange(P.value.length, P.value.length); toast(t('Elemento adicionado à mensagem'));
}
// Scrolling: the conversation follows the end only while the user is there; scrolling up stops it.
function toBottom(v) { const sc = v.el && $('.scroller', v.el); if (!sc) return; v._prog = true; sc.scrollTop = sc.scrollHeight; }
function updateJump(v) {
  const sc = v.el && $('.scroller', v.el), b = v.el && $('.jumpdown', v.el); if (!sc || !b) return;
  const far = sc.scrollHeight - sc.scrollTop - sc.clientHeight > 120; const L = v.sid && S.live[v.sid];
  b.classList.toggle('show', far); b.classList.toggle('live', far && busy(L));
  b.style.bottom = ($('.composer', v.el)?.offsetHeight || 120) + 14 + 'px';
}
function bindScroll(v) {
  const sc = $('.scroller', v.el); if (!sc || sc._bound) return; sc._bound = true;
  sc.addEventListener('wheel', e => { if (e.deltaY < 0) v.stick = false; }, { passive: true });
  sc.addEventListener('scroll', () => {
    const dist = sc.scrollHeight - sc.scrollTop - sc.clientHeight;
    if (v._prog) v._prog = false; else if (dist > 24) v.stick = false;
    if (dist < 8) v.stick = true;
    updateJump(v); markToc(v);
  }, { passive: true });
  $('.jumpdown', v.el).onclick = () => { v.stick = true; sc.scrollTo({ top: sc.scrollHeight, behavior: 'smooth' }); };
}
// Chapters: one per turn, titled by what the user asked; hover lists them, a click goes to the exact point.
function chapterTitle(m) {
  const n = missionOf(m);
  const tx = n ? (n.turn ? missionTag(n) : `${t('Missão')}: ${n.goal}`) : msgText(m).trim();
  return tx.split('\n')[0].slice(0, 90) || t('(sem texto)');
}
function renderToc(v) {
  const L = v.sid && S.live[v.sid], toc = v.el && $('.toc', v.el); if (!toc) return; bindScroll(v);
  const s = L?.snap; const starts = s?.turnStarts || [];
  if (starts.length < 2) { toc.classList.add('hidden'); return; }
  toc.classList.remove('hidden');
  const items = starts.map((idx, k) => ({ turn: k + 1, title: chapterTitle(s.messages[idx] || { content: [] }), ts: s.messages[idx]?.ts }));
  $('.ticks', toc).innerHTML = items.slice(-14).map(x => `<i data-t="${x.turn}"></i>`).join('');
  $('.toclist', toc).innerHTML = `<div class="th">${t('Capítulos')}</div>` + items.map(x => `<div class="ti" data-t="${x.turn}"><span class="n">${x.turn}</span><span class="tt">${esc(x.title)}</span>${x.ts ? `<span class="tm">${fmtClock(x.ts)}</span>` : ''}</div>`).join('');
  $$('[data-t]', toc).forEach(el => el.onclick = () => { const sep = $(`.turnsep[data-turn="${el.dataset.t}"]`, v.el); if (sep) { v.stick = false; sep.scrollIntoView({ behavior: 'smooth', block: 'start' }); } });
  markToc(v);
}
function markToc(v) {
  const sc = $('.scroller', v.el), toc = $('.toc', v.el); if (!sc || !toc) return;
  const top = sc.getBoundingClientRect().top + 40; let cur = 1;
  for (const sep of $$('.turnsep', sc)) { if (sep.getBoundingClientRect().top <= top) cur = +sep.dataset.turn; else break; }
  $$('[data-t]', toc).forEach(el => el.classList.toggle('on', +el.dataset.t === cur));
}
// Read aloud with the system's voices; a second click stops.
function speakMsg(btn, text) {
  if (!('speechSynthesis' in window)) return toast(t('Este navegador não tem leitura em voz alta'));
  if (speechSynthesis.speaking) { speechSynthesis.cancel(); $$('[data-speak].on').forEach(x => x.classList.remove('on')); return; }
  const clean = text.replace(/```[\s\S]*?```/g, ' ').replace(/[*_`#>]/g, '').slice(0, 20000);
  const u = new SpeechSynthesisUtterance(clean); u.lang = S.lang === 'en' ? 'en-US' : 'pt-BR'; u.rate = 1.05;
  u.onend = u.onerror = () => btn.classList.remove('on'); btn.classList.add('on'); speechSynthesis.speak(u);
}
function turnOf(s, msgIndex) { const k = s.turnStarts.indexOf(msgIndex); return k >= 0 ? k + 1 : 0; }
function msgText(m) { return (m.content || []).filter(b => b.type === 'text').map(b => b.display || b.text).join('\n\n').trim(); }
function copyText(txt, what) { navigator.clipboard.writeText(txt).then(() => toast(t(what || 'Copiado')), () => toast('Não foi possível copiar')); }
// The whole conversation as Markdown: prompts, answers, and each tool call with its input and result.
function transcriptMarkdown(L) {
  const s = L.snap; const res = {}; for (const m of s.messages) for (const b of m.content || []) if (b.type === 'tool_result') res[b.tool_use_id] = b;
  let out = `# ${s.title || 'Sessão'}\n\n_${projectLabel(L.root)} · ${s.model || ''} · ${new Date().toLocaleString()}_\n\n`;
  for (const m of s.messages) {
    if (m.role === 'user') { const tx = msgText(m); if (tx) out += `## ${t('Você')}\n\n${tx}\n\n`; continue; }
    out += `## ${t('Agente')}\n\n`;
    for (const b of m.content) {
      if (b.type === 'thinking' && b.thinking) out += `<details><summary>${t('Raciocínio')}</summary>\n\n${b.thinking}\n\n</details>\n\n`;
      if (b.type === 'text' && b.text?.trim()) out += b.text.trim() + '\n\n';
      if (b.type === 'tool_use') { out += `**${b.name}**\n\n\`\`\`json\n${JSON.stringify(b.input, null, 2)}\n\`\`\`\n\n`; const r = res[b.id]; if (r) out += `\`\`\`\n${String(r.content || '').slice(0, 8000)}\n\`\`\`\n\n`; }
    }
  }
  return out;
}
function exportTranscript(L) {
  const blob = new Blob([transcriptMarkdown(L)], { type: 'text/markdown' }); const a = document.createElement('a');
  a.href = URL.createObjectURL(blob); a.download = (L.snap.title || 'sessao').replace(/[^\w\- ]+/g, '').trim().slice(0, 60) + '.md'; a.click(); setTimeout(() => URL.revokeObjectURL(a.href), 2000);
}
function bindChat(v, root) {
  const L = v.sid && S.live[v.sid];
  $$('[data-copy-msg]', root).forEach(b => b.onclick = e => { e.stopPropagation(); const m = L.snap.messages[+b.dataset.copyMsg]; copyText(msgText(m), 'Mensagem copiada'); });
  $$('[data-copy-turn]', root).forEach(b => b.onclick = e => { e.stopPropagation(); const [a, z] = b.dataset.copyTurn.split(':').map(Number); copyText(L.snap.messages.slice(a + 1, z + 1).filter(m => m.role === 'assistant').map(msgText).filter(Boolean).join('\n\n'), 'Respostas copiadas'); });
  $$('.codecopy', root).forEach(b => b.onclick = () => copyText(b.nextElementSibling.innerText, 'Código copiado'));
  $$('[data-url]', root).forEach(a => a.onclick = e => { e.preventDefault(); togglePane('browser', true); setTimeout(() => paneCall('browser', 'go', a.dataset.url), 300); });
  $$('[data-fork]', root).forEach(b => b.onclick = async e => { e.stopPropagation(); const r = await api(`/api/session/${L.id}/fork`, { body: { upTo: +b.dataset.fork }, root: L.root }); if (!r.ok) return toast(r.error); await loadSessions(); await openSession(r.root, r.id); toast(t('Nova sessão a partir desta mensagem')); });
  $$('[data-pinmsg]', root).forEach(b => b.onclick = async e => { e.stopPropagation(); const r = await api(`/api/session/${L.id}/pinmsg`, { body: { index: +b.dataset.pinmsg }, root: L.root }); L.snap.pinnedMsgs = r.pinned || []; renderChat(v); renderRail(v); });
  $$('[data-speak]', root).forEach(b => b.onclick = e => { e.stopPropagation(); speakMsg(b, msgText(L.snap.messages[+b.dataset.speak])); });
  $$('[data-fixreview]', root).forEach(b => b.onclick = () => { const P = $('.prompt', v.el); P.value = t('Corrija os achados da revisão independente acima. Para cada um, diga o que mudou ou por que não procede.'); send(v); });
  $$('[data-next]', root).forEach(b => b.onclick = () => { const P = $('.prompt', v.el); P.value = b.dataset.next; grow(P); P.focus(); });
  $$('[data-copy-think]', root).forEach(b => b.onclick = e => { e.stopPropagation(); copyText(b.closest('.tool').querySelector('.thinktext')?.innerText || '', 'Raciocínio copiado'); });
  $$('[data-copy-tool]', root).forEach(b => b.onclick = e => { e.stopPropagation(); copyText(b.closest('.tool').querySelector(b.dataset.copyTool === 'out' ? '.res' : '.inp')?.innerText || '', 'Copiado'); });
  $$('[data-rw]', root).forEach(b => b.onclick = () => rewindDialog(v, +b.dataset.rw));
  $$('[data-fold]', root).forEach(x => x.onclick = e => { if (e.target.closest('[data-path]')) return; const box = x.parentNode; const open = !box.classList.contains('open'); box.classList.toggle('open', open); S.fold[x.dataset.fold] = open; });
  $$('[data-path]', root).forEach(x => { x.onclick = () => openFileInPane(x.dataset.path); x.oncontextmenu = e => { e.preventDefault(); pathMenu(x.dataset.path, e); }; });
  $$('[data-perm]', root).forEach(b => b.onclick = () => decide(v, b.dataset.id, b.dataset.perm));
  $$('[data-ans]', root).forEach(b => b.onclick = () => answer(v, b.dataset.id, b.dataset.ans));
  const free = $('.free input', root); if (free) free.onkeydown = e => { if (e.key === 'Enter' && free.value.trim()) answer(v, free.dataset.id, free.value.trim()); };
}
// The tool input while it streams: complete fields and the string being written so far, read from the
// incomplete JSON (so the code shows as the model writes it).
function partialJSON(src) {
  try { return JSON.parse(src); } catch { }
  const out = { _streaming: true };
  const re = /"([A-Za-z_]+)"\s*:\s*/g; let m;
  while ((m = re.exec(src))) {
    const key = m[1]; let i = re.lastIndex;
    if (src[i] === '"') { // a string, possibly unfinished
      let s = '', j = i + 1, done = false;
      for (; j < src.length; j++) {
        const ch = src[j];
        if (ch === '\\') { const nx = src[j + 1]; if (nx === undefined) break; const map = { n: '\n', t: '\t', r: '\r', '"': '"', '\\': '\\', '/': '/', b: '\b', f: '\f' }; if (nx === 'u') { const hex = src.slice(j + 2, j + 6); if (hex.length < 4) break; s += String.fromCharCode(parseInt(hex, 16)); j += 5; } else { s += map[nx] ?? nx; j++; } continue; }
        if (ch === '"') { done = true; break; }
        s += ch;
      }
      out[key] = s; if (!done) out._open = key; re.lastIndex = j + 1;
    } else { const mm = /^(-?\d+(?:\.\d+)?|true|false|null)/.exec(src.slice(i)); if (mm) out[key] = JSON.parse(mm[1]); }
  }
  return out;
}
// Code being written: numbered lines, following the end while it grows.
function liveCodeHTML(text, cls) {
  const lines = String(text || '').split('\n');
  return `<div class="livecode ${cls || ''}">${lines.map((l, k) => `<div class="lc"><span class="no">${k + 1}</span><span class="tx">${esc(l) || ' '}</span></div>`).join('')}<span class="cursor"></span></div>`;
}
document.addEventListener('click', e => { if (e.target.closest('.agentsgo')) togglePane('agents', true); });
function renderLive(v) {
  if (!v.el) return; const el = $('.live', v.el); const L = v.sid && S.live[v.sid]; if (!el || !L) return;
  const parts = [];
  if (L.think) parts.push(['think', thinkBlock(`${L.id}:live`, L.think, true)]);
  if (L.text) parts.push(['text', `<div class="a"><div class="md sel">${md(L.text)}<span class="cursor"></span></div></div>`]);
  for (const [id, tl] of Object.entries(L.tools)) { const inp = partialJSON(tl.input); parts.push(['tool:' + id, toolBlock(id, tl.name, inp, null, L.running?.id === id)]); }
  let out = '';
  const working = (L.snap?.agents || []).filter(a => a.state === 'running');
  if (working.length && busy(L)) out += `<div class="status sub agentsgo" title="${esc(t('Abrir o painel Agentes'))}"><span class="spin"></span><span>${working.length === 1 ? t('1 subagente trabalhando') : working.length + ' ' + t('subagentes trabalhando')}: ${working.map(a => `<b>${esc(a.title)}</b> · ${a.steps} ${t('passos')}${a.tool ? ' · ' + esc(a.tool) : ''}`).join(' &nbsp;|&nbsp; ')}</span></div>`;
  const st = L.snap?.state;
  if (st === 'thinking' || st === 'tool' || st === 'compacting') out += `<div class="status"><span class="spin"></span><span>${st === 'tool' ? t('executando ') + esc(L.running?.name || '') : st === 'compacting' ? t('compactando contexto') : t('pensando')}</span><span class="faint el" data-el="${L.id}"></span></div>`;
  if (out) parts.push(['status', `<div>${out}</div>`]);
  patchParts(v, el, parts);
  if (v.stick !== false) toBottom(v); updateJump(v);
}
// Updates a container part by part (keyed): an unchanged part keeps its element, so nothing flashes,
// folds stay as they are and a code view keeps its scroll (following the end only if it was there).
function patchParts(v, el, parts) {
  const have = new Map([...el.children].map(c => [c.dataset.k, c]));
  let prev = null;
  for (const [key, html] of parts) {
    let node = have.get(key);
    if (!node || node._html !== html) {
      const n = document.createElement('div'); n.dataset.k = key; n.className = 'lk'; n.innerHTML = html; n._html = html;
      if (node) {
        const codes = $$('.livecode', node).map(c => ({ end: c.scrollHeight - c.scrollTop - c.clientHeight < 8, top: c.scrollTop }));
        node.replaceWith(n);
        $$('.livecode', n).forEach((c, i) => { const was = codes[i]; c.scrollTop = !was || was.end ? c.scrollHeight : was.top; });
      } else {
        if (prev) prev.after(n); else el.prepend(n);
        $$('.livecode', n).forEach(c => { c.scrollTop = c.scrollHeight; });
      }
      bindChat(v, n); node = n;
    }
    have.delete(key); prev = node;
  }
  have.forEach(n => n.remove());
}
setInterval(() => { for (const el of $$('[data-el]')) { const L = S.live[el.dataset.el]; if (L?.started) el.textContent = Math.round((Date.now() - L.started) / 1000) + 's'; } }, 500);

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
function permCard(p) {
  let inp = {}; try { inp = typeof p.input === 'string' ? JSON.parse(p.input) : p.input || {}; } catch { }
  const verb = p.category === 'exec' ? 'executar um comando' : p.category === 'write' ? 'alterar um arquivo' : p.category === 'net' ? 'acessar a web' : p.category === 'mcp' ? 'usar um conector (roda fora do filtro)' : p.category === 'memory' ? 'registrar na memória' : 'usar uma ferramenta';
  const detail = p.tool === 'Bash' ? `<pre class="cmd">${esc(inp.command || '')}</pre>` : p.tool === 'Edit' ? `<div class="mono" style="margin-top:8px">${esc(inp.path)}</div>` + diffHTML((inp.old_string || '').split('\n').map(l => '-' + l).join('\n') + '\n' + (inp.new_string || '').split('\n').map(l => '+' + l).join('\n')) : p.tool === 'Write' ? `<div class="mono" style="margin-top:8px">${esc(inp.path)}</div>` + diffHTML((inp.content || '').split('\n').slice(0, 200).map(l => '+' + l).join('\n')) : p.tool === 'WebFetch' ? `<pre class="cmd">${esc(inp.url || '')}</pre>` : `<pre class="cmd">${esc(JSON.stringify(inp, null, 2))}</pre>`;
  return `<div class="pcard"><div class="hd"><b>${esc(p.tool)}</b><span class="muted">${t('quer')} ${t(verb)}</span></div>${p.reason ? `<div class="sentinel">${t('Sentinela:')} ${esc(t(p.reason))}. ${t('O infuse pergunta mesmo no modo "aceitar tudo": enfraquecer o teste faz ele passar sem resolver o problema.')}</div>` : ''}${detail}
  <div class="opts"><div class="opt" data-perm="allow" data-id="${p.id}">${t('Sim')}<span class="kbd">Y</span></div><div class="opt" data-perm="always" data-id="${p.id}">${t('Sim, e não perguntar de novo nesta sessão')}${p.tool === 'Bash' ? ` <span class="faint">(${esc((inp.command || '').trim().split(/\s+/).slice(0, 2).join(' '))})</span>` : ''}<span class="kbd">A</span></div><div class="opt" data-perm="all" data-id="${p.id}">${t('Sim, e aceitar tudo daqui em diante')}<span class="kbd">T</span></div><div class="opt" data-perm="deny" data-id="${p.id}">${t('Não, e dizer ao agente o que fazer diferente')}<span class="kbd">N</span></div></div></div>`;
}
function questionCard(q) {
  return `<div class="pcard"><div class="hd"><b>${esc(q.question)}</b></div><div class="opts">${(q.options || []).map((o, i) => `<div class="opt" data-ans="${esc(o)}" data-id="${q.id}">${esc(o)}<span class="kbd">${i + 1}</span></div>`).join('')}</div>
  <div class="free"><input data-id="${q.id}" placeholder="${esc(t('ou responda com suas palavras…'))}"></div></div>`;
}
async function decide(v, id, d) {
  if (d === 'all') { await setSessionOpts(v, { mode: 'bypass' }); d = 'allow'; toast(t('Modo: ') + t('Aceitar tudo')); }
  const L = S.live[v.sid]; await api(`/api/sessions/${v.sid}/permission`, { body: { id, decision: d }, root: L.root }); if (L.snap) L.snap.pendingPermission = null; renderChat(v); if (d === 'deny') setTimeout(() => $('.prompt', v.el).focus(), 50); }
async function answer(v, id, a) { const L = S.live[v.sid]; await api(`/api/sessions/${v.sid}/answer`, { body: { id, answer: a }, root: L.root }); if (L.snap) L.snap.pendingQuestion = null; renderChat(v); }

// markdown (small, safe): code blocks, inline code (paths become links), headings, lists, quotes, tables, links
// Lists, nested by indentation: a sub-item stays inside its item; ordered lists keep their numbers.
function mdList(m) {
  let html = ''; const stack = [];
  for (const l of m.replace(/\n$/, '').split('\n')) {
    const mm = l.match(/^([ \t]*)([-*]|\d+\.) (.*)$/);
    if (!mm) { html += ' ' + l.trim(); continue; } // a wrapped line of the item above
    const ind = mm[1].replace(/\t/g, '    ').length, type = /\d/.test(mm[2]) ? 'ol' : 'ul', n = parseInt(mm[2], 10);
    while (stack.length > 1 && ind < stack[stack.length - 1].ind) html += `</li></${stack.pop().type}>`;
    const top = stack[stack.length - 1];
    const open = () => { html += `<${type}${type === 'ol' && n > 1 ? ` start="${n}"` : ''}><li>`; stack.push({ ind, type }); };
    if (!top || ind > top.ind) open();
    else if (top.type !== type) { html += `</li></${stack.pop().type}>`; open(); }
    else html += '</li><li>';
    html += mm[3];
  }
  while (stack.length) html += `</li></${stack.pop().type}>`;
  return html;
}
function md(src) {
  const blocks = [];
  src = src.replace(/```[\w+-]*\n([\s\S]*?)```/g, (_, c) => { blocks.push(`<div class="codewrap"><button class="ib codecopy" title="Copiar">${I.copy}</button><pre><code>${esc(c.replace(/\n$/, ''))}</code></pre></div>`); return `\u0000${blocks.length - 1}\u0000`; });
  let x = esc(src);
  x = x.replace(/^\|(.+)\|\n\|[-:| ]+\|\n((?:\|.*\|\n?)*)/gm, (_, head, body) => `<table><tr>${head.split('|').map(c => `<th>${c.trim()}</th>`).join('')}</tr>${body.trim().split('\n').map(r => `<tr>${r.replace(/^\||\|$/g, '').split('|').map(c => `<td>${c.trim()}</td>`).join('')}</tr>`).join('')}</table>`);
  x = x.replace(/^### (.*)$/gm, '<h3>$1</h3>').replace(/^## (.*)$/gm, '<h2>$1</h2>').replace(/^# (.*)$/gm, '<h1>$1</h1>').replace(/^&gt; (.*)$/gm, '<blockquote>$1</blockquote>');
  x = x.replace(/`([^`\n]+)`/g, (_, c) => /^[\w.\-/\\]+\.[a-z0-9]{1,6}(:\d+)?$/i.test(c) && c.includes('.') ? `<code class="path" data-path="${c.replace(/:\d+$/, '')}">${c}</code>` : `<code>${c}</code>`);
  x = x.replace(/\*\*([^*\n]+)\*\*/g, '<b>$1</b>').replace(/(^|[^*\w])\*([^*\n]+)\*/g, '$1<i>$2</i>').replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  x = x.replace(/(?:^[ \t]*(?:[-*]|\d+\.) .*(?:\n|$)(?:^[ \t]{2,}\S.*(?:\n|$))*)+/gm, mdList);
  x = x.split(/\n{2,}/).map(p => /^\s*<(h\d|ul|ol|pre|table|blockquote)/.test(p) || /^\u0000\d+\u0000$/.test(p.trim()) ? p : `<p>${p.replace(/\n/g, '<br>')}</p>`).join('');
  return x.replace(/\u0000(\d+)\u0000/g, (_, i) => blocks[i]);
}
