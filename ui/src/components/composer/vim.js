// composer vim mode — a minimal normal/insert state machine over the textarea.
import { cache } from '../../core/store.js';
import { t } from '../../core/i18n.js';
import { PROMPT_PH } from './options.js';

export function createVim(ctx) {
  return e => vimKey(ctx, e);
}

function vimKey(ctx, e) {
  const { v, P, grow } = ctx;
  if (!cache.settings.get().vim) return false;
  v.vim = v.vim || 'INSERT';
  if (v.vim === 'INSERT') {
    if (e.key === 'Escape' && !ctx.ac.isOpen() && P.value) { v.vim = 'NORMAL'; P.placeholder = '-- NORMAL --'; e.preventDefault(); return true; }
    return false;
  }
  if (e.ctrlKey || e.metaKey) return false;
  e.preventDefault();
  const val = P.value, p = P.selectionStart;
  const set = n => { P.selectionStart = P.selectionEnd = Math.max(0, Math.min(val.length, n)); };
  const ls = val.lastIndexOf('\n', p - 1) + 1, le = val.indexOf('\n', p) === -1 ? val.length : val.indexOf('\n', p);
  switch (e.key) {
    case 'h': set(p - 1); break; case 'l': set(p + 1); break; case '0': set(ls); break; case '$': set(le); break;
    case 'w': { const m = val.slice(p).match(/^\S*\s+/); set(p + (m ? m[0].length : val.length)); break; }
    case 'b': { const m = val.slice(0, p).match(/\S+\s*$/); set(p - (m ? m[0].length : p)); break; }
    case 'j': case 'k': {
      const lines = val.split('\n'); let acc = 0, li = 0, col = p;
      for (; li < lines.length; li++) { if (col <= lines[li].length) break; col -= lines[li].length + 1; }
      const tl = li + (e.key === 'j' ? 1 : -1);
      if (tl >= 0 && tl < lines.length) { for (let i = 0; i < tl; i++) acc += lines[i].length + 1; set(acc + Math.min(col, lines[tl].length)); }
      break;
    }
    case 'i': v.vim = 'INSERT'; break;
    case 'a': set(p + 1); v.vim = 'INSERT'; break;
    case 'A': set(le); v.vim = 'INSERT'; break;
    case 'I': set(ls); v.vim = 'INSERT'; break;
    case 'o': P.value = val.slice(0, le) + '\n' + val.slice(le); set(le + 1); v.vim = 'INSERT'; break;
    case 'x': P.value = val.slice(0, p) + val.slice(p + 1); set(p); break;
    case 'd': if (v.vimD) { P.value = val.slice(0, ls) + val.slice(Math.min(val.length, le + 1)); set(ls); v.vimD = false; } else { v.vimD = true; return true; } break;
    case 'Enter': ctx.doSend(); break;
  }
  v.vimD = false;
  P.placeholder = v.vim === 'NORMAL' ? '-- NORMAL --' : t(PROMPT_PH);
  grow();
  return true;
}
