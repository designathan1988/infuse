// tasks — the agent's todo list, live. Same data as the rail's plan card, focused here.
import { esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { effect } from '../core/signals.js';
import { busy } from '../core/store.js';
import { PLAN_ICON } from '../components/cards-shared.js';

export function mount(pb, extra, c) {
  effect(() => {
    const L = c.L;
    const tasks = L?.snap.get()?.tasks || [];
    const running = L?.running.get();
    pb.innerHTML = `<div class="pane-list">${tasks.length ? tasks.map(x => {
      const st = x.status === 'in_progress' && !(L && busy(L)) ? 'stopped' : x.status;
      const [ic, cls] = PLAN_ICON[st] || PLAN_ICON.pending;
      return `<div class="plan-task ${st}"><span class="result-badge rst ${cls}">${ic}</span><span class="grow"><span class="tt">${esc(x.text)}</span>${x.proof ? `<span class="pf">${t('plan.proof')}: <code>${esc(x.proof)}</code></span>` : ''}${x.note ? `<span class="pn">${esc(t(x.note))}</span>` : ''}</span></div>`;
    }).join('') : `<div class="empty">${t('tasks.empty')}</div>`}${running ? `<div class="status mt-5" role="status"><span class="spin"></span>${t('live.runningTool')} ${esc(running.name)}</div>` : ''}</div>`;
  });
  return {};
}
