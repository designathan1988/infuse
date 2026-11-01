// composer options — the mode/effort tables and the effective options of the view's session or draft.
import { cache, liveOf } from '../../core/store.js';

export const MODE_INFO = {
  default: ['mode.ask', 'mode.askDesc'],
  acceptEdits: ['mode.acceptEdits', 'mode.acceptEditsDesc'],
  plan: ['mode.plan', 'mode.planDesc'],
  bypass: ['mode.bypass', 'mode.bypassDesc'],
};
export const EFFORT_INFO = { low: 'effort.low', medium: 'effort.medium', high: 'effort.high', max: 'effort.max' };
export const PROMPT_PH = 'composer.placeholder';

export const vopts = v => {
  const L = v.sid ? liveOf(v.sid) : null;
  if (L?.snap.get()) {
    const s = L.snap.get();
    return { root: L.root, mode: s.mode, provider: s.provider, model: s.model, effort: s.effort, worktree: s.worktree, branch: s.branch, session: true };
  }
  const st = cache.settings.get();
  const d = v.draft;
  return { root: d.root, mode: d.mode || st.mode || 'default', provider: d.provider || st.provider, model: d.model || st.model, effort: d.effort || st.effort, worktree: d.worktree, session: false };
};
