// dialog — modal with focus trap (WAI-ARIA APG): focus moves in on open, stays in while open
// (Tab cycles), returns to the opener on close, Esc closes. Only one modal at a time.
import { h } from '../core/dom.js';

let active = null;
export const dialogOpen = () => !!active;

export function overlay(inner, onClose) {
  const o = h('<div class="overlay"></div>');
  o.appendChild(inner);
  document.body.appendChild(o);
  const opener = document.activeElement;
  const focusables = () => [...inner.querySelectorAll('button, input, select, textarea, a[href], [tabindex]:not([tabindex="-1"])')].filter(el => !el.disabled && el.offsetParent !== null);
  const close = () => {
    if (!active) return;
    active = null;
    o.remove();
    document.removeEventListener('keydown', key, true);
    onClose && onClose();
    opener?.focus?.();
  };
  const key = e => {
    if (!active) return;
    if (e.key === 'Escape') { e.stopPropagation(); e.preventDefault(); close(); return; }
    if (e.key === 'Tab') { // trap: cycle within the dialog
      const list = focusables();
      if (!list.length) { e.preventDefault(); return; }
      const first = list[0], last = list[list.length - 1];
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
    }
  };
  document.addEventListener('keydown', key, true);
  o.onmousedown = e => { if (e.target === o) close(); };
  active = { close, inner };
  inner.setAttribute('role', 'dialog');
  inner.setAttribute('aria-modal', 'true');
  requestAnimationFrame(() => (focusables()[0] || inner).focus());
  return close;
}
export const closeDialog = () => active?.close();
