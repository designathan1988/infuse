// turnSummary — the line that closes a turn: how long it worked, tools, tokens, what it cost and, on
// the last turn, the outcome when it did not plainly finish.
import { t } from '../core/i18n.js';
import { fmtDur, fmtTok } from '../core/fmt.js';
import { I } from '../core/icons.js';
import { isUnverified, turnCostOf, turnNumberAt } from '../core/spend.js';
import { costHTML, outcomeBadge } from './cost.js';

function tally(msgs, from, to) {
  let tools = 0, tokens = 0, texts = 0;
  for (let k = from; k <= to; k++) {
    const m = msgs[k]; if (m.role !== 'assistant') continue;
    tokens += m.tokens || 0;
    for (const b of m.content || []) { if (b.type === 'tool_use') tools++; if (b.type === 'text' && b.text?.trim()) texts++; }
  }
  return { tools, tokens, texts };
}

export function turnSummary(s, msgs, from, to) {
  if (from == null) return '';
  const first = msgs[from], last = msgs[to];
  const { tools, tokens, texts } = tally(msgs, from, to);
  const unverified = to === msgs.length - 1 && isUnverified(s);
  if (!texts && !tools && !unverified) return '';
  const dur = first?.ts && last?.ts ? last.ts - first.ts : null;
  const cost = costHTML(s, turnCostOf(s, turnNumberAt(s.turnStarts, from)));
  const meta = [dur != null && `${t('turn.workedFor')} ${fmtDur(dur)}`, tools && t('turn.toolsCount', { count: tools }), tokens && `${fmtTok(tokens)} tokens`, cost].filter(Boolean).join(' · ');
  return `<div class="turn-summary">${unverified ? outcomeBadge() : '<span class="ok">✓</span>'}<span class="meta"><span>${meta}</span></span><span class="grow"></span>${texts ? `<button class="btn sm ghost" data-copy-turn="${from}:${to}">${I.copy} ${t('action.copy')}</button>` : ''}</div>`;
}
