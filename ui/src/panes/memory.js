// memory — typed items the agent must always remember (correction, decision, criterion…), per
// session/project/global scope. They ride every request and survive compaction.
import { $, $$, h, esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { api } from '../core/api.js';
import { pref } from '../core/store.js';
import { menu } from '../components/menu.js';

const MEM_TYPES = {
  correcao: ['mem.type.correction', 'mem.type.correctionDesc', 'use pnpm, not npm'],
  criterio: ['mem.type.criterion', 'mem.type.criterionDesc', 'the form validates the email before sending'],
  decisao: ['mem.type.decision', 'mem.type.decisionDesc', 'CommonJS instead of ESM: runs on Node 22 without a build'],
  beco: ['mem.type.deadend', 'mem.type.deadendDesc', 'vite 6 breaks the PWA plugin; stay on 5'],
  comando: ['mem.type.command', 'mem.type.commandDesc', 'pnpm test -- --run'],
  arquivo: ['mem.type.file', 'mem.type.fileDesc', 'src/auth.ts holds the user session'],
  fato: ['mem.type.fact', 'mem.type.factDesc', 'the production API is at api.example.com'],
};
const MEM_SCOPES = { session: 'mem.scope.session', project: 'mem.scope.project', global: 'mem.scope.global' };
const MEM_PRIO = ['', 'mem.prio.normal', 'mem.prio.important', 'mem.prio.essential'];

export function mount(pb, extra, c) {
  extra.innerHTML = `<button class="icon-btn help" title="${esc(t('common.howItWorks'))}" aria-label="${esc(t('common.howItWorks'))}">?</button>`;
  const sid = c.L?.id || '';
  let data = { session: [], project: [], global: [] };
  let scope = pref('memScope') === 'session' && !sid ? 'project' : pref('memScope');
  let newType = 'correcao';
  let helpOpen = pref('memHelp');
  const save = sc => api('/api/memory/items/save', { body: { scope: sc, session: sid, items: data[sc] }, root: c.root }).catch(() => { });
  const load = async () => {
    const r = await api('/api/memory/items' + (sid ? '?session=' + encodeURIComponent(sid) : ''), { root: c.root }).catch(() => ({}));
    data = { session: r.session || [], project: r.project || [], global: r.global || [] };
    draw();
  };
  const add = (type, text) => {
    text = text.trim();
    if (!text) return;
    data[scope].unshift({ id: 'm' + Date.now() + Math.floor(Math.random() * 1000), type, text, origin: 'usuario', priority: type === 'correcao' ? 2 : 1, created: Date.now() });
    save(scope); draw();
  };
  const draw = () => {
    const items = data[scope] || [];
    pb.innerHTML = `<div class="pane-list mem">
      <div class="mintro">${t('mem.intro')}</div>
      <div class="howto" ${helpOpen ? '' : 'hidden'}>
        <div class="row"><b class="grow">${t('common.howItWorks')}</b><button class="icon-btn hx" aria-label="${esc(t('common.close'))}">${I.close}</button></div>
        <ol>
          <li>${t('mem.step1')}</li><li>${t('mem.step2')}</li><li>${t('mem.step3')}</li><li>${t('mem.step4')}</li>
        </ol>
        <div class="kinds">${Object.entries(MEM_TYPES).map(([k, [n, d, ex]]) => `<div class="mem-example" data-k="${k}" data-ex="${esc(ex)}" tabindex="0" role="button" title="${esc(t('mem.useExample'))}"><b>${t(n)}</b><span class="faint">${t(d)}</span><span class="mono faint">${t('common.eg')} ${esc(ex)}</span></div>`).join('')}</div>
      </div>
      <div class="seg-group" role="group">${Object.entries(MEM_SCOPES).map(([k, l]) => `<button class="${k === scope ? 'on' : ''}" data-sc="${k}" ${k === 'session' && !sid ? `disabled title="${esc(t('mem.needSession'))}"` : ''}>${t(l)} <span class="faint">${(data[k] || []).length}</span></button>`).join('')}</div>
      <div class="memory-add"><span class="chip mtype" role="button" tabindex="0">${t(MEM_TYPES[newType][0])} ${I.caret}</span><input class="mnew" placeholder="${esc(t('mem.placeholder') + ' ' + t(MEM_TYPES[newType][2]))}" spellcheck="false" aria-label="${esc(t('pane.memory'))}"></div>
      <div class="mlist">${items.length ? '' : `<div class="empty">${t('mem.empty')}</div>`}</div></div>`;
    const list = pb.querySelector('.mlist');
    items.forEach((it, i) => {
      const row = h(`<div class="memory-item p${it.priority || 1}"><span class="chip mt" role="button" tabindex="0">${t((MEM_TYPES[it.type] || MEM_TYPES.fato)[0])} ${I.caret}</span><textarea class="mtext" rows="1" spellcheck="false">${esc(it.text)}</textarea><button class="icon-btn star" title="${esc(t('mem.priority') + ': ' + t(MEM_PRIO[it.priority || 1]))}">${(it.priority || 1) > 1 ? '★' : '☆'}${(it.priority || 1) > 2 ? '<sup>+</sup>' : ''}</button><button class="icon-btn del" aria-label="${esc(t('common.delete'))}">${I.close}</button><div class="mmeta faint">${t(it.origin === 'agente' ? 'mem.originAgent' : 'mem.originYou')} · ${new Date(it.created).toLocaleString([], { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })}</div></div>`);
      const ta = row.querySelector('.mtext');
      const fit = () => { ta.style.height = 'auto'; ta.style.height = ta.scrollHeight + 'px'; };
      ta.oninput = () => { it.text = ta.value; fit(); save(scope); };
      setTimeout(fit, 0);
      row.querySelector('.star').onclick = () => { it.priority = (it.priority || 1) % 3 + 1; save(scope); draw(); };
      row.querySelector('.del').onclick = () => { items.splice(i, 1); save(scope); draw(); };
      row.querySelector('.mt').onclick = e => menu(e.currentTarget, Object.entries(MEM_TYPES).map(([k, [n, d]]) => ({ label: t(n), d: t(d), check: it.type === k, f: () => { it.type = k; save(scope); draw(); } })));
      list.appendChild(row);
    });
    $$('[data-sc]', pb).forEach(b => b.onclick = () => { scope = b.dataset.sc; pref('memScope', scope); draw(); });
    const inp = pb.querySelector('.mnew');
    inp.onkeydown = e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); add(newType, inp.value); setTimeout(() => pb.querySelector('.mnew')?.focus(), 0); } };
    pb.querySelector('.mtype').onclick = e => menu(e.currentTarget, Object.entries(MEM_TYPES).map(([k, [n, d]]) => ({ label: t(n), d: t(d), check: newType === k, f: () => { newType = k; const v = inp.value; draw(); const ni = pb.querySelector('.mnew'); ni.value = v; ni.focus(); } })));
    $$('.mem-example', pb).forEach(x => x.onclick = () => {
      newType = x.dataset.k; draw();
      const ni = pb.querySelector('.mnew');
      ni.value = x.dataset.ex; ni.focus(); ni.select();
    });
    pb.querySelector('.hx').onclick = () => { helpOpen = false; pref('memHelp', false); draw(); };
  };
  extra.querySelector('.help').onclick = () => { helpOpen = !helpOpen; pref('memHelp', helpOpen); draw(); };
  load();
  // the agent's memory events must not redraw the row being typed in
  return { refresh: () => { if (!pb.contains(document.activeElement)) load(); } };
}
