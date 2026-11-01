// panes — the right-side pane stack. A pane is a registry entry (id, title, mount); mounting is lazy,
// instances expose { refresh, destroy, ...methods } called via paneCall. Layout (open order, width,
// heights) persists in the versioned prefs. Drag between panes reorders; grips resize.
import { $, $$, h, esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { pref, ui, curRoot, curDir, liveOf } from '../core/store.js';
import { effect } from '../core/signals.js';

const PANES = {
  changes: { title: 'pane.changes', load: () => import('./changes.js') },
  terminal: { title: 'pane.terminal', load: () => import('./terminal.js'), keep: true },
  files: { title: 'pane.files', load: () => import('./files.js') },
  browser: { title: 'pane.browser', load: () => import('./browser.js'), keep: true },
  tasks: { title: 'pane.tasks', load: () => import('./tasks.js') },
  procs: { title: 'pane.procs', load: () => import('./procs.js') },
  agents: { title: 'pane.agents', load: () => import('./agents.js') },
  plan: { title: 'pane.plan', load: () => import('./plan.js') },
  oracle: { title: 'pane.oracle', load: () => import('./oracle.js') },
  memory: { title: 'pane.memory', load: () => import('./memory.js') },
  rules: { title: 'pane.rules', load: () => import('./rules.js') },
  journal: { title: 'pane.journal', load: () => import('./journal.js') },
  vices: { title: 'pane.vices', load: () => import('./vices.js') },
  map: { title: 'pane.map', load: () => import('./map.js') },
};

const state = { inst: {}, focus: null };
let bound = '';
const sessionKey = () => { const v = ui.views.get()[ui.focus.get()]; return (v?.sid || '') + '|' + curRoot(); };

export function bindPanes() {
  bound = sessionKey();
  effect(() => {
    const key = sessionKey();
    if (key === bound) return;
    bound = key;
    for (const id of Object.keys(state.inst)) state.inst[id]?.destroy?.();
    state.inst = {};
    document.querySelectorAll('#panes .pane').forEach(el => el.remove());
    render();
  });
}

export function togglePane(id, forceOpen) {
  const p = pref('panes');
  const open = p.open || [];
  const isOpen = open.includes(id);
  const willOpen = forceOpen !== undefined ? forceOpen : !isOpen;
  let next = open;
  if (willOpen && !isOpen) next = [...open, id];
  if (!willOpen && isOpen) next = open.filter(x => x !== id);
  pref('panes', { ...p, open: next });
  state.focus = willOpen ? id : (next[next.length - 1] || null);
  render();
  markButtons();
}
export function closePane(id) {
  const p = pref('panes');
  state.inst[id]?.destroy?.();
  delete state.inst[id];
  pref('panes', { ...p, open: (p.open || []).filter(x => x !== id) });
  if (state.focus === id) state.focus = null;
  render(); markButtons();
}
export const closeFocusedPane = () => { if (state.focus) closePane(state.focus); };
export function paneRefresh(id) { state.inst[id]?.refresh?.(); }
export function paneCall(id, fn, ...a) { state.inst[id]?.[fn]?.(...a); }
export async function openFileInPane(path) {
  togglePane('files', true);
  setTimeout(() => paneCall('files', 'open', path), 50);
}
function markButtons() {
  $$('.view-bar [data-a]').forEach(b => {
    if (PANES[b.dataset.a]) b.classList.toggle('on', (pref('panes').open || []).includes(b.dataset.a));
  });
}

const CHAT_MIN = 380, PANE_MIN = 300;
export function render() {
  const host = $('#panes');
  const p = pref('panes');
  const open = (p.open || []).filter(id => PANES[id]);
  const existing = new Map([...host.querySelectorAll('.pane')].map(el => [el.dataset.id, el]));
  for (const id of Object.keys(state.inst)) {
    if (open.includes(id)) continue;
    state.inst[id]?.destroy?.();
    delete state.inst[id];
  }
  host.classList.toggle('empty', !open.length);
  host.innerHTML = '<div class="pane-grip" role="separator" aria-orientation="vertical"></div>';
  const sizes = p.sizes || {};
  open.forEach((id, idx) => {
    const pane = existing.get(id) || newPane(id);
    pane.classList.toggle('focused', state.focus === id);
    pane.style.flex = sizes[id] ? `0 0 ${sizes[id]}px` : '';
    host.appendChild(pane);
    if (!existing.has(id)) {
      bindPane(id, pane);
      mount(id, pane);
    }
    if (idx < open.length - 1) {
      const split = h('<div class="pane-split" role="separator" aria-orientation="horizontal"></div>');
      host.appendChild(split);
      split.onmousedown = e => dragSplit(e, open[idx], open[idx + 1], 'v', split);
    }
  });
  host.querySelector('.pane-grip').onmousedown = e => dragWidth(e);
  fitPaneWidth();
}

function newPane(id) {
  return h(`<section class="pane" data-id="${id}" aria-label="${esc(t(PANES[id].title))}">
      <header class="pane-head"><span class="ttl">${esc(t(PANES[id].title))}</span><span class="extra row row-tight"></span><span class="sp"></span>
        <button class="icon-btn x" aria-label="${esc(t('common.close'))} (Ctrl\)">${I.close}</button></header>
      <div class="pane-body"></div>
    </section>`);
}

function bindPane(id, pane) {
  pane.querySelector('.x').onclick = () => closePane(id);
  pane.onmousedown = () => {
    if (state.focus === id) return;
    state.focus = id;
    $$('.pane', $('#panes')).forEach(x => x.classList.toggle('focused', x.dataset.id === id));
  };
}

async function mount(id, pane) {
  const mod = await PANES[id].load();
  if (!pane.isConnected || !(pref('panes').open || []).includes(id)) return;
  if (state.inst[id]?._pane === pane) return;
  state.inst[id]?.destroy?.();
  const pb = pane.querySelector('.pane-body'), extra = pane.querySelector('.extra');
  const c = { root: curRoot(), dir: curDir(), L: liveOf(ui.views.get()[ui.focus.get()]?.sid) };
  state.inst[id] = mod.mount(pb, extra, c) || {};
  state.inst[id]._pane = pane;
  markButtons();
}

// horizontal width drag (chat | panes)
function dragWidth(e) {
  e.preventDefault();
  const startX = e.clientX, startW = $('#panes').offsetWidth;
  const shield = h('<div class="drag-shield col"></div>');
  document.body.appendChild(shield);
  document.body.classList.add('dragging');
  const move = ev => {
    const w = Math.max(PANE_MIN, Math.min(startW + (startX - ev.clientX), paneRoom()));
    $('#panes').style.width = w + 'px';
  };
  const up = () => {
    shield.remove(); document.body.classList.remove('dragging');
    removeEventListener('mousemove', move); removeEventListener('mouseup', up);
    pref('panes', { ...pref('panes'), width: $('#panes').offsetWidth });
  };
  addEventListener('mousemove', move); addEventListener('mouseup', up);
}
function paneRoom() {
  const main = $('#main');
  return Math.max(PANE_MIN, (main ? main.clientWidth : innerWidth) - CHAT_MIN);
}
export function fitPaneWidth() {
  const p = pref('panes');
  const host = $('#panes');
  if (host) host.style.width = Math.min(p.width || 520, paneRoom()) + 'px';
}

// vertical split drag between two stacked panes
function dragSplit(e, topId, bottomId, dir, el) {
  e.preventDefault();
  const top = state.inst[topId]._pane, bottom = state.inst[bottomId]._pane;
  const startY = e.clientY, startH = top.offsetHeight;
  const shield = h('<div class="drag-shield row"></div>');
  document.body.appendChild(shield);
  const move = ev => {
    const hpx = Math.max(90, startH + (ev.clientY - startY));
    top.style.flex = `0 0 ${hpx}px`;
  };
  const up = () => {
    shield.remove();
    removeEventListener('mousemove', move); removeEventListener('mouseup', up);
    pref('panes', { ...pref('panes'), sizes: { ...pref('panes').sizes, [topId]: top.offsetHeight } });
  };
  addEventListener('mousemove', move); addEventListener('mouseup', up);
}

// pane head drag & drop reordering
document.addEventListener('dragstart', e => {
  const head = e.target.closest?.('.pane-head');
  if (!head) return;
  e.dataTransfer.setData('text/infuse-pane', head.closest('.pane').dataset.id);
});
document.addEventListener('dragover', e => {
  const pane = e.target.closest?.('.pane');
  if (pane && e.dataTransfer.types.includes('text/infuse-pane')) { e.preventDefault(); pane.classList.add('drag-over'); }
});
document.addEventListener('dragleave', e => e.target.closest?.('.pane')?.classList.remove('drag-over'));
document.addEventListener('drop', e => {
  const pane = e.target.closest?.('.pane');
  if (!pane) return;
  pane.classList.remove('drag-over');
  const from = e.dataTransfer.getData('text/infuse-pane');
  const to = pane.dataset.id;
  if (!from || from === to) return;
  e.preventDefault();
  const open = (pref('panes').open || []).slice();
  const i = open.indexOf(from), j = open.indexOf(to);
  if (i < 0 || j < 0) return;
  open.splice(i, 1); open.splice(j, 0, from);
  pref('panes', { ...pref('panes'), open });
  render();
});
