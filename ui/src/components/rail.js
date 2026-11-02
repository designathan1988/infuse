// rail — the session-details column: bans, diagnosis, plan, changes, context, environment, pins,
// running (procs/agents), mission, done-criteria (Done?) with the executor button.
import { $, $$, esc, h } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { money, ktok, elapsed, fmtDur } from '../core/fmt.js';
import { md } from '../core/markdown.js';
import { I } from '../core/icons.js';
import { effect } from '../core/signals.js';
import { api } from '../core/api.js';
import { pref, liveOf, busy, cache, projectLabel, ctxLimit, refreshChanges } from '../core/store.js';
import { PLAN_ICON } from './cards-shared.js';
import { costHTML } from './cost.js';

const MSTATE = { running: ['warn', 'mission.stateRunning'], aprovada: ['ok', 'mission.stateApproved'], orcamento: ['bad', 'mission.stateBudget'], impasse: ['bad', 'mission.stateImpasse'], parada: ['', 'mission.stateStopped'] };
const CHG_ST = { A: ['changes.new', 'ok'], D: ['changes.deleted', 'bad'] };

export function mountRail(v, el) {
  effect(() => {
    const ctx = railContext(v, el);
    if (!ctx) { el.innerHTML = ''; return; }
    el.innerHTML = railMarkup(ctx);
    bindRailFolds(ctx);
    bindRailActions(ctx);
  });
}

function railContext(v, el) {
  const L = v.sid ? liveOf(v.sid) : null;
  const snap = L?.snap.get();
  if (!snap) return null;
  const changes = L.changes.get() || {};
  const files = changes.files || [];
  const tasks = snap.tasks || [];
  const pct = Math.min(100, Math.round(((L.lastIn || 0) / ctxLimit(L)) * 100));
  return { v, el, L, snap, changes, files, tasks, pct, bans: L.bans.get(), prefsRailFolds: pref('railFolds') };
}

function railMarkup(ctx) {
  const { L, snap, changes: c, files, tasks, pct, bans } = ctx;
  return `
      ${bansCard(L, bans)}
      ${diagnosisCard(L, snap)}
      ${planCard(L, snap, tasks)}
      <div class="rail-card" data-card="changes"><div class="card-head">${t('rail.changes')}<span class="r mono">${c.isGit === false && !c.local ? t('changes.noGit') : `<span class="add">+${c.added || 0}</span> <span class="del">−${c.removed || 0}</span>`}</span></div>${c.local ? `<div class="segrow">${scopeSwitch()}</div>` : ''}
        ${files.length ? files.slice(0, 8).map(f => `<div class="li${f.status === 'D' ? ' gone' : ''}" data-f="${esc(f.path)}" tabindex="0" role="button"><span class="n" title="${esc(f.path)}">${esc(f.path)}</span>${CHG_ST[f.status] ? `<span class="tag ${CHG_ST[f.status][1]}">${t(CHG_ST[f.status][0])}</span>` : ''}<span class="add">+${f.added}</span><span class="del">−${f.removed}</span></div>`).join('') + (files.length > 8 ? `<div class="faint rail-more">+${files.length - 8} ${t('common.files')}</div>` : '') : `<div class="faint">${t('changes.none')}</div>`}
        <div class="prline"></div></div>
      <div class="rail-card ctxcard clickable" data-card="context" tabindex="0" role="button"><div class="card-head">${t('rail.context')}<span class="r">${pct}%</span></div><div class="bar100"><i style="--w:${pct}%"></i></div><div class="faint">${ktok(L.lastIn)} / ${ktok(ctxLimit(L))} tokens · ${costHTML(snap, snap.cost, true)}</div></div>
      <div class="rail-card" data-card="env"><div class="card-head">${t('rail.environment')}</div>
        <div class="kv-row"><span>${t('env.where')}</span><span>${t('env.thisMachine')}</span></div>
        <div class="kv-row"><span>${t('env.folder')}</span><span title="${esc(snap.workRoot)}">${esc((snap.workRoot || L.root || '').replace(/^.*[\\/]/, ''))}</span></div>
        ${snap.branch ? `<div class="kv-row"><span>${t('env.branch')}</span><span class="mono">${esc(snap.branch)}</span></div>` : ''}
        ${snap.worktree ? `<div class="kv-row"><span>${t('env.worktree')}</span><span>${t('env.isolated')}</span></div>` : ''}
        <div class="kv-row"><span>${t('env.model')}</span><span class="mono">${esc(snap.model || '')}</span></div></div>
      ${(snap.pinnedMsgs || []).length ? `<div class="rail-card pins" data-card="pins"><div class="card-head">${t('rail.pinnedMsgs')}<span class="r">${snap.pinnedMsgs.length}</span></div>${snap.pinnedMsgs.map(i => `<div class="li pinli" data-goto="${i}" tabindex="0" role="button"><span class="n">${esc(((snap.messages[i] || { content: [] }).content || []).filter(b => b.type === 'text').map(b => b.display || b.text).join('\n').split('\n')[0].slice(0, 80) || '…')}</span></div>`).join('')}</div>` : ''}
      ${runningCard(L, snap)}
      ${missionCard(L, snap)}
      <div class="rail-card" data-card="done"><div class="card-head">${t('rail.done')}<span class="r">${L.verdict.get() ? `<span class="tag ${{ aprovado: 'ok', reprovado: 'bad' }[L.verdict.get().verdict] || 'warn'}">${esc(t('verdict.' + L.verdict.get().verdict))}</span>` : `<span class="faint">${t('verdict.notChecked')}</span>`}</span></div>
        <div class="faint mb-3">${L.verdict.get() ? esc(L.verdict.get().summary || '') : t('verdict.hint')}</div>
        <div class="row"><button class="btn sm line oraclego">${t('rail.doneBtn')}</button><button class="btn sm" data-open="oracle">${t('rail.criteriaBtn')}</button><button class="btn sm revgo" title="${esc(t('review.aria'))}">${t('rail.reviewBtn')}</button></div></div>`;
}

function bindRailFolds(ctx) {
  const { el, prefsRailFolds } = ctx;
  $$('.rail-card[data-card]', el).forEach(card => {
    const key = card.dataset.card, ch = card.querySelector('.card-head');
    if (!ch || ch.querySelector('.fold')) return;
    ch.insertAdjacentHTML('afterbegin', `<span class="fold" aria-hidden="true">›</span>`);
    card.classList.toggle('folded', !!prefsRailFolds[key]);
    ch.onclick = e => {
      if (e.target.closest('button, a, .tag, input, .seg2')) return;
      e.stopPropagation();
      const folds = { ...pref('railFolds') };
      folds[key] = !folds[key];
      import('../core/store.js').then(st => st.pref('railFolds', folds));
      card.classList.toggle('folded', !!folds[key]);
    };
  });
}

function bindRailActions(ctx) {
  const { v, el, L, snap } = ctx;
  el.querySelector('.revgo')?.addEventListener('click', () => import('./dialogs.js').then(d => d.runReview(v)));
  $$('.card.bans [data-ban]', el).forEach(b => b.onclick = async e => {
    e.stopPropagation();
    const r = await api(`/api/session/${L.id}/bans`, { body: { action: 'set', index: +b.dataset.ban, off: b.dataset.off === '1' }, root: L.root });
    L.bans.set(r.bans || L.bans.get());
  });
  $$('[data-pane]', el).forEach(x => x.onclick = () => import('../panes/index.js').then(p => p.togglePane(x.dataset.pane, true)));
  bindScope(el);
  $$('[data-goto]', el).forEach(x => x.onclick = () => {
    const m = v.el?.querySelector(`[data-mi="${x.dataset.goto}"]`);
    if (m) { v.stick = false; m.scrollIntoView({ behavior: 'smooth', block: 'center' }); m.classList.add('flash-once'); setTimeout(() => m.classList.remove('flash-once'), 1400); }
  });
  const ms = el.querySelector('.mission-stop');
  if (ms) ms.onclick = () => api(`/api/session/${L.id}/mission`, { body: { action: 'stop' }, root: L.root }).catch(() => { });
  const mn = el.querySelector('.mission-new');
  if (mn) mn.onclick = () => import('./dialogs.js').then(d => d.missionDialog(v));
  el.querySelector('.oraclego').onclick = () => {
    import('../panes/index.js').then(p => {
      p.togglePane('oracle', true);
      setTimeout(() => {
        const out = document.querySelector('.pane[data-id="oracle"] .verdict-host');
        if (out) p.paneCall('oracle', 'run', out);
      }, 400);
    });
  };
  $$('[data-f]', el).forEach(x => x.onclick = () => import('../panes/index.js').then(p => { p.togglePane('changes', true); setTimeout(() => p.paneCall('changes', 'select', x.dataset.f), 50); }));
  el.querySelector('.ctxcard').onclick = () => import('./dialogs.js').then(d => d.contextDialog());
  $$('[data-open]', el).forEach(x => x.onclick = () => import('../panes/index.js').then(p => p.togglePane(x.dataset.open, true)));
  $$('.card.diag [data-path]', el).forEach(x => x.onclick = e => { e.stopPropagation(); import('../panes/index.js').then(p => p.openFileInPane(x.dataset.path)); });
  renderPR(v, el.querySelector('.prline'), L, snap);
}

function scopeSwitch() {
  const s = pref('chgScope');
  return `<span class="seg2" role="group">${[['all', 'changes.scopeAll'], ['turn', 'changes.scopeTurn']].map(([k, l]) => `<button data-scope="${k}" class="${s === k ? 'on' : ''}" title="${esc(t(k === 'all' ? 'changes.scopeAllHint' : 'changes.scopeTurnHint'))}">${t(l)}</button>`).join('')}</span>`;
}
function bindScope(el) {
  $$('.seg2 button', el).forEach(b => b.onclick = e => {
    e.stopPropagation();
    import('../core/store.js').then(st => {
      st.pref('chgScope', b.dataset.scope);
      for (const v of st.ui.views.get()) { const L = v?.sid && st.liveOf(v.sid); if (L) st.refreshChanges(L); }
    });
    import('../panes/index.js').then(p => p.paneRefresh('changes'));
  });
}

function bansCard(L, bans) {
  if (bans === undefined) {
    api(`/api/session/${L.id}/bans`, { root: L.root }).then(r => L.bans.set(r.bans || [])).catch(() => L.bans.set([]));
    return '';
  }
  const bs = bans || [];
  if (!bs.length) return '';
  const on = bs.filter(b => !b.off).length;
  return `<div class="rail-card bans-card" data-card="bans"><div class="card-head">${t('rail.bans')}<span class="r">${on}</span></div>
    ${bs.map((b, i) => `<div class="ban${b.off ? ' off' : ''}"><q>${esc(b.quote)}</q><span class="faint">${esc(t('ban.kind.' + (b.kind || 'action')))}: <code>${esc(b.value)}</code> · ${t('common.turn')} ${b.turn}</span><button class="btn sm line" data-ban="${i}" data-off="${b.off ? '0' : '1'}">${t(b.off ? 'ban.restore' : 'ban.lift')}</button></div>`).join('')}</div>`;
}

function diagnosisCard(L, snap) {
  const ds = snap.diagnoses || [];
  if (!ds.length) return '';
  const d = ds[ds.length - 1];
  const tag = d.turn !== snap.turn ? `<span class="tag">${t('common.turn')} ${d.turn}</span>` : d.continuing ? `<span class="tag">${t('diag.continuation')}</span>` : d.redo_text ? `<span class="tag warn" title="${esc(d.redo_text)}">${t('diag.redone')}</span>` : `<span class="tag ok">${t('diag.checked')}</span>`;
  const row = (k, val) => `<div class="dg"><div class="dk">${t(k)}</div><div class="dv markdown sel">${md(val || '')}</div></div>`;
  return `<div class="rail-card diag-card" data-card="diagnosis"><div class="card-head">${t('diag.title')}<span class="r">${tag}</span></div>
    ${row('diag.problem', d.problem)}${row('diag.rootCause', d.root_cause)}${row('diag.approach', d.approach)}${row('diag.why', d.why_cause)}
    <div class="dk">${t('diag.evidence')} <span class="faint">(${(d.evidence || []).length}, ${t('diag.checkedInFile')})</span></div>
    ${(d.evidence || []).map(e => `<div class="dev"><span class="path" data-path="${esc(e.file)}" tabindex="0" role="button">${esc(e.file)}</span><q>${esc(e.quote)}</q><span class="faint">${esc(e.supports || '')}</span></div>`).join('')}
    ${ds.length > 1 ? `<div class="faint dhist">${ds.length - 1} ${t('diag.previous')}</div>` : ''}</div>`;
}

function planCard(L, snap, tasks) {
  if (!tasks.length) return '';
  const done = tasks.filter(x => x.status === 'done').length;
  const pct = Math.round(done / tasks.length * 100);
  const live = busy(L), open = tasks.filter(x => x.status !== 'done' && x.status !== 'blocked').length;
  const head = !live && open ? `<span class="r warn">${t('plan.stopped')} · ${done} ${t('common.of')} ${tasks.length}</span>` : `<span class="r">${done} ${t('common.of')} ${tasks.length}</span>`;
  return `<div class="rail-card plan-card${live ? '' : ' idle'}" data-card="plan"><div class="card-head">${t('plan.title')}${head}</div><div class="bar100 pbar"><i style="--w:${pct}%"></i></div>
    ${tasks.map(x => {
    const st = x.status === 'in_progress' && !live ? 'stopped' : x.status;
    const [ic, cls] = PLAN_ICON[st] || PLAN_ICON.pending;
    return `<div class="plan-task ${st}"><span class="result-badge rst ${cls}">${ic}</span><span class="grow"><span class="tt">${esc(x.text)}</span>${x.proof ? `<span class="pf">${t('plan.proof')}: <code>${esc(x.proof)}</code></span>` : ''}${x.note ? `<span class="pn">${esc(t(x.note))}</span>` : ''}</span></div>`;
  }).join('')}</div>`;
}

const agentHeadText = a => a ? `${a.steps} ${t('common.steps')} · ${a.state === 'running' ? elapsed(a.started) : fmtDur((a.ended || Date.now()) - a.started)}` : '';
function runningCard(L, snap) {
  const ps = snap.procs || [], ag = snap.agents || [];
  const live = ps.filter(p => p.state === 'running'), working = ag.filter(a => a.state === 'running');
  if (!ps.length && !working.length) return '';
  return `<div class="rail-card running-card" data-card="running"><div class="card-head">${t('rail.running')}<span class="r">${live.length + working.length}</span></div>
    ${live.map(p => `<div class="li" data-pane="procs" tabindex="0" role="button"><span class="dot run"></span><span class="n mono" title="${esc(p.command)}">${esc(p.description || p.command)}</span><span class="faint">${elapsed(p.started)}</span></div>`).join('')}
    ${working.map(a => `<div class="li" data-pane="agents" tabindex="0" role="button"><span class="spin sm"></span><span class="n" title="${esc(a.task)}">${esc(a.title)}</span><span class="faint">${agentHeadText(a)}</span></div>`).join('')}
    ${ps.length > live.length ? `<div class="li faint" data-pane="procs">${ps.length - live.length} ${t('procs.endedCount')}</div>` : ''}</div>`;
}

function missionCard(L, snap) {
  const m = snap.mission;
  if (!m) return '';
  const spent = Math.max(0, (snap.cost || 0) - (m.startCost || 0));
  const turns = (m.steps || []).length + (m.state === 'running' ? 1 : 0);
  const [cls, label] = MSTATE[m.state] || ['', m.state];
  const pct = Math.min(100, Math.round(spent / (m.budgetUSD || 1) * 100));
  const last = (m.steps || []).slice(-1)[0];
  return `<div class="rail-card mission-card" data-card="mission"><div class="card-head">${t('mission.title')}<span class="r"><span class="tag ${cls}">${t(label)}</span></span></div>
    <div class="faint trunc" title="${esc(m.goal)}">${esc(m.goal)}</div>
    <div class="bar100 mission-bar"><i style="--w:${pct}%"></i></div><div class="faint">${t('mission.turnOf', { cur: Math.min(turns, m.maxTurns), max: m.maxTurns })} · ${money(spent)} ${t('common.of')} ${money(m.budgetUSD)}</div>
    ${m.noPrice && m.state === 'running' ? `<div class="faint mt-2">${t('mission.noPrice')}</div>` : ''}
    ${m.reason ? `<div class="mt-3">${esc(m.reason)}</div>` : ''}
    ${m.impasse ? `<div class="mimp"><b>${t('mission.impasse')}:</b> ${esc(m.impasse.reason)}${(m.impasse.tried || []).length ? `<div>${t('mission.tried')}: ${m.impasse.tried.map(esc).join('; ')}</div>` : ''}${m.impasse.evidence ? `<div class="mono">${esc(m.impasse.evidence)}</div>` : ''}<div><b>${t('mission.needs')}:</b> ${esc(m.impasse.needs)}</div></div>` : ''}
    ${last?.auditor ? `<div class="maud" title="${esc(t('mission.auditorHint'))}">${esc(last.auditor)}</div>` : ''}
    ${(m.steps || []).length ? `<details class="details-fold"><summary>${t('mission.timeline')} (${m.steps.length})</summary>${m.steps.map(st => `<div class="mission-step"><span class="tag ${{ aprovado: 'ok', reprovado: 'bad' }[st.verdict] || 'warn'}">${esc(t('verdict.' + (st.verdict || '—')))}</span><span class="faint">${t('common.turn')} ${st.turn} · ${new Date(st.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} · ${money(st.cost)}</span><div class="mono faint">${esc(st.summary || '')}</div>${st.auditor ? `<div class="maud">${esc(st.auditor)}</div>` : ''}</div>`).join('')}</details>` : ''}
    <div class="row mt-3">${m.state === 'running' ? `<button class="btn sm mission-stop">${t('mission.stop')}</button>` : `<button class="btn sm mission-new">${t('mission.new')}</button>`}</div></div>`;
}

// PR status (rail): create/menu/auto-fix, polled at most once a minute per session.
async function renderPR(v, el, L, snap) {
  if (!el || !L.changes.get()?.isGit) return;
  if (!L.pr || Date.now() - (L.prAt || 0) > 60000) {
    L.pr = await api('/api/pr', { root: L.root, dir: snap.workRoot || L.root }).catch(() => ({}));
    L.prAt = Date.now();
  }
  const pr = L.pr?.pr;
  if (!L.pr?.gh) {
    el.innerHTML = `<div class="row pr-note"><span class="faint grow">${t('pr.needsGh')}</span><button class="btn sm" data-gh-install>${t('pr.installGh')}</button></div>`;
    el.querySelector('[data-gh-install]').onclick = () => {
      import('../panes/index.js').then(p => {
        p.togglePane('terminal', true);
        setTimeout(() => p.paneCall('terminal', 'openAt', snap.workRoot || L.root, 'winget install --id GitHub.cli -e && gh auth login', 'GitHub CLI'), 300);
      });
    };
    return;
  }
  if (!pr) { el.innerHTML = `<div class="row mt-3"><button class="btn sm line" data-pr-create>${t('pr.create')}</button></div>`; }
  else {
    const checks = pr.statusCheckRollup || [];
    const fail = checks.filter(x => /FAIL|ERROR/i.test(x.conclusion || x.state || '')).length;
    const pend = checks.filter(x => !(x.conclusion || '') && !/SUCCESS/.test(x.state || '')).length;
    el.innerHTML = `<div class="row pr-row"><a href="${esc(pr.url)}" target="_blank" rel="noreferrer">#${pr.number}</a><span class="trunc grow">${esc(pr.title)}</span><span class="tag ${fail ? 'bad' : pend ? 'warn' : 'ok'}">${t(fail ? 'pr.ciFailed' : pend ? 'pr.ciRunning' : 'pr.ciOk')}</span><button class="icon-btn icon-22" data-pr-menu>${I.more}</button></div>`;
  }
  el.querySelectorAll('[data-pr-create],[data-pr-menu]').forEach(b => b.onclick = async e => {
    if (b.dataset.prCreate) {
      b.disabled = true;
      const r = await api('/api/pr', { body: { action: 'create' }, root: L.root, dir: snap.workRoot || L.root });
      import('./toast.js').then(m => m.toast(r.ok ? t('pr.created') : (r.error || r.output || '')));
      L.prAt = 0;
      return;
    }
    const fail = (pr.statusCheckRollup || []).some(x => /FAIL|ERROR/i.test(x.conclusion || x.state || ''));
    import('./menu.js').then(m => m.menu(b, [
      { label: t('pr.openGithub'), f: () => window.open(pr.url, '_blank') },
      fail ? { label: t('pr.fixCiNow'), sub: t('pr.fixCiHint'), f: () => fixCI(v, L, pr) } : null,
      { label: t(L.autofix ? 'pr.autofixOff' : 'pr.autofixOn'), sub: t('pr.autofixHint'), check: !!L.autofix, f: () => { L.autofix = !L.autofix; pref('autofix', { ...pref('autofix'), [L.id]: L.autofix }); } },
      { label: t('pr.autoMerge'), sub: t('pr.autoMergeHint'), f: async () => { const r = await api('/api/pr', { body: { action: 'merge-auto' }, root: L.root, dir: snap.workRoot || L.root }); import('./toast.js').then(m => m.toast(r.ok ? t('pr.autoMergeOn') : (r.error || r.output || ''))); } },
    ], null, { alignRight: true }));
  });
}
async function fixCI(v, L, pr) {
  const r = await api('/api/pr', { body: { action: 'failed-logs' }, root: L.root, dir: L.snap.get().workRoot || L.root });
  if (!r.ok) return import('./toast.js').then(m => m.toast(r.error || r.output || t('pr.noLog')));
  const P = v._promptEl;
  P.value = `${t('pr.ciFailedPrompt', { n: pr.number })}\n\n\`\`\`\n${r.output}\n\`\`\``;
  v._sendFn?.();
}
