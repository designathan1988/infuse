// composer wiring — one place for the browser events the composer owns.
import { $ } from '../../core/dom.js';
import { api } from '../../core/api.js';
import { cache, liveOf, busy } from '../../core/store.js';
import { composerKey } from './keys.js';
import { plusMenu } from './menus.js';

export function wireComposer(ctx) {
  const { v, el, P } = ctx;
  $('.send-btn', el).onclick = () => {
    const L = v.sid ? liveOf(v.sid) : null;
    if (L && busy(L) && !P.value.trim()) ctx.stopCur(); else ctx.doSend();
  };
  $('.plusbtn', el).onclick = e => plusMenu(ctx, e.currentTarget);
  $('.ctx-ring', el).onclick = () => import('../dialogs.js').then(d => d.contextDialog());
  P.oninput = () => { ctx.grow(); ctx.ac.autocomplete(); };
  P.onkeydown = e => composerKey(ctx, e);
  P.onpaste = e => pasteImages(ctx, e);
  wireGhost(ctx);
}

function pasteImages(ctx, e) {
  for (const it of e.clipboardData.items) if (it.type.startsWith('image/')) {
    const f = it.getAsFile();
    const r = new FileReader();
    r.onload = () => { ctx.v.images.push({ mediaType: f.type, data: r.result.split(',')[1] }); ctx.renderAtts(); };
    r.readAsDataURL(f);
    e.preventDefault();
  }
}

function wireGhost(ctx) {
  const P = ctx.P;
  import('../../core/store.js').then(st => st.bus.on('suggest-next', L => {
    if (cache.settings.get().promptSuggestions === false || !L) return;
    api(`/api/session/${L.id}/suggest`, { body: {}, root: L.root }).then(r => {
      if (r.text && !P.value && !ctx.v.atts.length && !ctx.v.images.length && !busy(liveOf(L.id))) ctx.setGhost(r.text);
    }).catch(() => { });
  }));
}
