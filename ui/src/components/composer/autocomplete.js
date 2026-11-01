// composer autocomplete — @files and /commands in the prompt, navigated by keyboard or click.
import { $, $$ } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { menu, closeMenu } from '../menu.js';
import { commandsOf, filesOf } from '../../core/store.js';
import { vopts } from './options.js';

export function createAutocomplete(ctx) {
  const st = { ctx, v: ctx.v, el: ctx.el, P: ctx.P, grow: ctx.grow, AC: null };
  return {
    autocomplete: () => autocomplete(st),
    hide: () => hide(st),
    key: e => key(st, e),
    pick: k => pick(st, k),
    isOpen: () => !!st.AC,
  };
}

async function autocomplete(st) {
  const { v, P } = st;
  const before = P.value.slice(0, P.selectionStart);
  const m = before.match(/(^|\s)([@/])([^\s]*)$/);
  if (!m || (m[2] === '/' && before.trim() !== m[2] + m[3])) return hide(st);
  const root = vopts(v).root, q = m[3].toLowerCase();
  let items;
  if (m[2] === '@') {
    if (!root) return hide(st);
    const files = await filesOf(root);
    items = (files || []).filter(f => f.toLowerCase().includes(q))
      .sort((a, b) => (a.toLowerCase().indexOf(q) - b.toLowerCase().indexOf(q)) || a.length - b.length).slice(0, 50)
      .map(f => ({ v: f, d: f.endsWith('/') ? t('common.folder') : '' }));
  } else {
    items = (await commandsOf(root)).filter(c => c.name.startsWith(q)).map(c => ({ v: c.name, d: c.builtin ? t(c.description) : c.description }));
  }
  if (!items.length) return hide(st);
  st.AC = { kind: m[2], items, i: 0 };
  renderAC(st);
}

function renderAC(st) {
  const { AC, el } = st;
  closeMenu();
  const m = menu($('.composer-box', el), AC.items.map((it, k) => ({ label: AC.kind + it.v, d: it.d, f: () => pick(st, k) })), null, { above: true, numbers: false });
  m.classList.add('ac-menu');
  const mi = $$('.menu-item', m);
  mi.forEach((x, k) => x.classList.toggle('k', k === AC.i));
  mi[AC.i]?.scrollIntoView({ block: 'nearest' });
  m.querySelector('.menu-item[tabindex="0"]')?.setAttribute('tabindex', '-1');
  mi[AC.i]?.setAttribute('tabindex', '0');
}

function hide(st) { if (st.AC) { st.AC = null; closeMenu(); } }

function key(st, e) {
  const { AC, P } = st;
  if (!AC) return false;
  if (e.key === 'ArrowDown') AC.i = (AC.i + 1) % AC.items.length;
  else if (e.key === 'ArrowUp') AC.i = (AC.i - 1 + AC.items.length) % AC.items.length;
  else if (e.key === 'Tab' || e.key === 'Enter') { e.preventDefault(); pick(st, AC.i); return true; }
  else if (e.key === 'Escape') { hide(st); e.preventDefault(); return true; }
  else return false;
  e.preventDefault(); renderAC(st); P.focus(); return true;
}

function pick(st, k) {
  const { AC, P, v, grow } = st;
  const it = AC.items[k], pos = P.selectionStart, kind = AC.kind;
  const before = P.value.slice(0, pos).replace(/([@/])[^\s]*$/, AC.kind + it.v + ' ');
  P.value = before + P.value.slice(pos);
  P.selectionStart = P.selectionEnd = before.length;
  hide(st); P.focus(); grow();
  if (kind === '@' && !it.v.endsWith('/')) {
    const root = vopts(v).root;
    const abs = root.replace(/[\\/]+$/, '') + '/' + it.v;
    if (!v.atts.find(a => a.path === abs)) { v.atts.push({ path: abs, name: it.v, size: 0 }); st.ctx.renderAtts(); }
  }
}
