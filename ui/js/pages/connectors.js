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
  el.append(h(`<div class="row"><h1 class="grow">${t('ui.page.connectors.conectores.35b005')}</h1><button class="ib help" title="${esc(t('ui.page.connectors.como-funciona.9b4488'))}">?</button></div>`), h(`<p class="lead">${t('ui.page.connectors.conectores-dao-ao-agente.e3dd4c')}</p>`));
  const help = h(`<div class="howto" style="display:none;margin-bottom:14px"><ol>
    <li><b>${t('ui.page.connectors.escolha-um-conector-abaixo.c29fd8')}</b> ${t('ui.page.connectors.e-clique-em-adicionar.5f22af')}</li>
    <li><b>${t('ui.page.connectors.cada-conector-e-um.c1271a')}</b> ${t('ui.page.connectors.fora-do-filtro-do.480dfd')}</li>
    <li><b>${t('ui.page.connectors.na-proxima-mensagem.f6f7e5')}</b> ${t('ui.page.connectors.o-agente-ja-ve.c4fcc3')}</li></ol></div>`);
  el.append(help); $('.help', el).onclick = () => help.style.display = help.style.display ? '' : 'none';
  const list = h('<div class="mcplist"></div>'); el.append(list);
  const act = async (path, body) => { const r = await api('/api/mcp/' + path, { body, root }); if (r.error) toast(r.error); draw(r.servers); };
  const draw = async servers => {
    servers = servers || await api('/api/mcp', { root });
    list.innerHTML = servers.length ? '' : `<div class="oempty">${t('ui.page.connectors.nenhum-conector-ainda-adicione.4c32e2')}</div>`;
    for (const s of servers) {
      const [cls, label] = !s.approved ? ['warn', 'aguardando sua aprovação'] : s.disabled ? ['', 'desligado'] : MCP_STATE[s.state] || ['', s.state];
      const card = h(`<div class="mcpcard"><div class="row"><span class="mdot ${cls}"></span><b>${esc(s.name)}</b><span class="tag">${t(s.scope === 'global' ? 'todos os projetos' : 'este projeto')}</span><span class="faint">${t(label)}${s.state === 'running' && !s.disabled ? ' · ' + s.tools.length + ' ' + t(s.tools.length === 1 ? 'ferramenta' : 'ferramentas') : ''}</span><span class="grow"></span>${s.approved ? `<span class="sw${s.disabled ? '' : ' on'} tg" title="${esc(t('ui.page.connectors.ligar-ou-desligar.2d5c2b'))}"></span>` : ''}<button class="ib more" title="${esc(t('ui.page.connectors.mais.4f728e'))}">${I.more}</button></div>
        <div class="mono faint mcptarget">${esc(s.target)}</div>
        ${!s.approved ? `<div class="mcpappr"><span>${t('ui.page.connectors.este-comando-vai-rodar.70d93f')}</span><button class="btn sm pri ap">${t('ui.page.connectors.aprovar-e-iniciar.390a90')}</button></div>` : ''}
        ${s.error ? `<div class="errbox" style="margin-top:8px">${esc(s.error)}${s.stderr ? '\n\n' + esc(s.stderr) : ''}</div>` : ''}
        ${s.tools.length ? `<details class="odet"><summary>${t('ui.page.connectors.ferramentas.0cdcbe')} (${s.tools.length})</summary>${s.tools.map(x => `<div class="mtool"><span class="mono">${esc(x.name)}</span>${x.readOnly ? `<span class="tag ok">${t('ui.page.connectors.so-leitura.8d4a0f')}</span>` : ''}<div class="faint">${esc(x.description || '')}</div></div>`).join('')}</details>` : ''}</div>`);
      const ap = $('.ap', card); if (ap) ap.onclick = () => { ap.textContent = t('ui.page.connectors.iniciando.08bd81'); act('approve', { scope: s.scope, name: s.name }); };
      const tg = $('.tg', card); if (tg) tg.onclick = () => act('toggle', { scope: s.scope, name: s.name, disabled: !s.disabled });
      $('.more', card).onclick = e => menu(e.currentTarget, [{ label: 'Reiniciar', f: () => act('restart', { scope: s.scope, name: s.name }) }, { sep: 1 }, { label: 'Remover', danger: 1, f: () => act('remove', { scope: s.scope, name: s.name }) }], null, { alignRight: true });
      list.append(card);
    }
    drawPresets(servers);
  };
  const presets = h('<div></div>'); el.append(h(`<h2>${t('ui.page.connectors.adicionar-conector.80a931')}</h2>`), presets);
  const addCfg = (name, cfg, scope) => act('add', { name, scope, config: cfg, approve: true });
  const drawPresets = servers => {
    const have = new Set(servers.map(s => s.name));
    presets.innerHTML = `<div class="mcpgrid">${MCP_PRESETS.map((p, i) => `<div class="mcppre${have.has(p.name) ? ' on' : ''}"><b>${t(p.title)}</b><span class="faint">${t(p.what)}</span><span class="mono faint">${esc(p.cfg.url || [p.cfg.command, ...p.cfg.args].join(' '))}</span><span class="faint">${t('ui.page.connectors.precisa-de.75e090')} ${t(p.needs)}</span><div class="row">${have.has(p.name) ? `<span class="tag ok">${t('ui.page.connectors.adicionado.be0a03')}</span>` : `<button class="btn sm" data-p="${i}" data-sc="project">${I.plus} ${t('ui.page.connectors.este-projeto.1d86ac')}</button><button class="btn sm" data-p="${i}" data-sc="global">${t('ui.page.connectors.todos-os-projetos.23728b')}</button>`}</div></div>`).join('')}</div>
      <div class="mcpcustom"><b>${t('ui.page.connectors.outro-conector.f17827')}</b><div class="cs">${t('ui.page.connectors.nome.de4da4')} <input class="slot mono cn" placeholder="meu-servidor" spellcheck="false"> · ${t('ui.page.connectors.rodar.d25c49')} <input class="slot mono cc" placeholder="npx -y pacote-mcp" spellcheck="false"> ${t('ui.page.connectors.ou-conectar-a.3cb037')} <input class="slot mono cu" placeholder="https://exemplo.com/mcp" spellcheck="false"></div><div class="row"><button class="btn sm pri cadd">${t('ui.page.connectors.adicionar-a-este-projeto.21f0cd')}</button><span class="faint">${t('ui.page.connectors.o-comando-e-dividido.109e81')}</span></div></div>`;
    $$('[data-p]', presets).forEach(b => b.onclick = () => { const p = MCP_PRESETS[+b.dataset.p]; addCfg(p.name, p.cfg, b.dataset.sc); });
    $('.cadd', presets).onclick = () => {
      const name = $('.cn', presets).value.trim(), cmd = $('.cc', presets).value.trim(), url = $('.cu', presets).value.trim();
      if (!name || (!cmd && !url)) return toast(t('ui.page.connectors.informe-o-nome-e.a52654'));
      const parts = cmd.split(/\s+/); addCfg(name, url && !cmd ? { type: 'http', url } : { command: parts[0], args: parts.slice(1) }, 'project');
    };
  };
  draw();
}
