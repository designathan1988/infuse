// changes — file list + rich diff with line comments (sent to the agent), commit, discard.
import { $, $$, h, esc, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { api, tokenHeader, urlWith } from '../core/api.js';
import { pref, liveOf, busy, refreshChanges } from '../core/store.js';
import { menu } from '../components/menu.js';
import { toast } from '../components/toast.js';

const CHG_ST = { A: ['changes.new', 'ok'], D: ['changes.deleted', 'bad'] };

function makeState(pb, extra, c) {
  pb.innerHTML = `<div class="changes-split"><div class="file-list" role="list"></div><div class="diff-view-host"></div></div>
    <div class="pane-foot"><span class="faint ncom"></span><button class="btn sm line sendc hidden">${t('changes.sendComments')} <span class="kbd">Ctrl Enter</span></button><span class="grow"></span><span class="mono faint trunc brname"></span><button class="btn sm line newbranch">${t('changes.newBranch')}</button><input class="brinput hidden" placeholder="${esc(t('changes.branchName'))}" aria-label="${esc(t('changes.branchName'))}" spellcheck="false"><input class="cmsg" placeholder="${esc(t('changes.commitMsg'))}"><button class="btn sm pri commit">${t('changes.commit')}</button></div>`;
  extra.innerHTML = `<span class="scopeslot"></span><button class="icon-btn" title="${esc(t('common.refresh'))}">${I.reload}</button>`;
  return {
    pb, extra, c, sel: null, files: [], comments: [],
    fl: pb.querySelector('.file-list'), dv: pb.querySelector('.diff-view-host')
  };
}

function updateComments(s) {
  s.pb.querySelector('.ncom').textContent = s.comments.length ? t('changes.commentCount', { count: s.comments.length }) : '';
  s.pb.querySelector('.sendc').classList.toggle('hidden', !s.comments.length);
}

function scopeSwitchHTML() {
  const cur = pref('chgScope');
  return `<span class="seg2">${[['all', 'changes.scopeAll'], ['turn', 'changes.scopeTurn']].map(([k, l]) => `<button data-scope="${k}" class="${cur === k ? 'on' : ''}">${t(l)}</button>`).join('')}</span>`;
}

function bindScopeSwitch(s) {
  $$('.seg2 button', s.extra).forEach(b => b.onclick = e => {
    e.stopPropagation();
    import('../core/store.js').then(st => {
      st.pref('chgScope', b.dataset.scope);
      for (const v of st.ui.views.get()) { const lv = v?.sid && st.liveOf(v.sid); if (lv) st.refreshChanges(lv); }
    });
    loadChanges(s);
  });
}

function renderFiles(s) {
  s.fl.innerHTML = s.files.length ? '' : `<div class="empty">${t('changes.none')}</div>`;
  s.files.forEach(f => {
    const it = h(`<div class="file-item${f.path === s.sel ? ' on' : ''}${f.status === 'D' ? ' gone' : ''}" title="${esc(f.path)}" role="listitem" tabindex="0"><span class="n">${esc(f.path)}</span>${CHG_ST[f.status] ? `<span class="tag ${CHG_ST[f.status][1]}">${t(CHG_ST[f.status][0])}</span>` : ''}<span class="add">+${f.added}</span><span class="del">−${f.removed}</span></div>`);
    it.onclick = () => selectFile(s, f.path);
    it.oncontextmenu = e => {
      e.preventDefault();
      menu(null, [
        { label: t('files.openInPane'), f: () => import('./index.js').then(p => p.openFileInPane(f.path)) },
        { sep: 1 },
        { label: t('changes.discard'), danger: 1, f: () => discardPath(s, f.path) },
      ], e);
    };
    s.fl.appendChild(it);
  });
}

async function loadChanges(s) {
  const L = s.c.L;
  const sid = L?.id || '';
  const r = await api(`/api/changes?session=${encodeURIComponent(sid)}&scope=${pref('chgScope')}`, { root: s.c.root, dir: s.c.dir }).catch(() => ({}));
  s.files = r.files || [];
  s.extra.querySelector('.scopeslot').innerHTML = r.local ? scopeSwitchHTML() : '';
  bindScopeSwitch(s);
  s.pb.querySelector('.cmsg').classList.toggle('hidden', !!r.local);
  s.pb.querySelector('.commit').classList.toggle('hidden', !!r.local);
  s.pb.querySelector('.newbranch').classList.toggle('hidden', !!r.local);
  s.pb.querySelector('.brname').textContent = r.branch || '';
  renderFiles(s);
  if (s.sel && s.files.find(f => f.path === s.sel)) selectFile(s, s.sel);
  else if (s.files[0]) selectFile(s, s.files[0].path);
  else s.dv.innerHTML = `<div class="empty">${t('changes.nothingToReview')}</div>`;
}

async function discardPath(s, p) {
  if (!confirm(t('changes.discardConfirm', { path: p }))) return;
  await fetch(urlWith(`/api/git?project=${encodeURIComponent(s.c.root)}`, { root: s.c.dir }), {
    method: 'POST', headers: tokenHeader(), body: JSON.stringify({ action: 'discard', path: p })
  });
  loadChanges(s);
  if (s.c.L) refreshChanges(s.c.L);
}

function noteEl(s, cm) {
  const n = h(`<div class="comment-note"><span class="grow">${esc(cm.text)}</span><button class="icon-btn icon-20" aria-label="${esc(t('action.remove'))}">${I.close}</button></div>`);
  n.querySelector('button').onclick = () => { s.comments.splice(s.comments.indexOf(cm), 1); n.remove(); updateComments(s); };
  return n;
}

function addComment(s, row, path, line, code) {
  const box = h(`<div class="comment-box"><textarea placeholder="${esc(t('changes.commentHint'))}" aria-label="${esc(t('changes.comment'))}"></textarea></div>`);
  row.after(box);
  const ta = box.querySelector('textarea'); ta.focus();
  ta.onkeydown = e => {
    e.stopPropagation();
    if (e.key === 'Escape') box.remove();
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); if (ta.value.trim()) s.comments.push({ path, line, code, text: ta.value.trim() }); box.remove(); sendComments(s); return; }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (ta.value.trim()) { const cm = { path, line, code, text: ta.value.trim() }; s.comments.push(cm); box.replaceWith(noteEl(s, cm)); updateComments(s); }
    }
  };
}

async function sendComments(s) {
  const L = s.c.L;
  if (!L) return toast(t('changes.needSession'));
  if (!s.comments.length) return;
  const r = await api(`/api/session/${L.id}/comments`, { body: { comments: s.comments }, root: L.root }).catch(e => ({ error: String(e) }));
  if (r.error) return toast(r.error);
  s.comments.length = 0; updateComments(s);
  toast(t('changes.commentsSent'));
  selectFile(s, s.sel);
}

function diffHead(s, p, r) {
  const head = h(`<div class="row changes-head"><span class="mono trunc grow">${esc(p)}</span>${r.accepted ? `<span class="tag ok">${t('changes.acceptedCount', { count: r.accepted })}</span>` : ''}${r.allAccepted ? '' : `<button class="btn sm danger">${t('changes.discard')}</button>`}</div>`);
  const discard = head.querySelector('button');
  if (discard) discard.onclick = () => discardPath(s, p);
  return head;
}

// Accept keeps the hunk (staged) and Reject reverts just it; the server re-reads the diff and acts
// on its own copy, so a hunk that changed in the meantime is refused, not guessed at.
async function decideHunk(s, p, hunk, op) {
  if (op === 'reject' && !confirm(t('changes.rejectConfirm', { path: p }))) return;
  const scope = `project=${encodeURIComponent(s.c.root)}&session=${encodeURIComponent(s.c.L?.id || '')}&scope=${pref('chgScope')}`;
  const r = await api(`/api/git/hunk?${scope}`, { body: { path: p, id: hunk.id, op }, root: s.c.dir })
    .catch(e => ({ error: e.body?.code ? t(e.body.code, e.body.args) : e.message }));
  if (r.error) toast(r.error);
  await loadChanges(s);
  if (s.c.L) refreshChanges(s.c.L);
}

function hunkRow(s, p, l, hunk, local) {
  const row = h(`<div class="ln diff-hunk"><span class="no"></span><span class="tx">${esc(l)}</span>${hunk ? `<span class="hunk-acts">${local ? '' : `<button class="btn sm line hunk-accept" title="${esc(t('changes.acceptHint'))}">${t('changes.accept')}</button>`}<button class="btn sm danger hunk-reject" title="${esc(t('changes.rejectHint'))}">${t('changes.reject')}</button></span>` : ''}</div>`);
  if (hunk) {
    row.querySelector('.hunk-reject').onclick = () => decideHunk(s, p, hunk, 'reject');
    if (!local) row.querySelector('.hunk-accept').onclick = () => decideHunk(s, p, hunk, 'accept');
  }
  return row;
}

function renderDiff(s, p, r) {
  const lines = (r.diff || '').replace(/\n$/, '').split('\n');
  let nNew = 0, nHunk = 0;
  const out = h('<div class="rich-diff"></div>');
  out.appendChild(diffHead(s, p, r));
  for (const l of lines) {
    if (/^(diff |index |--- |\+\+\+ )/.test(l)) continue;
    const m = l.match(/^@@ -\d+(?:,\d+)? \+(\d+)/);
    if (m) { nNew = +m[1]; out.appendChild(hunkRow(s, p, l, r.hunks?.[nHunk++], r.local)); continue; }
    const cls = l.startsWith('+') ? 'diff-add-line' : l.startsWith('-') ? 'diff-del-line' : '';
    const lineNo = l.startsWith('-') ? '' : nNew;
    const row = h(`<div class="ln ${cls}"><span class="no">${lineNo}</span><span class="tx">${esc(l) || ' '}</span>${lineNo ? `<span class="comment-add" role="button" tabindex="0" aria-label="${esc(t('changes.comment'))}" title="${esc(t('changes.comment'))}">+</span>` : ''}</div>`);
    if (lineNo) row.querySelector('.comment-add').onclick = () => addComment(s, row, p, lineNo, l.slice(1));
    out.appendChild(row);
    for (const cm of s.comments.filter(x => x.path === p && x.line === lineNo && lineNo)) out.appendChild(noteEl(s, cm));
    if (!l.startsWith('-')) nNew++;
  }
  return out;
}

async function selectFile(s, p) {
  s.sel = p;
  $$('.file-item', s.fl).forEach(x => x.classList.toggle('on', x.title === p));
  const L = s.c.L;
  const r = await api(`/api/diff?path=${encodeURIComponent(p)}&session=${encodeURIComponent(L?.id || '')}&scope=${pref('chgScope')}&project=${encodeURIComponent(s.c.root)}`, { root: s.c.dir }).catch(() => ({}));
  const out = renderDiff(s, p, r);
  s.dv.innerHTML = ''; s.dv.appendChild(out);
}

async function createBranch(s, name) {
  const scope = `project=${encodeURIComponent(s.c.root)}&session=${encodeURIComponent(s.c.L?.id || '')}`;
  const r = await api(`/api/git/branch?${scope}`, { body: { name }, root: s.c.dir })
    .catch(e => ({ error: e.body?.code ? t(e.body.code, e.body.args) : e.message }));
  toast(r.error || t('changes.branchCreated', { name: r.branch }));
  if (!r.error) loadChanges(s);
}

function wireBranch(s) {
  const btn = s.pb.querySelector('.newbranch'), inp = s.pb.querySelector('.brinput');
  const fold = () => { inp.value = ''; inp.classList.add('hidden'); btn.classList.remove('hidden'); };
  btn.onclick = () => { btn.classList.add('hidden'); inp.classList.remove('hidden'); inp.focus(); };
  inp.onblur = fold;
  inp.onkeydown = e => {
    e.stopPropagation();
    if (e.key === 'Escape') fold();
    if (e.key === 'Enter' && inp.value.trim()) { const name = inp.value.trim(); fold(); createBranch(s, name); }
  };
}

async function commitChanges(s) {
  const msg = s.pb.querySelector('.cmsg').value.trim();
  if (!msg) return toast(t('changes.writeCommit'));
  const r = await api('/api/git', { body: { action: 'commit', message: msg }, root: s.c.dir }).catch(e => ({ error: String(e) }));
  toast(r.ok ? t('changes.committed') : (r.output || r.error || ''));
  if (r.ok) { s.pb.querySelector('.cmsg').value = ''; loadChanges(s); if (s.c.L) refreshChanges(s.c.L); }
}

function wireChanges(s) {
  s.pb.querySelector('.sendc').onclick = () => sendComments(s);
  s.pb.addEventListener('keydown', e => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && s.comments.length) { e.preventDefault(); sendComments(s); } });
  wireBranch(s);
  s.pb.querySelector('.commit').onclick = () => commitChanges(s);
  s.pb.querySelector('.cmsg').onkeydown = e => { if (e.key === 'Enter') commitChanges(s); };
  s.extra.querySelector('button:not(.scopeslot *)')?.addEventListener('click', () => loadChanges(s));
  s.extra.querySelector('.icon-btn')?.addEventListener('click', () => loadChanges(s));
}

export function mount(pb, extra, c) {
  const s = makeState(pb, extra, c);
  wireChanges(s);
  loadChanges(s);
  return { refresh: debounce(() => loadChanges(s), 300), select: p => selectFile(s, p) };
}
