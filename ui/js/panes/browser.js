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
  const b = makeBrowserPane(pb, extra, c);
  b.start();
  return { go: v => { if (!b.cur) b.add(''); b.go(v); }, pick: b.pick, destroy: () => b.destroy() };
}

function makeBrowserPane(pb, extra, c) {
  const b = Object.assign(Object.create(makeBrowserPane.prototype), {
    pb, extra, c, tabs: [], cur: null, native: !!(window.infuse && window.infuse.native), picking: false,
    lastPlace: '', resizeBusy: false, resizeAgain: false, resizeRaf: 0, timer: 0, ro: null,
  });
  pb.innerHTML = `<div class="urlbar"><button class="ib back" title="${esc(t('ui.page.voltar.1505ad'))}">‹</button><button class="ib fwd" title="${esc(t('ui.pane.browser.avancar.81dae5'))}">›</button><button class="ib rl" title="${esc(t('ui.pane.browser.recarregar.6b6960'))}">${I.reload}</button><input class="url" placeholder="${esc(t('ui.pane.browser.endereco-ou-pesquisa-ex.70b87a'))}"><span class="spin bload hidden"></span><button class="ib pick" title="${esc(t('ui.pane.browser.selecionar-um-elemento-da.d27c69'))}">⌖</button><button class="ib ext" title="${esc(t('ui.pane.browser.abrir-no-navegador-padrao.b49f1a'))}">${I.pop}</button></div><div class="frames"></div>`;
  extra.innerHTML = `<span class="row tabs u-gap2px-minw0"></span><button class="ib nt" title="${esc(t('ui.pane.browser.nova-aba.de48fd'))}">${I.plus}</button><button class="btn sm srv">${t('ui.pane.browser.servidores.ae86a5')} ${I.caret}</button>`;
  b.frames = $('.frames', pb);
  b.url = $('.url', pb);
  b.bar = $('.tabs', extra);
  b.spin = $('.bload', pb);
  b.label = tb => tb.title || (tb.url ? tb.url.replace(/^https?:\/\/(www\.)?/, '').split(/[/?#]/)[0] : t('ui.pane.browser.nova-aba.de48fd'));
  b.draw = () => {
    b.bar.innerHTML = '';
    b.tabs.forEach(tb => {
      const el = h(`<span class="ptab${tb === b.cur ? ' on' : ''}" title="${esc(tb.url || '')}"><span class="trunc maxw120">${esc(b.label(tb))}</span><span class="x">×</span></span>`);
      el.onclick = e => { if (e.target.classList.contains('x')) return b.closeTab(tb); b.show(tb); };
      b.bar.appendChild(el);
    });
    b.spin.classList.toggle('hidden', !b.cur?.loading);
  };
  b.onEv = ev => {
    const tb = b.tabs.find(x => x.id === ev.id);
    if (!tb) return;
    if (ev.url !== undefined) tb.url = ev.url;
    if (ev.title !== undefined) tb.title = ev.title;
    tb.loading = !!ev.loading;
    if (tb === b.cur && document.activeElement !== b.url) b.url.value = tb.url === 'about:blank' ? '' : (tb.url || '');
    if (tb.url && tb.url !== 'about:blank') lsSet('browserUrl:' + c.root, tb.url);
    b.draw();
  };
  b.start = () => {
    if (b.native) infuse.on('browser', ev => { if (document.body.contains(pb)) b.onEv(ev); });
    b.ro = new ResizeObserver(() => { b.place(); b.resizeStreams(); });
    b.ro.observe(b.frames);
    b.timer = setInterval(() => b.place(), 200);
    $('.back', pb).onclick = () => b.nav('back');
    $('.fwd', pb).onclick = () => b.nav('forward');
    $('.rl', pb).onclick = () => b.nav('reload');
    $('.ext', pb).onclick = () => b.cur?.url && api('/api/open', { body: { url: b.cur.url } });
    $('.nt', extra).onclick = () => b.add('');
    $('.pick', pb).onclick = () => b.pick();
    $('.srv', extra).onclick = e => b.serversMenu(e.currentTarget);
    b.url.onkeydown = e => { if (e.key === 'Enter') { b.go(b.url.value); b.url.blur(); } if (e.key === 'Escape') { b.url.value = b.cur?.url || ''; b.url.blur(); } };
    b.url.onfocus = () => b.url.select();
    b.add(lsGet('browserUrl:' + c.root, ''));
  };
  b.destroy = () => {
    clearInterval(b.timer);
    b.ro?.disconnect();
    b.tabs.forEach(tb => { if (tb.opened) infuse.call('browser', 'close', { id: tb.id }).catch(() => { }); if (tb.es) { tb.es.close(); b.input(tb, { kind: 'close' }); } });
  };
  return b;
}


// native views (app window): kept exactly over the pane; hidden while a menu or dialog is open
makeBrowserPane.prototype.rectNow = function () {
  const r = this.frames.getBoundingClientRect(), d = window.devicePixelRatio || 1;
  return { x: Math.round(r.left * d), y: Math.round(r.top * d), w: Math.round(r.width * d), h: Math.round(r.height * d) };
};

makeBrowserPane.prototype.place = function (force) {
  if (!this.native) return;
  const rect = this.rectNow();
  const visible = document.body.contains(this.frames) && this.frames.offsetParent !== null && rect.w > 4 && rect.h > 4 && !document.querySelector('.menu, .overlay, .dragshield') && !document.hidden;
  const key = JSON.stringify([rect, visible, this.cur?.id]);
  if (!force && key === this.lastPlace) return;
  this.lastPlace = key;
  const cur = this.cur;
  this.tabs.forEach(tb => tb.opened && infuse.call('browser', 'place', { id: tb.id, ...rect, visible: visible && tb === cur }).catch(() => { }));
};

makeBrowserPane.prototype.size = function () {
  return { width: Math.max(200, Math.round(this.frames.clientWidth)), height: Math.max(150, Math.round(this.frames.clientHeight)), dpr: window.devicePixelRatio || 1 };
};

makeBrowserPane.prototype.input = function (tb, body) {
  return fetch(`/api/view/input?root=${encodeURIComponent(this.c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ id: tb.id, ...body }) }).then(r => r.json()).catch(() => ({}));
};

// streamed tabs (any other place): the real browser's screen, our mouse and keyboard
makeBrowserPane.prototype.stream = function (tb) {
  const b = this;
  const img = tb.img = h('<img class="rview" draggable="false" alt="">');
  b.frames.appendChild(img);
  const kb = tb.kb = h('<textarea class="rkb" aria-label="teclado da página" autocomplete="off" spellcheck="false"></textarea>');
  b.frames.appendChild(kb); // receives the keyboard for the page
  tb.es = new EventSource(`/api/view/stream?root=${encodeURIComponent(b.c.root)}&id=${encodeURIComponent(tb.id)}`);
  tb.es.onmessage = e => {
    const d = JSON.parse(e.data);
    if (d.type === 'frame') { img.src = 'data:image/jpeg;base64,' + d.data; tb.vw = d.w; tb.vh = d.h; if (d.w) { img.style.width = d.w + 'px'; img.style.height = d.h + 'px'; } }
    else if (d.type === 'nav') b.onEv({ id: tb.id, url: d.url, title: d.title, loading: d.loading });
  };
  const at = e => { const r = img.getBoundingClientRect(); return { x: Math.round(e.clientX - r.left), y: Math.round(e.clientY - r.top) }; }; // the page's CSS pixels
  const btn = e => ['left', 'middle', 'right'][e.button] || 'left';
  img.onmousedown = async e => {
    e.preventDefault(); setTimeout(() => kb.focus(), 0);
    if (b.picking) {
      b.picking = false; img.classList.remove('picking');
      const r = await b.input(tb, { kind: 'pick', ...at(e) });
      if (r.pick) insertPick(r.pick); else toast(r.error || t('ui.pane.browser.nada-nesse-ponto.20df73'));
      return;
    }
    b.input(tb, { kind: 'mouse', type: 'mousePressed', ...at(e), button: btn(e), clickCount: e.detail || 1, modifiers: keyMods(e) });
  };
  img.onmouseup = e => { if (!b.picking) b.input(tb, { kind: 'mouse', type: 'mouseReleased', ...at(e), button: btn(e), clickCount: e.detail || 1, modifiers: keyMods(e) }); };
  let moveAt = 0;
  img.onmousemove = e => { const now = Date.now(); if (now - moveAt < 40) return; moveAt = now; b.input(tb, { kind: 'mouse', type: 'mouseMoved', ...at(e), modifiers: keyMods(e) }); };
  img.addEventListener('wheel', e => { e.preventDefault(); b.input(tb, { kind: 'wheel', ...at(e), dx: e.deltaX, dy: e.deltaY }); }, { passive: false });
  img.oncontextmenu = e => e.preventDefault();
  kb.onkeydown = e => {
    if (e.isComposing || e.key === 'Dead' || e.key === 'Process') return; // accents and IME arrive as input
    if ((e.ctrlKey || e.metaKey) && ['l', 't', 'w'].includes(e.key.toLowerCase())) return; // the app's own shortcuts
    e.preventDefault(); e.stopPropagation();
    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) return b.input(tb, { kind: 'text', text: e.key });
    const text = e.key === 'Enter' ? '\r' : '';
    b.input(tb, { kind: 'key', type: 'keyDown', key: e.key, code: e.code, keyCode: e.keyCode, text, modifiers: keyMods(e) }).then(() => b.input(tb, { kind: 'key', type: 'keyUp', key: e.key, code: e.code, keyCode: e.keyCode, modifiers: keyMods(e) }));
  };
  kb.addEventListener('paste', e => { const txt = e.clipboardData?.getData('text'); if (txt) { e.preventDefault(); b.input(tb, { kind: 'text', text: txt }); } });
  kb.oninput = () => { if (kb.value) { b.input(tb, { kind: 'text', text: kb.value }); kb.value = ''; } }; // composed characters (accents, IME)
};

// live resize: the page reflows while the pane is dragged — one request in flight, the latest size wins
makeBrowserPane.prototype.sendResize = async function () {
  if (this.resizeBusy) { this.resizeAgain = true; return; }
  this.resizeBusy = true;
  await Promise.all(this.tabs.filter(tb => tb.img).map(tb => this.input(tb, { kind: 'resize', ...this.size() })));
  this.resizeBusy = false;
  if (this.resizeAgain) { this.resizeAgain = false; this.sendResize(); }
};

makeBrowserPane.prototype.resizeStreams = function () {
  if (this.native) return;
  cancelAnimationFrame(this.resizeRaf);
  this.resizeRaf = requestAnimationFrame(() => this.sendResize());
};

makeBrowserPane.prototype.show = function (tb) {
  this.cur = tb;
  this.tabs.forEach(x => { if (x.img) x.img.classList.toggle('hidden', x !== tb); });
  this.url.value = tb.url && tb.url !== 'about:blank' ? tb.url : '';
  this.draw(); this.place(true);
  if (this.native && tb.opened) infuse.call('browser', 'focus', { id: tb.id }).catch(() => { });
};

makeBrowserPane.prototype.add = function (u) {
  const tb = { id: 'b' + (++browserSeq) + '-' + Date.now(), url: u };
  this.tabs.push(tb);
  if (!this.native) this.stream(tb);
  this.show(tb);
  if (u) this.go(u); else setTimeout(() => this.url.focus(), 30);
  return tb;
};

makeBrowserPane.prototype.closeTab = function (tb) {
  const i = this.tabs.indexOf(tb);
  this.tabs.splice(i, 1);
  if (tb.opened) infuse.call('browser', 'close', { id: tb.id }).catch(() => { });
  if (tb.es) { tb.es.close(); this.input(tb, { kind: 'close' }); }
  tb.img?.remove(); tb.kb?.remove();
  if (!this.tabs.length) this.add(''); else this.show(this.tabs[Math.min(i, this.tabs.length - 1)]);
};

makeBrowserPane.prototype.go = async function (v) {
  if (!this.cur) return;
  const typed = v.trim();
  const addr = addressOf(typed) || (typed ? searchURL(typed) : '');
  if (!addr) return;
  this.cur.url = addr; this.cur.loading = true; this.url.value = addr; this.draw();
  if (this.native) {
    try { await infuse.call('browser', this.cur.opened ? 'navigate' : 'open', { id: this.cur.id, url: addr, ...this.rectNow(), visible: true }); this.cur.opened = true; this.place(true); return; }
    catch (e) { this.native = false; this.tabs.forEach(x => { if (!x.img) this.stream(x); }); this.show(this.cur); }
  }
  const r = await api('/api/view/open', { body: { id: this.cur.id, url: addr, ...this.size() }, root: this.c.root });
  if (!r.ok) { this.cur.loading = false; this.draw(); toast(r.error || t('ui.pane.browser.nao-abriu.6fa7de')); }
  this.cur.kb?.focus();
};

makeBrowserPane.prototype.nav = function (op) {
  if (!this.cur) return;
  if (this.native && this.cur.opened) return infuse.call('browser', op, { id: this.cur.id }).catch(() => { });
  if (this.cur.img) this.input(this.cur, { kind: op });
};

makeBrowserPane.prototype.pick = async function () {
  if (!this.cur?.url) return toast(t('ui.pane.browser.abra-uma-pagina-primeiro.70164d'));
  if (this.native && this.cur.opened) { toast(t('ui.pane.browser.clique-num-elemento-da.0bf27b')); return infuse.call('browser', 'pick', { id: this.cur.id }).catch(e => toast(e.message)); }
  this.picking = true; this.cur.img?.classList.add('picking'); toast(t('ui.pane.browser.clique-num-elemento-da.b24e5f'));
};

makeBrowserPane.prototype.serversMenu = async function (anchor) {
  const b = this;
  const list = await api('/api/devservers', { root: b.c.root });
  const items = list.length ? list.flatMap(s => [{ label: s.name, d: `:${s.port} · ${s.running || s.listening ? t('ui.pane.browser.rodando.cb3b19') : t('ui.chat.parado.ca805e')}${s.approved ? '' : ' · ' + t('ui.pane.browser.nao-aprovado.88d9fb')}`, f: () => b.startServer(s) }]) : [{ label: 'Nenhum servidor configurado', sub: 'Declare em .infuse/launch.json ou .claude/launch.json', f: () => { } }];
  if (list.some(s => s.running)) items.push({ sep: 1 }, ...list.filter(s => s.running).map(s => ({ label: t('ui.pane.browser.parar.5d3458') + s.name, f: () => api('/api/devservers', { body: { action: 'stop', name: s.name }, root: b.c.root }) })), { label: 'Parar todos', danger: 1, f: () => api('/api/devservers', { body: { action: 'stopall' }, root: b.c.root }) });
  menu(anchor, items, null, { alignRight: true });
};

makeBrowserPane.prototype.startServer = async function (s) {
  if (s.running || s.listening) return this.go(s.url);
  let r = await api('/api/devservers', { body: { action: 'start', name: s.name }, root: this.c.root });
  if (r.needsApproval) {
    if (!confirm(t('ui.pane.browser.executar-este-comando-de.8fcd45') + '\n\n' + r.command)) return;
    r = await api('/api/devservers', { body: { action: 'approve', name: s.name }, root: this.c.root });
  }
  if (r.error) return toast(r.error);
  toast(t('ui.pane.browser.iniciando.3ff323') + s.name);
  setTimeout(() => this.go(s.url), 2500);
};
