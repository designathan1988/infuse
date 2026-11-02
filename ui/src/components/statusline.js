// statusline — the configurable status line under the composer. The server renders it from the
// statusLine setting; this module only asks, at most once in a while, and shows the text.
import { h, esc } from '../core/dom.js';
import { api } from '../core/api.js';

const MIN_INTERVAL = 3000; // the line may run a command: redraws of the composer must not multiply it
const last = new Map(); // session id → { at, text }

export function statusLineChip(sid, root) {
  const node = h('<span class="chip static status-line"><span class="v faint"></span></span>');
  const show = text => { node.firstChild.innerHTML = esc(text || ''); node.hidden = !text; };
  const known = last.get(sid);
  show(known?.text);
  if (known && Date.now() - known.at < MIN_INTERVAL) return node;
  last.set(sid, { at: Date.now(), text: known?.text });
  api(`/api/session/${encodeURIComponent(sid)}/statusline`, { root }).then(r => {
    last.set(sid, { at: Date.now(), text: r.text });
    show(r.text);
  }).catch(() => show(''));
  return node;
}
