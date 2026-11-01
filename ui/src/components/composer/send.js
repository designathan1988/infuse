// composer send — slash commands are handled first; everything else goes to the store's send action.
import { liveOf, send } from '../../core/store.js';
import { handleSlash } from './slash.js';

export function doSend(ctx) {
  const text = ctx.P.value.trim();
  if (text.startsWith('/')) {
    handleSlash(ctx, text).then(handled => {
      if (!handled) { send(ctx.v); ctx.grow(); } else { ctx.P.value = ''; ctx.grow(); ctx.ac.hide(); }
    });
    return;
  }
  send(ctx.v); ctx.grow(); ctx.ac.hide();
}

export function stopCurrent(ctx) {
  const L = ctx.v.sid ? liveOf(ctx.v.sid) : null;
  if (L) import('../../core/store.js').then(st => st.stop(L));
}
