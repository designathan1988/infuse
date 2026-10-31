'use strict';
// docs
// Customize: pinned documentation and conventions.

// ---------------------------------------------------------------- Customize: pinned docs, conventions
async function docsSection(el, root) {
  el.append(h(`<h1>${t('ui.page.customize.documentacao-fixada.1371ec')}</h1>`), h(`<p class="lead">${t('ui.page.docs.paginas-de-documentacao-que.39e4d2')}</p>`));
  const add = h(`<div class="mcpcustom"><div class="cs">${t('ui.page.docs.fixar-a-pagina.caba6e')} <input class="slot mono du u-minw36ch" placeholder="https://react.dev/reference/react/useState" spellcheck="false"> <button class="btn sm pri dadd">${t('ui.page.docs.fixar.1354ce')}</button></div></div>`);
  const list = h('<div class="mcplist u-mt12px"></div>'), sugg = h('<div></div>');
  el.append(add, list, sugg);
  const pin = async (url, btn) => { if (btn) { btn.disabled = true; btn.textContent = t('ui.page.docs.baixando.b17cc2'); } const r = await api('/api/docs/pin', { body: { url }, root }); if (r.error) toast(r.error); else toast(t('ui.page.docs.documentacao-fixada.8309a1')); draw(); };
  $('.dadd', add).onclick = e => { const u = $('.du', add).value.trim(); if (u) pin(u, e.currentTarget); };
  $('.du', add).onkeydown = e => { if (e.key === 'Enter' && e.target.value.trim()) pin(e.target.value.trim(), $('.dadd', add)); };
  const draw = async () => {
    const r = await api('/api/docs', { root });
    list.innerHTML = (r.docs || []).length ? '' : `<div class="oempty">${t('ui.page.docs.nenhuma-pagina-fixada-use.d43e1e')}</div>`;
    for (const d of r.docs || []) {
      const card = h(`<div class="mcpcard"><div class="row"><b class="grow trunc">${esc(d.title)}</b><span class="faint">${Math.round(d.bytes / 1024)} KB · ${new Date(d.fetched).toLocaleDateString()}</span><button class="ib rf" title="${esc(t('ui.page.docs.baixar-de-novo.3b0a74'))}">${I.reload}</button><button class="ib del" title="${esc(t('ui.page.connectors.remover.72c3b1'))}">${I.trash}</button></div><div class="mono faint mcptarget u-ml0">${esc(d.url)} → ${esc(d.file)}</div></div>`);
      $('.rf', card).onclick = e => pin(d.url, null);
      $('.del', card).onclick = async () => { await api('/api/docs/unpin', { body: { url: d.url }, root }); draw(); };
      list.append(card);
    }
    const s = r.suggestions || [];
    sugg.innerHTML = s.length ? `<h2>${t('ui.page.docs.sugestoes-para-este-projeto.c15490')}</h2><div class="mcpgrid">${s.map((x, i) => `<div class="mcppre"><b>${esc(x.name)}</b><span class="faint">${esc(x.why)}</span><span class="mono faint">${esc(x.url)}</span><div class="row"><button class="btn sm" data-s="${i}">${I.plus} ${t('ui.page.docs.fixar.1354ce')}</button></div></div>`).join('')}</div>` : '';
    $$('[data-s]', sugg).forEach(b => b.onclick = () => pin(s[+b.dataset.s].url, b));
  };
  draw();
}
async function conventionsSection(el, root) {
  el.append(h(`<div class="row"><h1 class="grow">${t('ui.page.docs.convencoes-do-projeto.b68e22')}</h1><button class="btn line rl">${I.reload} ${t('ui.page.docs.ler-o-codigo-de.726eda')}</button></div>`), h(`<p class="lead">${t('ui.page.docs.o-estilo-do-projeto.26e609')}</p>`));
  const r = await api('/api/conventions', { root });
  const ta = h(`<textarea class="doc u-minh300px">${esc(r.text || '')}</textarea>`); const st = h('<span class="faint"></span>');
  ta.oninput = debounce(async () => { await api('/api/conventions', { body: { text: ta.value }, root }); st.textContent = t('ui.page.customize.salvo.7af5c9'); setTimeout(() => st.textContent = '', 1200); }, 600);
  $('.rl', el).onclick = async () => { const x = await api('/api/conventions', { body: { relearn: true }, root }); ta.value = x.text || ''; toast(t('ui.page.docs.convencoes-lidas-de-novo.0260f7')); };
  el.append(ta, st);
}
