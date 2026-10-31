'use strict';
// tasks
// Tasks, processes, agents and plan panes.

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
