// menu — ARIA APG menu pattern: roving tabindex, ArrowUp/Down, Home/End, Enter/Space, Esc closes
// (submenus close level by level). Items: {label, sub, d, check, icon, items, danger, f, sep, h, html, foot, keep}.
import { h, esc, $$ } from '../core/dom.js';
import { t } from '../core/i18n.js';

let openStack = [];
export const menuOpen = () => openStack.length > 0;

function closeToLevel(level = 0) {
  while (openStack.length > level) {
    const m = openStack.pop();
    m._onClose?.(); m.remove();
  }
}
export const closeMenu = () => closeToLevel(0);

export function menu(anchor, items, ev, opts = {}) {
  const level = opts.level || 0;
  closeToLevel(level);
  const m = h('<div class="menu" role="menu" tabindex="-1"></div>');
  const head = h('<div class="mhead"></div>'), body = h('<div class="mbody" role="presentation"></div>'), foot = h('<div class="mfoot" role="presentation"></div>');
  m.append(head, body, foot);
  let n = 0;
  const rows = [];
  for (const it of items || []) {
    if (!it) continue;
    const dest = it.foot ? foot : body;
    if (it.sep) { dest.appendChild(h('<div class="menu-sep" role="separator"></div>')); continue; }
    if (it.h) { (it.head ? head : dest).appendChild(h(`<div class="menu-heading${it.plain ? ' plain' : ''}" role="presentation">${esc(t(it.h))}</div>`)); continue; }
    if (it.html) { dest.appendChild(it.html); continue; }
    n++;
    const row = h(`<div class="menu-item${it.danger ? ' danger' : ''}${it.sub ? ' col' : ''}${it.items ? ' hassub' : ''}" role="${it.check !== undefined ? 'menuitemcheckbox' : 'menuitem'}" aria-checked="${it.check ? 'true' : 'false'}" tabindex="-1">${it.check !== undefined ? `<span class="ck">${it.check ? '✓' : ''}</span>` : ''}${it.icon || ''}<span class="two"><span>${esc(t(it.label))}</span>${it.sub ? `<small>${esc(t(it.sub))}</small>` : ''}</span>${it.items ? '<span class="d">›</span>' : it.d ? `<span class="d">${esc(t(it.d))}</span>` : n <= 9 && opts.numbers ? `<span class="d">${n}</span>` : ''}</div>`);
    row.dataset.n = n;
    if (it.items) {
      const open = () => {
        if (row.classList.contains('k') && openStack[level + 1]) return;
        rows.forEach(x => x.el.classList.toggle('k', x.el === row));
        menu(row, it.items, null, { level: level + 1, side: true });
      };
      row.onmouseenter = () => { clearTimeout(m._t); m._t = setTimeout(open, 120); };
      row.onclick = e => { e.stopPropagation(); open(); };
    } else {
      row.onmouseenter = () => { clearTimeout(m._t); m._t = setTimeout(() => { rows.forEach(x => x.el.classList.remove('k')); closeToLevel(level + 1); }, 120); };
      row.onclick = e => { e.stopPropagation(); if (!it.keep) closeMenu(); it.f && it.f(); };
    }
    rows.push({ el: row, it });
    dest.appendChild(row);
  }
  document.body.appendChild(m);
  openStack.push(m);
  m._onClose = () => { m._opener?.focus(); };

  // place: at cursor, beside anchor, or under it — fitting the screen; submenus flip side if needed
  let x, y, above = !!opts.above;
  if (ev) { x = ev.clientX; y = ev.clientY; m.style.maxHeight = (innerHeight - 12) + 'px'; }
  else if (opts.side) {
    const r = anchor.getBoundingClientRect(); m.style.maxHeight = (innerHeight - 12) + 'px';
    x = r.right + 2 + m.offsetWidth > innerWidth ? r.left - m.offsetWidth - 2 : r.right + 2; y = r.top - 5;
  } else {
    const r = anchor.getBoundingClientRect();
    const spaceAbove = r.top - 12, spaceBelow = innerHeight - r.bottom - 12;
    if (above && spaceAbove < Math.min(m.offsetHeight, 260) && spaceBelow > spaceAbove) above = false;
    if (!above && spaceBelow < Math.min(m.offsetHeight, 260) && spaceAbove > spaceBelow) above = true;
    m.style.maxHeight = Math.max(160, above ? spaceAbove : spaceBelow) + 'px';
    x = opts.alignRight ? r.right - m.offsetWidth : r.left; y = above ? r.top - m.offsetHeight - 6 : r.bottom + 4;
  }
  x = Math.max(6, Math.min(x, innerWidth - m.offsetWidth - 6)); y = Math.max(6, Math.min(y, innerHeight - m.offsetHeight - 6));
  m.style.left = x + 'px'; m.style.top = y + 'px';

  // keyboard: roving focus over this level's items
  const items1 = rows.filter(r => !r.it.items);
  const focusRow = r => { rows.forEach(x => x.el.setAttribute('tabindex', '-1')); r.el.setAttribute('tabindex', '0'); r.el.focus(); };
  m.onkeydown = e => {
    const idx = items1.findIndex(x => x.el === document.activeElement || x.el.contains(document.activeElement));
    const k = e.key;
    if (k === 'ArrowDown' || k === 'ArrowUp') {
      e.preventDefault();
      if (!items1.length) return;
      const next = k === 'ArrowDown' ? (idx + 1) % items1.length : (idx - 1 + items1.length) % items1.length;
      focusRow(items1[next]);
    } else if (k === 'Home' || k === 'End') {
      e.preventDefault(); if (items1.length) focusRow(k === 'Home' ? items1[0] : items1[items1.length - 1]);
    } else if (k === 'Escape') {
      e.preventDefault(); e.stopPropagation();
      closeToLevel(level); (level === 0 ? m._opener : openStack[level - 1])?.focus?.();
    } else if ((k === 'Enter' || k === ' ') && document.activeElement?.classList?.contains('menu-item')) {
      e.preventDefault(); document.activeElement.click();
    } else if (/^[1-9]$/.test(k) && opts.numbers !== false) {
      const r = rows.find(x => x.el.dataset.n === k);
      if (r) { e.preventDefault(); r.el.click(); }
    }
  };
  if (level === 0) { m._opener = anchor instanceof HTMLElement ? anchor : null; }
  if (items1.length) focusRow(items1[0]); else m.focus();
  return m;
}

document.addEventListener('mousedown', e => {
  if (openStack.length && !openStack.some(m => m.contains(e.target))) closeMenu();
});
