// scheduled/chips — the folder, preset, time, mode, model, worktree and active controls.
import { h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { I } from '../../core/icons.js';
import { cache, projectLabel } from '../../core/store.js';
import { selectEl } from '../shell.js';
import { menu } from '../../components/menu.js';
import { MODE_INFO } from '../../components/composer.js';
import { presetText } from './preset.js';

export function drawChips(chips, tk) {
  chips.innerHTML = '';
  const c1 = h(`<span class="chip" role="button" tabindex="0">${I.folder}<span class="v">${esc(tk.root ? projectLabel(tk.root) : t('env.chooseFolder'))}</span>${I.caret}</span>`);
  c1.onclick = e => menu(e.currentTarget, cache.projects.get().map(p => ({ label: p.label || p.root.replace(/^.*[\\/]/, ''), d: p.root, check: p.root === tk.root, f: () => { tk.root = p.root; drawChips(chips, tk); } })));
  chips.appendChild(c1);
  const c2 = h(`<span class="chip" role="button" tabindex="0"><span class="v">${esc(presetText(tk))}</span>${I.caret}</span>`);
  c2.onclick = e => menu(e.currentTarget, [['manual', 'tasks.preset.manual'], ['hourly', 'tasks.preset.hourly'], ['daily', 'tasks.preset.daily'], ['weekdays', 'tasks.preset.weekdays'], ['weekly', 'tasks.preset.weekly']].map(([k, l]) => ({ label: t(l), check: tk.preset === k, f: () => { tk.preset = k; drawChips(chips, tk); } })));
  chips.appendChild(c2);
  if (tk.preset !== 'manual') {
    const tm = h(`<input type="time" value="${String(tk.hour).padStart(2, '0')}:${String(tk.minute).padStart(2, '0')}" aria-label="${esc(t('tasks.time'))}">`);
    tm.className = 'chip-input';
    tm.onchange = () => { const [hh, mm] = tm.value.split(':'); tk.hour = +hh; tk.minute = +mm; };
    chips.appendChild(tm);
  }
  if (tk.preset === 'weekly') {
    const wd = selectEl([0, 1, 2, 3, 4, 5, 6].map(i => [String(i), ['tasks.weekday.0', 'tasks.weekday.1', 'tasks.weekday.2', 'tasks.weekday.3', 'tasks.weekday.4', 'tasks.weekday.5', 'tasks.weekday.6'][i]]), String(tk.weekday), v => tk.weekday = +v);
    wd.className = 'chip-input';
    chips.appendChild(wd);
  }
  const c3 = h(`<span class="chip mode-${tk.mode || 'default'}" role="button" tabindex="0"><span class="v">${t(MODE_INFO[tk.mode || 'acceptEdits']?.[0] || '')}</span>${I.caret}</span>`);
  c3.onclick = e => menu(e.currentTarget, Object.entries(MODE_INFO).map(([k, [l]]) => ({ label: t(l), check: (tk.mode || 'acceptEdits') === k, f: () => { tk.mode = k; drawChips(chips, tk); } })));
  chips.appendChild(c3);
  const c4 = h(`<span class="chip" role="button" tabindex="0"><span class="v">${esc(tk.model || cache.settings.get().model || t('tasks.defaultModel'))}</span>${I.caret}</span>`);
  c4.onclick = e => menu(e.currentTarget, [
    { label: t('tasks.defaultModel'), check: !tk.model, f: () => { tk.model = ''; tk.provider = ''; drawChips(chips, tk); } },
    ...cache.models.get().slice(0, 40).map(m => ({ label: m.model, d: m.label, check: tk.model === m.model, f: () => { tk.model = m.model; tk.provider = m.provider; drawChips(chips, tk); } }))
  ]);
  chips.appendChild(c4);
  const c5 = h(`<span class="chip" role="button" tabindex="0"><span class="switch${tk.worktree ? ' on' : ''}"></span>${t('env.worktree')}</span>`);
  c5.onclick = () => { tk.worktree = !tk.worktree; drawChips(chips, tk); };
  chips.appendChild(c5);
  const c6 = h(`<span class="chip" role="button" tabindex="0"><span class="switch${tk.active ? ' on' : ''}"></span>${t(tk.active ? 'tasks.active' : 'tasks.paused')}</span>`);
  c6.onclick = () => { tk.active = !tk.active; drawChips(chips, tk); };
  chips.appendChild(c6);
}
