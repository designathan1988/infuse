'use strict';
// Full pages that replace the session area: Settings, Scheduled, Customize. Every control applies on change.

function showPage(name) {
  S.page = name; let pg = $('#page');
  if (!name) { if (pg) pg.remove(); $('#views').classList.remove('hidden'); $('#panes').classList.toggle('empty', !P2.open.length); renderSidebar(); return null; }
  if (!pg) { pg = h('<div id="page" class="page"></div>'); $('#main').prepend(pg); }
  pg.innerHTML = ''; $('#views').classList.add('hidden'); $('#panes').classList.add('empty'); renderSidebar();
  return pg;
}
function pageShell(title, sections, active, onSelect) {
  const pg = showPage(title);
  const nav = h(`<nav class="pnav"><div class="nav" style="margin-bottom:12px;color:var(--text3)">← ${t('Voltar')}</div></nav>`);
  nav.firstChild.onclick = () => showPage(null);
  sections.forEach(([id, label]) => { const n = h(`<div class="nav${id === active ? ' on' : ''}">${esc(t(label))}</div>`); n.onclick = () => onSelect(id); nav.appendChild(n); });
  const content = h('<div class="pcontent"><div class="inner"></div></div>');
  pg.append(nav, content); return $('.inner', content);
}
const setRow = (label, desc, control) => { const r = h(`<div class="set"><div class="l"><b>${esc(t(label))}</b>${desc ? `<span>${esc(t(desc))}</span>` : ''}</div></div>`); r.appendChild(control); return r; };
const toggleEl = (on, f) => { const s = h(`<span class="sw${on ? ' on' : ''}" style="cursor:pointer"></span>`); s.onclick = () => { s.classList.toggle('on'); f(s.classList.contains('on')); }; return s; };
const selectEl = (opts, val, f) => { const s = h(`<select>${opts.map(([v, l]) => `<option value="${esc(v)}" ${v === val ? 'selected' : ''}>${esc(t(l))}</option>`).join('')}</select>`); s.onchange = () => f(s.value); return s; };
const inputEl = (val, f, type = 'text', ph = '') => { const i = h(`<input type="${type}" value="${esc(val ?? '')}" placeholder="${esc(t(ph))}">`); i.onchange = () => f(type === 'number' ? Number(i.value) : i.value); return i; };

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
    el.append(setRow('Mostrar raciocínio na conversa', 'Blocos de raciocínio abertos por padrão; o caret recolhe cada um', toggleEl(s.showThinking !== false, v => save({ showThinking: v }))));
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
    el.append(setRow('Limite de contexto', 'Tokens; a conversa é compactada em 75%', inputEl(s.contextLimit, v => save({ contextLimit: v }), 'number')));
    el.append(setRow('Limite de gasto por sessão (US$)', 'O agente para ao ultrapassar; 0 desliga', inputEl(s.dailyLimitUsd, v => save({ dailyLimitUsd: v }), 'number')));
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

// ---------------------------------------------------------------- Scheduled tasks
async function openScheduled(name) {
  const pg = showPage('scheduled'); const content = h('<div class="pcontent" style="padding-left:48px"><div class="inner"></div></div>'); pg.appendChild(content); const el = $('.inner', content);
  const tasks = await api('/api/tasks', { root: '' });
  if (name === undefined) {
    el.append(h(`<div class="row" style="margin-bottom:6px"><h1 class="grow">${t('Agendadas')}</h1><button class="btn line nt">${I.plus} ${t('Nova tarefa')}</button></div>`), h(`<p class="lead">${t('Uma tarefa abre uma sessão nova na pasta escolhida, no horário definido, enquanto o infuse estiver aberto. Execuções perdidas são recuperadas uma vez (até 7 dias).')}</p>`));
    $('.nt', el).onclick = () => openScheduled(null);
    if (!tasks.length) el.append(h(`<div class="empty">${t('Nenhuma tarefa agendada.')}</div>`));
    tasks.forEach(tk => { const r = h(`<div class="taskrow"><span class="st ${tk.active ? 'busy' : ''}" style="animation:none"></span><div class="grow"><b>${esc(tk.name)}</b><div class="faint trunc">${esc(presetText(tk))} · ${esc(projectLabel(tk.root))}</div></div><span class="faint">${tk.nextRun ? t('próxima: ') + new Date(tk.nextRun).toLocaleString() : t(tk.active ? 'manual' : 'pausada')}</span></div>`); r.onclick = () => openScheduled(tk.name); el.append(r); });
    return;
  }
  const tk = tasks.find(x => x.name === name) || { name: '', prompt: '', root: curRoot() || S.projects[0]?.root || '', preset: 'daily', hour: 9, minute: 0, weekday: 1, active: true, history: [], alwaysAllowed: [] };
  const prev = tk.name;
  el.append(h(`<div class="nav" style="margin:0 0 12px -10px;color:var(--text3)">← ${t('Agendadas')}</div>`)); el.lastChild.onclick = () => openScheduled();
  const doc = h(`<div class="editdoc"><input class="name" placeholder="${esc(t('nome-da-tarefa'))}" value="${esc(tk.name)}"><textarea placeholder="${esc(t('Instruções: o que o agente deve fazer a cada execução'))}">${esc(tk.prompt)}</textarea><div class="chips"></div></div>`);
  el.append(doc); const chips = $('.chips', doc);
  const drawChips = () => {
    chips.innerHTML = '';
    const c1 = h(`<span class="chip">${I.folder}<span class="v">${esc(tk.root ? projectLabel(tk.root) : t('Escolher pasta'))}</span>${I.caret}</span>`); c1.onclick = e => menu(e.currentTarget, S.projects.map(p => ({ label: p.label || baseName(p.root), d: p.root, check: p.root === tk.root, f: () => { tk.root = p.root; drawChips(); } }))); chips.appendChild(c1);
    const c2 = h(`<span class="chip"><span class="v">${esc(presetText(tk))}</span>${I.caret}</span>`); c2.onclick = e => menu(e.currentTarget, [['manual', 'Manual'], ['hourly', 'A cada hora'], ['daily', 'Diariamente'], ['weekdays', 'Dias úteis'], ['weekly', 'Semanalmente']].map(([k, l]) => ({ label: l, check: tk.preset === k, f: () => { tk.preset = k; drawChips(); } }))); chips.appendChild(c2);
    if (tk.preset !== 'manual') { const tm = h(`<input type="time" value="${String(tk.hour).padStart(2, '0')}:${String(tk.minute).padStart(2, '0')}" style="background:var(--s2);border-radius:7px;padding:3px 8px;font-size:12px">`); tm.onchange = () => { const [hh, mm] = tm.value.split(':'); tk.hour = +hh; tk.minute = +mm; }; chips.appendChild(tm); }
    if (tk.preset === 'weekly') { const wd = selectEl([0, 1, 2, 3, 4, 5, 6].map(i => [String(i), ['Domingo', 'Segunda', 'Terça', 'Quarta', 'Quinta', 'Sexta', 'Sábado'][i]]), String(tk.weekday), v => tk.weekday = +v); wd.style.cssText = 'background:var(--s2);border-radius:7px;padding:3px 8px;font-size:12px'; chips.appendChild(wd); }
    const c3 = h(`<span class="chip mode-${tk.mode || 'default'}"><span class="v">${t(modeInfo[tk.mode || 'acceptEdits']?.[0])}</span>${I.caret}</span>`); c3.onclick = e => menu(e.currentTarget, Object.entries(modeInfo).map(([k, [l]]) => ({ label: l, check: (tk.mode || 'acceptEdits') === k, f: () => { tk.mode = k; drawChips(); } }))); chips.appendChild(c3);
    const c4 = h(`<span class="chip"><span class="v">${esc(tk.model || S.settings.model || t('modelo padrão'))}</span>${I.caret}</span>`); c4.onclick = e => menu(e.currentTarget, [{ label: 'modelo padrão', check: !tk.model, f: () => { tk.model = ''; tk.provider = ''; drawChips(); } }, ...S.models.slice(0, 40).map(m => ({ label: m.model, d: m.label, check: tk.model === m.model, f: () => { tk.model = m.model; tk.provider = m.provider; drawChips(); } }))]); chips.appendChild(c4);
    const c5 = h(`<span class="chip"><span class="sw${tk.worktree ? ' on' : ''}"></span>${t('Worktree')}</span>`); c5.onclick = () => { tk.worktree = !tk.worktree; drawChips(); }; chips.appendChild(c5);
    const c6 = h(`<span class="chip"><span class="sw${tk.active ? ' on' : ''}"></span>${t(tk.active ? 'Ativa' : 'Pausada')}</span>`); c6.onclick = () => { tk.active = !tk.active; drawChips(); }; chips.appendChild(c6);
  };
  drawChips();
  const actions = h(`<div class="row" style="margin-top:14px"><button class="btn pri sv">${t(prev ? 'Salvar alterações' : 'Criar tarefa')}</button>${prev ? `<button class="btn line rn">${t('Executar agora')}</button><span class="grow"></span><button class="btn danger dl">${t('Excluir')}</button>` : ''}</div>`);
  el.append(actions);
  $('.sv', actions).onclick = async () => { tk.name = $('.name', doc).value; tk.prompt = $('textarea', doc).value; const r = await api('/api/tasks', { method: 'PUT', body: { ...tk, previousName: prev }, root: '' }); if (r.error) return toast(r.error); toast('Tarefa salva'); openScheduled(r.name); };
  $('.rn', actions) && ($('.rn', actions).onclick = async () => { const r = await api('/api/tasks/run', { body: { name: prev }, root: '' }); if (r.error) return toast(r.error); await loadSessions(); openSession(r.root, r.session); });
  $('.dl', actions) && ($('.dl', actions).onclick = async () => { if (!confirm(t('Excluir a tarefa?'))) return; await api('/api/tasks/delete', { body: { name: prev }, root: '' }); openScheduled(); });
  if (prev) {
    el.append(h(`<h2>${t('Histórico')}</h2>`));
    if (!tk.history?.length) el.append(h(`<div class="faint">${t('Ainda não executou.')}</div>`));
    (tk.history || []).forEach(r => { const row = h(`<div class="taskrow" style="padding:8px 10px"><span class="tag ${r.status === 'started' ? 'ok' : r.status === 'failed' ? 'bad' : 'warn'}">${t(r.status === 'started' ? 'executou' : r.status === 'failed' ? 'falhou' : 'pulada')}</span><span class="grow">${new Date(r.ts).toLocaleString()}</span><span class="faint">${esc(r.note || '')}</span></div>`); if (r.session) row.onclick = () => openSession(tk.root, r.session); el.append(row); });
  }
}
function presetText(tk) {
  const hm = `${String(tk.hour).padStart(2, '0')}:${String(tk.minute).padStart(2, '0')}`;
  return { manual: t('Manual'), hourly: t('A cada hora, no minuto ') + tk.minute, daily: t('Diariamente às ') + hm, weekdays: t('Dias úteis às ') + hm, weekly: t(['Domingo', 'Segunda', 'Terça', 'Quarta', 'Quinta', 'Sexta', 'Sábado'][tk.weekday || 0]) + ' ' + hm }[tk.preset] || tk.preset;
}

// ---------------------------------------------------------------- Customize: commands, instructions
async function openCustomize(section = 'commands') {
  const el = pageShell('customize', [['commands', 'Comandos de barra'], ['instructions', 'Instruções do projeto']], section, openCustomize);
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
  if (section === 'instructions') {
    el.append(h(`<h1>${t('Instruções do projeto')}</h1>`), h(`<p class="lead">${t('Memória do projeto (.infuse/memory.md), lida pelo agente no início de cada turno. Arquivos CLAUDE.md, AGENTS.md e INFUSE.md da pasta também são lidos.')}</p>`));
    const r = await api('/api/memory', { root });
    const ta = h(`<textarea class="doc" style="min-height:320px">${esc(r.content || '')}</textarea>`); const st = h('<span class="faint"></span>');
    ta.oninput = debounce(async () => { await api('/api/memory', { method: 'PUT', body: { content: ta.value }, root }); st.textContent = t('salvo'); setTimeout(() => st.textContent = '', 1200); }, 600);
    el.append(ta, st);
    if (r.instructions) el.append(h(`<h2>${t('Instruções encontradas na pasta')}</h2>`), h(`<pre class="doc sel" style="white-space:pre-wrap;font-family:var(--mono);font-size:12px;background:var(--s2);border-radius:8px;padding:10px 12px;max-height:360px;overflow:auto">${esc(r.instructions)}</pre>`));
  }
}
