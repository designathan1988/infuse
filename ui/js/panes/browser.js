'use strict';
// browser
// Browser pane: native or streamed tabs, URL bar, dev servers.

// ---------------------------------------------------------------- Browser: tabs, URL bar, dev servers
// The address bar takes an address or words to search. In the app window every tab is a native WebView2
// view placed over the pane. Anywhere else (the UI in a regular browser) every tab is a real browser
// without a window whose screen is streamed here and which receives the mouse, wheel and keyboard —
// so any site opens, with no exceptions, in both cases.
function addressOf(v) {
  v = v.trim(); if (!v) return '';
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(v) || /^(about|data|file):/i.test(v)) return v;
  if (/^(localhost|127\.0\.0\.1|\[::1\]|\d{1,3}(\.\d{1,3}){3})(:\d+)?(\/|$)/i.test(v)) return 'http://' + v;
  if (!/\s/.test(v) && /^[\w-]+(\.[\w-]+)+(:\d+)?(\/.*)?$/.test(v)) return 'https://' + v;
  return '';
}
const searchURL = q => 'https://www.google.com/search?q=' + encodeURIComponent(q);
let browserSeq = 0;
// DevTools modifier bits: Alt 1, Ctrl 2, Meta 4, Shift 8
const keyMods = e => (e.altKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.metaKey ? 4 : 0) | (e.shiftKey ? 8 : 0);
function mountBrowser(pb, extra, c) {
  const tabs = []; let cur = null; let native = !!(window.infuse && window.infuse.native); let picking = false;
  pb.innerHTML = `<div class="urlbar"><button class="ib back" title="${esc(t('ui.page.voltar.1505ad'))}">‹</button><button class="ib fwd" title="${esc(t('ui.pane.browser.avancar.81dae5'))}">›</button><button class="ib rl" title="${esc(t('ui.pane.browser.recarregar.6b6960'))}">${I.reload}</button><input class="url" placeholder="${esc(t('ui.pane.browser.endereco-ou-pesquisa-ex.70b87a'))}"><span class="spin bload" style="display:none"></span><button class="ib pick" title="${esc(t('ui.pane.browser.selecionar-um-elemento-da.d27c69'))}">⌖</button><button class="ib ext" title="${esc(t('ui.pane.browser.abrir-no-navegador-padrao.b49f1a'))}">${I.pop}</button></div><div class="frames" style="flex:1;display:flex;min-height:0;position:relative;overflow:hidden"></div>`;
  extra.innerHTML = `<span class="row tabs" style="gap:2px;min-width:0"></span><button class="ib nt" title="${esc(t('ui.pane.browser.nova-aba.de48fd'))}">${I.plus}</button><button class="btn sm srv">${t('ui.pane.browser.servidores.ae86a5')} ${I.caret}</button>`;
  const frames = $('.frames', pb), url = $('.url', pb), bar = $('.tabs', extra), spin = $('.bload', pb);
  const label = tb => tb.title || (tb.url ? tb.url.replace(/^https?:\/\/(www\.)?/, '').split(/[/?#]/)[0] : t('ui.pane.browser.nova-aba.de48fd'));
  const draw = () => { bar.innerHTML = ''; tabs.forEach(tb => { const el = h(`<span class="ptab${tb === cur ? ' on' : ''}" title="${esc(tb.url || '')}"><span class="trunc" style="max-width:120px">${esc(label(tb))}</span><span class="x">×</span></span>`); el.onclick = e => { if (e.target.classList.contains('x')) return closeTab(tb); show(tb); }; bar.appendChild(el); }); spin.style.display = cur?.loading ? '' : 'none'; };
  const onEv = ev => { const tb = tabs.find(x => x.id === ev.id); if (!tb) return; if (ev.url !== undefined) tb.url = ev.url; if (ev.title !== undefined) tb.title = ev.title; tb.loading = !!ev.loading; if (tb === cur && document.activeElement !== url) url.value = tb.url === 'about:blank' ? '' : (tb.url || ''); if (tb.url && tb.url !== 'about:blank') lsSet('browserUrl:' + c.root, tb.url); draw(); };

  // ---- native views (app window): kept exactly over the pane; hidden while a menu or dialog is open
  let lastPlace = '';
  const rectNow = () => { const r = frames.getBoundingClientRect(), d = window.devicePixelRatio || 1; return { x: Math.round(r.left * d), y: Math.round(r.top * d), w: Math.round(r.width * d), h: Math.round(r.height * d) }; };
  const place = force => {
    if (!native) return;
    const rect = rectNow();
    const visible = document.body.contains(frames) && frames.offsetParent !== null && rect.w > 4 && rect.h > 4 && !document.querySelector('.menu, .overlay, .dragshield') && !document.hidden;
    const key = JSON.stringify([rect, visible, cur?.id]); if (!force && key === lastPlace) return; lastPlace = key;
    tabs.forEach(tb => tb.opened && infuse.call('browser', 'place', { id: tb.id, ...rect, visible: visible && tb === cur }).catch(() => { }));
  };
  if (native) infuse.on('browser', ev => { if (document.body.contains(pb)) onEv(ev); });

  // ---- streamed tabs (any other place): the real browser's screen, our mouse and keyboard
  const size = () => ({ width: Math.max(200, Math.round(frames.clientWidth)), height: Math.max(150, Math.round(frames.clientHeight)), dpr: window.devicePixelRatio || 1 });
  const input = (tb, body) => fetch(`/api/view/input?root=${encodeURIComponent(c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ id: tb.id, ...body }) }).then(r => r.json()).catch(() => ({}));
  const stream = tb => {
    const img = tb.img = h('<img class="rview" draggable="false" alt="">'); frames.appendChild(img);
    const kb = tb.kb = h('<textarea class="rkb" aria-label="teclado da página" autocomplete="off" spellcheck="false"></textarea>'); frames.appendChild(kb); // receives the keyboard for the page
    tb.es = new EventSource(`/api/view/stream?root=${encodeURIComponent(c.root)}&id=${encodeURIComponent(tb.id)}`);
    tb.es.onmessage = e => { const d = JSON.parse(e.data); if (d.type === 'frame') { img.src = 'data:image/jpeg;base64,' + d.data; tb.vw = d.w; tb.vh = d.h; if (d.w) { img.style.width = d.w + 'px'; img.style.height = d.h + 'px'; } } else if (d.type === 'nav') onEv({ id: tb.id, url: d.url, title: d.title, loading: d.loading }); };
    const at = e => { const r = img.getBoundingClientRect(); return { x: Math.round(e.clientX - r.left), y: Math.round(e.clientY - r.top) }; }; // the page's CSS pixels
    const btn = e => ['left', 'middle', 'right'][e.button] || 'left';
    img.onmousedown = async e => {
      e.preventDefault(); setTimeout(() => kb.focus(), 0);
      if (picking) { picking = false; img.classList.remove('picking'); const r = await input(tb, { kind: 'pick', ...at(e) }); if (r.pick) insertPick(r.pick); else toast(r.error || t('ui.pane.browser.nada-nesse-ponto.20df73')); return; }
      input(tb, { kind: 'mouse', type: 'mousePressed', ...at(e), button: btn(e), clickCount: e.detail || 1, modifiers: keyMods(e) });
    };
    img.onmouseup = e => { if (!picking) input(tb, { kind: 'mouse', type: 'mouseReleased', ...at(e), button: btn(e), clickCount: e.detail || 1, modifiers: keyMods(e) }); };
    let moveAt = 0; img.onmousemove = e => { const now = Date.now(); if (now - moveAt < 40) return; moveAt = now; input(tb, { kind: 'mouse', type: 'mouseMoved', ...at(e), modifiers: keyMods(e) }); };
    img.addEventListener('wheel', e => { e.preventDefault(); input(tb, { kind: 'wheel', ...at(e), dx: e.deltaX, dy: e.deltaY }); }, { passive: false });
    img.oncontextmenu = e => e.preventDefault();
    kb.onkeydown = e => {
      if (e.isComposing || e.key === 'Dead' || e.key === 'Process') return; // accents and IME arrive as input
      if ((e.ctrlKey || e.metaKey) && ['l', 't', 'w'].includes(e.key.toLowerCase())) return; // the app's own shortcuts
      e.preventDefault(); e.stopPropagation();
      if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) return input(tb, { kind: 'text', text: e.key });
      const text = e.key === 'Enter' ? '\r' : '';
      input(tb, { kind: 'key', type: 'keyDown', key: e.key, code: e.code, keyCode: e.keyCode, text, modifiers: keyMods(e) }).then(() => input(tb, { kind: 'key', type: 'keyUp', key: e.key, code: e.code, keyCode: e.keyCode, modifiers: keyMods(e) }));
    };
    kb.addEventListener('paste', e => { const txt = e.clipboardData?.getData('text'); if (txt) { e.preventDefault(); input(tb, { kind: 'text', text: txt }); } });
    kb.oninput = () => { if (kb.value) { input(tb, { kind: 'text', text: kb.value }); kb.value = ''; } }; // composed characters (accents, IME)
  };
  // live resize: the page reflows while the pane is dragged — one request in flight, the latest size wins
  let resizeBusy = false, resizeAgain = false, resizeRaf = 0;
  const sendResize = async () => {
    if (resizeBusy) { resizeAgain = true; return; }
    resizeBusy = true; const sz = size();
    await Promise.all(tabs.filter(tb => tb.img).map(tb => input(tb, { kind: 'resize', ...sz })));
    resizeBusy = false; if (resizeAgain) { resizeAgain = false; sendResize(); }
  };
  const resizeStreams = () => { if (native) return; cancelAnimationFrame(resizeRaf); resizeRaf = requestAnimationFrame(sendResize); };
  const ro = new ResizeObserver(() => { place(); resizeStreams(); }); ro.observe(frames);
  const timer = setInterval(() => place(), 200);

  const show = tb => {
    cur = tb; tabs.forEach(x => { if (x.img) x.img.style.display = x === tb ? '' : 'none'; });
    url.value = tb.url && tb.url !== 'about:blank' ? tb.url : ''; draw(); place(true); if (native && tb.opened) infuse.call('browser', 'focus', { id: tb.id }).catch(() => { });
  };
  const add = u => { const tb = { id: 'b' + (++browserSeq) + '-' + Date.now(), url: u }; tabs.push(tb); if (!native) stream(tb); show(tb); if (u) go(u); else setTimeout(() => url.focus(), 30); return tb; };
  const closeTab = tb => { const i = tabs.indexOf(tb); tabs.splice(i, 1); if (tb.opened) infuse.call('browser', 'close', { id: tb.id }).catch(() => { }); if (tb.es) { tb.es.close(); input(tb, { kind: 'close' }); } tb.img?.remove(); tb.kb?.remove(); if (!tabs.length) add(''); else show(tabs[Math.min(i, tabs.length - 1)]); };
  const go = async v => {
    if (!cur) return; const typed = v.trim(); const addr = addressOf(typed) || (typed ? searchURL(typed) : ''); if (!addr) return;
    cur.url = addr; cur.loading = true; url.value = addr; draw();
    if (native) {
      try { await infuse.call('browser', cur.opened ? 'navigate' : 'open', { id: cur.id, url: addr, ...rectNow(), visible: true }); cur.opened = true; place(true); return; }
      catch (e) { native = false; tabs.forEach(x => { if (!x.img) stream(x); }); show(cur); }
    }
    const r = await api('/api/view/open', { body: { id: cur.id, url: addr, ...size() }, root: c.root });
    if (!r.ok) { cur.loading = false; draw(); toast(r.error || t('ui.pane.browser.nao-abriu.6fa7de')); }
    cur.kb?.focus();
  };
  const nav = op => { if (!cur) return; if (native && cur.opened) return infuse.call('browser', op, { id: cur.id }).catch(() => { }); if (cur.img) input(cur, { kind: op }); };
  url.onkeydown = e => { if (e.key === 'Enter') { go(url.value); url.blur(); } if (e.key === 'Escape') { url.value = cur?.url || ''; url.blur(); } };
  url.onfocus = () => url.select();
  $('.back', pb).onclick = () => nav('back'); $('.fwd', pb).onclick = () => nav('forward'); $('.rl', pb).onclick = () => nav('reload');
  $('.ext', pb).onclick = () => cur?.url && api('/api/open', { body: { url: cur.url } });
  $('.nt', extra).onclick = () => add('');
  const pick = async () => {
    if (!cur?.url) return toast(t('ui.pane.browser.abra-uma-pagina-primeiro.70164d'));
    if (native && cur.opened) { toast(t('ui.pane.browser.clique-num-elemento-da.0bf27b')); return infuse.call('browser', 'pick', { id: cur.id }).catch(e => toast(e.message)); }
    picking = true; cur.img?.classList.add('picking'); toast(t('ui.pane.browser.clique-num-elemento-da.b24e5f'));
  };
  $('.pick', pb).onclick = pick;
  $('.srv', extra).onclick = async e => {
    const anchor = e.currentTarget; const list = await api('/api/devservers', { root: c.root });
    const items = list.length ? list.flatMap(s => [{ label: s.name, d: `:${s.port} · ${s.running || s.listening ? t('ui.pane.browser.rodando.cb3b19') : t('ui.chat.parado.ca805e')}${s.approved ? '' : ' · ' + t('ui.pane.browser.nao-aprovado.88d9fb')}`, f: async () => { if (!s.running && !s.listening) { let r = await api('/api/devservers', { body: { action: 'start', name: s.name }, root: c.root }); if (r.needsApproval) { if (!confirm(t('ui.pane.browser.executar-este-comando-de.8fcd45') + '\n\n' + r.command)) return; r = await api('/api/devservers', { body: { action: 'approve', name: s.name }, root: c.root }); } if (r.error) return toast(r.error); toast(t('ui.pane.browser.iniciando.3ff323') + s.name); setTimeout(() => go(s.url), 2500); } else go(s.url); } }]) : [{ label: 'Nenhum servidor configurado', sub: 'Declare em .infuse/launch.json ou .claude/launch.json', f: () => { } }];
    if (list.some(s => s.running)) items.push({ sep: 1 }, ...list.filter(s => s.running).map(s => ({ label: t('ui.pane.browser.parar.5d3458') + s.name, f: () => api('/api/devservers', { body: { action: 'stop', name: s.name }, root: c.root }) })), { label: 'Parar todos', danger: 1, f: () => api('/api/devservers', { body: { action: 'stopall' }, root: c.root }) });
    menu(anchor, items, null, { alignRight: true });
  };
  add(lsGet('browserUrl:' + c.root, ''));
  return { go: v => { if (!cur) add(''); go(v); }, pick, destroy: () => { clearInterval(timer); ro.disconnect(); tabs.forEach(tb => { if (tb.opened) infuse.call('browser', 'close', { id: tb.id }).catch(() => { }); if (tb.es) { tb.es.close(); input(tb, { kind: 'close' }); } }); } };
}
