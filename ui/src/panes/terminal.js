// terminal — real interactive shells (ConPTY/pty) in tabs, wired to xterm.js (vendored).
// Theme comes from the CSS token custom properties; resize streams to the pty; input is batched (8ms).
import { $, h, esc, baseName, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { api, tokenHeader, sseUrl, urlWith } from '../core/api.js';

function makeState(pb, extra, c) {
  const s = { pb, extra, c, tabs: [], cur: null };
  // This shell is the human's: it runs unfiltered, so it says so rather than look like the agent's.
  pb.appendChild(h(`<div class="term-note" role="note">${esc(t('terminal.outsideFilter'))}</div>`));
  s.host = h('<div class="term-host"></div>');
  pb.appendChild(s.host);
  s.bar = h('<span class="row row-tight" role="tablist"></span>');
  extra.appendChild(s.bar);
  s.addBtn = h(`<button class="icon-btn" title="${esc(t('terminal.new'))}" aria-label="${esc(t('terminal.new'))}">${I.plus}</button>`);
  extra.appendChild(s.addBtn);
  return s;
}

const cssVar = name => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
const termTheme = () => ({ background: cssVar('--color-surface'), foreground: cssVar('--color-text'), cursor: cssVar('--color-text-secondary'), cursorAccent: cssVar('--color-accent'), selectionBackground: cssVar('--color-selection') });

function drawTabs(s) {
  s.bar.innerHTML = '';
  s.tabs.forEach(tb => {
    const el = h(`<span class="pane-tab${tb === s.cur ? ' on' : ''}" role="tab" tabindex="0">${esc(tb.name)}<span class="x" role="button" aria-label="${esc(t('common.close'))}">×</span></span>`);
    el.onclick = e => { if (e.target.classList.contains('x')) return closeTerm(s, tb); showTab(s, tb); };
    s.bar.appendChild(el);
  });
}

function showTab(s, tb) {
  s.tabs.forEach(x => { if (x.el) x.el.style.display = x === tb ? '' : 'none'; });
  s.cur = tb; drawTabs(s);
  setTimeout(() => { tb.fit?.fit(); tb.term?.focus(); }, 10);
}

function closeTerm(s, tb) {
  tb.dead = true;
  tb.stopInput?.();
  api(`/api/term/${tb.id}/close`, { body: {}, root: s.c.root }).catch(() => { });
  tb.es?.close(); tb.term?.dispose(); tb.el?.remove();
  s.tabs.splice(s.tabs.indexOf(tb), 1);
  if (s.cur === tb) s.cur = s.tabs[s.tabs.length - 1] || null;
  if (s.cur) showTab(s, s.cur); else drawTabs(s);
}

function writeFrame(term, e) {
  const bin = atob(e.data);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  term.write(bytes);
}

function bindExit(tb, es, term) {
  es.addEventListener('exit', () => { term.write(`\r\n\x1b[2m[${t('terminal.exited')}]\x1b[0m\r\n`); es.close(); tb.dead = true; tb.stopInput?.(); });
  es.onerror = () => {
    if (es.readyState === EventSource.CLOSED || tb.dead) return;
    fetch(urlWith(`/api/term/${tb.id}/resize`, { root: tb.root }), {
      method: 'POST', headers: tokenHeader(), body: JSON.stringify({ cols: term.cols, rows: term.rows })
    }).then(x => {
      if (x.status === 404) { es.close(); tb.dead = true; term.write(`\r\n\x1b[2m[${t('terminal.ended')}]\x1b[0m\r\n`); }
    });
  };
}

function bindInput(tb, term) {
  let pending = '', timer = null;
  const flush = () => {
    timer = null;
    if (!pending) return;
    const data = pending; pending = '';
    api(`/api/term/${tb.id}/input`, { body: { data }, root: tb.root }).catch(() => { });
  };
  term.onData(d => {
    if (tb.dead) return;
    pending += d;
    if (!timer) timer = setTimeout(flush, 8);
  });
  tb.stopInput = () => { if (timer) clearTimeout(timer); timer = null; pending = ''; };
  term.onResize(({ cols, rows }) => api(`/api/term/${tb.id}/resize`, { body: { cols, rows }, root: tb.root }).catch(() => { }));
}

// Ctrl+letter arriving without a key code (remote desktop, layouts, automation) is still a control
// character. Copy and paste belong to the browser, so they are never turned into a control byte.
function bindControlKeys(term) {
  term.attachCustomKeyEventHandler(e => {
    if (e.type !== 'keydown' || !e.ctrlKey || e.altKey || e.metaKey || e.shiftKey || e.keyCode) return true;
    const key = e.key.toLowerCase();
    if (!/^[a-z]$/.test(key) || key === 'v' || (key === 'c' && term.hasSelection())) return true;
    term.input(String.fromCharCode(key.toUpperCase().charCodeAt(0) - 64), true);
    e.preventDefault(); return false;
  });
}

function uniqueName(s, base) {
  const taken = new Set(s.tabs.map(tb => tb.name));
  if (!taken.has(base)) return base;
  let n = 2;
  while (taken.has(`${base} ${n}`)) n++;
  return `${base} ${n}`;
}

async function openTerm(s, cwd, run, name) {
  if (!window.Terminal) { s.host.innerHTML = `<div class="empty">xterm.js ${t('terminal.didNotLoad')}</div>`; return; }
  const el = h('<div></div>');
  s.host.appendChild(el);
  const cs = getComputedStyle(document.documentElement);
  const term = new Terminal({ fontFamily: cs.getPropertyValue('--font-mono'), fontSize: 13, cursorBlink: true, theme: termTheme(), allowProposedApi: true, scrollback: 5000 });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit); term.open(el); fit.fit();
  bindControlKeys(term);
  const r = await api('/api/term/open', { body: { cwd: cwd || s.c.dir, cols: term.cols, rows: term.rows, run: run || '' }, root: s.c.root })
    .catch(e => ({ error: e.message, code: e.body?.code, args: e.body?.args }));
  if (r.error) { term.write('\r\n' + (r.code ? t(r.code, r.args) : r.error) + '\r\n'); return; }
  const tb = { id: r.id, root: s.c.root, term, fit, el, name: uniqueName(s, name || baseName(r.cwd) || t('terminal.shell')) };
  const es = new EventSource(sseUrl(`/api/term/${r.id}/stream`, s.c.root));
  tb.es = es;
  es.onmessage = e => writeFrame(term, e);
  bindExit(tb, es, term);
  bindInput(tb, term);
  s.tabs.push(tb); showTab(s, tb);
}

export function mount(pb, extra, c) {
  const s = makeState(pb, extra, c);
  s.addBtn.onclick = () => openTerm(s);
  openTerm(s);
  const themeWatch = new MutationObserver(() => {
    const theme = termTheme();
    for (const tb of s.tabs) if (tb.term) tb.term.options.theme = theme;
  });
  themeWatch.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
  return {
    fit: () => s.cur?.fit?.fit(),
    destroy: () => { themeWatch.disconnect(); s.tabs.slice().forEach(tb => closeTerm(s, tb)); },
    openAt: (cwd, run, name) => openTerm(s, cwd, run, name)
  };
}
