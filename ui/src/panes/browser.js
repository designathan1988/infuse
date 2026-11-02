// browser — tabs, URL bar, dev servers. Inside the app window every tab is a native WebView2 view
// placed exactly over the pane; anywhere else (UI in a regular browser) each tab is a windowless real
// browser whose screen streams here (mouse, wheel, keyboard forwarded) — so any site opens, always.
import { $, h, esc, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { api, tokenHeader, urlWith, sseUrl } from '../core/api.js';
import { pref } from '../core/store.js';
import { menu } from '../components/menu.js';
import { toast } from '../components/toast.js';

function addressOf(v) {
  v = v.trim();
  if (!v) return '';
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(v) || /^(about|data|file):/i.test(v)) return v;
  if (/^(localhost|127\.0\.0\.1|\[::1\]|\d{1,3}(\.\d{1,3}){3})(:\d+)?(\/|$)/i.test(v)) return 'http://' + v;
  if (!/\s/.test(v) && /^[\w-]+(\.[\w-]+)+(:\d+)?(\/.*)?$/.test(v)) return 'https://' + v;
  return '';
}
const searchURL = q => 'https://www.google.com/search?q=' + encodeURIComponent(q);
let seq = 0;
const keyMods = e => (e.altKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.metaKey ? 4 : 0) | (e.shiftKey ? 8 : 0);
const labelOf = tb => tb.title || (tb.url ? tb.url.replace(/^https?:\/\/(www\.)?/, '').split(/[/?#]/)[0] : t('browser.newTab'));

function makeState(pb, extra, c) {
  const b = {
    pb, extra, c, tabs: [], cur: null, picking: false, lastPlace: '',
    native: !!(window.infuse && window.infuse.native), resizeBusy: false, resizeAgain: false, resizeRaf: 0
  };
  pb.innerHTML = `<div class="urlbar">
      <button class="icon-btn back" aria-label="${esc(t('browser.back'))}">‹</button>
      <button class="icon-btn fwd" aria-label="${esc(t('browser.forward'))}">›</button>
      <button class="icon-btn rl" aria-label="${esc(t('browser.reload'))}">${I.reload}</button>
      <input class="url" placeholder="${esc(t('browser.addressPlaceholder'))}" aria-label="${esc(t('browser.address'))}">
      <span class="spin bload" hidden aria-hidden="true"></span>
      <button class="icon-btn pick" title="${esc(t('pick.aria'))}">⌖</button>
      <button class="icon-btn ext" title="${esc(t('browser.openExternal'))}">${I.pop}</button>
    </div><div class="frames"><div class="browser-empty"><b>${t('browser.emptyTitle')}</b><span>${t('browser.emptyHint')}</span></div></div>`;
  extra.innerHTML = `<span class="row tabbar row-tight" role="tablist"></span><button class="icon-btn nt" title="${esc(t('browser.newTab'))}" aria-label="${esc(t('browser.newTab'))}">${I.plus}</button><button class="btn sm srv">${t('browser.servers')} ${I.caret}</button>`;
  b.frames = pb.querySelector('.frames');
  b.url = pb.querySelector('.url');
  b.bar = extra.querySelector('.tabbar');
  b.spin = pb.querySelector('.bload');
  b.empty = pb.querySelector('.browser-empty');
  return b;
}

function drawTabs(b) {
  b.bar.innerHTML = '';
  b.tabs.forEach(tb => {
    const el = h(`<span class="pane-tab${tb === b.cur ? ' on' : ''}" role="tab" tabindex="0" title="${esc(tb.url || '')}"><span class="trunc tab-label">${esc(labelOf(tb))}</span><span class="x" role="button" aria-label="${esc(t('common.close'))}">×</span></span>`);
    el.onclick = e => { if (e.target.classList.contains('x')) return closeTab(b, tb); showTab(b, tb); };
    b.bar.appendChild(el);
  });
  b.spin.hidden = !b.cur?.loading;
}

function onBrowserEvent(b, ev) {
  const tb = b.tabs.find(x => x.id === ev.id);
  if (!tb) return;
  if (ev.url !== undefined) tb.url = ev.url;
  if (ev.title !== undefined) tb.title = ev.title;
  tb.loading = !!ev.loading;
  if (tb === b.cur && document.activeElement !== b.url) b.url.value = tb.url === 'about:blank' ? '' : (tb.url || '');
  if (tb.url && tb.url !== 'about:blank') pref('browserUrl', { ...pref('browserUrl'), [b.c.root]: tb.url });
  drawTabs(b);
}

function rectNow(b) {
  const r = b.frames.getBoundingClientRect(), d = window.devicePixelRatio || 1;
  return { x: Math.round(r.left * d), y: Math.round(r.top * d), w: Math.round(r.width * d), h: Math.round(r.height * d) };
}

function placeNative(b, force) {
  if (!b.native) return;
  const rect = rectNow(b);
  const visible = document.body.contains(b.frames) && b.frames.offsetParent !== null && rect.w > 4 && rect.h > 4 && !document.querySelector('.menu, .overlay') && !document.hidden;
  const key = JSON.stringify([rect, visible, b.cur?.id]);
  if (!force && key === b.lastPlace) return;
  b.lastPlace = key;
  b.tabs.forEach(tb => tb.opened && infuse.call('browser', 'place', { id: tb.id, ...rect, visible: visible && tb === b.cur }).catch(() => { }));
}

const sizeOf = b => ({ width: Math.max(200, Math.round(b.frames.clientWidth)), height: Math.max(150, Math.round(b.frames.clientHeight)), dpr: window.devicePixelRatio || 1 });

function input(b, tb, body) {
  return fetch(urlWith('/api/view/input', { root: b.c.root }), {
    method: 'POST', headers: tokenHeader(), body: JSON.stringify({ id: tb.id, ...body })
  }).then(r => r.json()).catch(() => ({}));
}

function bindStreamMouse(b, tb, img, kb) {
  const at = e => {
    const r = img.getBoundingClientRect();
    return { x: Math.round(e.clientX - r.left), y: Math.round(e.clientY - r.top) };
  };
  const btn = e => ['left', 'middle', 'right'][e.button] || 'left';
  img.onmousedown = async e => {
    e.preventDefault();
    setTimeout(() => kb.focus(), 0);
    if (b.picking) {
      b.picking = false; img.classList.remove('picking');
      const r = await input(b, tb, { kind: 'pick', ...at(e) });
      if (r.pick) import('../components/view.js').then(m => m.insertPick(r.pick));
      else toast(r.error || t('pick.nothingThere'));
      return;
    }
    input(b, tb, { kind: 'mouse', type: 'mousePressed', ...at(e), button: btn(e), clickCount: e.detail || 1, modifiers: keyMods(e) });
  };
  img.onmouseup = e => { if (!b.picking) input(b, tb, { kind: 'mouse', type: 'mouseReleased', ...at(e), button: btn(e), clickCount: e.detail || 1, modifiers: keyMods(e) }); };
  let moveAt = 0;
  img.onmousemove = e => {
    const now = Date.now();
    if (now - moveAt < 40) return;
    moveAt = now;
    input(b, tb, { kind: 'mouse', type: 'mouseMoved', ...at(e), modifiers: keyMods(e) });
  };
  img.addEventListener('wheel', e => { e.preventDefault(); input(b, tb, { kind: 'wheel', ...at(e), dx: e.deltaX, dy: e.deltaY }); }, { passive: false });
  img.oncontextmenu = e => e.preventDefault();
}

function bindStreamKeyboard(b, tb, kb) {
  kb.onkeydown = e => {
    if (e.isComposing || e.key === 'Dead' || e.key === 'Process') return;
    if ((e.ctrlKey || e.metaKey) && ['l', 't', 'w'].includes(e.key.toLowerCase())) return;
    e.preventDefault(); e.stopPropagation();
    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) return input(b, tb, { kind: 'text', text: e.key });
    const text = e.key === 'Enter' ? '\r' : '';
    input(b, tb, { kind: 'key', type: 'keyDown', key: e.key, code: e.code, keyCode: e.keyCode, text, modifiers: keyMods(e) })
      .then(() => input(b, tb, { kind: 'key', type: 'keyUp', key: e.key, code: e.code, keyCode: e.keyCode, modifiers: keyMods(e) }));
  };
  kb.addEventListener('paste', e => {
    const txt = e.clipboardData?.getData('text');
    if (txt) { e.preventDefault(); input(b, tb, { kind: 'text', text: txt }); }
  });
  kb.oninput = () => { if (kb.value) { input(b, tb, { kind: 'text', text: kb.value }); kb.value = ''; } };
}

function streamTab(b, tb) {
  const img = tb.img = h('<img class="remote-view" draggable="false" alt="">');
  b.frames.appendChild(img);
  const kb = tb.kb = h(`<textarea class="remote-kb" aria-label="${esc(t('browser.pageKeyboard'))}" autocomplete="off" spellcheck="false"></textarea>`);
  b.frames.appendChild(kb);
  tb.es = new EventSource(sseUrl(`/api/view/stream?id=${encodeURIComponent(tb.id)}`, b.c.root));
  tb.es.onmessage = e => {
    const d = JSON.parse(e.data);
    if (d.type === 'frame') {
      img.src = 'data:image/jpeg;base64,' + d.data;
      tb.vw = d.w; tb.vh = d.h;
      if (d.w) { img.style.width = d.w + 'px'; img.style.height = d.h + 'px'; }
    } else if (d.type === 'nav') onBrowserEvent(b, { id: tb.id, url: d.url, title: d.title, loading: d.loading });
  };
  bindStreamMouse(b, tb, img, kb);
  bindStreamKeyboard(b, tb, kb);
}

async function sendResize(b) {
  if (b.resizeBusy) { b.resizeAgain = true; return; }
  b.resizeBusy = true;
  const sz = sizeOf(b);
  await Promise.all(b.tabs.filter(tb => tb.img).map(tb => input(b, tb, { kind: 'resize', ...sz })));
  b.resizeBusy = false;
  if (b.resizeAgain) { b.resizeAgain = false; sendResize(b); }
}

function resizeStreams(b) {
  if (b.native) return;
  cancelAnimationFrame(b.resizeRaf);
  b.resizeRaf = requestAnimationFrame(() => sendResize(b));
}

function showTab(b, tb) {
  b.cur = tb;
  b.empty.hidden = !!(tb.url && tb.url !== 'about:blank');
  b.tabs.forEach(x => { if (x.img) x.img.style.display = x === tb ? '' : 'none'; });
  b.url.value = tb.url && tb.url !== 'about:blank' ? tb.url : '';
  drawTabs(b); placeNative(b, true);
  if (b.native && tb.opened) infuse.call('browser', 'focus', { id: tb.id }).catch(() => { });
}

function addTab(b, u) {
  const tb = { id: 'b' + (++seq) + '-' + Date.now(), url: u };
  b.tabs.push(tb);
  if (!b.native) streamTab(b, tb);
  showTab(b, tb);
  if (u) goTo(b, u); else setTimeout(() => b.url.focus(), 30);
  return tb;
}

function closeTab(b, tb) {
  const i = b.tabs.indexOf(tb);
  b.tabs.splice(i, 1);
  if (tb.opened) infuse.call('browser', 'close', { id: tb.id }).catch(() => { });
  if (tb.es) { tb.es.close(); input(b, tb, { kind: 'close' }); }
  tb.img?.remove(); tb.kb?.remove();
  if (!b.tabs.length) addTab(b, ''); else showTab(b, b.tabs[Math.min(i, b.tabs.length - 1)]);
}

async function goTo(b, v) {
  if (!b.cur) return;
  const typed = v.trim();
  const addr = addressOf(typed) || (typed ? searchURL(typed) : '');
  if (!addr) return;
  b.cur.url = addr; b.cur.loading = true; b.url.value = addr; b.empty.hidden = true; drawTabs(b);
  if (b.native) {
    try {
      await infuse.call('browser', b.cur.opened ? 'navigate' : 'open', { id: b.cur.id, url: addr, ...rectNow(b), visible: true });
      b.cur.opened = true; placeNative(b, true); return;
    } catch (e) { b.native = false; b.tabs.forEach(x => { if (!x.img) streamTab(b, x); }); showTab(b, b.cur); }
  }
  const r = await api('/api/view/open', { body: { id: b.cur.id, url: addr, ...sizeOf(b) }, root: b.c.root }).catch(e => ({ error: String(e) }));
  if (!r.ok) { b.cur.loading = false; drawTabs(b); toast(r.error || t('browser.didNotOpen')); }
  b.cur.kb?.focus();
}

function navigate(b, op) {
  if (!b.cur) return;
  if (b.native && b.cur.opened) return infuse.call('browser', op, { id: b.cur.id }).catch(() => { });
  if (b.cur.img) input(b, b.cur, { kind: op });
}

async function pickInPage(b) {
  if (!b.cur?.url) return toast(t('pick.openPageFirst'));
  if (b.native && b.cur.opened) {
    toast(t('pick.clickHint'));
    return infuse.call('browser', 'pick', { id: b.cur.id }).catch(e => toast(e.message));
  }
  b.picking = true; b.cur.img?.classList.add('picking');
  toast(t('pick.clickHint'));
}

function serverItems(b) {
  return async s => {
    if (!s.running && !s.listening) {
      let r = await api('/api/devservers', { body: { action: 'start', name: s.name }, root: b.c.root });
      if (r.needsApproval) {
        if (!confirm(t('devserver.approveConfirm') + '\n\n' + r.command)) return;
        r = await api('/api/devservers', { body: { action: 'approve', name: s.name }, root: b.c.root });
      }
      if (r.error) return toast(r.error);
      toast(t('devserver.starting') + s.name);
      setTimeout(() => goTo(b, s.url), 2500);
    } else goTo(b, s.url);
  };
}

async function openServers(b, anchor) {
  const list = await api('/api/devservers', { root: b.c.root }).catch(() => []);
  const start = serverItems(b);
  const items = list.length ? list.flatMap(s => [{
    label: s.name, d: `:${s.port} · ${s.running || s.listening ? t('devserver.running') : t('devserver.stopped')}${s.approved ? '' : ' · ' + t('devserver.notApproved')}`,
    f: () => start(s)
  }]) : [{ label: t('devserver.none'), sub: t('devserver.declareHint'), f: () => { } }];
  if (list.some(s => s.running)) items.push({ sep: 1 }, ...list.filter(s => s.running).map(s => ({ label: t('devserver.stopOne') + s.name, f: () => api('/api/devservers', { body: { action: 'stop', name: s.name }, root: b.c.root }) })), { label: t('devserver.stopAll'), danger: 1, f: () => api('/api/devservers', { body: { action: 'stopall' }, root: b.c.root }) });
  menu(anchor, items, null, { alignRight: true });
}

function wireBrowser(b) {
  b.url.onkeydown = e => {
    if (e.key === 'Enter') { goTo(b, b.url.value); b.url.blur(); }
    if (e.key === 'Escape') { b.url.value = b.cur?.url || ''; b.url.blur(); }
  };
  b.url.onfocus = () => b.url.select();
  b.pb.querySelector('.back').onclick = () => navigate(b, 'back');
  b.pb.querySelector('.fwd').onclick = () => navigate(b, 'forward');
  b.pb.querySelector('.rl').onclick = () => navigate(b, 'reload');
  b.pb.querySelector('.ext').onclick = () => b.cur?.url && api('/api/open', { body: { url: b.cur.url }, root: '' }).catch(() => { });
  b.extra.querySelector('.nt').onclick = () => addTab(b, '');
  b.pb.querySelector('.pick').onclick = () => pickInPage(b);
  b.extra.querySelector('.srv').onclick = e => openServers(b, e.currentTarget);
}

function destroyBrowser(b) {
  clearInterval(b.timer); b.ro.disconnect();
  b.tabs.forEach(tb => {
    if (tb.opened) infuse.call('browser', 'close', { id: tb.id }).catch(() => { });
    if (tb.es) { tb.es.close(); input(b, tb, { kind: 'close' }); }
  });
}

export function mount(pb, extra, c) {
  const b = makeState(pb, extra, c);
  if (b.native) infuse.on('browser', ev => { if (document.body.contains(pb)) onBrowserEvent(b, ev); });
  b.ro = new ResizeObserver(() => { placeNative(b); resizeStreams(b); });
  b.ro.observe(b.frames);
  b.timer = setInterval(() => placeNative(b), 200);
  wireBrowser(b);
  addTab(b, pref('browserUrl')[c.root] || '');
  return {
    go: v => { if (!b.cur) addTab(b, ''); goTo(b, v); },
    pick: () => pickInPage(b),
    destroy: () => destroyBrowser(b)
  };
}
