'use strict';
// tasks
// Tasks, processes, agents and plan panes.

// ---------------------------------------------------------------- Tasks, Plan
function mountTasks(pb) {
  const draw = () => { const L = curL(); const ts = L?.snap?.tasks || []; pb.innerHTML = `<div class="plist">${ts.length ? `<div class="card plan u-b0-p0">${planCard(L).replace(/^<div class="card plan" data-card="plan">/, '').replace(/<\/div>$/, '')}</div>` : `<div class="empty">${t('ui.pane.tasks.o-agente-ainda-nao.89e131')}</div>`}${L?.running ? `<div class="status u-mt10px"><span class="spin"></span>${t('ui.feed.executando.0c69a4')} ${esc(L.running.name)}</div>` : ''}</div>`; };
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
      <span class="faint meta">${p.id} · ${p.state === 'running' ? t('ui.pane.tasks.rodando-ha.a485fc') + ' ' + elapsed(p.started) : (p.state === 'stopped' ? t('ui.pane.tasks.encerrado.7a81a9') : t('ui.pane.tasks.terminou.30e883') + (p.exit != null ? ' · ' + t('ui.pane.tasks.codigo.4bb9ff') + ' ' + p.exit : '')) + ' · ' + t('ui.pane.tasks.durou.16fbd0') + ' ' + elapsed(p.started, p.ended)}</span></span>
      ${p.state === 'running' ? `<button class="btn sm danger stop">${t('ui.pane.tasks.encerrar.78cebb')}</button>` : ''}</div>
      ${open.has(p.id) ? `<pre class="pout">${esc(full[p.id]?.text ?? '')}</pre>` : ''}</div>`).join('')}</div>`
      : `<div class="empty">${t('ui.pane.tasks.nenhum-processo-em-segundo.a1a6ed')}</div>`;
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
      <span class="faint">${a.steps} ${t('ui.chat.passos.7734a8')} · ${elapsed(a.started, a.ended)}</span></div>
      ${a.state === 'running' && a.tool ? `<div class="faint now mono">${esc(a.tool)} ${esc(a.summary || '')}</div>` : ''}
      ${open.has(a.id) ? `<div class="abody"><details class="atask"><summary>${t('ui.feed.tarefa-dada-ao-subagente.b8cb3e')}</summary><div class="md sel">${md(a.task || '')}</div></details><div class="atrail">${agentTrailHTML(a, 'pane:' + a.id)}</div></div>` : ''}</div>`).join('')}</div>`
      : `<div class="empty">${t('ui.pane.tasks.nenhum-subagente-nesta-sessao.f5da70')}</div>`;
    $$('.agent .ah', pb).forEach(h => h.onclick = () => { const id = h.parentElement.dataset.id; open.has(id) ? open.delete(id) : open.add(id); draw(); });
    bindFolds(pb);
  };
  draw(); return { refresh: draw };
}

function mountPlan(pb) {
  const draw = () => { const L = curL(); const p = L?.snap?.plan; pb.innerHTML = p ? `<div class="plist md sel">${md(p)}</div><div class="pfoot"><span class="grow faint">${t('ui.pane.tasks.plano-proposto-no-modo.2de16c')}</span><button class="btn sm pri">${t('ui.pane.tasks.aprovar-e-executar.1ed7c3')}</button></div>` : `<div class="empty">${t('ui.pane.tasks.nenhum-plano-ainda-use.a5d33f')}</div>`; const b = $('.btn', pb); if (b) b.onclick = async () => { const v = V(); await setSessionOpts(v, { mode: 'acceptEdits' }); const P = $('.prompt', elOf(v)); P.value = t('ui.pane.tasks.plano-aprovado-execute-o.9feb1e'); send(v); }; };
  draw(); return { refresh: draw };
}
