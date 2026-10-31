'use strict';
// rules
// Filter rules pane.

function mountMemory(pb, extra, c) {
  extra.innerHTML = `<button class="ib help" title="${esc(t('Como funciona'))}">?</button>`;
  const sid = c.L?.id || '';
  let data = { session: [], project: [], global: [] }, scope = lsGet('memScope', 'project'), newType = 'correcao', helpOpen = lsGet('memHelp', true);
  if (scope === 'session' && !sid) scope = 'project';
  const save = debounce(sc => api('/api/memory/items/save', { body: { scope: sc, session: sid, items: data[sc] }, root: c.root }), 300);
  const load = async () => { const r = await api('/api/memory/items' + (sid ? '?session=' + encodeURIComponent(sid) : ''), { root: c.root }); data = { session: r.session || [], project: r.project || [], global: r.global || [] }; draw(); };
  const add = (type, text) => { text = text.trim(); if (!text) return; data[scope].unshift({ id: 'm' + Date.now() + Math.floor(Math.random() * 1000), type, text, origin: 'usuario', priority: type === 'correcao' ? 2 : 1, created: Date.now() }); save(scope); draw(); };
  const draw = () => {
    const items = data[scope] || [];
    pb.innerHTML = `<div class="plist mem">
      <div class="mintro">${t('O que o agente precisa lembrar sempre. Estes itens vão em toda mensagem, mesmo depois que a conversa é resumida — os mais importantes primeiro.')}</div>
      <div class="howto" style="${helpOpen ? '' : 'display:none'}"><div class="row"><b class="grow">${t('Como funciona')}</b><button class="ib hx" title="${esc(t('Fechar'))}">${I.close}</button></div>
        <ol><li>${t('Escreva uma frase e tecle Enter. Escolha o tipo à esquerda — o tipo ajuda o agente a pesar o item.')}</li>
        <li>${t('A estrela marca o que é importante ou essencial: vai primeiro e nunca fica de fora.')}</li>
        <li>${t('O agente também registra itens sozinho (ex.: quando você o corrige). Os dele aparecem marcados "agente"; edite ou apague à vontade.')}</li>
        <li>${t('Escopo: "Esta sessão" vale só aqui; "Este projeto" para toda sessão deste projeto; "Todos os projetos" para tudo.')}</li></ol>
        <div class="kinds">${Object.entries(MEM_TYPES).map(([k, [n, d, ex]]) => `<div class="mex" data-k="${k}" data-ex="${esc(ex)}" title="${esc(t('Usar este exemplo'))}"><b>${t(n)}</b><span class="faint">${t(d)}</span><span class="mono faint">${t('ex.:')} ${esc(ex)}</span></div>`).join('')}</div></div>
      <div class="seg">${Object.entries(MEM_SCOPES).map(([k, l]) => `<button class="${k === scope ? 'on' : ''}" data-sc="${k}" ${k === 'session' && !sid ? 'disabled title="' + esc(t('Abra uma sessão')) + '"' : ''}>${t(l)} <span class="faint">${(data[k] || []).length}</span></button>`).join('')}</div>
      <div class="madd"><span class="chip mtype">${t(MEM_TYPES[newType][0])} ${I.caret}</span><input class="mnew" placeholder="${esc(t('Algo para lembrar — ex.:') + ' ' + t(MEM_TYPES[newType][2]))}" spellcheck="false"></div>
      <div class="mlist">${items.length ? '' : `<div class="oempty">${t('Nada aqui ainda. Escreva acima, ou clique num exemplo em "Como funciona" (?).')}</div>`}</div></div>`;
    const list = $('.mlist', pb);
    items.forEach((it, i) => {
      const row = h(`<div class="mitem p${it.priority || 1}"><span class="chip mt">${t((MEM_TYPES[it.type] || MEM_TYPES.fato)[0])} ${I.caret}</span><textarea class="mtext" rows="1" spellcheck="false">${esc(it.text)}</textarea><button class="ib star" title="${esc(t('Prioridade') + ': ' + t(MEM_PRIO[it.priority || 1]))}">${(it.priority || 1) > 1 ? '★' : '☆'}${(it.priority || 1) > 2 ? '<sup>+</sup>' : ''}</button><button class="ib del" title="${esc(t('Apagar'))}">${I.close}</button><div class="mmeta faint">${t(it.origin === 'agente' ? 'agente' : 'você')} · ${new Date(it.created).toLocaleString([], { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })}</div></div>`);
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
