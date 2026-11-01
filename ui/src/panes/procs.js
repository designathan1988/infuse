// procs — background processes of the session: state, elapsed time, live output, stop.
import { $, $$, esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { effect } from '../core/signals.js';
import { api } from '../core/api.js';
import { elapsed } from '../core/fmt.js';
import { liveOf } from '../core/store.js';

export function mount(pb, extra, c) {
  const open = new Set(), full = {};
  const load = async (L, p) => {
    if (full[p.id]?.bytes === p.bytes) return;
    const r = await api(`/api/session/${L.id}/procs`, { body: { action: 'output', id: p.id }, root: L.root }).catch(() => ({}));
    full[p.id] = { bytes: p.bytes, text: r.output ?? '' };
    draw();
  };
  const draw = () => {
    const L = c.L;
    const ps = (L?.snap.get()?.procs || []).slice().reverse();
    ps.filter(p => open.has(p.id) && full[p.id]?.bytes !== p.bytes).forEach(p => c.L && load(c.L, p));
    pb.innerHTML = ps.length ? `<div class="pane-list">${ps.map(p => `<div class="proc-card" data-id="${esc(p.id)}">
      <div class="row ph2" role="button" tabindex="0"><span class="dot ${p.state === 'running' ? 'run' : p.exit ? 'bad' : 'ok'}"></span><span class="grow">${p.description ? `<span class="d">${esc(p.description)}</span>` : ''}<span class="mono cmd">${esc(p.command)}</span>
      <span class="faint meta">${p.id} · ${p.state === 'running' ? t('procs.runningFor') + ' ' + elapsed(p.started) : (p.state === 'stopped' ? t('procs.stopped') : t('procs.ended') + (p.exit != null ? ' · ' + t('procs.code') + ' ' + p.exit : '')) + ' · ' + t('procs.took') + ' ' + elapsed(p.started, p.ended)}</span></span>
      ${p.state === 'running' ? `<button class="btn sm danger stop">${t('procs.stopBtn')}</button>` : ''}</div>
      ${open.has(p.id) ? `<pre class="proc-out">${esc(full[p.id]?.text ?? '')}</pre>` : ''}</div>`).join('')}</div>`
      : `<div class="empty">${t('procs.empty')}</div>`;
    $$('.proc-card', pb).forEach(el => {
      const id = el.dataset.id;
      el.querySelector('.ph2').onclick = e => { if (e.target.closest('.stop')) return; open.has(id) ? open.delete(id) : open.add(id); draw(); };
      el.querySelector('.ph2').onkeydown = e => { if (e.key === 'Enter') el.querySelector('.ph2').click(); };
      const st = el.querySelector('.stop');
      if (st) st.onclick = async () => { const L2 = c.L; if (L2) await api(`/api/session/${L2.id}/procs`, { body: { action: 'stop', id }, root: L2.root }).catch(() => { }); };
      const pre = el.querySelector('.proc-out');
      if (pre) pre.scrollTop = pre.scrollHeight;
    });
  };
  effect(draw);
  const timer = setInterval(() => { if ((c.L?.snap.get()?.procs || []).some(p => p.state === 'running')) draw(); }, 1000);
  return { refresh: draw, destroy: () => clearInterval(timer) };
}
