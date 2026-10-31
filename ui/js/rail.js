'use strict';
// rail
// Session details rail: environment, PR, CI, change badge.

// ---------------------------------------------------------------- rail (session details)
function renderRail(v) {
  v = freshView(v);
  if (!elOf(v)) return; const el = $('.rail', elOf(v)); const L = v.sid && S.live[v.sid]; if (!L?.snap) { el.innerHTML = ''; return; }
  const R = liveRT(L.id); const c = L.changes || {}; const files = c.files || [];
  const pct = Math.min(100, Math.round(((L.lastIn || 0) / ctxLimit(L)) * 100));
  const tasks = L.snap.tasks || [];
  el.innerHTML = `
  ${bansCard(L)}
  ${diagnosisCard(L)}
  ${planCard(L)}
  <div class="card" data-card="changes"><div class="ch">${t('ui.rail.alteracoes.db6cfc')}<span class="r mono">${c.isGit === false && !c.local ? t('ui.rail.sem-git.f3cb20') : `<span class="add">+${c.added || 0}</span> <span class="del">−${c.removed || 0}</span>`}</span></div>${c.local ? `<div class="segrow">${scopeSwitch()}</div>` : ''}
    ${files.length ? files.slice(0, 8).map(f => `<div class="li${f.status === 'D' ? ' gone' : ''}" data-f="${esc(f.path)}"><span class="n" title="${esc(f.path)}">${esc(f.path)}</span>${chgTag(f)}<span class="add">+${f.added}</span><span class="del">−${f.removed}</span></div>`).join('') + (files.length > 8 ? `<div class="faint u-p3px4px">+${files.length - 8} ${t('ui.rail.arquivos.0dd06b')}</div>` : '') : `<div class="faint">${t('ui.rail.nenhuma-alteracao.63c618')}</div>`}
    <div class="prline"></div></div>
  <div class="card ctxcard pointer" data-card="context"><div class="ch">${t('ui.composer.contexto.4b9a77')}<span class="r">${pct}%</span></div><div class="bar100"><i style="--w:${pct}%"></i></div><div class="faint">${ktok(L.lastIn)} / ${ktok(ctxLimit(L))} tokens · ${money(L.snap.cost)}</div></div>
  <div class="card" data-card="env"><div class="ch">${t('ui.rail.ambiente.45c6f1')}</div><div class="kv"><span>${t('ui.rail.onde.e53176')}</span><span>${t('ui.rail.este-computador.2175ee')}</span></div><div class="kv"><span>${t('ui.sidebar.pasta.46139b')}</span><span title="${esc(L.snap.workRoot)}">${esc(baseName(L.snap.workRoot || L.root))}</span></div>${L.snap.branch ? `<div class="kv"><span>${t('ui.rail.branch.10d735')}</span><span class="mono">${esc(L.snap.branch)}</span></div>` : ''}${L.snap.worktree ? `<div class="kv"><span>${t('ui.composer.worktree.94449c')}</span><span>${t('ui.rail.isolado.955cdd')}</span></div>` : ''}<div class="kv"><span>${t('ui.rail.modelo.dab9c1')}</span><span class="mono">${esc(L.snap.model || '')}</span></div></div>
  ${(L.snap.pinnedMsgs || []).length ? `<div class="card pins" data-card="pins"><div class="ch">${t('ui.rail.mensagens-fixadas.eb6a13')}<span class="r">${L.snap.pinnedMsgs.length}</span></div>${L.snap.pinnedMsgs.map(i => `<div class="li pinli" data-goto="${i}"><span class="n">${esc(msgText(L.snap.messages[i] || { content: [] }).split('\n')[0].slice(0, 80) || '…')}</span></div>`).join('')}</div>` : ''}
  ${runningCard(L)}
  ${missionCard(L)}
  <div class="card" data-card="pronto"><div class="ch">${t('ui.rail.pronto.2726ba')}<span class="r">${R.verdict ? `<span class="tag ${{ aprovado: 'ok', reprovado: 'bad' }[R.verdict.verdict] || 'warn'}">${esc(t(R.verdict.verdict))}</span>` : `<span class="faint">${t('ui.rail.nao-verificado.5c9de0')}</span>`}</span></div>
    <div class="faint u-mb6px">${R.verdict ? esc(R.verdict.summary) : t('ui.rail.so-a-execucao-dos.dc8982')}</div>
    <div class="row"><button class="btn sm line oraclego">${t('ui.rail.pronto.1cfd95')}</button><button class="btn sm" data-open="oracle">${t('ui.rail.criterios.ea0e17')}</button><button class="btn sm revgo" title="${esc(t('ui.rail.um-revisor-com-contexto.d4aecd'))}">${t('ui.rail.revisar.4275a6')}</button></div></div>`;
  $('.revgo', el).onclick = () => runReview(v);
  railFolds(v, el);
  $$('.card.diag [data-path]', el).forEach(x => x.onclick = e => { e.stopPropagation(); openFileInPane(x.dataset.path); });
  $$('.card.bans [data-ban]', el).forEach(b => b.onclick = async e => { e.stopPropagation(); const r = await api(`/api/session/${L.id}/bans`, { body: { action: 'set', index: +b.dataset.ban, off: b.dataset.off === '1' }, root: L.root }); Live.patch(L.id, { bans: r.bans || L.bans }); renderRail(v); });
  $$('[data-pane]', el).forEach(x => x.onclick = () => togglePane(x.dataset.pane, true));
  bindScope(el);
  $$('[data-goto]', el).forEach(x => x.onclick = () => { const m = $(`[data-mi="${x.dataset.goto}"]`, elOf(v)); if (m) { viewRT(v).stick = false; m.scrollIntoView({ behavior: 'smooth', block: 'center' }); m.classList.add('flash'); setTimeout(() => m.classList.remove('flash'), 1400); } });
  const ms = $('.mstop', el); if (ms) ms.onclick = () => api(`/api/session/${L.id}/mission`, { body: { action: 'stop' }, root: L.root });
  const mn = $('.mnew', el); if (mn) mn.onclick = () => missionDialog(v);
  $('.oraclego', el).onclick = () => { togglePane('oracle', true); setTimeout(() => { const out = $('.pane[data-id="oracle"] .verdict'); if (out) runOracleUI({ root: L.root, dir: L.snap.workRoot || L.root }, out); }, 400); };
  $$('[data-f]', el).forEach(x => x.onclick = () => { togglePane('changes', true); setTimeout(() => paneCall('changes', 'select', x.dataset.f), 50); });
  $('.ctxcard', el).onclick = () => contextDialog(v);
  $$('[data-open]', el).forEach(x => x.onclick = () => togglePane(x.dataset.open, true));
  renderPR(v, $('.prline', el));
}
// Auto-fix: the failing CI run's log goes to the agent (by click, or on its own when turned on).
async function fixCI(v, L, pr) {
  const r = await api('/api/pr', { body: { action: 'failed-logs' }, root: L.root, dir: L.snap.workRoot || L.root });
  if (!r.ok) return toast(r.error || r.output || t('ui.rail.nao-foi-possivel-ler.360592'));
  const P = $('.prompt', elOf(v)); P.value = `${t('ui.rail.o-ci-do-pr.76e14a')} #${pr.number} ${t('ui.rail.falhou-leia-o-log.b7f62c')}\n\n\`\`\`\n${r.output}\n\`\`\``; send(v);
}
setInterval(async () => { // checks PRs of sessions with auto-fix on, every 2 minutes
  for (const v of S.views) {
    const L = v.sid && S.live[v.sid]; if (!L) continue;
    const R = liveRT(L.id);
    if (!R.autofixInit) { R.autofix = lsGet('autofix:' + L.id, false); R.autofixInit = true; }
    if (!R.autofix || busy(L)) continue;
    R.prAt = 0; await renderPR(v, $('.prline', elOf(v)) || document.createElement('div'));
    const pr = R.pr?.pr; if (!pr) continue;
    const failed = (pr.statusCheckRollup || []).filter(c => /FAIL|ERROR/i.test(c.conclusion || c.state || ''));
    const sig = pr.number + ':' + failed.map(c => c.name + c.completedAt).join('|');
    if (failed.length && R.autofixSig !== sig) { R.autofixSig = sig; toast(t('ui.rail.ci-falhou-enviando-o.73ae70')); fixCI(v, L, pr); }
  }
}, 120000);
async function renderPR(v, el) {
  const L = S.live[v.sid]; if (!L || !L.changes?.isGit || !el) return;
  const R = liveRT(L.id);
  if (!R.pr || Date.now() - R.prAt > 60000) { R.pr = await api('/api/pr', { root: L.root, dir: L.snap.workRoot || L.root }); R.prAt = Date.now(); }
  const pr = R.pr?.pr;
  if (!R.pr?.gh) { // without the GitHub CLI: say so and offer to install it, instead of hiding PRs
    el.innerHTML = `<div class="row u-mt6px-fs12px"><span class="faint grow">${t('ui.rail.pull-requests-precisam-do.5b399b')}</span><button class="btn sm" data-gh="install">${t('ui.rail.instalar.f93057')}</button></div>`;
    $('[data-gh]', el).onclick = () => { togglePane('terminal', true); setTimeout(() => paneCall('terminal', 'openAt', L.snap.workRoot || L.root, 'winget install --id GitHub.cli -e && gh auth login', 'GitHub CLI'), 300); };
    return;
  }
  if (!pr) { el.innerHTML = `<div class="row u-mt6px"><button class="btn sm line" data-pr="create">${t('ui.rail.criar-pr.6ddbaf')}</button></div>`; }
  else {
    var checks = pr.statusCheckRollup || []; var fail = checks.filter(c => /FAIL|ERROR/i.test(c.conclusion || c.state || '')).length, pend = checks.filter(c => !(c.conclusion || '') && !/SUCCESS/.test(c.state || '')).length;
    el.innerHTML = `<div class="row u-mt8px-fs12px"><a href="${esc(pr.url)}" target="_blank">#${pr.number}</a><span class="trunc grow">${esc(pr.title)}</span><span class="tag ${fail ? 'bad' : pend ? 'warn' : 'ok'}">${fail ? t('ui.rail.ci-falhou.e74fef') : pend ? t('ui.rail.ci-rodando.6ce084') : t('ui.rail.ci-ok.8dca54')}</span><button class="ib u-w22px-h22px" data-pr="menu">${I.more}</button></div>`;
  }
  $$('[data-pr]', el).forEach(b => b.onclick = async e => {
    if (b.dataset.pr === 'create') { b.disabled = true; const r = await api('/api/pr', { body: { action: 'create' }, root: L.root, dir: L.snap.workRoot || L.root }); toast(r.ok ? 'PR criado' : (r.error || r.output)); R.prAt = 0; renderRail(v); }
    else menu(b, [{ label: 'Abrir no GitHub', f: () => { togglePane('browser', true); setTimeout(() => paneCall('browser', 'go', pr.url), 300); } }, fail ? { label: 'Corrigir o CI agora', sub: 'O agente lê o log da falha e corrige', f: () => fixCI(v, L, pr) } : null, { label: R.autofix ? 'Desligar correção automática do CI' : 'Corrigir o CI automaticamente', sub: 'Quando o CI falhar e a sessão estiver parada, o agente recebe o log e corrige', check: !!R.autofix, f: () => { R.autofix = !R.autofix; lsSet('autofix:' + L.id, R.autofix); toast(t(R.autofix ? 'Correção automática do CI ligada' : 'Correção automática do CI desligada')); } }, { label: 'Mesclar quando pronto', sub: 'Ativa o auto-merge (squash) no GitHub', f: async () => { const r = await api('/api/pr', { body: { action: 'merge-auto' }, root: L.root, dir: L.snap.workRoot || L.root }); toast(r.ok ? 'Auto-merge ativado' : (r.error || r.output)); } }]);
  });
}
// The toolbar buttons of the Agents and Processes panes show how many are at work.
function renderRunBadges(v) {
  const L = v.sid && S.live[v.sid]; if (!elOf(v)) return;
  const n = { agents: (L?.snap?.agents || []).filter(a => a.state === 'running').length, procs: (L?.snap?.procs || []).filter(p => p.state === 'running').length };
  for (const k of ['agents', 'procs']) { const b = $(`.vbar [data-a="${k}"] i`, elOf(v)); if (b) { b.textContent = n[k] || ''; b.parentElement.classList.toggle('working', !!n[k]); } }
}
// the session's context: its model's window (or less, if set), from the server
const ctxLimit = L => L?.snap?.contextLimit || S.settings.contextLimit || 1000000;
function renderChangesBadge(v) {
  v = freshView(v);
  renderRunBadges(v);
  const L = v.sid && S.live[v.sid]; const b = $('.chg', elOf(v)); if (!b) return;
  const c = L?.changes; b.innerHTML = (c?.isGit || c?.local) && (c.added || c.removed) ? `<span class="add">+${c.added || 0}</span><span class="del">−${c.removed || 0}</span>` : I.diff;
}
