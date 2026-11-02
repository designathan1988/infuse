// composer under-row — folder/worktree chips for a draft and mode/model chips for either state.
import { $, h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { I } from '../../core/icons.js';
import { liveOf, busy, projectLabel } from '../../core/store.js';
import { MODE_INFO, EFFORT_INFO, vopts } from './options.js';
import { renderAttachments } from './attachments.js';
import { statusLineChip } from '../statusline.js';
import { folderMenu, modeMenu, modelMenu } from './menus.js';

export function renderUnder(ctx) {
  const o = vopts(ctx.v);
  const under = $('.composer-under', ctx.el);
  under.innerHTML = '';
  const add = node => under.appendChild(node);
  if (!o.session) {
    add(h(`<span class="chip static" title="${esc(t('env.thisMachine'))}">${I.pc}<span class="v">${t('env.local')}</span></span>`));
    const fc = h(`<span class="chip" role="button" tabindex="0" title="${esc(o.root || '')}">${I.folder}<span class="v">${esc(o.root ? projectLabel(o.root) : t('env.chooseFolder'))}</span>${I.caret}</span>`);
    fc.onclick = e => folderMenu(ctx, e.currentTarget); add(fc);
    const wt = h(`<span class="chip" role="button" tabindex="0" title="${esc(t('env.worktreeHint'))}"><span class="switch${o.worktree ? ' on' : ''}"></span><span>${t('env.worktree')}</span></span>`);
    wt.onclick = () => { ctx.v.draft.worktree = !ctx.v.draft.worktree; renderUnder(ctx); }; add(wt);
  } else {
    const L = liveOf(ctx.v.sid); const s = L.snap.get();
    add(h(`<span class="chip static" title="${esc(s.workRoot || o.root)}">${I.folder}<span class="v">${esc(projectLabel(o.root))}</span></span>`));
    add(statusLineChip(ctx.v.sid, o.root));
  }
  add(h('<span class="flex-1"></span>'));
  const mo = h(`<span class="chip mode-${o.mode}" role="button" tabindex="0" title="${esc(t('mode.aria'))}"><span class="v">${t(MODE_INFO[o.mode]?.[0] || o.mode)}</span>${I.caret}</span>`);
  mo.onclick = e => modeMenu(ctx, e.currentTarget); add(mo);
  const md = h(`<span class="chip" role="button" tabindex="0" title="${esc(t('model.aria'))}"><span class="v">${esc(o.model || t('model.choose'))}</span><span class="faint">${t(EFFORT_INFO[o.effort] || '')}</span>${I.caret}</span>`);
  md.onclick = e => modelMenu(ctx, e.currentTarget); add(md);
  renderSendButton(ctx);
  renderAttachments(ctx);
}

export function renderSendButton(ctx) {
  const L = ctx.v.sid ? liveOf(ctx.v.sid) : null;
  const isBusy = busy(L);
  const b = $('.send-btn', ctx.el);
  const stopMode = isBusy && !ctx.P.value.trim();
  b.classList.toggle('stop', !!stopMode);
  b.innerHTML = stopMode ? I.stop : I.up;
  b.title = t(stopMode ? 'composer.stop' : isBusy ? 'composer.queue' : 'composer.send');
}
