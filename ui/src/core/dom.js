// dom — tiny helpers. esc() is the only escape hatch; every dynamic interpolation must go through it.
export const $ = (s, el = document) => el.querySelector(s);
export const $$ = (s, el = document) => [...el.querySelectorAll(s)];
export const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
export const h = html => { const t = document.createElement('template'); t.innerHTML = html.trim(); return t.content.firstElementChild; };
export const debounce = (fn, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; };
export const baseName = p => (p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop();

// Keyed list reconciliation: rerenders only items whose key+html changed, preserves element identity
// (and with it: scroll, focus and expanded <details>), removes gone items, inserts new in order.
export function patchKeyed(container, items) {
  const have = new Map([...container.children].map(c => [c.dataset.k, c]));
  let prev = null;
  for (const [key, html, bind] of items) {
    let node = have.get(key);
    if (!node || node._html !== html) {
      const n = document.createElement('div');
      n.dataset.k = key; n.className = 'list-key'; n.innerHTML = html; n._html = html;
      if (node) node.replaceWith(n); else (prev ? prev.after(n) : container.prepend(n));
      bind && bind(n);
      node = n;
    }
    have.delete(key); prev = node;
  }
  have.forEach(n => n.remove());
}
