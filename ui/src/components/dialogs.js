// dialogs — rewind (turn by turn), context & cost, command palette, shortcuts, mission start,
// independent review. All modal, focus-trapped (dialog.js).
import { $, $$, h, esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { money, ktok } from '../core/fmt.js';
import { effect } from '../core/signals.js';
import { api } from '../core/api.js';
import { overlay } from './dialog.js';
import { ui, pref, liveOf, busy, focusedView, cache, projectLabel, openSession, newSessionScreen, curRoot, curDir } from '../core/store.js';
import { togglePane } from '../panes/index.js';
import { MODE_INFO } from './composer.js';
import { toast } from './toast.js';

// ---------------------------------------------------------------- rewind
export function rewindDialog(v, preselect) {
  const L = v?.sid ? liveOf(v.sid) : (focusedView()?.sid ? liveOf(focusedView().sid) : null);
  if (!L?.snap.get()?.turn) return toast(t('rewind.nothing'));
  if (busy(L)) return toast(t('rewind.stopFirst'));
  const s = L.snap.get();
  const turns = s.turnStarts.map((idx, i) => ({
    n: i + 1,
    text: ((s.messages[idx]?.content || []).find(b => b.type === 'text')?.text) || '',
    files: (s.checkpoints[i + 1] || []).length
  })).reverse();
  const dlg = h(`<div class="dialog" aria-label="${esc(t('rewind.title'))}"><div class="dh">${t('rewind.title')}</div><div class="faint dialog-sub">${t('rewind.pickPoint')}</div><div class="db"></div></div>`);
  const close = overlay(dlg);
  const db = dlg.querySelector('.db');
  let k = 0;
  const list = () => {
    db.innerHTML = '';
    turns.forEach((tu, i) => {
      const it = h(`<div class="it${i === k ? ' k' : ''}" tabindex="0" role="button"><span class="faint">${tu.n}</span><span class="trunc grow">${esc(tu.text.split('\n')[0] || '—')}</span><span class="d">${tu.files ? tu.files + ' ' + t('common.fileCount') : t('rewind.noChanges')}</span></div>`);
      it.onclick = () => choose(tu);
      db.appendChild(it);
    });
  };
  const choose = tu => {
    db.innerHTML = `<div class="faint dialog-pick">${t('common.turn')} ${tu.n}: ${esc(tu.text.split('\n')[0].slice(0, 90))}</div>`;
    const opts = [
      tu.files && { l: 'rewind.codeAndChat', f: () => run({ turn: tu.n, code: true, conversation: true }) },
      { l: 'rewind.chatOnly', d: 'rewind.keepsCode', f: () => run({ turn: tu.n, code: false, conversation: true }) },
      tu.files && { l: 'rewind.codeOnly', d: 'rewind.keepsChat', f: () => run({ turn: tu.n, code: true, conversation: false }) },
      tu.n > 1 && { l: 'rewind.summarize', d: 'rewind.compacts', f: () => summarize(tu.n) },
      { l: 'rewind.neverMind', f: list },
    ].filter(Boolean);
    opts.forEach((o, i) => {
      const it = h(`<div class="it" tabindex="0" role="button">${t(o.l)}<span class="d">${o.d ? t(o.d) + ' · ' : ''}${i + 1}</span></div>`);
      it.onclick = o.f;
      db.appendChild(it);
    });
  };
  const run = async body => {
    const r = await api(`/api/session/${L.id}/rewind`, { body, root: L.root }).catch(e => ({ error: String(e) }));
    close();
    if (r.error) return toast(r.error);
    L.snap.set(r.session || L.snap.get());
    import('../core/store.js').then(st => st.refreshChanges(L));
    if (body.conversation && r.prompt && v?._promptEl) { v._promptEl.value = r.prompt; v._promptEl.focus(); }
    toast(t(body.code && body.conversation ? 'rewind.doneBoth' : body.code ? 'rewind.doneCode' : 'rewind.doneChat'));
  };
  const summarize = async n => {
    db.innerHTML = `<div class="status dialog-loading"><span class="spin"></span>${t('rewind.summarizing')}</div>`;
    const r = await api(`/api/session/${L.id}/summarize`, { body: { turn: n }, root: L.root }).catch(e => ({ error: String(e) }));
    close();
    if (r.error) return toast(r.error);
    L.snap.set(r.session || L.snap.get());
    toast(t('chat.compactedToast'));
  };
  if (preselect) choose(turns.find(x => x.n === preselect) || turns[0]); else list();
}

// ---------------------------------------------------------------- context & cost
export async function contextDialog() {
  const v = focusedView();
  const L = v?.sid ? liveOf(v.sid) : null;
  if (!L) return;
  const r = await api(`/api/session/${L.id}/context`, { root: L.root }).catch(e => ({ error: String(e) }));
  if (r.error) return toast(r.error);
  const u = L.snap.get()?.usage || {};
  const dlg = h(`<div class="dialog" aria-label="${esc(t('context.title'))}"><div class="dh">${t('context.title')}</div><div class="db dialog-body-pad"></div></div>`);
  overlay(dlg);
  const barPct = x => Math.round(x / (r.total || 1) * 100);
  dlg.querySelector('.db').innerHTML = `
    <div class="context-bar">${(r.categories || []).map((c, i) => `<i class="chart-seg cat-${i % 6 + 1}" style="--w:${barPct(c.tokens)}%"></i>`).join('')}</div>
    <table class="data-table">${(r.categories || []).map((c, i) => `<tr><td><span class="chart-dot cat-${i % 6 + 1}"></span> ${esc(t(c.name))}</td><td class="mono">${ktok(c.tokens)}</td><td class="faint">${barPct(c.tokens)}%</td></tr>`).join('')}
    <tr><td><b>${t('context.total')}</b></td><td class="mono"><b>${ktok(r.total)}</b></td><td class="faint">${t('common.of')} ${ktok(r.limit)}</td></tr></table>
    <h4 class="context-h4">${t('context.thisSession')}</h4>
    <div class="kv-row"><span>${t('context.input')}</span><span class="mono">${ktok((u.input || 0) + (u.cacheRead || 0) + (u.cacheWrite || 0))} tokens</span></div>
    <div class="kv-row"><span>${t('context.output')}</span><span class="mono">${ktok(u.output)} tokens</span></div>
    <div class="kv-row"><span>${t('context.cost')}</span><span class="mono">${money(L.snap.get()?.cost)}</span></div>
    <div class="faint mt-4">${t('context.hint')}</div>`;
}

// ---------------------------------------------------------------- palette
export function palette(kind) {
  const items = paletteItems(kind);
  const dlg = h(`<div class="dialog" role="dialog" aria-label="${esc(t('palette.title'))}"><input class="q" placeholder="${esc(t(kind === 'sessions' ? 'palette.searchSessions' : 'palette.searchAll'))}"><div class="db palette-list"></div></div>`);
  const close = overlay(dlg);
  const q = dlg.querySelector('input'), db = dlg.querySelector('.db');
  let k = 0, list = [];
  const draw = () => {
    const s = q.value.toLowerCase();
    list = items.filter(x => t(x.label).toLowerCase().includes(s) || (x.d || '').toLowerCase().includes(s)).slice(0, 80);
    k = Math.min(k, Math.max(0, list.length - 1));
    db.innerHTML = '';
    list.forEach((x, i) => {
      const it = h(`<div class="it${i === k ? ' k' : ''}" tabindex="0" role="button"><span class="trunc">${esc(t(x.label))}</span><span class="d">${esc(x.d || '')}</span></div>`);
      it.onclick = () => { close(); x.f(); };
      db.appendChild(it);
    });
    db.children[k]?.scrollIntoView({ block: 'nearest' });
  };
  q.oninput = () => { k = 0; draw(); };
  q.onkeydown = e => {
    if (e.key === 'ArrowDown') { k = Math.min(k + 1, list.length - 1); draw(); e.preventDefault(); }
    else if (e.key === 'ArrowUp') { k = Math.max(0, k - 1); draw(); e.preventDefault(); }
    else if (e.key === 'Enter' && list[k]) { close(); list[k].f(); }
  };
  draw(); q.focus();
}
function paletteItems(kind) {
  const sess = cache.sessions.get().map(s => ({ label: s.title || t('sidebar.newSession'), d: projectLabel(s.root) + ' · ' + s.updated, f: () => openSession(s.root, s.id) }));
  if (kind === 'sessions') return sess;
  const v = focusedView();
  const pane = id => () => togglePane(id, true);
  return [
    { label: t('sidebar.newSession'), d: 'Ctrl N', f: () => newSessionScreen() },
    { label: t('nav.scheduled'), f: () => import('../pages/scheduled.js').then(p => p.openScheduled()) },
    { label: t('nav.customize'), f: () => import('../pages/customize.js').then(p => p.openCustomize()) },
    { label: t('nav.settings'), d: 'Ctrl ,', f: () => import('../pages/settings.js').then(p => p.openSettings()) },
    { label: t('settings.providers'), f: () => import('../pages/settings.js').then(p => p.openSettings('providers')) },
    { label: t('rewind.title') + '…', d: 'Esc Esc', f: () => rewindDialog(v) },
    { label: t('side.title'), d: 'Ctrl ;', f: () => import('./view.js').then(m => m.openSideChat(v)) },
    { label: t('pane.changes'), d: 'Ctrl Shift D', f: pane('changes') },
    { label: t('pane.terminal'), d: 'Ctrl `', f: pane('terminal') },
    { label: t('pane.files'), f: pane('files') },
    { label: t('pane.browser'), d: 'Ctrl Shift B', f: pane('browser') },
    { label: t('pane.tasks'), f: pane('tasks') }, { label: t('pane.plan'), f: pane('plan') },
    { label: t('pane.procs'), f: pane('procs') }, { label: t('pane.agents'), f: pane('agents') },
    { label: t('pane.oracle'), f: pane('oracle') }, { label: t('pane.memory'), f: pane('memory') },
    { label: t('pane.rules'), f: pane('rules') }, { label: t('pane.journal'), f: pane('journal') },
    { label: t('pane.vices'), f: pane('vices') }, { label: t('pane.map'), f: pane('map') },
    ...Object.entries(MODE_INFO).map(([k, [l]]) => ({
      label: t('mode.changedTo') + t(l),
      f: () => { const L = v?.sid ? liveOf(v.sid) : null; if (L) import('../core/store.js').then(st => st.setSessionOpts(L, { mode: k })); }
    })),
    { label: t('view.viewMode') + ': ' + t('view.normal'), f: () => pref('viewMode', 'normal') },
    { label: t('view.viewMode') + ': ' + t('view.thinking'), f: () => pref('viewMode', 'thinking') },
    { label: t('view.viewMode') + ': ' + t('view.verbose'), d: 'Ctrl O', f: () => pref('viewMode', 'verbose') },
    { label: t('palette.toggleSidebar'), d: 'Ctrl B', f: () => document.body.classList.toggle('sidebar-collapsed') },
    { label: t('palette.shortcuts'), d: 'Ctrl /', f: shortcutsDialog },
    ...sess,
  ];
}

export function shortcutsDialog() {
  const rows = [
    ['sidebar.newSession', 'Ctrl N'], ['palette.title', 'Ctrl K'], ['palette.switchSession', 'Ctrl Tab'],
    ['palette.nextPrev', 'Ctrl Shift ] / ['], ['palette.closeSession', 'Ctrl W'], ['composer.stop', 'Esc'],
    ['rewind.title', 'Esc Esc'], ['mode.cycle', 'Shift Tab'], ['mode.menu', 'Ctrl Shift M'],
    ['model.menu', 'Ctrl Shift I'], ['model.effortMenu', 'Ctrl Shift E'], ['pane.changes', 'Ctrl Shift D'],
    ['pane.browser', 'Ctrl Shift B'], ['pane.terminal', 'Ctrl `'], ['palette.closePane', 'Ctrl \\'],
    ['view.exitSplit', 'Ctrl Alt W'], ['side.title', 'Ctrl ;'], ['view.viewMode', 'Ctrl O'],
    ['palette.toggleSidebar', 'Ctrl B'], ['perm.hotkeys', 'Y / A / N'], ['composer.history', '↑ / ↓'],
    ['menu.itemByNumber', '1–9'], ['changes.sendComments', 'Ctrl Enter'], ['sidebar.openSplit', 'Alt + click'],
  ];
  const dlg = h(`<div class="dialog" aria-label="${esc(t('palette.shortcuts'))}"><div class="dh">${t('palette.shortcuts')}</div><div class="db"><div class="sc-grid">${rows.map(([a, b]) => `<span>${t(a)}</span><span class="kbd">${b}</span>`).join('')}</div></div></div>`);
  overlay(dlg);
}

// ---------------------------------------------------------------- mission
export function missionDialog(v) {
  const L = v?.sid ? liveOf(v.sid) : null;
  const P = v?._promptEl;
  const d = h(`<div class="dialog dialog-mission" aria-label="${esc(t('mission.start'))}"><h3 class="mission-title">${t('mission.start')}</h3>
    <div class="faint mission-explainer">${t('mission.explainer')}</div>
    <textarea class="doc-editor mission-goal" placeholder="${esc(t('mission.goalPlaceholder'))}">${esc(P?.value || '')}</textarea>
    <div class="crit-sentence mission-crit-sentence">${t('mission.maxSpend')} <input class="slot mono mb" value="2.00"> ${t('common.and')} <input class="slot mono mt" value="10"> ${t('mission.turns')}.</div>
    <div class="mission-crit faint mt-3"></div>
    <div class="row mission-actions"><button class="btn cancel">${t('common.cancel')}</button><button class="btn pri go">${t('mission.startBtn')}</button></div></div>`);
  const close = overlay(d);
  api('/api/oracle/spec', { root: curRoot() }).then(r => {
    const n = (r.spec?.checks || []).length;
    d.querySelector('.mission-crit').innerHTML = n
      ? `✓ ${t('mission.criteriaCount', { count: n })}`
      : `<span class="del">${t('mission.noCriteria')}</span> <a href="#" class="defc">${t('mission.defineCriteria')}</a>`;
    const dc = d.querySelector('.defc');
    if (dc) dc.onclick = e => { e.preventDefault(); close(); togglePane('oracle', true); };
  }).catch(() => { });
  d.querySelector('.cancel').onclick = close;
  d.querySelector('.go').onclick = async () => {
    const goal = d.querySelector('textarea').value.trim();
    if (!goal) return toast(t('mission.describeGoal'));
    if (!L) { if (P) { P.value = goal; v._sendFn?.(); } return close(); }
    const r = await api(`/api/session/${L.id}/mission`, { body: { goal, budget: parseFloat(d.querySelector('.mb').value) || 2, maxTurns: parseInt(d.querySelector('.mt').value) || 10 }, root: L.root });
    if (!r.ok) return toast(r.error || String(r));
    if (P) P.value = '';
    close();
  };
  setTimeout(() => d.querySelector('textarea').focus(), 30);
}

// ---------------------------------------------------------------- independent review
export async function runReview(v, focus) {
  const L = v?.sid ? liveOf(v.sid) : null;
  if (!L) return toast(t('review.needSession'));
  if (busy(L)) return toast(t('review.waitIdle'));
  const live = v.el?.querySelector('.live-strip');
  if (live) live.innerHTML = `<div class="status" role="status"><span class="spin"></span>${t('review.running')}</div>`;
  const r = await api(`/api/session/${L.id}/review`, { body: { focus: focus || '' }, root: L.root }).catch(e => ({ error: String(e) }));
  if (!r.ok) { toast(r.error || t('review.failed')); }
}
