// agents — session subagents, live: who is doing what, each step, the conclusion.
import { $, $$, esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { effect } from '../core/signals.js';
import { md } from '../core/markdown.js';
import { agentTrailHTML } from '../components/toolblock.js';

export function mount(pb, extra, c) {
  const open = new Set();
  const draw = () => {
    const L = c.L;
    const as = (L?.snap.get()?.agents || []).slice().reverse();
    pb.innerHTML = as.length ? `<div class="pane-list">${as.map(a => `<div class="agent-card" data-id="${esc(a.id)}">
      <div class="row ah" role="button" tabindex="0" aria-expanded="${open.has(a.id)}">${a.state === 'running' ? '<span class="spin sm"></span>' : `<span class="dot ${a.state === 'done' ? 'ok' : 'bad'}"></span>`}<span class="tag">${esc(t('agent.kind.' + (a.kind || 'general')))}</span><span class="grow tt">${esc(a.title)}</span>
      <span class="faint">${a.steps} ${t('common.steps')} · ${a.state === 'running' ? elapsedStr(a) : durStr(a)}</span></div>
      ${a.state === 'running' && a.tool ? `<div class="faint now mono">${esc(a.tool)} ${esc(a.summary || '')}</div>` : ''}
      ${open.has(a.id) ? `<div class="abody"><details class="agent-task"><summary>${t('agent.taskGiven')}</summary><div class="markdown sel">${md(a.task || '')}</div></details><div class="atrail">${agentTrailHTML(a, 'pane:' + a.id)}</div></div>` : ''}</div>`).join('')}</div>`
      : `<div class="empty">${t('agents.empty')}</div>`;
    $$('.agent-card .ah', pb).forEach(hd => {
      hd.onclick = () => { const id = hd.parentElement.dataset.id; open.has(id) ? open.delete(id) : open.add(id); draw(); };
      hd.onkeydown = e => { if (e.key === 'Enter') hd.click(); };
    });
  };
  const elapsedStr = a => { const s = Math.max(0, Math.round((Date.now() - a.started) / 1000)); return s < 60 ? s + 's' : Math.floor(s / 60) + 'min ' + (s % 60) + 's'; };
  const durStr = a => { const ms = (a.ended || Date.now()) - a.started; const s = Math.round(ms / 1000); return s < 60 ? s + 's' : Math.floor(s / 60) + 'm ' + Math.round(s % 60) + 's'; };
  effect(draw);
  return { refresh: draw };
}
