// composer effect — the reactive chrome: home subtitle, queue, context ring, vim outline, send/under.
import { $, h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { I } from '../../core/icons.js';
import { effect } from '../../core/signals.js';
import { liveOf, ui, cache, ctxLimit } from '../../core/store.js';
import { ktok } from '../../core/fmt.js';

export function mountEffect(ctx) {
  effect(() => {
    renderHome(ctx);
    renderQueue(ctx);
    renderRing(ctx);
    $('.composer-box', ctx.el)?.classList.toggle('vim', !!cache.settings.get().vim);
    ctx.renderSendBtn();
    ctx.renderUnder();
  });
}

export function renderHome(ctx) {
  const L = ctx.v.sid ? liveOf(ctx.v.sid) : null;
  const snap = L?.snap.get();
  const hs = $('.hsub', ctx.el);
  if (hs) hs.textContent = snap ? '' : (ctx.v.draft.root ? '' : t('home.pickFolder'));
}

function renderQueue(ctx) {
  const L = ctx.v.sid ? liveOf(ctx.v.sid) : null;
  const q = $('.queue', ctx.el);
  q.innerHTML = '';
  (L?.queue || []).forEach((m, i) => {
    const row = h(`<div class="qi"><span>⏎ ${esc(m.text)}</span><span class="faint">${t('composer.queued')}</span><button class="icon-btn icon-20" aria-label="${esc(t('action.remove'))}">${I.close}</button></div>`);
    row.querySelector('button').onclick = () => { L.queue.splice(i, 1); ui.views.set([...ui.views.get()]); };
    q.appendChild(row);
  });
}

function renderRing(ctx) {
  const L = ctx.v.sid ? liveOf(ctx.v.sid) : null;
  const ring = $('.ctx-ring', ctx.el);
  const pct = L ? Math.min(100, Math.round(((L.lastIn || 0) / ctxLimit(L)) * 100)) : 0;
  const c = 2 * Math.PI * 8;
  ring.innerHTML = L ? `<svg width="18" height="18" viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="8" stroke="var(--color-surface-3)"/><circle cx="10" cy="10" r="8" stroke="${pct > 80 ? 'var(--color-warn)' : 'var(--color-text-secondary)'}" stroke-dasharray="${c * pct / 100} ${c}" transform="rotate(-90 10 10)"/></svg>` : '';
  ring.title = L ? `${t('composer.context')}: ${pct}% · ${ktok(L.lastIn)} / ${ktok(ctxLimit(L))}` : '';
}
