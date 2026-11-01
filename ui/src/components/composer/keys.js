// composer keys — normal editing, ghost acceptance, mode cycling, history and vim handoff.
import { t } from '../../core/i18n.js';
import { pref, liveOf, busy, setSessionOpts } from '../../core/store.js';
import { toast } from '../toast.js';
import { MODE_INFO, vopts } from './options.js';

export async function composerKey(ctx, e) {
  const { v, P, grow } = ctx;
  if (ctx.vim(e)) return;
  if (ctx.ac.key(e)) return;
  if ((e.key === 'Tab' || e.key === 'ArrowRight') && !e.shiftKey && !P.value && P.dataset.ghost) {
    e.preventDefault(); P.value = P.dataset.ghost; ctx.setGhost(''); grow(); return;
  }
  if (e.key === 'Enter' && !e.shiftKey && !e.altKey) { e.preventDefault(); ctx.doSend(); return; }
  if (e.key === 'Tab' && e.shiftKey) {
    e.preventDefault();
    const order = Object.keys(MODE_INFO), cur = vopts(v).mode;
    const next = order[(order.indexOf(cur) + 1) % order.length];
    if (v.sid) setSessionOpts(liveOf(v.sid), { mode: next }); else { v.draft.mode = next; ctx.renderUnder(); }
    toast(t('mode.changedTo') + t(MODE_INFO[next][0]));
    return;
  }
  if (e.key === 'Escape') {
    const L = v.sid ? liveOf(v.sid) : null;
    if (busy(L)) { ctx.stopCur(); return; }
    const now = Date.now();
    if (now - ctx.lastEsc < 500) { ctx.lastEsc = 0; if (P.value.trim()) { P.value = ''; grow(); } else if (L) import('../dialogs.js').then(d => d.rewindDialog(v)); }
    else ctx.lastEsc = now;
    return;
  }
  if (e.key === 'ArrowUp' && (P.selectionStart === 0 || !P.value)) {
    const hist = pref('hist');
    if (!hist.length) return;
    v.hi = Math.min((v.hi ?? -1) + 1, hist.length - 1);
    P.value = hist[hist.length - 1 - v.hi]; grow(); e.preventDefault(); return;
  }
  if (e.key === 'ArrowDown' && (v.hi ?? -1) >= 0 && P.selectionStart === P.value.length) {
    v.hi--;
    P.value = v.hi < 0 ? '' : pref('hist')[pref('hist').length - 1 - v.hi];
    grow(); e.preventDefault(); return;
  }
  setTimeout(ctx.renderSendBtn, 0);
}
