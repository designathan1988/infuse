'use strict';
// rail
// Session details rail: environment, PR, CI, change badge.

// ---------------------------------------------------------------- rail (session details)
function renderRail(v) {
  if (!v.el) return; const el = $('.rail', v.el); const L = v.sid && S.live[v.sid]; if (!L?.snap) { el.innerHTML = ''; return; }
  const c = L.changes || {}; const files = c.files || [];
  const pct = Math.min(100, Math.round(((L.lastIn || 0) / ctxLimit(L)) * 100));
  const tasks = L.snap.tasks || [];
  el.innerHTML = `
  ${bansCard(L)}
  ${diagnosisCard(L)}
  ${planCard(L)}
  <div class="card" data-card="changes"><div class="ch">${t('Alterações')}<span class="r mono">${c.isGit === false && !c.local ? t('sem git') : `<span class="add">+${c.added || 0}</span> <span class="del">−${c.removed || 0}</span>`}</span></div>${c.local ? `<div class="segrow">${scopeSwitch()}</div>` : ''}
    ${files.length ? files.slice(0, 8).map(f => `<div class="li${f.status === 'D' ? ' gone' : ''}" data-f="${esc(f.path)}"><span class="n" title="${esc(f.path)}">${esc(f.path)}</span>${chgTag(f)}<span class="add">+${f.added}</span><span class="del">−${f.removed}</span></div>`).join('') + (files.length > 8 ? `<div class="faint" style="padding:3px 4px">+${files.length - 8} ${t('arquivos')}</div>` : '') : `<div class="faint">${t('Nenhuma alteração')}</div>`}
    <div class="prline"></div></div>
  <div class="card ctxcard" data-card="context" style="cursor:pointer"><div class="ch">${t('Contexto')}<span class="r">${pct}%</span></div><div class="bar100"><i style="width:${pct}%"></i></div><div class="faint">${ktok(L.lastIn)} / ${ktok(ctxLimit(L))} tokens · ${money(L.snap.cost)}</div></div>
  <div class="card" data-card="env"><div class="ch">${t('Ambiente')}</div><div class="kv"><span>${t('Onde')}</span><span>${t('Este computador')}</span></div><div class="kv"><span>${t('Pasta')}</span><span title="${esc(L.snap.workRoot)}">${esc(baseName(L.snap.workRoot || L.root))}</span></div>${L.snap.branch ? `<div class="kv"><span>${t('Branch')}</span><span class="mono">${esc(L.snap.branch)}</span></div>` : ''}${L.snap.worktree ? `<div class="kv"><span>${t('Worktree')}</span><span>${t('isolado')}</span></div>` : ''}<div class="kv"><span>${t('Modelo')}</span><span class="mono">${esc(L.snap.model || '')}</span></div></div>
  ${(L.snap.pinnedMsgs || []).length ? `<div class="card pins" data-card="pins"><div class="ch">${t('Mensagens fixadas')}<span class="r">${L.snap.pinnedMsgs.length}</span></div>${L.snap.pinnedMsgs.map(i => `<div class="li pinli" data-goto="${i}"><span class="n">${esc(msgText(L.snap.messages[i] || { content: [] }).split('\n')[0].slice(0, 80) || '…')}</span></div>`).join('')}</div>` : ''}
  ${runningCard(L)}
  ${missionCard(L)}
  <div class="card" data-card="pronto"><div class="ch">${t('Pronto')}<span class="r">${L.verdict ? `<span class="tag ${{ aprovado: 'ok', reprovado: 'bad' }[L.verdict.verdict] || 'warn'}">${esc(t(L.verdict.verdict))}</span>` : `<span class="faint">${t('não verificado')}</span>`}</span></div>
    <div class="faint" style="margin-bottom:6px">${L.verdict ? esc(L.verdict.summary) : t('Só a execução dos critérios decide; a palavra do agente não conta.')}</div>
    <div class="row"><button class="btn sm line oraclego">${t('Pronto?')}</button><button class="btn sm" data-open="oracle">${t('Critérios')}</button><button class="btn sm revgo" title="${esc(t('Um revisor com contexto limpo lê o diff, o plano e os critérios e aponta problemas'))}">${t('Revisar')}</button></div></div>`;
  $('.revgo', el).onclick = () => runReview(v);
  railFolds(v, el);
  $$('.card.diag [data-path]', el).forEach(x => x.onclick = e => { e.stopPropagation(); openFileInPane(x.dataset.path); });
  $$('.card.bans [data-ban]', el).forEach(b => b.onclick = async e => { e.stopPropagation(); const r = await api(`/api/session/${L.id}/bans`, { body: { action: 'set', index: +b.dataset.ban, off: b.dataset.off === '1' }, root: L.root }); L.bans = r.bans || L.bans; renderRail(v); });
  $$('[data-pane]', el).forEach(x => x.onclick = () => togglePane(x.dataset.pane, true));
  bindScope(el);
  $$('[data-goto]', el).forEach(x => x.onclick = () => { const m = $(`[data-mi="${x.dataset.goto}"]`, v.el); if (m) { v.stick = false; m.scrollIntoView({ behavior: 'smooth', block: 'center' }); m.classList.add('flash'); setTimeout(() => m.classList.remove('flash'), 1400); } });
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
  if (!r.ok) return toast(r.error || r.output || t('Não foi possível ler o log do CI'));
  const P = $('.prompt', v.el); P.value = `${t('O CI do PR')} #${pr.number} ${t('falhou. Leia o log abaixo, encontre a causa, corrija, rode localmente o que falhou e faça o commit. Não altere testes para passar.')}\n\n\`\`\`\n${r.output}\n\`\`\``; send(v);
}
setInterval(async () => { // checks PRs of sessions with auto-fix on, every 2 minutes
  for (const v of S.views) {
    const L = v.sid && S.live[v.sid]; if (!L) continue;
    if (L.autofix === undefined) L.autofix = lsGet('autofix:' + L.id, false);
    if (!L.autofix || busy(L)) continue;
    L.prAt = 0; await renderPR(v, $('.prline', v.el) || document.createElement('div'));
    const pr = L.pr?.pr; if (!pr) continue;
    const failed = (pr.statusCheckRollup || []).filter(c => /FAIL|ERROR/i.test(c.conclusion || c.state || ''));
    const sig = pr.number + ':' + failed.map(c => c.name + c.completedAt).join('|');
    if (failed.length && L.autofixSig !== sig) { L.autofixSig = sig; toast(t('CI falhou: enviando o log ao agente')); fixCI(v, L, pr); }
  }
}, 120000);
async function renderPR(v, el) {
  const L = S.live[v.sid]; if (!L || !L.changes?.isGit || !el) return;
  if (!L.pr || Date.now() - L.prAt > 60000) { L.pr = await api('/api/pr', { root: L.root, dir: L.snap.workRoot || L.root }); L.prAt = Date.now(); }
  const pr = L.pr?.pr;
  if (!L.pr?.gh) { // without the GitHub CLI: say so and offer to install it, instead of hiding PRs
    el.innerHTML = `<div class="row" style="margin-top:6px;font-size:12px"><span class="faint grow">${t('Pull requests precisam do GitHub CLI (gh).')}</span><button class="btn sm" data-gh="install">${t('Instalar')}</button></div>`;
    $('[data-gh]', el).onclick = () => { togglePane('terminal', true); setTimeout(() => paneCall('terminal', 'openAt', L.snap.workRoot || L.root, 'winget install --id GitHub.cli -e && gh auth login', 'GitHub CLI'), 300); };
    return;
  }
  if (!pr) { el.innerHTML = `<div class="row" style="margin-top:6px"><button class="btn sm line" data-pr="create">${t('Criar PR')}</button></div>`; }
  else {
    var checks = pr.statusCheckRollup || []; var fail = checks.filter(c => /FAIL|ERROR/i.test(c.conclusion || c.state || '')).length, pend = checks.filter(c => !(c.conclusion || '') && !/SUCCESS/.test(c.state || '')).length;
    el.innerHTML = `<div class="row" style="margin-top:8px;font-size:12px"><a href="${esc(pr.url)}" target="_blank">#${pr.number}</a><span class="trunc grow">${esc(pr.title)}</span><span class="tag ${fail ? 'bad' : pend ? 'warn' : 'ok'}">${fail ? t('CI falhou') : pend ? t('CI rodando') : t('CI ok')}</span><button class="ib" data-pr="menu" style="width:22px;height:22px">${I.more}</button></div>`;
  }
  $$('[data-pr]', el).forEach(b => b.onclick = async e => {
    if (b.dataset.pr === 'create') { b.disabled = true; const r = await api('/api/pr', { body: { action: 'create' }, root: L.root, dir: L.snap.workRoot || L.root }); toast(r.ok ? 'PR criado' : (r.error || r.output)); L.prAt = 0; renderRail(v); }
    else menu(b, [{ label: 'Abrir no GitHub', f: () => { togglePane('browser', true); setTimeout(() => paneCall('browser', 'go', pr.url), 300); } }, fail ? { label: 'Corrigir o CI agora', sub: 'O agente lê o log da falha e corrige', f: () => fixCI(v, L, pr) } : null, { label: L.autofix ? 'Desligar correção automática do CI' : 'Corrigir o CI automaticamente', sub: 'Quando o CI falhar e a sessão estiver parada, o agente recebe o log e corrige', check: !!L.autofix, f: () => { L.autofix = !L.autofix; lsSet('autofix:' + L.id, L.autofix); toast(t(L.autofix ? 'Correção automática do CI ligada' : 'Correção automática do CI desligada')); } }, { label: 'Mesclar quando pronto', sub: 'Ativa o auto-merge (squash) no GitHub', f: async () => { const r = await api('/api/pr', { body: { action: 'merge-auto' }, root: L.root, dir: L.snap.workRoot || L.root }); toast(r.ok ? 'Auto-merge ativado' : (r.error || r.output)); } }]);
  });
}
// The toolbar buttons of the Agents and Processes panes show how many are at work.
function renderRunBadges(v) {
  const L = v.sid && S.live[v.sid]; if (!v.el) return;
  const n = { agents: (L?.snap?.agents || []).filter(a => a.state === 'running').length, procs: (L?.snap?.procs || []).filter(p => p.state === 'running').length };
  for (const k of ['agents', 'procs']) { const b = $(`.vbar [data-a="${k}"] i`, v.el); if (b) { b.textContent = n[k] || ''; b.parentElement.classList.toggle('working', !!n[k]); } }
}
// the session's context: its model's window (or less, if set), from the server
const ctxLimit = L => L?.snap?.contextLimit || S.settings.contextLimit || 1000000;
function renderChangesBadge(v) {
  renderRunBadges(v);
  const L = v.sid && S.live[v.sid]; const b = $('.chg', v.el); if (!b) return;
  const c = L?.changes; b.innerHTML = (c?.isGit || c?.local) && (c.added || c.removed) ? `<span class="add">+${c.added || 0}</span><span class="del">−${c.removed || 0}</span>` : I.diff;
}
