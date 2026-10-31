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
  el.append(h(`<div class="row"><h1 class="grow">${t('Hooks')}</h1></div>`), h(`<p class="lead">${t('Comandos que rodam sozinhos em momentos fixos do trabalho do agente — para garantir regras do projeto sem depender de o agente lembrar. Mesmo formato do Claude Code: os hooks de .claude/settings.json já valem aqui.')}</p>`));
  const list = h('<div class="mcplist"></div>'); el.append(list);
  const draw = async () => {
    const hooks = await api('/api/hooks', { root });
    list.innerHTML = hooks.length ? '' : `<div class="oempty">${t('Nenhum hook ainda. Use um exemplo abaixo.')}</div>`;
    for (const x of hooks) {
      const card = h(`<div class="mcpcard"><div class="row"><span class="mdot ${x.approved ? 'ok' : 'warn'}"></span><b>${t(HOOK_EVENTS[x.event]?.[0] || x.event)}</b>${x.matcher ? `<span class="tag mono">${esc(x.matcher)}</span>` : ''}<span class="faint">${t(x.approved ? 'ativo' : 'aguardando sua aprovação')}</span><span class="grow"></span><span class="tag">${t(x.scope === 'global' ? 'todos os projetos' : 'este projeto')}</span><button class="ib del" title="${esc(t('Remover'))}">${I.trash}</button></div><pre class="mono hookcmd">${esc(x.command)}</pre><div class="faint" style="font-size:11.5px">${esc(x.source)}</div>${x.approved ? '' : `<div class="mcpappr"><span>${t('Este comando vai rodar no seu computador sempre que o momento acontecer. Confira antes de aprovar.')}</span><button class="btn sm pri ap">${t('Aprovar')}</button></div>`}</div>`);
      const ap = $('.ap', card); if (ap) ap.onclick = async () => { await api('/api/hooks/approve', { body: { key: x.key }, root }); draw(); };
      $('.del', card).onclick = async () => { const r = await api('/api/hooks/remove', { body: { key: x.key }, root }); if (r.error) toast(r.error); draw(); };
      list.append(card);
    }
  };
  const add = async (event, matcher, command, scope = 'project') => { const r = await api('/api/hooks/add', { body: { event, matcher, command, scope }, root }); if (r.error) return toast(r.error); toast(t('Hook adicionado')); draw(); };
  el.append(h(`<h2>${t('Exemplos prontos')}</h2>`));
  const grid = h(`<div class="mcpgrid">${HOOK_PRESETS.map((p, i) => `<div class="mcppre"><b>${t(p.title)}</b><span class="faint">${t(p.what)}</span><span class="faint">${t('Momento:')} ${t(HOOK_EVENTS[p.event][0])}${p.matcher ? ' · ' + esc(p.matcher) : ''}</span><span class="mono faint">${esc(p.command)}</span><div class="row"><button class="btn sm" data-h="${i}">${I.plus} ${t('Este projeto')}</button><button class="btn sm" data-h="${i}" data-sc="global">${t('Todos os projetos')}</button></div></div>`).join('')}</div>`);
  el.append(grid);
  $$('[data-h]', grid).forEach(b => b.onclick = () => { const p = HOOK_PRESETS[+b.dataset.h]; add(p.event, p.matcher, p.command, b.dataset.sc || 'project'); });
  let ev = 'PreToolUse';
  const custom = h(`<div class="mcpcustom"><b>${t('Outro hook')}</b><div class="cs"><span class="chip hev">${t(HOOK_EVENTS[ev][0])} ${I.caret}</span> ${t('para as ferramentas')} <input class="slot mono hm" placeholder="Bash|Write" spellcheck="false"> ${t('rodar')} <input class="slot mono hc" style="min-width:24ch" placeholder="./scripts/checar.sh" spellcheck="false"></div><div class="faint hevd" style="font-size:12px">${t(HOOK_EVENTS[ev][1])} ${t('O comando recebe o evento em JSON na entrada padrão.')}</div><div class="row"><button class="btn sm pri hadd">${t('Adicionar a este projeto')}</button></div></div>`);
  el.append(custom);
  $('.hev', custom).onclick = e => menu(e.currentTarget, Object.entries(HOOK_EVENTS).map(([k, [l, d]]) => ({ label: l, d, check: ev === k, f: () => { ev = k; $('.hev', custom).firstChild.textContent = t(l) + ' '; $('.hevd', custom).textContent = t(d) + ' ' + t('O comando recebe o evento em JSON na entrada padrão.'); } })));
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
  el.append(h(`<div class="row"><h1 class="grow">${t('Skills')}</h1><button class="btn line ns">${I.plus} ${t('Nova skill')}</button></div>`), h(`<p class="lead">${t('Procedimentos que o agente segue quando a tarefa corresponde: ele vê o nome e a descrição de cada skill e carrega as instruções só quando precisa. Mesmo formato do Claude Code (pasta com SKILL.md): as skills de .claude/skills e ~/.claude/skills já valem aqui.')}</p>`));
  const list = h('<div class="mcplist"></div>'); el.append(list);
  const editor = (sk = { name: '', description: '', body: '' }) => {
    const d = h(`<div class="mcpcard"><div class="cs">${t('Nome')} <input class="slot mono sn" placeholder="publicar-versao" value="${esc(sk.name)}" spellcheck="false"> · ${t('use quando')} <input class="slot sd" style="min-width:30ch" placeholder="${esc(t('o usuário pedir para publicar uma versão'))}" value="${esc(sk.description)}"></div><textarea class="doc sb" placeholder="${esc(t('As instruções, passo a passo.'))}">${esc(sk.body)}</textarea><div class="row" style="margin-top:6px"><button class="btn sm pri sv">${t('Salvar neste projeto')}</button><button class="btn sm svg">${t('Salvar para todos os projetos')}</button><button class="btn sm ex">${t('Inserir exemplo')}</button><span class="grow"></span><button class="btn sm cancel">${t('Cancelar')}</button></div></div>`);
    const save = async scope => { const r = await api('/api/skills/save', { body: { scope, name: $('.sn', d).value.trim(), description: $('.sd', d).value.trim(), body: $('.sb', d).value }, root }); if (r.error) return toast(r.error); toast(t('Skill salva')); draw(); d.remove(); };
    $('.sv', d).onclick = () => save('project'); $('.svg', d).onclick = () => save('global');
    $('.ex', d).onclick = () => { if (!$('.sn', d).value) $('.sn', d).value = 'publicar-versao'; if (!$('.sd', d).value) $('.sd', d).value = t('o usuário pedir para publicar uma versão'); $('.sb', d).value = t(SKILL_EXAMPLE); };
    $('.cancel', d).onclick = () => d.remove();
    return d;
  };
  $('.ns', el).onclick = () => { const d = editor(); list.prepend(d); $('.sn', d).focus(); };
  const draw = async () => {
    const skills = await api('/api/skills', { root });
    list.innerHTML = skills.length ? '' : `<div class="oempty">${t('Nenhuma skill ainda. Clique em Nova skill — há um exemplo pronto.')}</div>`;
    for (const sk of skills) {
      const card = h(`<div class="mcpcard"><div class="row"><b class="mono">${esc(sk.name)}</b><span class="tag">${t(sk.scope === 'global' ? 'todos os projetos' : 'este projeto')}</span><span class="faint grow trunc">${esc(sk.description)}</span><button class="ib ed" title="${esc(t('Editar'))}">${I.edit || '✎'}</button><button class="ib del" title="${esc(t('Remover'))}">${I.trash}</button></div><details class="odet"><summary>${t('Instruções')}</summary><div class="md sel">${md(sk.body || '')}</div></details><div class="faint" style="font-size:11.5px">${esc(sk.dir)}</div></div>`);
      $('.ed', card).onclick = () => card.replaceWith(editor(sk));
      $('.del', card).onclick = async () => { await api('/api/skills/remove', { body: { name: sk.name }, root }); draw(); };
      list.append(card);
    }
  };
  draw();
}
