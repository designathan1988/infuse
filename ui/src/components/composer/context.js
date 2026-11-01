// composer context — owns the textarea and exposes the small action surface the parts share.
import { $ } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { createAutocomplete } from './autocomplete.js';
import { createVim } from './vim.js';
import { doSend, stopCurrent } from './send.js';
import { renderAttachments } from './attachments.js';
import { renderSendButton, renderUnder } from './under.js';
import { PROMPT_PH } from './options.js';

export function createComposerContext(v, el) {
  const P = $('.prompt-input', el);
  v._promptEl = P;
  const ctx = {
    v, el, P, lastEsc: 0,
    grow: () => { P.style.height = 'auto'; P.style.height = Math.min(P.scrollHeight, 300) + 'px'; },
    setGhost: txt => {
      const ghost = txt || '';
      P.dataset.ghost = ghost;
      P.placeholder = ghost ? ghost + '   ⇥ Tab' : t(PROMPT_PH);
      P.classList.toggle('ghost', !!ghost);
    },
  };
  ctx.renderAtts = () => renderAttachments(ctx);
  v.renderAtts = ctx.renderAtts; // the window's drop handler (app.js) adds files outside this module
  ctx.renderSendBtn = () => renderSendButton(ctx);
  ctx.renderUnder = () => renderUnder(ctx);
  ctx.ac = createAutocomplete(ctx);
  ctx.vim = createVim(ctx);
  ctx.stopCur = () => stopCurrent(ctx);
  ctx.doSend = () => doSend(ctx);
  v._sendFn = () => ctx.doSend();
  return ctx;
}
