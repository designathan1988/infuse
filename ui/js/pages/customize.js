'use strict';
// customize
// Customize page shell and commands/instructions.

// ---------------------------------------------------------------- Customize: commands, instructions
async function openCustomize(section = 'commands') {
  const el = pageShell('customize', [['commands', 'Comandos de barra'], ['skills', 'Skills'], ['connectors', 'Conectores'], ['hooks', 'Hooks'], ['docs', 'Documentação fixada'], ['conventions', 'Convenções'], ['instructions', 'Instruções do projeto']], section, openCustomize);
  const root = curRoot() || S.projects[0]?.root || '';
  if (!root) { el.append(h(`<div class="empty">${t('Abra um projeto primeiro.')}</div>`)); return; }
  el.append(h(`<p class="faint" style="margin:0 0 6px">${esc(projectLabel(root))} · ${esc(root)}</p>`));
  if (section === 'commands') {
    el.append(h(`<div class="row"><h1 class="grow">${t('Comandos de barra')}</h1><button class="btn line nc">${I.plus} ${t('Novo comando')}</button></div>`), h(`<p class="lead">${t('Prompts reutilizáveis: digite / na caixa de texto. $ARGUMENTS recebe o que vier depois do nome. Ficam em .infuse/commands (projeto) ou ~/.infuse/commands (todos).')}</p>`));
    const cmds = (await api('/api/commands', { root })).filter(c => !c.builtin);
    const editor = (c = { name: '', description: '', prompt: '' }) => {
      const d = h(`<div class="editdoc" style="margin:10px 0"><input class="name" placeholder="nome" value="${esc(c.name)}"><input class="desc" style="width:100%;margin-bottom:8px;color:var(--text2)" placeholder="${esc(t('descrição curta'))}" value="${esc(c.description)}"><textarea placeholder="${esc(t('O prompt. Ex.: Escreva testes para $ARGUMENTS seguindo o padrão do projeto.'))}">${esc(c.prompt)}</textarea><div class="row" style="margin-top:8px"><span class="chip scope">${t('este projeto')} ${I.caret}</span><span class="grow"></span>${c.name ? `<button class="btn danger del">${t('Excluir')}</button>` : ''}<button class="btn pri save">${t('Salvar')}</button></div></div>`);
      let scope = 'project'; $('.scope', d).onclick = e => menu(e.currentTarget, [{ label: 'este projeto', check: scope === 'project', f: () => { scope = 'project'; $('.scope', d).firstChild.textContent = t('este projeto') + ' '; } }, { label: 'todos os projetos', check: scope === 'global', f: () => { scope = 'global'; $('.scope', d).firstChild.textContent = t('todos os projetos') + ' '; } }]);
      $('.save', d).onclick = async () => { const r = await api('/api/commands/save', { body: { name: $('.name', d).value, description: $('.desc', d).value, prompt: $('textarea', d).value, scope, previousName: c.name }, root }); if (r.error) return toast(r.error); delete S.commands[root]; toast('Comando salvo'); openCustomize('commands'); };
      $('.del', d) && ($('.del', d).onclick = async () => { await api('/api/commands/delete', { body: { name: c.name }, root }); delete S.commands[root]; openCustomize('commands'); });
      return d;
    };
    $('.nc', el).onclick = () => { const d = editor(); el.insertBefore(d, el.children[2]); $('.name', d).focus(); };
    if (!cmds.length) el.append(h(`<div class="empty">${t('Nenhum comando personalizado.')}</div>`));
    cmds.forEach(c => { const r = h(`<div class="taskrow"><span class="mono">/${esc(c.name)}</span><span class="faint grow trunc">${esc(c.description)}</span></div>`); r.onclick = () => r.replaceWith(editor(c)); el.append(r); });
  }
  if (section === 'connectors') return connectorsSection(el, root);
  if (section === 'hooks') return hooksSection(el, root);
  if (section === 'docs') return docsSection(el, root);
  if (section === 'conventions') return conventionsSection(el, root);
  if (section === 'skills') return skillsSection(el, root);
  if (section === 'instructions') {
    el.append(h(`<h1>${t('Instruções do projeto')}</h1>`), h(`<p class="lead">${t('Memória do projeto (.infuse/memory.md), lida pelo agente no início de cada turno. Arquivos CLAUDE.md, AGENTS.md e INFUSE.md da pasta também são lidos.')}</p>`));
    const r = await api('/api/memory', { root });
    const ta = h(`<textarea class="doc" style="min-height:320px">${esc(r.content || '')}</textarea>`); const st = h('<span class="faint"></span>');
    ta.oninput = debounce(async () => { await api('/api/memory', { method: 'PUT', body: { content: ta.value }, root }); st.textContent = t('salvo'); setTimeout(() => st.textContent = '', 1200); }, 600);
    el.append(ta, st);
    if (r.instructions) el.append(h(`<h2>${t('Instruções encontradas na pasta')}</h2>`), h(`<pre class="doc sel" style="white-space:pre-wrap;font-family:var(--mono);font-size:12px;background:var(--s2);border-radius:8px;padding:10px 12px;max-height:360px;overflow:auto">${esc(r.instructions)}</pre>`));
  }
}
