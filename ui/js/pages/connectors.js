'use strict';
// connectors
// Customize: MCP connectors.

// ---------------------------------------------------------------- Customize: connectors (MCP)
const MCP_PRESETS = [
  { name: 'playwright', title: 'Navegador automatizado (Playwright)', what: 'O agente abre páginas, clica, preenche formulários e tira capturas de tela para conferir a interface que construiu.', cfg: { command: 'npx', args: ['-y', '@playwright/mcp@latest'] }, needs: 'Node.js' },
  { name: 'context7', title: 'Documentação atualizada (Context7)', what: 'Busca a documentação oficial e exemplos da versão exata da biblioteca que o projeto usa, em vez de o agente lembrar de cabeça.', cfg: { command: 'npx', args: ['-y', '@upstash/context7-mcp'] }, needs: 'Node.js' },
  { name: 'github', title: 'GitHub', what: 'Issues, pull requests, revisões e Actions do GitHub. Usa o token da variável de ambiente GITHUB_TOKEN.', cfg: { type: 'http', url: 'https://api.githubcopilot.com/mcp/', headers: { Authorization: 'Bearer ${GITHUB_TOKEN}' } }, needs: 'variável GITHUB_TOKEN' },
  { name: 'fetch', title: 'Leitor de páginas', what: 'Baixa uma página e a converte em texto limpo para o agente ler.', cfg: { command: 'uvx', args: ['mcp-server-fetch'] }, needs: 'Python com uv' },
  { name: 'sequential-thinking', title: 'Raciocínio em etapas', what: 'Ajuda o agente a decompor problemas difíceis em passos que ele revisa e corrige.', cfg: { command: 'npx', args: ['-y', '@modelcontextprotocol/server-sequential-thinking'] }, needs: 'Node.js' },
  { name: 'knowledge-graph', title: 'Grafo de conhecimento', what: 'Guarda entidades e relações entre sessões (complementa a Memória do infuse).', cfg: { command: 'npx', args: ['-y', '@modelcontextprotocol/server-memory'] }, needs: 'Node.js' },
];
const MCP_STATE = { running: ['ok', 'conectado'], starting: ['warn', 'iniciando…'], error: ['bad', 'erro'], stopped: ['', 'parado'] };
async function connectorsSection(el, root) {
  el.append(h(`<div class="row"><h1 class="grow">${t('Conectores')}</h1><button class="ib help" title="${esc(t('Como funciona'))}">?</button></div>`), h(`<p class="lead">${t('Conectores dão ao agente ferramentas de outros programas: um navegador automatizado, documentação atualizada, o GitHub… É o padrão MCP, o mesmo do Claude Code: os arquivos .mcp.json do projeto já valem aqui.')}</p>`));
  const help = h(`<div class="howto" style="display:none;margin-bottom:14px"><ol>
    <li><b>${t('Escolha um conector abaixo')}</b> ${t('e clique em Adicionar. Ele vale para este projeto ou para todos.')}</li>
    <li><b>${t('Cada conector é um programa que roda no seu computador')}</b> ${t('— fora do filtro do infuse. Por isso só inicia depois da sua aprovação, e uma mudança no comando pede aprovação de novo (um .mcp.json escrito pelo agente nunca inicia sozinho).')}</li>
    <li><b>${t('Na próxima mensagem')}</b> ${t('o agente já vê as ferramentas do conector. As marcadas como "só leitura" rodam direto; as outras pedem sua permissão, como os comandos.')}</li></ol></div>`);
  el.append(help); $('.help', el).onclick = () => help.style.display = help.style.display ? '' : 'none';
  const list = h('<div class="mcplist"></div>'); el.append(list);
  const act = async (path, body) => { const r = await api('/api/mcp/' + path, { body, root }); if (r.error) toast(r.error); draw(r.servers); };
  const draw = async servers => {
    servers = servers || await api('/api/mcp', { root });
    list.innerHTML = servers.length ? '' : `<div class="oempty">${t('Nenhum conector ainda. Adicione um abaixo.')}</div>`;
    for (const s of servers) {
      const [cls, label] = !s.approved ? ['warn', 'aguardando sua aprovação'] : s.disabled ? ['', 'desligado'] : MCP_STATE[s.state] || ['', s.state];
      const card = h(`<div class="mcpcard"><div class="row"><span class="mdot ${cls}"></span><b>${esc(s.name)}</b><span class="tag">${t(s.scope === 'global' ? 'todos os projetos' : 'este projeto')}</span><span class="faint">${t(label)}${s.state === 'running' && !s.disabled ? ' · ' + s.tools.length + ' ' + t(s.tools.length === 1 ? 'ferramenta' : 'ferramentas') : ''}</span><span class="grow"></span>${s.approved ? `<span class="sw${s.disabled ? '' : ' on'} tg" title="${esc(t('Ligar ou desligar'))}"></span>` : ''}<button class="ib more" title="${esc(t('Mais'))}">${I.more}</button></div>
        <div class="mono faint mcptarget">${esc(s.target)}</div>
        ${!s.approved ? `<div class="mcpappr"><span>${t('Este comando vai rodar no seu computador, fora do filtro. Confira antes de aprovar.')}</span><button class="btn sm pri ap">${t('Aprovar e iniciar')}</button></div>` : ''}
        ${s.error ? `<div class="errbox" style="margin-top:8px">${esc(s.error)}${s.stderr ? '\n\n' + esc(s.stderr) : ''}</div>` : ''}
        ${s.tools.length ? `<details class="odet"><summary>${t('Ferramentas')} (${s.tools.length})</summary>${s.tools.map(x => `<div class="mtool"><span class="mono">${esc(x.name)}</span>${x.readOnly ? `<span class="tag ok">${t('só leitura')}</span>` : ''}<div class="faint">${esc(x.description || '')}</div></div>`).join('')}</details>` : ''}</div>`);
      const ap = $('.ap', card); if (ap) ap.onclick = () => { ap.textContent = t('iniciando…'); act('approve', { scope: s.scope, name: s.name }); };
      const tg = $('.tg', card); if (tg) tg.onclick = () => act('toggle', { scope: s.scope, name: s.name, disabled: !s.disabled });
      $('.more', card).onclick = e => menu(e.currentTarget, [{ label: 'Reiniciar', f: () => act('restart', { scope: s.scope, name: s.name }) }, { sep: 1 }, { label: 'Remover', danger: 1, f: () => act('remove', { scope: s.scope, name: s.name }) }], null, { alignRight: true });
      list.append(card);
    }
    drawPresets(servers);
  };
  const presets = h('<div></div>'); el.append(h(`<h2>${t('Adicionar conector')}</h2>`), presets);
  const addCfg = (name, cfg, scope) => act('add', { name, scope, config: cfg, approve: true });
  const drawPresets = servers => {
    const have = new Set(servers.map(s => s.name));
    presets.innerHTML = `<div class="mcpgrid">${MCP_PRESETS.map((p, i) => `<div class="mcppre${have.has(p.name) ? ' on' : ''}"><b>${t(p.title)}</b><span class="faint">${t(p.what)}</span><span class="mono faint">${esc(p.cfg.url || [p.cfg.command, ...p.cfg.args].join(' '))}</span><span class="faint">${t('precisa de')} ${t(p.needs)}</span><div class="row">${have.has(p.name) ? `<span class="tag ok">${t('adicionado')}</span>` : `<button class="btn sm" data-p="${i}" data-sc="project">${I.plus} ${t('Este projeto')}</button><button class="btn sm" data-p="${i}" data-sc="global">${t('Todos os projetos')}</button>`}</div></div>`).join('')}</div>
      <div class="mcpcustom"><b>${t('Outro conector')}</b><div class="cs">${t('Nome')} <input class="slot mono cn" placeholder="meu-servidor" spellcheck="false"> · ${t('rodar')} <input class="slot mono cc" placeholder="npx -y pacote-mcp" spellcheck="false"> ${t('ou conectar a')} <input class="slot mono cu" placeholder="https://exemplo.com/mcp" spellcheck="false"></div><div class="row"><button class="btn sm pri cadd">${t('Adicionar a este projeto')}</button><span class="faint">${t('O comando é dividido nos espaços; para argumentos com espaço, edite o .mcp.json.')}</span></div></div>`;
    $$('[data-p]', presets).forEach(b => b.onclick = () => { const p = MCP_PRESETS[+b.dataset.p]; addCfg(p.name, p.cfg, b.dataset.sc); });
    $('.cadd', presets).onclick = () => {
      const name = $('.cn', presets).value.trim(), cmd = $('.cc', presets).value.trim(), url = $('.cu', presets).value.trim();
      if (!name || (!cmd && !url)) return toast(t('Informe o nome e o comando (ou o endereço)'));
      const parts = cmd.split(/\s+/); addCfg(name, url && !cmd ? { type: 'http', url } : { command: parts[0], args: parts.slice(1) }, 'project');
    };
  };
  draw();
}
