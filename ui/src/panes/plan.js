// plan — the plan proposed in Plan mode, with approve-and-run.
import { $, esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { effect } from '../core/signals.js';
import { md } from '../core/markdown.js';
import { liveOf, focusedView, setSessionOpts } from '../core/store.js';

export function mount(pb, extra, c) {
  let inEffect = false;
  const draw = () => {
    const L = c.L;
    const p = L?.snap.get()?.plan;
    pb.innerHTML = p
      ? `<div class="pane-list markdown sel">${md(p)}</div><div class="pane-foot"><span class="grow faint">${t('plan.proposedInPlanMode')}</span><button class="btn sm pri">${t('plan.approveRun')}</button></div>`
      : `<div class="empty">${t('plan.emptyState')}</div>`;
    const b = pb.querySelector('.pane-foot button');
    if (b) b.onclick = async () => {
      const v = focusedView();
      const L2 = v?.sid ? liveOf(v.sid) : null;
      if (L2) await setSessionOpts(L2, { mode: 'acceptEdits' });
      if (v?._promptEl) { v._promptEl.value = t('plan.approved'); v._sendFn?.(); }
    };
  };
  effect(draw);
  return { refresh: draw };
}
