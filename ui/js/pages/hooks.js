'use strict';
// hooks
// Customize: hooks and skills.

// ---------------------------------------------------------------- Customize: hooks, skills
const HOOK_EVENTS = { PreToolUse: ['Antes de uma ferramenta', 'Pode bloquear a ação: sair com código 2 recusa, e o que for escrito em stderr vai para o agente.'], PostToolUse: ['Depois de uma ferramenta', 'Roda depois da ação (ex.: formatar o arquivo editado). Com código 2, o stderr vai para o agente.'], UserPromptSubmit: ['Ao enviar uma mensagem', 'O que o comando imprimir entra na mensagem como contexto; código 2 impede o envio.'], Stop: ['Quando o agente termina', 'Código 2 manda o agente continuar, com o stderr como motivo (ex.: os testes falharam).'] };
const HOOK_PRESETS = [
  { title: 'Rodar os testes antes de terminar', what: 'Quando o agente diz que terminou, os testes rodam; se falharem, ele volta a trabalhar com o erro.', event: 'Stop', matcher: '', command: "npm test --silent >/dev/null 2>&1 || { echo 'Os testes falharam. Rode npm test, corrija e só então termine.' >&2; exit 2; }" },
  { title: 'Bloquear git push', what: 'O agente não publica nada no repositório remoto; push fica com você.', event: 'PreToolUse', matcher: 'Bash', command: "grep -q 'git push' && { echo 'git push é feito pelo usuário, não pelo agente.' >&2; exit 2; } || exit 0" },
  { title: 'Formatar com Prettier depois de editar', what: 'Depois de cada arquivo escrito ou editado, o projeto é formatado com o Prettier.', event: 'PostToolUse', matcher: 'Write|Edit|MultiEdit', command: 'npx --no-install prettier --write . --log-level silent >/dev/null 2>&1 || true' },
  { title: 'Estado do git em cada mensagem', what: 'Cada mensagem sua leva junto os arquivos alterados, para o agente não trabalhar às cegas.', event: 'UserPromptSubmit', matcher: '', command: 'git status --short 2>/dev/null | head -30' },
];
async function hooksSection(el, root) {
  el.append(h(`<div class="row"><h1 class="grow">${t('ui.page.customize.hooks.7e6ba5')}</h1></div>`), h(`<p class="lead">${t('ui.page.hooks.comandos-que-rodam-sozinhos.0d5d4c')}</p>`));
  const list = h('<div class="mcplist"></div>'); el.append(list);
  const draw = async () => {
    const hooks = await api('/api/hooks', { root });
    list.innerHTML = hooks.length ? '' : `<div class="oempty">${t('ui.page.hooks.nenhum-hook-ainda-use.2a3b7f')}</div>`;
    for (const x of hooks) {
      const card = h(`<div class="mcpcard"><div class="row"><span class="mdot ${x.approved ? 'ok' : 'warn'}"></span><b>${t(HOOK_EVENTS[x.event]?.[0] || x.event)}</b>${x.matcher ? `<span class="tag mono">${esc(x.matcher)}</span>` : ''}<span class="faint">${t(x.approved ? 'ativo' : 'aguardando sua aprovação')}</span><span class="grow"></span><span class="tag">${t(x.scope === 'global' ? 'todos os projetos' : 'este projeto')}</span><button class="ib del" title="${esc(t('ui.page.connectors.remover.72c3b1'))}">${I.trash}</button></div><pre class="mono hookcmd">${esc(x.command)}</pre><div class="faint u-fs115">${esc(x.source)}</div>${x.approved ? '' : `<div class="mcpappr"><span>${t('ui.page.hooks.este-comando-vai-rodar.ca7f1e')}</span><button class="btn sm pri ap">${t('ui.page.hooks.aprovar.5d97af')}</button></div>`}</div>`);
      const ap = $('.ap', card); if (ap) ap.onclick = async () => { await api('/api/hooks/approve', { body: { key: x.key }, root }); draw(); };
      $('.del', card).onclick = async () => { const r = await api('/api/hooks/remove', { body: { key: x.key }, root }); if (r.error) toast(r.error); draw(); };
      list.append(card);
    }
  };
  const add = async (event, matcher, command, scope = 'project') => { const r = await api('/api/hooks/add', { body: { event, matcher, command, scope }, root }); if (r.error) return toast(r.error); toast(t('ui.page.hooks.hook-adicionado.dfe520')); draw(); };
  el.append(h(`<h2>${t('ui.page.hooks.exemplos-prontos.2323b0')}</h2>`));
  const grid = h(`<div class="mcpgrid">${HOOK_PRESETS.map((p, i) => `<div class="mcppre"><b>${t(p.title)}</b><span class="faint">${t(p.what)}</span><span class="faint">${t('ui.page.hooks.momento.cf6efa')} ${t(HOOK_EVENTS[p.event][0])}${p.matcher ? ' · ' + esc(p.matcher) : ''}</span><span class="mono faint">${esc(p.command)}</span><div class="row"><button class="btn sm" data-h="${i}">${I.plus} ${t('ui.page.connectors.este-projeto.1d86ac')}</button><button class="btn sm" data-h="${i}" data-sc="global">${t('ui.page.connectors.todos-os-projetos.23728b')}</button></div></div>`).join('')}</div>`);
  el.append(grid);
  $$('[data-h]', grid).forEach(b => b.onclick = () => { const p = HOOK_PRESETS[+b.dataset.h]; add(p.event, p.matcher, p.command, b.dataset.sc || 'project'); });
  let ev = 'PreToolUse';
  const custom = h(`<div class="mcpcustom"><b>${t('ui.page.hooks.outro-hook.d9ff1a')}</b><div class="cs"><span class="chip hev">${t(HOOK_EVENTS[ev][0])} ${I.caret}</span> ${t('ui.page.hooks.para-as-ferramentas.b3a1ea')} <input class="slot mono hm" placeholder="Bash|Write" spellcheck="false"> ${t('ui.page.connectors.rodar.d25c49')} <input class="slot mono hc u-minw24ch" placeholder="./scripts/checar.sh" spellcheck="false"></div><div class="faint hevd u-fs12px">${t(HOOK_EVENTS[ev][1])} ${t('ui.page.hooks.o-comando-recebe-o.f61b26')}</div><div class="row"><button class="btn sm pri hadd">${t('ui.page.connectors.adicionar-a-este-projeto.21f0cd')}</button></div></div>`);
  el.append(custom);
  $('.hev', custom).onclick = e => menu(e.currentTarget, Object.entries(HOOK_EVENTS).map(([k, [l, d]]) => ({ label: l, d, check: ev === k, f: () => { ev = k; $('.hev', custom).firstChild.textContent = t(l) + ' '; $('.hevd', custom).textContent = t(d) + ' ' + t('ui.page.hooks.o-comando-recebe-o.f61b26'); } })));
  $('.hadd', custom).onclick = () => add(ev, $('.hm', custom).value.trim(), $('.hc', custom).value.trim());
  draw();
}
const SKILL_EXAMPLE = `Quando o usuário pedir para publicar uma versão:

1. Rode os testes (npm test). Se falharem, pare e mostre o erro.
2. Atualize a versão em package.json (patch, a menos que o usuário diga outra).
3. Escreva no CHANGELOG.md o que mudou desde a última tag.
4. Faça o commit "release: vX.Y.Z" e crie a tag.
5. Não faça push: mostre ao usuário o comando para ele rodar.`;
async function skillsSection(el, root) {
  el.append(h(`<div class="row"><h1 class="grow">${t('ui.page.customize.skills.816f2b')}</h1><button class="btn line ns">${I.plus} ${t('ui.page.hooks.nova-skill.04ec1b')}</button></div>`), h(`<p class="lead">${t('ui.page.hooks.procedimentos-que-o-agente.f3a91a')}</p>`));
  const list = h('<div class="mcplist"></div>'); el.append(list);
  const editor = (sk = { name: '', description: '', body: '' }) => {
    const d = h(`<div class="mcpcard"><div class="cs">${t('ui.page.connectors.nome.de4da4')} <input class="slot mono sn" placeholder="publicar-versao" value="${esc(sk.name)}" spellcheck="false"> · ${t('ui.page.hooks.use-quando.bd3b27')} <input class="slot sd u-minw30ch" placeholder="${esc(t('ui.page.hooks.o-usuario-pedir-para.840ba7'))}" value="${esc(sk.description)}"></div><textarea class="doc sb" placeholder="${esc(t('ui.page.hooks.as-instrucoes-passo-a.d566de'))}">${esc(sk.body)}</textarea><div class="row u-mt6px"><button class="btn sm pri sv">${t('ui.page.hooks.salvar-neste-projeto.a5ab4c')}</button><button class="btn sm svg">${t('ui.page.hooks.salvar-para-todos-os.421c57')}</button><button class="btn sm ex">${t('ui.page.hooks.inserir-exemplo.61c3bf')}</button><span class="grow"></span><button class="btn sm cancel">${t('ui.chat.cancelar.62b146')}</button></div></div>`);
    const save = async scope => { const r = await api('/api/skills/save', { body: { scope, name: $('.sn', d).value.trim(), description: $('.sd', d).value.trim(), body: $('.sb', d).value }, root }); if (r.error) return toast(r.error); toast(t('ui.page.hooks.skill-salva.b84010')); draw(); d.remove(); };
    $('.sv', d).onclick = () => save('project'); $('.svg', d).onclick = () => save('global');
    $('.ex', d).onclick = () => { if (!$('.sn', d).value) $('.sn', d).value = 'publicar-versao'; if (!$('.sd', d).value) $('.sd', d).value = t('ui.page.hooks.o-usuario-pedir-para.840ba7'); $('.sb', d).value = t(SKILL_EXAMPLE); };
    $('.cancel', d).onclick = () => d.remove();
    return d;
  };
  $('.ns', el).onclick = () => { const d = editor(); list.prepend(d); $('.sn', d).focus(); };
  const draw = async () => {
    const skills = await api('/api/skills', { root });
    list.innerHTML = skills.length ? '' : `<div class="oempty">${t('ui.page.hooks.nenhuma-skill-ainda-clique.5e0ef1')}</div>`;
    for (const sk of skills) {
      const card = h(`<div class="mcpcard"><div class="row"><b class="mono">${esc(sk.name)}</b><span class="tag">${t(sk.scope === 'global' ? 'todos os projetos' : 'este projeto')}</span><span class="faint grow trunc">${esc(sk.description)}</span><button class="ib ed" title="${esc(t('ui.page.hooks.editar.c9c43b'))}">${I.edit || '✎'}</button><button class="ib del" title="${esc(t('ui.page.connectors.remover.72c3b1'))}">${I.trash}</button></div><details class="odet"><summary>${t('ui.page.hooks.instrucoes.6aba61')}</summary><div class="md sel">${md(sk.body || '')}</div></details><div class="faint u-fs115">${esc(sk.dir)}</div></div>`);
      $('.ed', card).onclick = () => card.replaceWith(editor(sk));
      $('.del', card).onclick = async () => { await api('/api/skills/remove', { body: { name: sk.name }, root }); draw(); };
      list.append(card);
    }
  };
  draw();
}
