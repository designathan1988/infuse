// composer send — slash commands are handled first; everything else goes to the store's send action.
import { liveOf, send, curRoot } from '../../core/store.js';
import { api } from '../../core/api.js';
import { toast } from '../toast.js';
import { handleSlash } from './slash.js';

// "# note" saves to the project's memory; a markdown heading (##) or "#tag" stays a message
const memoryNote = text => (/^#\s+\S/.test(text) ? text.replace(/^#\s+/, '') : '');

export function doSend(ctx) {
  const text = ctx.P.value.trim();
  const note = memoryNote(text);
  if (note) {
    const root = curRoot();
    api('/api/memory/note', { body: { text: note }, root }).then(r => {
      toast(r.ok ? 'memory.savedNote' : 'memory.saveFailed');
    }).catch(() => toast('memory.saveFailed'));
    ctx.P.value = ''; ctx.grow();
    return;
  }
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
