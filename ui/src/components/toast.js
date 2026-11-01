// toast — transient notices, announced to assistive tech via aria-live.
import { h } from '../core/dom.js';
import { t } from '../core/i18n.js';

let el = null, timer = null;
export function toast(msg) {
  if (!el) { el = h('<div class="toast" role="status" aria-live="polite"></div>'); document.body.appendChild(el); }
  el.textContent = t(msg);
  el.classList.add('show');
  clearTimeout(timer);
  timer = setTimeout(() => el.classList.remove('show'), 2600);
}
