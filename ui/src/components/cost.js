// cost — a money value of a session. With an unpriced model the value is only a lower bound, and it
// says so where it is shown. The other badge here is the one a turn that ended unverified keeps.
import { esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { moneyFine } from '../core/fmt.js';
import { isLowerBound } from '../core/spend.js';

export function costHTML(snap, cost, always = false) {
  const lower = isLowerBound(snap);
  if (!cost && !lower && !always) return '';
  if (!lower) return `<span class="cost">${moneyFine(cost)}</span>`;
  const hint = esc(t('cost.unpricedHint', { models: snap.unpriced.join(', ') }));
  return `<span class="cost lower" title="${hint}">≥ ${moneyFine(cost)}</span> <span class="tag warn" title="${hint}">${t('cost.unpriced')}</span>`;
}

export const outcomeBadge = () => `<span class="tag warn outcome-badge" role="status" title="${esc(t('outcome.unverifiedHint'))}">⚠ ${t('outcome.unverified')}</span>`;
