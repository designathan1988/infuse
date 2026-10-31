'use strict';
// terminal
// Terminal pane: real shells in tabs.

// ---------------------------------------------------------------- Terminal: real shells (ConPTY / pty) in tabs
function mountTerminal(pb, extra, c) {
  const tabs = []; let cur = null;
  const host = h('<div class="termhost"></div>'); pb.appendChild(host);
  const bar = h('<span class="row u-gap2px-minw0"></span>'); extra.appendChild(bar);
  const addBtn = h(`<button class="ib" title="${esc(t('ui.pane.terminal.novo-terminal.1ada5a'))}">${I.plus}</button>`); extra.appendChild(addBtn);
  const dark = () => document.documentElement.classList.contains('dark');
  const theme = () => { const cs = getComputedStyle(document.documentElement); return { background: cs.getPropertyValue('--surface').trim(), foreground: cs.getPropertyValue('--text').trim(), cursor: cs.getPropertyValue('--text2').trim(), selectionBackground: cs.getPropertyValue('--sel-bg').trim() }; };
  const drawTabs = () => { bar.innerHTML = ''; tabs.forEach((tb, i) => { const el = h(`<span class="ptab${tb === cur ? ' on' : ''}">${esc(tb.name)}<span class="x">×</span></span>`); el.onclick = e => { if (e.target.classList.contains('x')) return closeTab(tb); show(tb); }; bar.appendChild(el); }); };
  const show = tb => { tabs.forEach(x => x.el.classList.toggle('hidden', x !== tb)); cur = tb; drawTabs(); setTimeout(() => { tb.fit.fit(); tb.term.focus(); }, 10); };
  const closeTab = tb => { tb.dead = true; api(`/api/term/${tb.id}/close`, { body: {}, root: c.root }); tb.es?.close(); tb.term.dispose(); tb.el.remove(); tabs.splice(tabs.indexOf(tb), 1); if (cur === tb) cur = tabs[tabs.length - 1] || null; if (cur) show(cur); else drawTabs(); };
  const open = async (cwd, run, name) => {
    if (!window.Terminal) { host.innerHTML = `<div class="empty">xterm.js não carregou</div>`; return; }
    const el = h('<div></div>'); host.appendChild(el);
    const term = new Terminal({ fontFamily: getComputedStyle(document.body).getPropertyValue('--mono'), fontSize: 13, cursorBlink: true, theme: theme(), allowProposedApi: true, scrollback: 5000 });
    const fit = new FitAddon.FitAddon(); term.loadAddon(fit); term.open(el); fit.fit();
    // Ctrl+letter that arrives without a key code (remote desktop, some layouts, automation) is still a control character
    term.attachCustomKeyEventHandler(e => { if (e.type === 'keydown' && e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && !e.keyCode && /^[a-z]$/i.test(e.key)) { term.input(String.fromCharCode(e.key.toUpperCase().charCodeAt(0) - 64), true); e.preventDefault(); return false; } return true; });
    const r = await api('/api/term/open', { body: { cwd: cwd || c.dir, cols: term.cols, rows: term.rows, run: run || '' }, root: c.root });
    if (r.error) { term.write('\r\n' + r.error + '\r\n'); return; }
    const tb = { id: r.id, term, fit, el, name: name || baseName(r.cwd) || 'shell' + (tabs.length + 1) };
    const es = new EventSource(`/api/term/${r.id}/stream?root=${encodeURIComponent(c.root)}`); tb.es = es;
    es.onmessage = e => { const bin = atob(e.data); const bytes = new Uint8Array(bin.length); for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i); term.write(bytes); };
    es.addEventListener('exit', () => { term.write(`\r\n\x1b[2m[${t('ui.pane.terminal.processo-encerrado.33bff5')}]\x1b[0m\r\n`); es.close(); tb.dead = true; });
    es.onerror = () => { if (es.readyState === EventSource.CLOSED || tb.dead) return; fetch(`/api/term/${r.id}/resize?root=${encodeURIComponent(c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ cols: term.cols, rows: term.rows }) }).then(x => { if (x.status === 404) { es.close(); tb.dead = true; term.write(`\r\n\x1b[2m[${t('ui.pane.terminal.terminal-encerrado.b3257b')}]\x1b[0m\r\n`); } }); };
    let pending = '', timer = null;
    term.onData(d => { if (tb.dead) return; pending += d; if (!timer) timer = setTimeout(() => { const data = pending; pending = ''; timer = null; fetch(`/api/term/${r.id}/input?root=${encodeURIComponent(c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ data }) }); }, 8); });
    term.onResize(({ cols, rows }) => api(`/api/term/${r.id}/resize`, { body: { cols, rows }, root: c.root }));
    tabs.push(tb); show(tb);
  };
  addBtn.onclick = () => open();
  open();
  return { fit: () => cur?.fit.fit(), destroy: () => tabs.slice().forEach(closeTab), openAt: (cwd, run, name) => open(cwd, run, name) };
}
