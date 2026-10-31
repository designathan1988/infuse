'use strict';
// customize
// Customize page shell and commands/instructions.

// ---------------------------------------------------------------- Customize: commands, instructions
async function openCustomize(section = 'commands') {
  UI.set('pageArg', section);
  const el = pageShell('customize', [['commands', 'Comandos de barra'], ['skills', 'Skills'], ['connectors', 'Conectores'], ['hooks', 'Hooks'], ['docs', 'Documentação fixada'], ['conventions', 'Convenções'], ['instructions', 'Instruções do projeto']], section, openCustomize);
  const root = curRoot() || S.projects[0]?.root || '';
  if (!root) { el.append(h(`<div class="empty">${t('ui.page.customize.abra-um-projeto-primeiro.962a7e')}</div>`)); return; }
  el.append(h(`<p class="faint u-m006px">${esc(projectLabel(root))} · ${esc(root)}</p>`));
  if (section === 'commands') {
    el.append(h(`<div class="row"><h1 class="grow">${t('ui.composer.comandos-de-barra.23e4a2')}</h1><button class="btn line nc">${I.plus} ${t('ui.page.customize.novo-comando.5b6f06')}</button></div>`), h(`<p class="lead">${t('ui.page.customize.prompts-reutilizaveis-digite-na.f517a6')}</p>`));
    const cmds = (await api('/api/commands', { root })).filter(c => !c.builtin);
    const editor = (c = { name: '', description: '', prompt: '' }) => {
      const d = h(`<div class="editdoc u-m10px0"><input class="name" placeholder="nome" value="${esc(c.name)}"><input class="desc u-w100-mb8px-c--text2" placeholder="${esc(t('ui.page.customize.descricao-curta.1f9e4f'))}" value="${esc(c.description)}"><textarea placeholder="${esc(t('ui.page.customize.o-prompt-ex-escreva.6b26d5'))}">${esc(c.prompt)}</textarea><div class="row u-mt8px"><span class="chip scope">${t('ui.page.connectors.este-projeto.1d86ac-2')} ${I.caret}</span><span class="grow"></span>${c.name ? `<button class="btn danger del">${t('ui.sidebar.excluir.e8b066')}</button>` : ''}<button class="btn pri save">${t('ui.page.customize.salvar.4a40c3')}</button></div></div>`);
      let scope = 'project'; $('.scope', d).onclick = e => menu(e.currentTarget, [{ label: 'este projeto', check: scope === 'project', f: () => { scope = 'project'; $('.scope', d).firstChild.textContent = t('ui.page.connectors.este-projeto.1d86ac-2') + ' '; } }, { label: 'todos os projetos', check: scope === 'global', f: () => { scope = 'global'; $('.scope', d).firstChild.textContent = t('ui.page.connectors.todos-os-projetos.23728b-2') + ' '; } }]);
      $('.save', d).onclick = async () => { const r = await api('/api/commands/save', { body: { name: $('.name', d).value, description: $('.desc', d).value, prompt: $('textarea', d).value, scope, previousName: c.name }, root }); if (r.error) return toast(r.error); UI.del('commands', root); toast('Comando salvo'); openCustomize('commands'); };
      $('.del', d) && ($('.del', d).onclick = async () => { await api('/api/commands/delete', { body: { name: c.name }, root }); UI.del('commands', root); openCustomize('commands'); });
      return d;
    };
    $('.nc', el).onclick = () => { const d = editor(); el.insertBefore(d, el.children[2]); $('.name', d).focus(); };
    if (!cmds.length) el.append(h(`<div class="empty">${t('ui.page.customize.nenhum-comando-personalizado.a6a535')}</div>`));
    cmds.forEach(c => { const r = h(`<div class="taskrow"><span class="mono">/${esc(c.name)}</span><span class="faint grow trunc">${esc(c.description)}</span></div>`); r.onclick = () => r.replaceWith(editor(c)); el.append(r); });
  }
  if (section === 'connectors') return connectorsSection(el, root);
  if (section === 'hooks') return hooksSection(el, root);
  if (section === 'docs') return docsSection(el, root);
  if (section === 'conventions') return conventionsSection(el, root);
  if (section === 'skills') return skillsSection(el, root);
  if (section === 'instructions') {
    el.append(h(`<h1>${t('ui.page.customize.instrucoes-do-projeto.263528')}</h1>`), h(`<p class="lead">${t('ui.page.customize.memoria-do-projeto-infuse.a4e893')}</p>`));
    const r = await api('/api/memory', { root });
    const ta = h(`<textarea class="doc u-minh320px">${esc(r.content || '')}</textarea>`); const st = h('<span class="faint"></span>');
    ta.oninput = debounce(async () => { await api('/api/memory', { method: 'PUT', body: { content: ta.value }, root }); st.textContent = t('ui.page.customize.salvo.7af5c9'); setTimeout(() => st.textContent = '', 1200); }, 600);
    el.append(ta, st);
    if (r.instructions) el.append(h(`<h2>${t('ui.page.customize.instrucoes-encontradas-na-pasta.8150f2')}</h2>`), h(`<pre class="doc sel u-741457b61d">${esc(r.instructions)}</pre>`));
  }
}
