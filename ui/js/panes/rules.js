'use strict';
// rules
// Filter rules pane.

function mountMemory(pb, extra, c) {
  extra.innerHTML = `<button class="ib help" title="${esc(t('ui.page.connectors.como-funciona.9b4488'))}">?</button>`;
  const sid = c.L?.id || '';
  let data = { session: [], project: [], global: [] }, scope = lsGet('memScope', 'project'), newType = 'correcao', helpOpen = lsGet('memHelp', true);
  if (scope === 'session' && !sid) scope = 'project';
  const save = debounce(sc => api('/api/memory/items/save', { body: { scope: sc, session: sid, items: data[sc] }, root: c.root }), 300);
  const load = async () => { const r = await api('/api/memory/items' + (sid ? '?session=' + encodeURIComponent(sid) : ''), { root: c.root }); data = { session: r.session || [], project: r.project || [], global: r.global || [] }; draw(); };
  const add = (type, text) => { text = text.trim(); if (!text) return; data[scope].unshift({ id: 'm' + Date.now() + Math.floor(Math.random() * 1000), type, text, origin: 'usuario', priority: type === 'correcao' ? 2 : 1, created: Date.now() }); save(scope); draw(); };
  const draw = () => {
    const items = data[scope] || [];
    pb.innerHTML = `<div class="plist mem">
      <div class="mintro">${t('ui.pane.rules.o-que-o-agente.f200e5')}</div>
      <div class="howto${helpOpen ? '' : ' hidden'}"><div class="row"><b class="grow">${t('ui.page.connectors.como-funciona.9b4488')}</b><button class="ib hx" title="${esc(t('ui.pane.oracle.fechar.5749fb'))}">${I.close}</button></div>
        <ol><li>${t('ui.pane.rules.escreva-uma-frase-e.994b45')}</li>
        <li>${t('ui.pane.rules.a-estrela-marca-o.16890c')}</li>
        <li>${t('ui.pane.rules.o-agente-tambem-registra.65a6d0')}</li>
        <li>${t('ui.pane.rules.escopo-esta-sessao-vale.8d62a6')}</li></ol>
        <div class="kinds">${Object.entries(MEM_TYPES).map(([k, [n, d, ex]]) => `<div class="mex" data-k="${k}" data-ex="${esc(ex)}" title="${esc(t('ui.pane.rules.usar-este-exemplo.49a6d1'))}"><b>${t(n)}</b><span class="faint">${t(d)}</span><span class="mono faint">${t('ui.pane.oracle.ex.645d99')} ${esc(ex)}</span></div>`).join('')}</div></div>
      <div class="seg">${Object.entries(MEM_SCOPES).map(([k, l]) => `<button class="${k === scope ? 'on' : ''}" data-sc="${k}" ${k === 'session' && !sid ? 'disabled title="' + esc(t('ui.pane.rules.abra-uma-sessao.510bf6')) + '"' : ''}>${t(l)} <span class="faint">${(data[k] || []).length}</span></button>`).join('')}</div>
      <div class="madd"><span class="chip mtype">${t(MEM_TYPES[newType][0])} ${I.caret}</span><input class="mnew" placeholder="${esc(t('ui.pane.rules.algo-para-lembrar-ex.d065e0') + ' ' + t(MEM_TYPES[newType][2]))}" spellcheck="false"></div>
      <div class="mlist">${items.length ? '' : `<div class="oempty">${t('ui.pane.rules.nada-aqui-ainda-escreva.8240af')}</div>`}</div></div>`;
    const list = $('.mlist', pb);
    items.forEach((it, i) => {
      const row = h(`<div class="mitem p${it.priority || 1}"><span class="chip mt">${t((MEM_TYPES[it.type] || MEM_TYPES.fato)[0])} ${I.caret}</span><textarea class="mtext" rows="1" spellcheck="false">${esc(it.text)}</textarea><button class="ib star" title="${esc(t('ui.pane.rules.prioridade.82846d') + ': ' + t(MEM_PRIO[it.priority || 1]))}">${(it.priority || 1) > 1 ? '★' : '☆'}${(it.priority || 1) > 2 ? '<sup>+</sup>' : ''}</button><button class="ib del" title="${esc(t('ui.pane.rules.apagar.8763ac'))}">${I.close}</button><div class="mmeta faint">${t(it.origin === 'agente' ? 'agente' : 'você')} · ${new Date(it.created).toLocaleString([], { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })}</div></div>`);
      const ta = $('.mtext', row); const fit = () => { ta.style.height = 'auto'; ta.style.height = ta.scrollHeight + 'px'; };
      ta.oninput = () => { it.text = ta.value; fit(); save(scope); }; setTimeout(fit, 0);
      $('.star', row).onclick = () => { it.priority = (it.priority || 1) % 3 + 1; save(scope); draw(); };
      $('.del', row).onclick = () => { items.splice(i, 1); save(scope); draw(); };
      $('.mt', row).onclick = e => menu(e.currentTarget, Object.entries(MEM_TYPES).map(([k, [n, d]]) => ({ label: n, d, check: it.type === k, f: () => { it.type = k; save(scope); draw(); } })));
      list.appendChild(row);
    });
    $$('[data-sc]', pb).forEach(b => b.onclick = () => { scope = b.dataset.sc; lsSet('memScope', scope); draw(); });
    const inp = $('.mnew', pb);
    inp.onkeydown = e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); add(newType, inp.value); setTimeout(() => $('.mnew', pb)?.focus(), 0); } };
    $('.mtype', pb).onclick = e => menu(e.currentTarget, Object.entries(MEM_TYPES).map(([k, [n, d]]) => ({ label: n, d, check: newType === k, f: () => { newType = k; const v = inp.value; draw(); const ni = $('.mnew', pb); ni.value = v; ni.focus(); } })));
    $$('.mex', pb).forEach(x => x.onclick = () => { newType = x.dataset.k; draw(); const ni = $('.mnew', pb); ni.value = x.dataset.ex; ni.focus(); ni.select(); });
    $('.hx', pb).onclick = () => { helpOpen = false; lsSet('memHelp', false); draw(); };
  };
  $('.help', extra).onclick = () => { helpOpen = !helpOpen; lsSet('memHelp', helpOpen); draw(); };
  load(); return { refresh: load };
}
