// transcript — the conversation of one session view: settled messages, live strip (streaming text,
// reasoning, tool calls in flight), permission and question cards, turn separators/summaries.
// Renders by effect(): reads signals, patches the container by keyed parts — no manual rerenders.
import { $, $$, esc, patchKeyed } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { fmtWhen, ago as agoText, fmtDur, fmtTok } from '../core/fmt.js';
import { md } from '../core/markdown.js';
import { I } from '../core/icons.js';
import { effect } from '../core/signals.js';
import { pref, liveOf, busy, curRoot } from '../core/store.js';
import { messageText as msgText } from '../core/messages.js';
import { toolBlock, thinkBlock, agentBlock } from './toolblock.js';


function permCard(p) {
  let inp = {}; try { inp = typeof p.input === 'string' ? JSON.parse(p.input) : p.input || {}; } catch { }
  const verbs = { exec: 'perm.exec', write: 'perm.write', net: 'perm.net', mcp: 'perm.mcp', memory: 'perm.memory' };
  const diff = (o, n) => `<div class="diff-view">${o.split('\n').map(l => `<div class="diff-del-line">${esc(l) || ' '}</div>`).join('')}${n.split('\n').map(l => `<div class="diff-add-line">${esc(l) || ' '}</div>`).join('')}</div>`;
  const detail = p.tool === 'Bash' ? `<pre class="cmd">${esc(inp.command || '')}</pre>`
    : p.tool === 'Edit' ? `<div class="mono mt-4">${esc(inp.path)}</div>` + diff(inp.old_string || '', inp.new_string || '')
      : p.tool === 'Write' ? `<div class="mono mt-4">${esc(inp.path)}</div>` + diff('', (inp.content || '').split('\n').slice(0, 200).join('\n'))
        : p.tool === 'WebFetch' ? `<pre class="cmd">${esc(inp.url || '')}</pre>`
          : `<pre class="cmd">${esc(JSON.stringify(inp, null, 2))}</pre>`;
  return `<div class="perm-card" role="alertdialog" aria-label="${esc(p.tool)}"><div class="hd"><b>${esc(p.tool)}</b><span class="muted">${t('perm.wants')} ${t(verbs[p.category] || 'perm.useTool')}</span></div>${p.reason ? `<div class="sentinel">${t('perm.sentinel')} ${esc(t(p.reason))}. ${t('perm.sentinelWhy')}</div>` : ''}${detail}
    <div class="perm-opts" role="group">
      <div class="perm-opt" data-id="${esc(p.id)}" data-perm="allow" tabindex="0" role="button">${t('perm.yes')}<span class="kbd">Y</span></div>
      <div class="perm-opt" data-id="${esc(p.id)}" data-perm="always" tabindex="0" role="button">${t('perm.always')}${p.tool === 'Bash' ? ` <span class="faint">(${esc((inp.command || '').trim().split(/\s+/).slice(0, 2).join(' '))})</span>` : ''}<span class="kbd">A</span></div>
      <div class="perm-opt" data-id="${esc(p.id)}" data-perm="all" tabindex="0" role="button">${t('perm.all')}<span class="kbd">T</span></div>
      <div class="perm-opt" data-id="${esc(p.id)}" data-perm="deny" tabindex="0" role="button">${t('perm.no')}<span class="kbd">N</span></div>
    </div></div>`;
}
const questionCard = q => `<div class="perm-card"><div class="hd"><b>${esc(q.question)}</b></div><div class="perm-opts">${(q.options || []).map((o, i) => `<div class="perm-opt" data-id="${esc(q.id)}" data-ans="${esc(o)}" tabindex="0" role="button">${esc(o)}<span class="kbd">${i + 1}</span></div>`).join('')}</div>
  <div class="question-free"><input data-id="${esc(q.id)}" data-freeans placeholder="${esc(t('question.freeForm'))}"></div></div>`;

function turnSummary(s, msgs, from, to) {
  if (from == null) return '';
  const first = msgs[from], last = msgs[to];
  let tools = 0, tokens = 0, texts = 0;
  for (let k = from; k <= to; k++) {
    const m = msgs[k]; if (m.role !== 'assistant') continue;
    tokens += m.tokens || 0;
    for (const b of m.content || []) { if (b.type === 'tool_use') tools++; if (b.type === 'text' && b.text?.trim()) texts++; }
  }
  if (!texts && !tools) return '';
  const dur = first?.ts && last?.ts ? last.ts - first.ts : null;
  return `<div class="turn-summary"><span class="ok">✓</span><span class="meta">${dur != null ? `${t('turn.workedFor')} ${fmtDur(dur)}` : ''}${tools ? ` · ${t('turn.toolsCount', { count: tools })}` : ''}${tokens ? ` · ${fmtTok(tokens)} tokens` : ''}</span><span class="grow"></span>${texts ? `<button class="btn sm ghost" data-copy-turn="${from}:${to}">${I.copy} ${t('action.copy')}</button>` : ''}</div>`;
}

function msgBar(s, m, i, user) {
  const pinned = (s.pinnedMsgs || []).includes(i);
  const when = m.ts ? `<span class="when" title="${esc(new Date(m.ts).toLocaleString())}">${fmtWhen(m.ts)}</span>` : '';
  const turnIdx = s.turnStarts.indexOf(i);
  const acts = `<span class="msg-acts"><button class="icon-btn" data-copy-msg="${i}" title="${esc(t('action.copy'))}">${I.copy}</button>${user && turnIdx >= 0 ? `<button class="icon-btn" data-rw="${turnIdx + 1}" title="${esc(t('action.editRedo'))}">↶</button>` : ''}<button class="icon-btn" data-fork="${i}" title="${esc(t('action.fork'))}">${I.branch}</button><button class="icon-btn${pinned ? ' on' : ''}" data-pinmsg="${i}" title="${esc(t(pinned ? 'action.unpin' : 'action.pin'))}">${I.pin}</button><button class="icon-btn" data-speak="${i}" title="${esc(t('action.speak'))}">${I.speak}</button></span>`;
  return `<div class="msg-meta-bar${user ? ' u' : ''}">${acts}${when}</div>`;
}

function msgFoot(m, i, s) {
  const meta = [
    m.ts && `<span title="${esc(new Date(m.ts).toLocaleString())}">${fmtWhen(m.ts)}</span>`,
    m.ms && `<span title="${esc(t('common.modelTime'))}">${fmtDur(m.ms)}</span>`,
    m.model && `<span>${esc(m.model)}</span>`,
    m.tokens && `<span>${fmtTok(m.tokens)} tokens</span>`
  ].filter(Boolean).join('<span class="dotsep">·</span>');
  const pinned = (s.pinnedMsgs || []).includes(i);
  return `<div class="msg-agent-foot"><span class="meta">${meta}</span><span class="msg-acts"><button class="icon-btn" data-copy-msg="${i}" title="${esc(t('action.copyResponse'))}">${I.copy}</button><button class="icon-btn" data-fork="${i}" title="${esc(t('action.fork'))}">${I.branch}</button><button class="icon-btn${pinned ? ' on' : ''}" data-pinmsg="${i}" title="${esc(t(pinned ? 'action.unpin' : 'action.pin'))}">${I.pin}</button><button class="icon-btn" data-speak="${i}" title="${esc(t('action.speak'))}">${I.speak}</button></span></div>`;
}

const missionOf = m => {
  if (m?.kind !== 'mission') return null;
  const b = (m.content || []).find(x => x.type === 'text' && x.meta);
  return b ? b.meta : { goal: '' };
};
const chapterTitle = m => {
  const n = missionOf(m);
  const tx = n ? (n.turn ? `${t('mission.title')} · ${t('mission.turnOf', { cur: n.turn, max: n.maxTurns })}` : `${t('mission.title')}: ${n.goal}`) : msgText(m).trim();
  return tx.split('\n')[0].slice(0, 90) || t('toc.noText');
};

export function mountTranscript(v, scroller, jumpBtn) {
  const chat = scroller.querySelector('.chat-feed') || scroller;
  v.stick = true;

  // scroll: follow the end only while the user is there
  scroller.addEventListener('wheel', e => { if (e.deltaY < 0) v.stick = false; }, { passive: true });
  scroller.addEventListener('scroll', () => {
    const dist = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;
    if (v._prog) v._prog = false; else if (dist > 24) v.stick = false;
    if (dist < 8) v.stick = true;
    updateJump();
  }, { passive: true });
  jumpBtn.onclick = () => { v.stick = true; scroller.scrollTo({ top: scroller.scrollHeight, behavior: 'smooth' }); };
  const toBottom = () => { v._prog = true; scroller.scrollTop = scroller.scrollHeight; };
  const updateJump = () => {
    const far = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight > 120;
    const L = v.sid ? liveOf(v.sid) : null;
    jumpBtn.classList.toggle('show', far);
    jumpBtn.classList.toggle('live', far && busy(L));
  };

  effect(() => {
    const L = v.sid ? liveOf(v.sid) : null;
    const snap = L?.snap.get();
    const text = L?.text.get(), think = L?.think.get(), tools = L?.tools.get(), running = L?.running.get();
    const fold = pref('fold');
    if (!snap) { chat.innerHTML = ''; return; }

    const msgs = snap.messages, results = {};
    for (const m of msgs) for (const b of m.content || []) if (b.type === 'tool_result') results[b.tool_use_id] = b;
    const starts = new Set(snap.turnStarts);
    let turn = 0, out = '';
    const parts = [];
    const cut = key => { if (out) parts.push([key, out, null]); out = ''; };

    msgs.forEach((m, i) => {
      if (starts.has(i)) {
        turn++;
        cut('pre:' + i);
        out += `<div class="turn-sep" data-turn="${turn}"><span class="l"></span><button class="btn sm" data-rw="${turn}" title="${esc(t('action.rewindHere'))}">↶ ${t('action.rewindHereShort')}</button></div>`;
      }
      if (m.role === 'user') {
        if (m.content.some(b => b.type === 'tool_result')) return;
        const texts = m.content.filter(b => b.type === 'text'), imgs = m.content.filter(b => b.type === 'image');
        if (!texts.length && !imgs.length) return;
        const main = texts[0]?.display || texts[0]?.text || '';
        if (m.kind === 'review') { out += reviewBlock(texts.slice(1).map(b => b.text).join('\n\n').trim(), i); return; }
        if (m.kind === 'compaction') { out += `<div class="marker" title="${esc(main.slice(0, 600))}">${t('chat.compacted')}</div>`; return; }
        const attached = texts.slice(1).map(b => (b.text.match(/path="([^"]+)"/) || [])[1]).filter(Boolean);
        const mission = missionOf(m);
        const body = mission
          ? `<span class="tag warn mr-3">${esc(mission.turn ? t('mission.turnOf', { cur: mission.turn, max: mission.maxTurns }) : t('mission.title'))}</span>${esc(mission.turn ? main : mission.goal)}`
          : esc(main);
        out += `<div class="msg-user${(snap.pinnedMsgs || []).includes(i) ? ' pinned' : ''}" data-mi="${i}"><div class="msg-user-text sel">${body}${attached.length ? `<div class="att">${attached.map(p => `<span class="chipx">📎 <span>${esc(p.replace(/^.*[\\/]/, ''))}</span></span>`).join('')}</div>` : ''}${imgs.map(b => `<img src="data:${b.mediaType};base64,${b.data}" alt="">`).join('')}</div>${msgBar(snap, m, i, true)}</div>`;
      } else {
        if (m.kind === 'review_ack' || m.kind === 'compaction') return;
        let inner = '';
        for (const [bi, b] of m.content.entries()) {
          if (b.type === 'text' && b.text?.trim()) inner += `<div class="markdown sel">${md(b.text.trim())}</div>`;
          else if (b.type === 'thinking' && b.thinking?.trim()) inner += thinkBlock(`${v.sid}:${i}:${bi}`, b.thinking, false, b.ms);
          else if (b.type === 'tool_use') inner += toolBlock(b.id, b.name, b.input, results[b.id], false, b.meta, curRoot());
        }
        const toolsOnly = !m.content.some(b => b.type === 'text' && b.text?.trim());
        if (inner) out += `<div class="msg-agent${toolsOnly ? ' tools-only' : ''}${(snap.pinnedMsgs || []).includes(i) ? ' pinned' : ''}" data-mi="${i}">${inner}${!toolsOnly ? msgFoot(m, i, snap) : ''}</div>`;
      }
      cut('m:' + i);
      const next = i + 1;
      if (turn && (starts.has(next) || (next === msgs.length && !['thinking', 'tool', 'waiting_permission', 'waiting_user', 'compacting'].includes(snap.state)))) {
        out += turnSummary(snap, msgs, snap.turnStarts[turn - 1], i); cut('sum:' + i);
      }
    });
    cut('tail');
    if (!msgs.length) parts.unshift(['empty', `<div class="empty chat-empty">${t('chat.empty')}</div>`, null]);
    parts.push(['live', '<div class="live-strip"></div>', null]);
    if (snap.pendingPermission) parts.push(['perm', permCard(snap.pendingPermission), null]);
    if (snap.pendingQuestion) parts.push(['question', questionCard(snap.pendingQuestion), null]);
    if (snap.error && snap.state === 'error') parts.push(['error', `<div class="errbox" role="alert">${esc(snap.error)}</div>`, null]);
    patchKeyed(chat, parts);
    bindChatActions(v, chat);
    renderLive();
    if (v.stick !== false) toBottom();
    updateJump();
    renderToc(v, scroller);

    function renderLive() {
      const el = chat.querySelector('.live-strip');
      if (!el) return;
      const lp = [];
      if (think) lp.push(['think', thinkBlock(`${v.sid}:live`, think, true)]);
      if (text) lp.push(['text', `<div class="msg-agent"><div class="markdown sel">${md(text)}<span class="cursor"></span></div></div>`]);
      for (const [id, tl] of Object.entries(tools || {})) {
        const inp = partialSafe(tl.input);
        lp.push(['tool:' + id, toolBlock(id, tl.name, inp, null, running?.id === id, null, curRoot())]);
      }
      const st = snap.state;
      const working = (snap.agents || []).filter(a => a.state === 'running');
      let out2 = '';
      if (working.length && busy(L)) out2 += `<div class="status agentsgo" title="${esc(t('agents.openPane'))}" role="status"><span class="spin"></span><span>${t('agents.workingCount', { count: working.length })}: ${working.map(a => `<b>${esc(a.title)}</b> · ${a.steps} ${t('common.steps')}${a.tool ? ' · ' + esc(a.tool) : ''}`).join(' &nbsp;|&nbsp; ')}</span></div>`;
      if (st === 'thinking' || st === 'tool' || st === 'compacting') out2 += `<div class="status" role="status"><span class="spin"></span><span>${st === 'tool' ? t('live.runningTool') + esc(running?.name || '') : st === 'compacting' ? t('live.compacting') : t('live.thinking')}</span><span class="faint" data-el="${v.sid}"></span></div>`;
      if (out2) lp.push(['status', `<div>${out2}</div>`]);
      patchKeyed(el, lp);
      if (v.stick !== false) toBottom();
    }
  });

  // elapsed-time ticker: one interval, self-contained spans (no whole-DOM scans)
  setInterval(() => {
    for (const el of chat.querySelectorAll('[data-el]')) {
      const L = liveOf(el.dataset.el);
      if (L?.started) el.textContent = Math.round((Date.now() - L.started) / 1000) + 's';
    }
  }, 500);
}

const partialSafe = src => { try { return JSON.parse(src); } catch { return { _streaming: true, _partial: src }; } };

function reviewBlock(reviewText, i) {
  return `<div class="report-card review"><div class="rh"><b>${t('review.title')}</b><span class="faint">${t('review.subtitle')}</span><span class="grow"></span><button class="icon-btn" data-copy-msg="${i}" title="${esc(t('action.copy'))}">${I.copy}</button></div><div class="markdown sel">${md(reviewText)}</div><div class="report-next"><span class="grow faint">${t('review.nextMsgNote')}</span><button class="btn sm pri" data-fixreview>${t('review.askFixes')}</button></div></div>`;
}

function renderToc(v, scroller) {
  const L = v.sid ? liveOf(v.sid) : null;
  const toc = scroller.closest('.view')?.querySelector('.chapter-toc');
  if (!toc) return;
  const s = L?.snap.get();
  const starts = s?.turnStarts || [];
  if (starts.length < 2) { toc.classList.add('hidden'); return; }
  toc.classList.remove('hidden');
  const items = starts.map((idx, k) => ({ turn: k + 1, title: chapterTitle(s.messages[idx] || { content: [] }), ts: s.messages[idx]?.ts }));
  const ticks = toc.querySelector('.toc-ticks'), list = toc.querySelector('.toc-list');
  ticks.innerHTML = items.slice(-14).map(x => `<i data-t="${x.turn}" role="button" tabindex="0"></i>`).join('');
  list.innerHTML = `<div class="toc-heading">${t('toc.chapters')}</div>` + items.map(x => `<div class="toc-item" data-t="${x.turn}" tabindex="0" role="link"><span class="n">${x.turn}</span><span class="tt">${esc(x.title)}</span>${x.ts ? `<span class="tm">${new Date(x.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>` : ''}</div>`).join('');
  const go = el => {
    const sep = scroller.querySelector(`.turn-sep[data-turn="${el.dataset.t}"]`);
    if (sep) { v.stick = false; sep.scrollIntoView({ behavior: 'smooth', block: 'start' }); }
  };
  toc.querySelectorAll('[data-t]').forEach(el => { el.onclick = () => go(el); el.onkeydown = e => { if (e.key === 'Enter') go(el); }; });
}

const activate = e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.currentTarget.click(); } };

function bindChatActions(v, chat) {
  const L = v.sid ? liveOf(v.sid) : null;
  const s = L?.snap.get();
  if (!s) return;
  chat.querySelectorAll('[data-copy-msg]').forEach(b => b.onclick = e => {
    e.stopPropagation();
    const m = s.messages[+b.dataset.copyMsg];
    navigator.clipboard.writeText(msgText(m)).then(() => import('./toast.js').then(m2 => m2.toast(t('action.copiedMsg'))));
  });
  chat.querySelectorAll('[data-copy-turn]').forEach(b => b.onclick = e => {
    e.stopPropagation();
    const [a, z] = b.dataset.copyTurn.split(':').map(Number);
    const txt = s.messages.slice(a + 1, z + 1).filter(m => m.role === 'assistant').map(msgText).filter(Boolean).join('\n\n');
    navigator.clipboard.writeText(txt);
  });
  chat.querySelectorAll('[data-copy-code]').forEach(b => b.onclick = () => navigator.clipboard.writeText(b.closest('.code-wrap').querySelector('pre').innerText));
  chat.querySelectorAll('[data-copy-think]').forEach(b => b.onclick = e => { e.stopPropagation(); navigator.clipboard.writeText(b.closest('.tool-block').querySelector('.thinktext')?.innerText || ''); });
  chat.querySelectorAll('[data-copy-tool]').forEach(b => b.onclick = e => { e.stopPropagation(); navigator.clipboard.writeText(b.closest('.tool-block').querySelector(b.dataset.copyTool === 'out' ? '.res' : '.inp')?.innerText || ''); });
  chat.querySelectorAll('[data-url]').forEach(a => a.onclick = e => { e.preventDefault(); import('../panes/index.js').then(p => { p.togglePane('browser', true); setTimeout(() => p.paneCall('browser', 'go', a.dataset.url), 300); }); });
  chat.querySelectorAll('[data-fork]').forEach(b => b.onclick = async e => {
    e.stopPropagation();
    const r = await import('../core/api.js').then(api => api.api(`/api/session/${L.id}/fork`, { body: { upTo: +b.dataset.fork }, root: L.root }));
    if (!r.ok) return;
    await import('../core/store.js').then(st => { st.reloadSessions(); st.openSession(r.root, r.id); });
  });
  chat.querySelectorAll('[data-pinmsg]').forEach(b => b.onclick = async e => {
    e.stopPropagation();
    const r = await import('../core/api.js').then(api => api.api(`/api/session/${L.id}/pinmsg`, { body: { index: +b.dataset.pinmsg }, root: L.root }));
    const snap2 = { ...L.snap.get(), pinnedMsgs: r.pinned || [] };
    L.snap.set(snap2);
  });
  chat.querySelectorAll('[data-speak]').forEach(b => b.onclick = e => {
    e.stopPropagation();
    if (speechSynthesis.speaking) { speechSynthesis.cancel(); return; }
    const text = msgText(s.messages[+b.dataset.speak] || { content: [] });
    const clean = text.replace(/```[\s\S]*?```/g, ' ').replace(/[*_`#>]/g, '').slice(0, 20000);
    const u = new SpeechSynthesisUtterance(clean);
    u.lang = document.documentElement.lang || 'pt-BR'; u.rate = 1.05;
    speechSynthesis.speak(u);
  });
  chat.querySelectorAll('[data-fixreview]').forEach(b => b.onclick = () => {
    const P = v._promptEl;
    if (P) { P.value = t('review.fixPrompt'); v._sendFn?.(); }
  });
  chat.querySelectorAll('[data-next]').forEach(b => b.onclick = () => {
    const P = v._promptEl;
    if (P) { P.value = b.dataset.next; P.focus(); }
  });
  chat.querySelectorAll('[data-rw]').forEach(b => b.onclick = () => import('./dialogs.js').then(d => d.rewindDialog(v, +b.dataset.rw)));
  chat.querySelectorAll('[data-fold]').forEach(x => x.onclick = e => {
    if (e.target.closest('[data-path]')) return;
    const box = x.closest('[data-foldable]') || x.parentNode;
    const open = !box.classList.contains('open');
    box.classList.toggle('open', open);
    x.setAttribute('aria-expanded', String(!!open));
    const folds = { ...pref('fold') }; folds[x.dataset.fold] = open;
    import('../core/store.js').then(st => st.pref('fold', folds));
  });
  chat.querySelectorAll('[data-path]').forEach(x => {
    x.onclick = () => import('../panes/index.js').then(p => p.openFileInPane(x.dataset.path));
  });
  chat.querySelectorAll('[data-perm]').forEach(b => { b.onclick = () => import('../core/store.js').then(st => st.decide(L, b.dataset.id, b.dataset.perm)); b.onkeydown = activate; });
  chat.querySelectorAll('[data-ans]').forEach(b => { b.onclick = () => import('../core/store.js').then(st => st.answer(L, b.dataset.id, b.dataset.ans)); b.onkeydown = activate; });
  const free = chat.querySelector('[data-freeans]');
  if (free) free.onkeydown = e => { if (e.key === 'Enter' && free.value.trim()) import('../core/store.js').then(st => st.answer(L, free.dataset.id, free.value.trim())); };
}

// The whole conversation as Markdown: prompts, answers, each tool call with its result, and reasoning.
export function transcriptMarkdown(L) {
  const s = L.snap.get();
  const res = {};
  for (const m of s.messages) for (const b of m.content || []) if (b.type === 'tool_result') res[b.tool_use_id] = b;
  let out = `# ${s.title || t('common.session')}\n\n_${L.root || ''} · ${s.model || ''} · ${new Date().toLocaleString()}_\n\n`;
  for (const m of s.messages) {
    if (m.role === 'user') {
      const tx = msgText(m);
      if (tx) out += `## ${t('chat.you')}\n\n${tx}\n\n`;
      continue;
    }
    out += `## ${t('chat.agent')}\n\n`;
    for (const b of m.content || []) {
      if (b.type === 'thinking' && b.thinking) out += `<details><summary>${t('think.reasoning')}</summary>\n\n${b.thinking}\n\n</details>\n\n`;
      if (b.type === 'text' && b.text?.trim()) out += b.text.trim() + '\n\n';
      if (b.type === 'tool_use') {
        out += `**${b.name}**\n\n\`\`\`json\n${JSON.stringify(b.input, null, 2)}\n\`\`\`\n\n`;
        const r = res[b.id];
        if (r) out += `\`\`\`\n${String(r.content || '').slice(0, 8000)}\n\`\`\`\n\n`;
      }
    }
  }
  return out;
}

export async function copyConversation(L) {
  try {
    await navigator.clipboard.writeText(transcriptMarkdown(L));
    const m = await import('./toast.js');
    m.toast(t('action.copiedMsg'));
  } catch {
    const m = await import('./toast.js');
    m.toast(t('action.copyFailed'));
  }
}

export function downloadConversation(L) {
  const blob = new Blob([transcriptMarkdown(L)], { type: 'text/markdown' });
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = (L.snap.get().title || t('common.session')).replace(/[^\w\- ]+/g, '').trim().slice(0, 60) + '.md';
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 2000);
}
