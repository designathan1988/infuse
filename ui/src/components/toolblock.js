// toolblock — renders one tool call (live or settled): header with status dot, summary, duration;
// body with command/entry/diff/result. Also reasoning blocks, the Report card and subagent trails.
import { esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { fmtDur } from '../core/fmt.js';
import { md } from '../core/markdown.js';
import { I } from '../core/icons.js';
import { pref, ui, liveOf, curDir } from '../core/store.js';

export const TOOL_LABELS = {
  Bash: 'tool.ran', Read: 'tool.read', Write: 'tool.wrote', Edit: 'tool.edited', Glob: 'tool.globbed',
  Grep: 'tool.grepped', LS: 'tool.listed', WebFetch: 'tool.fetched', WebSearch: 'tool.searched',
  TodoWrite: 'tool.todos', AskUserQuestion: 'tool.asked', MultiEdit: 'tool.edited', Browser: 'tool.browser',
  Skill: 'tool.skill', Agent: 'tool.subagent', Memory: 'tool.memory', Contract: 'tool.contract', Report: 'tool.report'
};

export function toolSummary(name, inp) {
  if (!inp) return '';
  if (inp._partial !== undefined) return inp._partial.slice(-90);
  switch (name) {
    case 'Bash': return inp.command || '';
    case 'Read': case 'Write': case 'Edit': case 'MultiEdit': return inp.path || inp.file_path || '';
    case 'WebSearch': return inp.query || '';
    case 'Browser': return [inp.action, inp.url || inp.selector || inp.key || (inp.width ? inp.width + 'x' + inp.height : '') || ''].filter(Boolean).join(' · ');
    case 'Skill': return inp.name || '';
    case 'Agent': return `${inp.kind || ''} · ${inp.description || (inp.prompt || '').slice(0, 80)}`;
    case 'Memory': return `${inp.action || ''}${inp.text ? ' · ' + inp.text : ''}`;
    case 'Contract': return inp.goal || '';
    case 'Glob': return inp.pattern || '';
    case 'Grep': return `${inp.pattern || ''}${inp.path ? ' · ' + inp.path : ''}`;
    case 'LS': return inp.path || '.';
    case 'WebFetch': return inp.url || '';
    case 'TodoWrite': return `${(inp.todos || inp.tasks || []).length} ${t('common.items')}`;
    case 'AskUserQuestion': return inp.question || '';
    default: return JSON.stringify(inp).slice(0, 120);
  }
}

export const relPath = p => {
  const root = (curDir() || '').replace(/[\\/]+$/, '');
  if (!root || !p) return p;
  const norm = s => s.replace(/\\/g, '/').toLowerCase();
  const np = norm(p), nr = norm(root) + '/';
  return np.startsWith(nr) ? p.replace(/\\/g, '/').slice(nr.length) : p;
};

export const diffHTML = text => `<div class="diff-view">${String(text).split('\n').map(l => `<div class="${l.startsWith('+') && !l.startsWith('+++') ? 'diff-add-line' : l.startsWith('-') && !l.startsWith('---') ? 'diff-del-line' : /^(@@|---|\+\+\+|diff )/.test(l) ? 'diff-hunk' : ''}">${esc(l) || ' '}</div>`).join('')}</div>`;

export function liveCodeHTML(text, cls) {
  const lines = String(text || '').split('\n');
  return `<div class="live-code ${cls || ''}">${lines.map((l, k) => `<div class="lc"><span class="no">${k + 1}</span><span class="tx">${esc(l) || ' '}</span></div>`).join('')}<span class="cursor"></span></div>`;
}

// Parse incomplete JSON as the model streams it, so code shows while being written.
export function partialJSON(src) {
  try { return JSON.parse(src); } catch { }
  const out = { _streaming: true };
  const re = /"([A-Za-z_]+)"\s*:\s*/g; let m;
  while ((m = re.exec(src))) {
    const key = m[1]; let i = re.lastIndex;
    if (src[i] === '"') {
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

const lineCounts = (name, inp) => {
  if (name === 'Write' && inp?.content != null) return `<span class="cnt add">+${inp.content.split('\n').length}</span>`;
  if (name === 'Edit' && inp?.old_string != null) return `<span class="cnt"><span class="add">+${(inp.new_string || '').split('\n').length}</span> <span class="del">−${inp.old_string.split('\n').length}</span></span>`;
  return '';
};

export function thinkBlock(key, text, live, ms) {
  const open = pref('fold')[key] ?? (pref('viewMode') !== 'normal');
  const words = text.trim().split(/\s+/).length;
  return `<div class="tool-block think2${open ? ' open' : ''}" data-foldable><div class="tool-head" data-fold="${esc(key)}" role="button" tabindex="0" aria-expanded="${!!open}"><span class="chev" aria-hidden="true">›</span><b>${live ? t('think.thinkingNow') : ms ? t('think.thoughtFor', { d: fmtDur(ms) }) : t('think.reasoning')}</b><span class="sum faint">${words} ${t('common.words')}</span><button class="icon-btn mini" data-copy-think title="${esc(t('action.copyReasoning'))}">${I.copy}</button></div><div class="tool-body"><pre class="thinktext">${esc(text)}</pre></div></div>`;
}

const CLAIM = { confirmado: ['✓', 'ok', 'report.confirmed'], parcial: ['◐', 'warn', 'report.partial'], 'nao-confirmado': ['✗', 'bad', 'report.unconfirmedState'], 'sem-evidencia': ['?', '', 'report.noEvidence'] };
export function reportCard(inp, result, chk) {
  const checked = chk?.claims || [];
  const rows = (inp.claims || []).map((c, k) => {
    const v = checked[k]; const [ic, cls, lab] = CLAIM[v?.status] || ['…', '', 'report.checking'];
    return `<div class="report-row"><span class="result-badge ${cls}" title="${esc(t(lab))}">${ic}</span><span class="grow"><span>${esc(c.text)}</span>${c.ref ? ` <code>${esc(c.ref)}</code>` : ''}${v?.display ? `<span class="faint rev">${esc(v.display)}</span>` : ''}</span></div>`;
  }).join('');
  const bad = chk?.unconfirmed || 0;
  const srcs = chk?.sources || [];
  const srcHTML = srcs.length ? `<details class="details-fold"><summary>${t('report.sources')} (${srcs.length})</summary>${srcs.map(x => x.kind === 'page' ? `<div class="mono text-sm"><a href="#" data-url="${esc(x.ref)}">${esc(x.ref)}</a></div>` : `<div class="faint text-sm">${esc(t(x.kind === 'search' ? 'report.srcSearch' : 'report.srcDocs'))}: ${esc(x.ref)}</div>`).join('')}</details>` : '';
  return `<div class="report-card" data-report><div class="rh"><b>${t('report.title')}</b><span class="faint">${t(chk ? 'report.checkedBy' : 'report.checking')}</span>${bad ? `<span class="tag bad">${bad} ${t('report.unconfirmedCount')}</span>` : ''}</div>${inp.summary ? `<div class="markdown sel">${md(inp.summary)}</div>` : ''}${rows}${srcHTML}${inp.next ? `<div class="report-next"><span class="faint">${t('report.nextStep')}</span><span class="grow">${esc(inp.next)}</span><button class="btn sm" data-next="${esc(inp.next)}">${t('report.useNext')}</button></div>` : ''}</div>`;
}

export function agentHead(a) {
  if (!a) return '';
  const dur = a.state === 'running'
    ? (() => { const s = Math.max(0, Math.round((Date.now() - a.started) / 1000)); return s < 60 ? s + 's' : Math.floor(s / 60) + 'min ' + (s % 60) + 's'; })()
    : fmtDur((a.ended || Date.now()) - a.started);
  return `${a.steps} ${t('common.steps')} · ${dur}`;
}
export function agentTrailHTML(a, bid, liveMap) {
  if (!a) return `<div class="agent-step faintstep"><span class="spin sm"></span>${t('agent.starting')}</div>`;
  const steps = (a.trail || []).map((st, i) => {
    if (st.kind === 'text') return `<div class="agent-step txt markdown sel">${md(st.text || '')}</div>`;
    let inp = {}; try { inp = JSON.parse(st.input || '{}'); } catch { }
    const k = `${bid}:s${i}`, open = pref('fold')[k] ?? (pref('viewMode') === 'verbose');
    return `<div class="tool-block agent-step${open ? ' open' : ''}" data-foldable><div class="tool-head" data-fold="${esc(k)}" role="button" tabindex="0" aria-expanded="${!!open}"><span class="chev" aria-hidden="true">›</span><span class="dot ${st.state === 'running' ? 'run' : st.state === 'error' ? 'err' : ''}"></span><b>${esc(t(TOOL_LABELS[st.tool] || st.tool))}</b><span class="sum">${esc(toolSummary(st.tool, inp))}</span>${st.state !== 'running' && st.ms != null ? `<span class="tdur faint">${fmtDur(st.ms)}</span>` : ''}</div><div class="tool-body">${st.output ? `<pre class="res">${esc(st.display || st.output)}</pre>` : `<div class="faint">${t('common.noOutput')}</div>`}</div></div>`;
  }).join('');
  const tail = a.state === 'running'
    ? `<div class="agent-step faintstep"><span class="spin sm"></span>${t('agent.working')}</div>`
    : a.result ? `<div class="agent-step concl"><div class="lab">${t('agent.conclusion')}</div><div class="markdown sel">${md(a.result)}</div></div>` : '';
  return steps + tail;
}

// A subagent call in the conversation: its trail appears step by step while it works.
function findAgentRun(inp) {
  const title = inp.description || '', head = String(inp.prompt || '').slice(0, 200);
  let best = null;
  for (const v of ui.views.get()) {
    const L = v?.sid ? liveOf(v.sid) : null;
    for (const a of (L?.snap.get()?.agents || [])) {
      if (inp.kind && a.kind !== inp.kind) continue;
      if (head && (a.task || '').includes(head)) best = a;
      else if (!best && title && a.title === title) best = a;
    }
  }
  return best;
}
export function agentBlock(id, inp, result) {
  const a = findAgentRun(inp);
  const running = a ? a.state === 'running' : !result;
  const open = pref('fold')[id] ?? true;
  const dot = running ? 'run' : result?.is_error || a?.state === 'failed' ? 'err' : '';
  return `<div class="tool-block agent-block${open ? ' open' : ''}" data-foldable><div class="tool-head" data-fold="${esc(id)}" role="button" tabindex="0" aria-expanded="${!!open}"><span class="chev" aria-hidden="true">›</span><span class="dot ${dot}"></span><b>${esc(t('tool.subagent'))}</b><span class="tag">${esc(t('agent.kind.' + (inp.kind || 'general')))}</span><span class="sum">${esc(inp.description || String(inp.prompt || '').slice(0, 80))}</span><span class="tdur faint asteps">${agentHead(a)}</span></div><div class="tool-body"><details class="agent-task"${pref('viewMode') === 'verbose' ? ' open' : ''}><summary>${t('agent.taskGiven')}</summary><div class="markdown sel">${md(String(inp.prompt || ''))}</div></details><div class="atrail">${agentTrailHTML(a, id)}</div></div></div>`;
}

// The main tool block: settled calls show entry/diff/result; streaming calls show live code.
export function toolBlock(id, name, input, result, running, meta, curRoot) {
  let inp = input;
  if (typeof inp === 'string') { try { inp = JSON.parse(inp); } catch { inp = {}; } }
  if (name === 'Report' && inp && Array.isArray(inp.claims)) return reportCard(inp, result, meta);
  if (name === 'Agent' && inp && !inp._streaming) return agentBlock(id, inp, result);
  if (inp?._streaming && ['Write', 'Edit', 'MultiEdit', 'Bash'].includes(name)) {
    const p = inp.path || inp.file_path || '';
    let body = '', what = '';
    if (name === 'Write') { body = liveCodeHTML(inp.content); what = `${String(inp.content || '').split('\n').length} ${t('common.lines')}`; }
    else if (name === 'Edit') { body = (inp.old_string != null ? liveCodeHTML(inp.old_string, 'old') : '') + (inp.new_string != null ? liveCodeHTML(inp.new_string, 'new') : ''); what = t('live.editing'); }
    else if (name === 'MultiEdit') { body = liveCodeHTML(inp._open === 'old_string' ? inp.old_string : inp.new_string, inp._open === 'old_string' ? 'old' : 'new'); what = t('live.editing'); }
    else { body = liveCodeHTML(inp.command, 'cmd'); what = t('live.composing'); }
    const open = pref('fold')[id] ?? (pref('viewMode') === 'verbose');
    return `<div class="tool-block live${open ? ' open' : ''}" data-foldable><div class="tool-head" data-fold="${esc(id)}" role="button" tabindex="0" aria-expanded="${!!open}"><span class="chev" aria-hidden="true">›</span><span class="dot run"></span><b>${esc(t({ Write: 'live.writing', Edit: 'live.editing', MultiEdit: 'live.editing', Bash: 'live.composing' }[name]))}</b><span class="sum mono">${esc(p ? relPath(p) : '')}</span><span class="tdur faint">${esc(what)}</span></div><div class="tool-body">${body}</div></div>`;
  }
  const isPath = ['Read', 'Write', 'Edit', 'MultiEdit'].includes(name) && (inp?.path || inp?.file_path);
  let body = '';
  if (name === 'Bash' && inp?.command) body += `<div class="lab">${t('common.command')}<button class="icon-btn mini" data-copy-tool="in" title="${esc(t('action.copyCommand'))}">${I.copy}</button></div><pre class="inp">${esc(inp.command)}</pre>`;
  else if (name === 'Write' && inp?.content != null) body += diffHTML(inp.content.split('\n').map(l => '+' + l).join('\n'));
  else if (name === 'MultiEdit' && Array.isArray(inp?.edits)) body += inp.edits.map(e => diffHTML((e.old_string || '').split('\n').map(l => '-' + l).join('\n') + '\n' + (e.new_string || '').split('\n').map(l => '+' + l).join('\n'))).join('');
  else if (name === 'Edit' && inp?.old_string != null) body += diffHTML(inp.old_string.split('\n').map(l => '-' + l).join('\n') + '\n' + inp.new_string.split('\n').map(l => '+' + l).join('\n'));
  else if (inp && Object.keys(inp).length && name !== 'TodoWrite') body += `<div class="lab">${t('common.input')}</div><pre>${esc(JSON.stringify(inp, null, 2))}</pre>`;
  if (name === 'TodoWrite' && Array.isArray(inp?.todos || inp?.tasks)) body += (inp.todos || inp.tasks).map(x => `<div>${x.status === 'done' ? '☑' : x.status === 'in_progress' ? '◐' : '☐'} ${esc(x.text || x.content || '')}</div>`).join('');
  if (result) {
    const main = String(result.display || result.content || '');
    body += `<div class="lab">${t('common.result')}<button class="icon-btn mini" data-copy-tool="out" title="${esc(t('action.copyResult'))}">${I.copy}</button></div><pre class="res">${esc(main.length > 20000 ? main.slice(0, 20000) + '\n…' : main)}</pre>`;
  }
  const dot = running ? 'run' : result ? (result.is_error ? 'err' : '') : 'pend';
  const open = pref('fold')[id] ?? (pref('viewMode') === 'verbose');
  const errLine = result?.is_error ? String(result.display || result.content || '').split('\n').find(l => l.trim()) || '' : '';
  const fullPath = isPath ? (inp.path || inp.file_path) : '';
  const shown = isPath ? relPath(fullPath) : '';
  const summ = isPath ? `<span class="sum"><span class="path-link" data-path="${esc(shown)}" title="${esc(fullPath)}">${esc(shown)}</span></span>` : `<span class="sum">${esc(toolSummary(name, inp))}</span>`;
  const mcp = name.startsWith('mcp__') ? name.slice(5).split('__') : null;
  const shot = name === 'Browser' && result && String(result.content || '').match(/\.infuse\/shots\/([\w.-]+\.png)/);
  if (shot) body = `<img class="shot" src="/api/shot?name=${encodeURIComponent(shot[1])}&root=${encodeURIComponent(curRoot)}" alt="${esc(t('common.screenshot'))}">` + body;
  return `<div class="tool-block${open || shot ? ' open' : ''}" data-foldable><div class="tool-head" data-fold="${esc(id)}" role="button" tabindex="0" aria-expanded="${!!open}"><span class="chev" aria-hidden="true">›</span><span class="dot ${dot}"></span><b>${mcp ? `${esc(t('common.connector'))} ${esc(mcp[0])}` : esc(t(TOOL_LABELS[name] || name))}</b>${mcp ? `<span class="sum mono">${esc(mcp[1] || '')}</span>` : ''}${summ}${errLine ? `<span class="tool-err" title="${esc(errLine)}">${esc(errLine.replace(/^\[infuse\]\s*/, '').slice(0, 120))}</span>` : ''}${lineCounts(name, inp)}${result?.ms != null ? `<span class="tdur faint" title="${esc(t('common.runTime'))}">${fmtDur(result.ms)}</span>` : ''}</div><div class="tool-body">${body}</div></div>`;
}
