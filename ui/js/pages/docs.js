'use strict';
// docs
// Customize: pinned documentation and conventions.

// ---------------------------------------------------------------- Customize: pinned docs, conventions
async function docsSection(el, root) {
  el.append(h(`<h1>${t('Documentação fixada')}</h1>`), h(`<p class="lead">${t('Páginas de documentação que o agente lê antes de usar uma biblioteca — a referência oficial da versão em uso, em vez do que o modelo lembra. O infuse abre a página num navegador sem janela, guarda o texto em .infuse/docs e avisa o agente.')}</p>`));
  const add = h(`<div class="mcpcustom"><div class="cs">${t('Fixar a página')} <input class="slot mono du" style="min-width:36ch" placeholder="https://react.dev/reference/react/useState" spellcheck="false"> <button class="btn sm pri dadd">${t('Fixar')}</button></div></div>`);
  const list = h('<div class="mcplist" style="margin-top:12px"></div>'), sugg = h('<div></div>');
  el.append(add, list, sugg);
  const pin = async (url, btn) => { if (btn) { btn.disabled = true; btn.textContent = t('baixando…'); } const r = await api('/api/docs/pin', { body: { url }, root }); if (r.error) toast(r.error); else toast(t('Documentação fixada.')); draw(); };
  $('.dadd', add).onclick = e => { const u = $('.du', add).value.trim(); if (u) pin(u, e.currentTarget); };
  $('.du', add).onkeydown = e => { if (e.key === 'Enter' && e.target.value.trim()) pin(e.target.value.trim(), $('.dadd', add)); };
  const draw = async () => {
    const r = await api('/api/docs', { root });
    list.innerHTML = (r.docs || []).length ? '' : `<div class="oempty">${t('Nenhuma página fixada. Use uma sugestão abaixo ou cole um endereço acima.')}</div>`;
    for (const d of r.docs || []) {
      const card = h(`<div class="mcpcard"><div class="row"><b class="grow trunc">${esc(d.title)}</b><span class="faint">${Math.round(d.bytes / 1024)} KB · ${new Date(d.fetched).toLocaleDateString()}</span><button class="ib rf" title="${esc(t('Baixar de novo'))}">${I.reload}</button><button class="ib del" title="${esc(t('Remover'))}">${I.trash}</button></div><div class="mono faint mcptarget" style="margin-left:0">${esc(d.url)} → ${esc(d.file)}</div></div>`);
      $('.rf', card).onclick = e => pin(d.url, null);
      $('.del', card).onclick = async () => { await api('/api/docs/unpin', { body: { url: d.url }, root }); draw(); };
      list.append(card);
    }
    const s = r.suggestions || [];
    sugg.innerHTML = s.length ? `<h2>${t('Sugestões para este projeto')}</h2><div class="mcpgrid">${s.map((x, i) => `<div class="mcppre"><b>${esc(x.name)}</b><span class="faint">${esc(x.why)}</span><span class="mono faint">${esc(x.url)}</span><div class="row"><button class="btn sm" data-s="${i}">${I.plus} ${t('Fixar')}</button></div></div>`).join('')}</div>` : '';
    $$('[data-s]', sugg).forEach(b => b.onclick = () => pin(s[+b.dataset.s].url, b));
  };
  draw();
}
async function conventionsSection(el, root) {
  el.append(h(`<div class="row"><h1 class="grow">${t('Convenções do projeto')}</h1><button class="btn line rl">${I.reload} ${t('Ler o código de novo')}</button></div>`), h(`<p class="lead">${t('O estilo do projeto, contado nos próprios arquivos (indentação, aspas, módulos, nomes de arquivo, testes, idioma dos comentários). O agente segue estas regras ao escrever. Edite à vontade — salva sozinho.')}</p>`));
  const r = await api('/api/conventions', { root });
  const ta = h(`<textarea class="doc" style="min-height:300px">${esc(r.text || '')}</textarea>`); const st = h('<span class="faint"></span>');
  ta.oninput = debounce(async () => { await api('/api/conventions', { body: { text: ta.value }, root }); st.textContent = t('salvo'); setTimeout(() => st.textContent = '', 1200); }, 600);
  $('.rl', el).onclick = async () => { const x = await api('/api/conventions', { body: { relearn: true }, root }); ta.value = x.text || ''; toast(t('Convenções lidas de novo')); };
  el.append(ta, st);
}
