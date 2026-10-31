'use strict';
// settings
// Settings page.

// ---------------------------------------------------------------- Settings
async function openSettings(section = 'general') {
  await loadSettings();
  const sections = [['general', 'Geral'], ['providers', 'Provedores'], ['models', 'Modelos e custos'], ['permissions', 'Permissões'], ['code', 'Sessões e worktrees'], ['apps', 'Aplicativos'], ['integration', 'Outros agentes'], ['about', 'Sobre']];
  const el = pageShell('settings', sections, section, openSettings);
  const s = S.settings;
  const save = patch => saveSetting(patch);
  if (section === 'general') {
    el.append(h(`<h1>${t('Geral')}</h1>`), h(`<p class="lead">${t('Tudo é aplicado ao mudar.')}</p>`));
    el.append(setRow('Idioma', 'Idioma da interface', selectEl([['pt-BR', 'Português (Brasil)'], ['en', 'English']], s.language === 'en' ? 'en' : 'pt-BR', v => save({ language: v }).then(() => location.reload()))));
    el.append(setRow('Tema', 'Claro, escuro ou seguir o sistema', selectEl([['system', 'Seguir o sistema'], ['light', 'Claro'], ['dark', 'Escuro']], s.theme || 'system', v => save({ theme: v }).then(applyTheme))));
    el.append(setRow('Exibição da conversa', 'Normal: texto completo, raciocínio e ferramentas em uma linha. Pensamento: abre o raciocínio. Detalhado: abre tudo, inclusive cada passo dos subagentes. Também em Ctrl+O e no menu da sessão.', selectEl(VIEW_MODES.map(([k, l]) => [k, l]), S.viewMode, v => setViewMode(v))));
    el.append(h(`<h2>${t('Protocolo do agente')}</h2>`), h(`<p class="lead" style="margin-top:0">${t('Regras que o infuse impõe a cada ação do agente — ele não escolhe segui-las. Sempre ativas: plano antes de alterar arquivos (cada tarefa com comprovação), documentação consultada antes de usar uma biblioteca ou API externa, pesquisa depois de duas falhas, testes que não podem ser enfraquecidos, critérios intocáveis, Relatório conferido e plano concluído antes de terminar.')}</p>`));
    el.append(setRow('Impor as convenções do projeto', 'Recusa escrita que quebra o estilo contado no código (indentação, aspas, ponto e vírgula, módulos, nomes de arquivo).', toggleEl(s.enforceConventions !== false, v => save({ enforceConventions: v }))));
    el.append(setRow('Revisão independente obrigatória', 'Ao fim de cada turno com alterações, um revisor com contexto limpo lê o diff; o agente precisa tratar cada achado antes de terminar.', toggleEl(s.requireReview !== false, v => save({ requireReview: v }))));
    el.append(setRow('Tamanho máximo de arquivo de código', 'Escrita que passa deste número de linhas é recusada (-1 desliga).', inputEl(s.maxFileLines || 400, v => save({ maxFileLines: v }), 'number')));
    el.append(setRow('Diagnóstico após cada edição', 'Cada arquivo que o agente escreve é conferido pelo verificador do projeto (node --check, tsc, go vet, py_compile, JSON); problemas voltam para ele na hora.', toggleEl(s.diagnostics !== false, v => save({ diagnostics: v }))));
    el.append(setRow('Sugestão da próxima mensagem', 'Depois de cada resposta, uma sugestão em cinza na caixa de mensagem; Tab ou → aceita. Usa o modelo rápido (custo mínimo).', toggleEl(s.promptSuggestions !== false, v => save({ promptSuggestions: v }))));
    el.append(setRow('Notificações', 'Avisar quando o agente precisar de você ou terminar, se você não estiver olhando a sessão', toggleEl(s.notify !== false, v => save({ notify: v }))));
    el.append(setRow('Ícone na bandeja', 'Mostrar o infuse na bandeja do sistema (Abrir / Sair)', toggleEl(!s.hideTray, v => save({ hideTray: !v }))));
    el.append(setRow('Fechar mantém na bandeja', 'Fechar a janela deixa as sessões rodando em segundo plano', toggleEl(!!s.closeToTray, v => save({ closeToTray: v }))));
    el.append(setRow('Modo vim na caixa de texto', 'Esc entra no modo NORMAL', toggleEl(!!s.vim, v => save({ vim: v }).then(() => S.views.forEach(renderComposer)))));
  }
  if (section === 'providers') {
    el.append(h(`<h1>${t('Provedores')}</h1>`), h(`<p class="lead">${t('Conecte um ou mais provedores de IA. As chaves ficam no cofre do sistema (Gerenciador de Credenciais do Windows, Keychain, libsecret), nunca em texto puro.')}</p>`));
    const conns = await api('/api/connections', { root: '' });
    const list = h('<div></div>'); el.append(list);
    const draw = (rows) => {
      list.innerHTML = '';
      rows.forEach(p => {
        const r = h(`<div class="prov"><div class="top"><span class="nm">${esc(p.label)}</span>${p.connected ? `<span class="tag ok">${t('conectado')}</span>${p.keyHint ? `<span class="faint mono">${esc(p.keyHint)}</span>` : ''}${p.default ? `<span class="tag">${t('padrão')}</span>` : ''}` : `<span class="faint">${p.local ? t('local, sem chave') : ''}</span>`}<span class="grow"></span>${p.connected ? `<button class="btn sm mk">${t('Tornar padrão')}</button><button class="btn sm ed">${t('Editar')}</button><button class="btn sm danger dc">${t('Desconectar')}</button>` : `<button class="btn sm line cn">${t('Conectar')}</button>`}</div></div>`);
        const expand = () => {
          if ($('.ex', r)) { $('.ex', r).remove(); return; }
          const ex = h(`<div class="ex">${p.needKey || p.id === 'custom' ? `<div class="f"><input class="key" type="password" placeholder="${esc(p.connected ? t('nova chave (deixe vazio para manter)') : t('cole a chave de API'))}">${p.keyUrl ? `<a href="${esc(p.keyUrl)}" target="_blank" class="faint">${t('obter chave')}</a>` : ''}</div>` : ''}
            <div class="f"><input class="base" placeholder="${esc(p.defaultBaseUrl || 'https://…')}" value="${esc(p.baseUrl !== p.defaultBaseUrl ? p.baseUrl : '')}"><span class="faint">${t('endpoint')}</span></div>
            <div class="row"><button class="btn sm pri go">${t('Testar e salvar')}</button><span class="faint res"></span></div>${p.models?.length ? `<div class="mlist">${p.models.slice(0, 60).map(m => `<span>${esc(m)}</span>`).join('')}</div>` : ''}</div>`);
          r.appendChild(ex); $('input', ex)?.focus();
          const go = async () => { const res = $('.res', ex); res.textContent = t('testando…'); const out = await api('/api/connections/connect', { body: { id: p.id, apiKey: $('.key', ex)?.value || '', baseUrl: $('.base', ex).value.trim() }, root: '' }); if (!out.ok) { res.innerHTML = `<span class="del">${esc(out.error)}</span>`; return; } toast(t('Conectado: ') + p.label + ` · ${out.connection.models?.length || 0} ${t('modelos')}`); S.models = await api('/api/models', { root: '' }); await loadSettings(); draw(await api('/api/connections', { root: '' })); S.views.forEach(renderComposer); };
          $('.go', ex).onclick = go; $$('input', ex).forEach(i => i.onkeydown = e => { if (e.key === 'Enter') go(); });
        };
        $('.cn', r) && ($('.cn', r).onclick = expand); $('.ed', r) && ($('.ed', r).onclick = expand);
        $('.dc', r) && ($('.dc', r).onclick = async () => { await api('/api/connections/disconnect', { body: { id: p.id }, root: '' }); S.models = await api('/api/models', { root: '' }); await loadSettings(); draw(await api('/api/connections', { root: '' })); });
        $('.mk', r) && ($('.mk', r).onclick = async () => { await api('/api/connections/default', { body: { id: p.id, model: p.models?.[0] || '' }, root: '' }); await loadSettings(); draw(await api('/api/connections', { root: '' })); });
        list.appendChild(r);
      });
    };
    draw(conns);
  }
  if (section === 'models') {
    el.append(h(`<h1>${t('Modelos e custos')}</h1>`));
    if (!S.models.length) S.models = await api('/api/models', { root: '' });
    const opts = S.models.map(m => [m.provider + '|' + m.model, `${m.label} · ${m.model}`]);
    el.append(setRow('Modelo padrão', 'Usado em novas sessões', selectEl(opts.length ? opts : [['', '— conecte um provedor —']], s.provider + '|' + s.model, v => { const [p, m] = v.split('|'); save({ provider: p, model: m }); })));
    el.append(setRow('Modelo rápido', 'Usado para resumir e compactar o contexto', selectEl(opts.length ? opts : [['', '—']], s.provider + '|' + s.fastModel, v => save({ fastModel: v.split('|')[1] }))));
    el.append(setRow('Esforço padrão', 'Raciocínio estendido em novas sessões', selectEl([['low', 'Baixo'], ['medium', 'Médio'], ['high', 'Alto'], ['max', 'Máximo']], s.effort || 'medium', v => save({ effort: v }))));
    el.append(setRow('Máximo de tokens de saída', 'Por resposta do modelo', inputEl(s.maxTokens, v => save({ maxTokens: v }), 'number')));
    el.append(setRow('Limite de contexto', 'Em branco ou 0: a janela do próprio modelo (1 milhão de tokens nos modelos Claude 5 e DeepSeek V4). Um número menor limita o contexto. A conversa é compactada em 75% do limite.', inputEl(s.contextLimit || '', v => save({ contextLimit: +v || 0 }), 'number')));
    el.append(setRow('Limite de gasto diário (US$)', 'Soma de todas as sessões no dia; o agente para ao ultrapassar; 0 desliga', inputEl(s.dailyLimitUsd, v => save({ dailyLimitUsd: v }), 'number')));
    const u = await api('/api/usage', { root: '' });
    el.append(h(`<h2>${t('Gasto')}</h2>`), h(`<div class="row" style="gap:36px;margin-bottom:8px"><div><div class="faint">${t('Hoje')}</div><div class="big">${money(u.today)}</div></div><div><div class="faint">${t('Este mês')}</div><div class="big">${money(u.month)}</div></div><div><div class="faint">${t('Tokens')}</div><div class="big">${ktok(u.tokens?.input)} / ${ktok(u.tokens?.output)}</div></div></div>`));
    const proj = Object.entries(u.byProject || {}).sort((a, b) => b[1] - a[1]);
    if (proj.length) el.append(h(`<table class="tbl"><tr><th>${t('Projeto')}</th><th>${t('Total')}</th></tr>${proj.map(([r, c]) => `<tr><td>${esc(projectLabel(r))} <span class="faint">${esc(r)}</span></td><td class="mono">${money(c)}</td></tr>`).join('')}</table>`));
    if ((u.byDay || []).length) el.append(h(`<table class="tbl" style="margin-top:12px"><tr><th>${t('Dia')}</th><th>${t('Total')}</th></tr>${u.byDay.slice(0, 14).map(d => `<tr><td>${esc(d.day)}</td><td class="mono">${money(d.cost)}</td></tr>`).join('')}</table>`));
    const sess = S.sessions.filter(x => x.cost > 0).sort((a, b) => b.cost - a.cost).slice(0, 10);
    if (sess.length) el.append(h(`<table class="tbl" style="margin-top:12px"><tr><th>${t('Sessão')}</th><th>${t('Total')}</th></tr>${sess.map(x => `<tr><td>${esc(x.title)} <span class="faint">${esc(projectLabel(x.root))}</span></td><td class="mono">${money(x.cost)}</td></tr>`).join('')}</table>`));
    el.append(h(`<h2>${t('Tabela de preços')}</h2>`), h(`<p class="lead">${t('US$ por milhão de tokens, por prefixo de modelo. São estimativas: confira a página de preços do provedor.')}</p>`));
    const tbl = h(`<table class="tbl"><tr><th>${t('Prefixo do modelo')}</th><th>${t('Entrada')}</th><th>${t('Saída')}</th><th></th></tr></table>`);
    const savePrices = () => { const pricing = {}; $$('tr[data-k]', tbl).forEach(tr => pricing[tr.dataset.k] = [+$('[data-i="0"]', tr).value, +$('[data-i="1"]', tr).value]); return save({ pricing }); };
    Object.entries(s.pricing || {}).sort().forEach(([k, v]) => { const tr = h(`<tr data-k="${esc(k)}"><td class="mono">${esc(k)}</td><td><input data-i="0" value="${v[0]}"></td><td><input data-i="1" value="${v[1]}"></td><td><button class="ib" style="width:22px;height:22px">${I.close}</button></td></tr>`); $$('input', tr).forEach(i => i.onchange = savePrices); $('button', tr).onclick = () => { tr.remove(); savePrices(); }; tbl.appendChild(tr); });
    const add = h(`<tr><td><input class="np" placeholder="${esc(t('novo prefixo'))}" style="width:100%"></td><td><input class="ni" placeholder="0"></td><td><input class="no" placeholder="0"></td><td></td></tr>`);
    $$('input', add).forEach(i => i.onkeydown = async e => { if (e.key === 'Enter' && $('.np', add).value.trim()) { await save({ pricing: { ...s.pricing, [$('.np', add).value.trim()]: [+$('.ni', add).value || 0, +$('.no', add).value || 0] } }); openSettings('models'); } });
    tbl.appendChild(add); el.append(tbl);
  }
  if (section === 'permissions') {
    el.append(h(`<h1>${t('Permissões')}</h1>`), h(`<p class="lead">${t('O modo escolhido na caixa de texto vale para a sessão e fica lembrado por pasta. Aqui ficam os padrões.')}</p>`));
    el.append(setRow('Modo padrão', 'Para pastas sem modo lembrado', selectEl(Object.entries(modeInfo).map(([k, [l]]) => [k, l]), s.mode || 'default', v => save({ mode: v }))));
    for (const tool of ['Bash', 'Write', 'Edit', 'WebFetch']) el.append(setRow(tool, { Bash: 'Executar comandos', Write: 'Criar arquivos', Edit: 'Editar arquivos', WebFetch: 'Acessar a web' }[tool], selectEl([['ask', 'Perguntar'], ['allow', 'Permitir'], ['deny', 'Negar']], (s.permissions || {})[tool] || 'ask', v => save({ permissions: { ...s.permissions, [tool]: v } }))));
    el.append(setRow('Caminhos protegidos', 'Globs separados por vírgula. .infuse/oracle/** é sempre protegido, por qualquer ferramenta', inputEl((s.protectedPaths || []).join(', '), v => save({ protectedPaths: v.split(',').map(x => x.trim()).filter(Boolean) }))));
  }
  if (section === 'code') {
    el.append(h(`<h1>${t('Sessões e worktrees')}</h1>`));
    el.append(setRow('Local dos worktrees', 'Relativo à pasta do projeto', inputEl(s.worktreeDir, v => save({ worktreeDir: v }))));
    el.append(setRow('Prefixo dos branches', 'Branches criados para sessões com worktree', inputEl(s.branchPrefix, v => save({ branchPrefix: v }))));
  }
  if (section === 'apps') {
    el.append(h(`<h1>${t('Aplicativos')}</h1>`), h(`<p class="lead">${t('Outros agentes encontrados nesta máquina. CLIs abrem num terminal na pasta do projeto selecionado.')}</p>`));
    const apps = await api('/api/apps', { root: curRoot() });
    apps.forEach(a => { const r = h(`<div class="prov"><div class="top"><span class="nm">${esc(a.name)}</span><span class="tag ${a.found ? 'ok' : ''}">${t(a.found ? 'encontrado' : 'não encontrado')}</span><span class="faint mono trunc grow">${esc(a.appId || a.path || '')}</span>${a.found ? `<button class="btn sm line">${t('Abrir')}</button>` : a.kind === 'cli' ? `<button class="btn sm line">${t('Instalar')}</button>` : `<button class="btn sm line" disabled>${t('Abrir')}</button>`}</div></div>`); $('button', r).onclick = async () => { if (a.kind === 'cli') return runAppInTerminal(a.id, !a.found); const res = await api('/api/apps/open', { body: { id: a.id }, root: curRoot() }); toast(res.ok ? 'Abrindo…' : res.error); }; el.append(r); });
    const eds = await api('/api/editors', { root: '' });
    el.append(h(`<h2>${t('Editores')}</h2>`), h(`<p class="lead">${eds.length ? eds.map(e => esc(e.name)).join(', ') : t('Nenhum editor encontrado no PATH.')} — ${t('usados em "Abrir em".')}</p>`));
  }
  if (section === 'integration') {
    const st = S.shell || {};
    el.append(h(`<h1>${t('Outros agentes')}</h1>`), h(`<p class="lead">${t('O shell interposto faz o Claude Code (e outros agentes) passarem pelo mesmo filtro quando usados fora do infuse.')}</p>`));
    const ctl = h(`<span class="row"><span class="tag ${st.shimInstalled ? 'ok' : ''}">${t(st.shimInstalled ? 'instalado' : 'não instalado')}</span><button class="btn sm line in">${t('Instalar')}</button><button class="btn sm danger un">${t('Remover')}</button></span>`);
    $('.in', ctl).onclick = async () => { const r = await api('/api/install', { body: {}, root: '' }); toast(r.error || 'Instalado — reinicie o Claude Code'); openSettings('integration'); };
    $('.un', ctl).onclick = async () => { await api('/api/uninstall', { body: {}, root: '' }); toast('Removido'); openSettings('integration'); };
    el.append(setRow('Shell interposto', st.shim || '', ctl));
  }
  if (section === 'about') el.append(h(`<h1>infuse</h1>`), h(`<p class="lead">${t('Versão')} ${esc(S.version || '')}</p>`), h(`<p class="faint">${t('Atalhos de teclado')}: <span class="kbd">Ctrl /</span></p>`));
}
