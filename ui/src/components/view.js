// view — one session view (or the new-session home): top bar (title, branch, changes, panes buttons),
// body (transcript + rail), composer. Two views side by side = split view. All rendering is effect-driven.
import { $, $$, h, esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { effect } from '../core/signals.js';
import { api } from '../core/api.js';
import { ui, pref, liveOf, setFocus, openInSplit, exitSplit, openSession } from '../core/store.js';
import { mountTranscript } from './transcript.js';
import { mountComposer } from './composer.js';
import { mountRail } from './rail.js';
import { menu } from './menu.js';
import { archiveSession, deleteSession } from './session-actions.js';
import { openInItems } from './open-in.js';
import { toast } from './toast.js';

const VIEW_MODES = [
  ['normal', 'view.normal', 'view.normalDesc'],
  ['thinking', 'view.thinking', 'view.thinkingDesc'],
  ['verbose', 'view.verbose', 'view.verboseDesc'],
];

export function buildView(v) {
  const el = h(viewMarkup());
  v.el = el;
  bindViewFocus(v, el);
  bindViewBar(v, el);
  el.classList.toggle('no-rail', !pref('rail'));
  el.querySelector('[data-a="rail"]').setAttribute('aria-pressed', String(!!pref('rail')));
  mountTranscript(v, el.querySelector('.chat-scroller'), el.querySelector('.jump-down'));
  mountComposer(v, el.querySelector('.composer'));
  mountRail(v, el.querySelector('.session-rail'));
  effect(() => viewChrome(v, el));
}

function viewMarkup() {
  return `<section class="view" aria-label="${esc(t('view.aria'))}">
    <header class="view-bar">
      <span class="title" tabindex="0" role="button" title="${esc(t('view.renameHint'))}"></span>
      <button class="icon-btn caret" title="${esc(t('view.sessionMenu'))}" aria-haspopup="menu">${I.caret}</button>
      <span class="branch"></span><span class="sp"></span>
      <span class="r row row-tight">
        <button class="changes-btn" data-a="changes" title="${esc(t('changes.aria'))}"></button>
        <button class="icon-btn" data-a="terminal" title="${esc(t('pane.terminal'))} (Ctrl\`)">${I.term}</button>
        <button class="icon-btn" data-a="files" title="${esc(t('pane.files'))}">${I.files}</button>
        <button class="icon-btn" data-a="browser" title="${esc(t('pane.browser'))}">${I.globe}</button>
        <button class="icon-btn run-badge" data-a="agents" title="${esc(t('agents.aria'))}">${I.agents}<i></i></button>
        <button class="icon-btn run-badge" data-a="procs" title="${esc(t('procs.aria'))}">${I.procs}<i></i></button>
        <button class="icon-btn" data-a="rail" title="${esc(t('rail.toggle'))}" aria-pressed="true">${I.rail}</button>
        <button class="icon-btn" data-a="more" title="${esc(t('common.more'))}">${I.vmore}</button>
      </span>
    </header>
    <div class="view-body">
      <div class="view-chat">
        <div class="chapter-toc hidden"><div class="toc-ticks"></div><div class="toc-list"></div></div>
        <div class="chat-scroller"><div class="chat-feed"></div></div>
        <button class="jump-down" title="${esc(t('chat.jumpToEnd'))}" aria-label="${esc(t('chat.jumpToEnd'))}">↓</button>
        <div class="composer"></div>
      </div>
      <aside class="session-rail" aria-label="${esc(t('rail.aria'))}"></aside>
    </div>
  </section>`;
}

function bindViewFocus(v, el) {
  el.addEventListener('mousedown', () => {
    const i = ui.views.get().indexOf(v);
    if (i >= 0 && i !== ui.focus.get()) setFocus(i);
  }, true);
}

function bindViewBar(v, el) {
  const title = el.querySelector('.title');
  title.onclick = () => renameInline(v);
  title.onkeydown = e => { if (e.key === 'Enter') renameInline(v); };
  el.querySelector('.caret').onclick = e => sessionMenu(v, e.currentTarget);
  el.querySelectorAll('[data-a]').forEach(b => b.onclick = e => {
    const a = b.dataset.a;
    if (a === 'rail') {
      const on = !pref('rail'); pref('rail', on);
      el.classList.toggle('no-rail', !on);
      b.setAttribute('aria-pressed', String(on));
    } else if (a === 'more') moreMenu(v, e.currentTarget);
    else import('../panes/index.js').then(p => p.togglePane(a));
  });
}

function viewChrome(v, el) {
  const L = v.sid ? liveOf(v.sid) : null;
  const snap = L?.snap.get();
  const changes = L?.changes.get();
  el.classList.toggle('home', !snap);
  // On the new-session screen there is no session to detail and the rail is hidden by CSS: hide the
  // control with it instead of leaving a visible button that cannot do anything.
  el.querySelector('[data-a="rail"]').classList.toggle('hidden', !snap);
  if (snap) {
    const title = el.querySelector('.title');
    title.textContent = snap.title || t('sidebar.newSession');
    el.querySelector('.branch').innerHTML = snap.branch ? `${snap.worktree ? '⑂ ' : ''}${esc(snap.branch)}` : '';
    el.querySelector('.branch').title = snap.worktree ? t('env.worktree') + ': ' + snap.worktree : '';
  }
  const cb = el.querySelector('.changes-btn');
  cb.innerHTML = (changes?.isGit || changes?.local) && (changes.added || changes.removed)
    ? `<span class="add">+${changes.added || 0}</span><span class="del">−${changes.removed || 0}</span>` : I.diff;
  const nA = (snap?.agents || []).filter(a => a.state === 'running').length;
  const nP = (snap?.procs || []).filter(p => p.state === 'running').length;
  for (const [k, n] of [['agents', nA], ['procs', nP]]) {
    const b = el.querySelector(`[data-a="${k}"] i`);
    if (b) { b.textContent = n || ''; b.parentElement.classList.toggle('working', !!n); }
  }
}

function renameInline(v) {
  const L = v.sid ? liveOf(v.sid) : null;
  if (!L) return;
  const el = v.el.querySelector('.title');
  const cur = L.snap.get()?.title || '';
  const inp = document.createElement('input');
  inp.className = 'title rename-inline'; inp.value = cur;
  el.replaceWith(inp); inp.focus(); inp.select();
  let done = false;
  const fin = ok => {
    if (done) return; done = true;
    const val = inp.value.trim();
    inp.replaceWith(el);
    if (ok && val && val !== cur) import('../core/store.js').then(st => st.setSessionOpts(L, { title: val }));
  };
  inp.onkeydown = e => { e.stopPropagation(); if (e.key === 'Enter') fin(true); if (e.key === 'Escape') fin(false); };
  inp.onblur = () => fin(true);
  inp.onclick = e => e.stopPropagation();
}

function sessionMenu(v, anchor) {
  const L = v.sid ? liveOf(v.sid) : null;
  if (!L) return;
  menu(anchor, [
    {
      label: t('view.viewMode'), items: [
        ...VIEW_MODES.map(([k, l, d]) => ({
          label: t(l), sub: t(d), check: pref('viewMode') === k,
          f: () => {
            const folds = {};
            import('../core/store.js').then(st => { st.pref('fold', {}); st.pref('viewMode', k); });
          }
        })),
        { sep: 1 },
        { label: t('view.expandAll'), f: () => foldAll(v, true) },
        { label: t('view.collapseAll'), f: () => foldAll(v, false) },
      ]
    },
    {
      label: t('view.copyGroup'), items: [
        { label: t('review.title'), sub: t('review.subtitle'), f: () => import('./dialogs.js').then(d => d.runReview(v)) },
        { label: t('mission.start') + '…', sub: t('mission.subtitle'), f: () => import('./dialogs.js').then(d => d.missionDialog(v)) },
        { label: t('view.copyConversation'), sub: t('view.copyConvHint'), f: () => import('./transcript.js').then(m => m.copyConversation(L)) },
        { label: t('view.exportConversation'), sub: t('view.exportConvHint'), f: () => import('./transcript.js').then(m => m.downloadConversation(L)) },
      ]
    },
    {
      label: t('view.openIn'), items: [
        { label: ui.views.get().length > 1 ? t('view.exitSplit') : t('view.splitView'), f: () => ui.views.get().length > 1 ? exitSplit() : openInSplit(L.root, L.id, true) },
        { sep: 1 },
        ...openInItems(L.snap.get()?.workRoot || L.root),
      ]
    },
    { sep: 1 },
    { label: t('action.rename'), f: () => renameInline(v) },
    { label: t('rewind.title') + '…', d: 'Esc Esc', f: () => import('./dialogs.js').then(d => d.rewindDialog(v)) },
    { label: t('side.title'), d: 'Ctrl ;', f: () => openSideChat(v) },
    { sep: 1 },
    { label: t('sidebar.archive'), f: () => archiveSession(L) },
    { label: t('sidebar.delete'), danger: 1, f: () => deleteSession(L) },
  ]);
}
function foldAll(v, open) {
  $$('[data-fold]', v.el).forEach(x => {
    const folds = { ...pref('fold') }; folds[x.dataset.fold] = open;
    import('../core/store.js').then(st => st.pref('fold', folds));
    x.closest('[data-foldable]')?.classList.toggle('open', open);
  });
}
function moreMenu(v, anchor) {
  const p = (id) => () => import('../panes/index.js').then(m => m.togglePane(id, true));
  menu(anchor, [
    { label: t('pane.tasks'), f: p('tasks') },
    { label: t('pane.plan'), f: p('plan') },
    { label: t('pane.procs'), f: p('procs') },
    { label: t('pane.agents'), f: p('agents') },
    {
      label: t('pane.filterGroup'), items: [
        { label: t('pane.oracle'), f: p('oracle') }, { label: t('pane.memory'), f: p('memory') },
        { label: t('pane.rules'), f: p('rules') }, { label: t('pane.journal'), f: p('journal') },
        { label: t('pane.vices'), f: p('vices') }, { label: t('pane.map'), f: p('map') },
      ]
    },
    { sep: 1 },
    { label: pref('rail') ? t('rail.hide') : t('rail.show'), f: () => { const on = !pref('rail'); pref('rail', on); v.el.classList.toggle('no-rail', !on); } },
  ], null, { alignRight: true });
}

// side chat: a parallel question that does not touch the transcript
export function openSideChat(v, first) {
  const L = v.sid ? liveOf(v.sid) : null;
  if (!L) return toast(t('side.needSession'));
  if (v.sideEl) { v.sideEl.remove(); v.sideEl = null; if (!first) return; }
  const el = h(`<div class="side-chat"><div class="sh">${t('side.title')}<span class="faint side-note">${t('side.notInHistory')}</span><span class="grow"></span><button class="icon-btn" aria-label="${esc(t('common.close'))}">${I.close}</button></div><div class="sb" aria-live="polite"></div><textarea rows="1" placeholder="${esc(t('side.placeholder'))}" aria-label="${esc(t('side.title'))}"></textarea></div>`);
  v.sideEl = el; v.el.appendChild(el);
  const sb = el.querySelector('.sb'), ta = el.querySelector('textarea');
  el.querySelector('.sh button').onclick = () => { el.remove(); v.sideEl = null; };
  const ask = async q => {
    sb.appendChild(h(`<div class="sq">${esc(q)}</div>`));
    const w = h('<div class="status"><span class="spin"></span></div>');
    sb.appendChild(w); sb.scrollTop = sb.scrollHeight;
    const { md } = await import('../core/markdown.js');
    const r = await api(`/api/session/${L.id}/btw`, { body: { question: q }, root: L.root }).catch(e => ({ error: String(e) }));
    w.replaceWith(h(`<div class="markdown sel">${r.ok !== undefined && r.answer ? md(r.answer) : `<span class="del">${esc(r.error || t('common.error'))}</span>`}</div>`));
    sb.scrollTop = sb.scrollHeight;
  };
  ta.onkeydown = e => {
    e.stopPropagation();
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); const q = ta.value.trim(); if (q) { ta.value = ''; ask(q); } }
    if (e.key === 'Escape') { el.remove(); v.sideEl = null; }
  };
  ta.focus();
  if (first) ask(first);
}

// The element picked in the browser pane goes into the message being written.
export function insertPick(p) {
  if (!p) return;
  const v = ui.views.get()[ui.focus.get()];
  if (!v?._promptEl) return;
  const P = v._promptEl;
  const st = p.style ? `\n${t('pick.style')}: ${p.style.font} · ${t('pick.color')} ${p.style.color} · ${t('pick.bg')} ${p.style.background} · margin ${p.style.margin} · padding ${p.style.padding}` : '';
  P.value = (P.value.trim() ? P.value.trimEnd() + '\n\n' : '') + `[${t('pick.pickedAt')} ${p.url}]\n${t('pick.selector')}: ${p.selector}\n${t('pick.position')}: ${p.rect.x},${p.rect.y} · ${p.rect.w}×${p.rect.h}${st}${p.text ? `\n${t('pick.text')}: ${p.text}` : ''}\n\`\`\`html\n${p.html}\n\`\`\`\n`;
  P.focus(); P.setSelectionRange(P.value.length, P.value.length);
  toast(t('pick.added'));
}
if (window.infuse?.on) window.infuse.on('browserPick', p => insertPick(p));
