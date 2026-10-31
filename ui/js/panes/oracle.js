'use strict';
// oracle
// Done criteria, execution and verdicts.

// ---------------------------------------------------------------- infuse: oracle, rules, journal, vices, map
// Pronto: "Pronto?" runs the criteria in a frozen copy of the project and shows what passed. Criteria
// read as sentences with the values in place; the project's own scripts become one-click suggestions.
const CRIT_KINDS = {
  command: { name: 'Um comando termina sem erro', ex: 'npm test · go test ./... · python -m pytest', help: 'Roda o comando numa cópia congelada do projeto. Passa quando o comando termina sem erro (código de saída 0). Pode exigir também um texto na saída, como "passed".', make: () => ({ kind: 'command', run: '', expect: { exit: 0 } }), title: 'Os testes passam' },
  http: { name: 'Uma página ou API responde', ex: 'http://localhost:3000/login responde 200 e mostra "Entrar"', help: 'Pede o endereço e confere a resposta: o código (200 = ok, 404 = não encontrado) e, se quiser, um texto que precisa aparecer. O app precisa estar rodando — inicie pelo Navegador › Servidores.', make: () => ({ kind: 'http', url: 'http://localhost:3000/', expect: { status: 200 } }), title: 'A página abre' },
  file: { name: 'Um arquivo existe ou contém um texto', ex: 'README.md contém "Instalação" · .env não existe', help: 'Confere um arquivo na cópia do projeto: que exista, que não exista (ex.: nenhum .env com senhas no repositório) ou que contenha um texto.', make: () => ({ kind: 'file', path: '', expect: {} }), title: 'O arquivo existe' },
  maxLines: { name: 'Nenhum arquivo passa de N linhas', ex: 'src/**/*.ts com no máximo 400 linhas', help: 'Percorre os arquivos que casam com o padrão e reprova os que passam do limite. No padrão, ** quer dizer "qualquer pasta" e * "qualquer nome": src/**/*.ts são todos os .ts dentro de src.', make: () => ({ kind: 'maxLines', glob: '**/*.ts', max: 400 }), title: 'Nenhum arquivo grande demais' },
};
const ORACLE_EXAMPLE = `# Pronto quando
- O login aceita e-mail e senha e mostra uma mensagem clara quando a senha está errada.
- Nenhuma tela quebra no celular (largura de 375px).
- Os testes que já existiam continuam passando.
`;
const oracleSpecs = {}; // root → the spec last loaded, for the titles in the verdict
const critTitle = (root, id) => (oracleSpecs[root]?.checks || []).find(k => k.id === id)?.description || id;
async function runOracleUI(c, out) {
  out.innerHTML = `<div class="vbox"><div class="status"><span class="spin"></span>${t('ui.pane.oracle.copiando-o-projeto-e.2bb12c')}</div></div>`;
  const L = curL();
  const r = await api('/api/oracle/run', { body: { session: L?.id || '' }, root: c.root, dir: c.dir });
  const v = r.verdict || {}; drawVerdict(c, out, v, r.error); if (L) { liveRT(L.id).verdict = v; viewsOf(L).forEach(renderRail); }
  out.dispatchEvent(new CustomEvent('verdict', { detail: v, bubbles: true }));
}
function drawVerdict(c, out, v, err) {
  if (!v || !v.run) { out.innerHTML = err ? `<div class="errbox">${esc(err)}</div>` : `<div class="vbox idle"><b>${t('ui.pane.oracle.ainda-nao-verificado.8172b9')}</b><span>${t('ui.pane.oracle.quando-quiser-conferir-o.5d8d7c')}</span></div>`; return; }
  const checks = v.checks || [], passed = checks.filter(k => k.passed).length;
  const head = { aprovado: [t('ui.rail.pronto.2726ba'), `${passed} ${t('ui.chat.de.600ccd')} ${checks.length} ${t('ui.pane.oracle.criterios-passaram.baefba')}`, 'ok'], reprovado: [t('ui.pane.oracle.ainda-nao.20d157'), `${checks.length - passed} ${t('ui.chat.de.600ccd')} ${checks.length} ${t('ui.pane.oracle.falharam-veja-abaixo-o.2ec889')}`, 'bad'], 'sem-criterios': [t('ui.pane.oracle.nada-para-verificar.68ee47'), t('ui.pane.oracle.adicione-pelo-menos-um.72ff23'), 'warn'], erro: [t('ui.pane.oracle.nao-foi-possivel-verificar.a21991'), v.summary || err || '', 'bad'] }[v.verdict] || [v.verdict, v.summary || '', ''];
  out.innerHTML = `<div class="vbox ${head[2]}"><b>${esc(head[0])}</b><span>${esc(head[1])}</span><span class="grow"></span><span class="faint" title="${esc(t('ui.pane.oracle.identificador-da-copia-congelada.97dc48'))}">${new Date(v.finished || v.started).toLocaleTimeString()}</span></div>
    ${checks.map(k => `<div class="vrow"><span class="${k.passed ? 'add' : 'del'}">${k.passed ? '✓' : '✗'}</span><span class="vt">${esc(critTitle(c.root, k.id))}</span><span class="faint grow trunc" title="${esc(k.detail)}">${esc(k.detail)}</span><span class="faint">${k.millis < 1000 ? k.millis + ' ms' : (k.millis / 1000).toFixed(1) + ' s'}</span>${k.evidence ? `<button class="btn sm" data-ev="${esc(k.evidence)}">${t('ui.pane.oracle.ver-saida.4aa800')}</button>` : ''}</div>`).join('')}
    <div class="evbox"></div>`;
  $$('[data-ev]', out).forEach(b => b.onclick = async () => { const box = $('.evbox', out); if (box.dataset.f === b.dataset.ev) { box.innerHTML = ''; box.dataset.f = ''; return; } const e = await api(`/api/oracle/evidence?run=${encodeURIComponent(v.run)}&file=${encodeURIComponent(b.dataset.ev)}`, { root: c.root }); box.dataset.f = b.dataset.ev; box.innerHTML = `<pre class="evpre sel">${esc(e.content || e.error || t('ui.pane.oracle.saida-vazia.a3a02a'))}</pre>`; });
}
function mountOracle(pb, extra, c) {
  extra.innerHTML = `<button class="ib help" title="${esc(t('ui.page.connectors.como-funciona.9b4488'))}">?</button>`;
  let spec = { checks: [] }, sugg = [], last = null, helpOpen = lsGet('oracleHelp', null);
  const save = debounce(() => api('/api/oracle/spec', { method: 'PUT', body: { spec }, root: c.root }).then(() => loadSugg()), 400);
  const loadSugg = async () => { sugg = await api('/api/oracle/suggest', { root: c.root }); if (!Array.isArray(sugg)) sugg = []; drawSugg(); };
  const uid = title => { const base = (title || 'criterio').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'criterio'; let id = base, n = 2; while (spec.checks.some(k => k.id === id)) id = base + '-' + n++; return id; };
  const draw = async () => {
    const [files, specR, hist] = await Promise.all([api('/api/oracle', { root: c.root }), api('/api/oracle/spec', { root: c.root }), api('/api/oracle/verdicts', { root: c.root })]);
    spec = specR.spec || { checks: [] }; spec.checks = spec.checks || []; oracleSpecs[c.root] = spec; last = hist[0];
    if (helpOpen === null) helpOpen = !spec.checks.length;
    pb.innerHTML = `<div class="plist oracle">
      <div class="ohead"><button class="btn pri run">${t('ui.rail.pronto.1cfd95')}</button><span class="grow">${t('ui.pane.oracle.confere-o-trabalho-por.fb55e6')}</span></div>
      <div class="howto" style="${helpOpen ? '' : 'display:none'}">
        <div class="row"><b class="grow">${t('ui.page.connectors.como-funciona.9b4488')}</b><button class="ib hx" title="${esc(t('ui.pane.oracle.fechar.5749fb'))}">${I.close}</button></div>
        <ol>
          <li><b>${t('ui.pane.oracle.diga-o-que-precisa.783226')}</b> ${t('ui.pane.oracle.quando-o-trabalho-estiver.8c7ff5')}</li>
          <li><b>${t('ui.pane.oracle.peca-o-trabalho-ao.c8c1a6')}</b> ${t('ui.pane.oracle.normalmente-na-conversa-ele.c84275')}</li>
          <li><b>${t('ui.pane.oracle.clique-em-pronto.fc48ba')}</b> ${t('ui.pane.oracle.o-infuse-copia-o.ffa450')}</li>
        </ol>
        <div class="kinds">${Object.entries(CRIT_KINDS).map(([k, x]) => `<div><b>${t(x.name)}</b><span class="faint">${t('ui.pane.oracle.ex.645d99')} ${esc(x.ex)}</span></div>`).join('')}</div>
      </div>
      <div class="verdict"></div>
      <div class="osect"><span class="grow">${t('ui.pane.oracle.o-que-precisa-ser.038b84')}</span><button class="btn sm addc">${I.plus} ${t('ui.pane.oracle.adicionar-criterio.430615')}</button></div>
      <div class="checks"></div>
      <div class="sugg"></div>
      <details class="odet"${spec.setup ? ' open' : ''}><summary>${t('ui.pane.oracle.antes-de-verificar-opcional.6e0abc')}</summary>
        <div class="cs">${t('ui.pane.oracle.rodar.d25c49')} <input class="slot mono setup" value="${esc(spec.setup || '')}" placeholder="npm ci" spellcheck="false"> ${t('ui.pane.oracle.na-copia-antes-dos.f0ad21')}</div>
        <div class="faint chelp">${t('ui.pane.oracle.normalmente-nao-e-preciso.38b2cb')}</div>
      </details>
      <details class="odet"${files.some(f => f.name !== 'checks.json' && f.content.trim()) ? ' open' : ''}><summary>${t('ui.pane.oracle.descricao-em-palavras-opcional.5bf940')}</summary>
        <div class="faint chelp">${t('ui.pane.oracle.o-agente-le-este.81b997')}</div>
        <div class="docs"></div>
      </details></div>`;
    $('.run', pb).onclick = () => runOracleUI(c, $('.verdict', pb));
    pb.addEventListener('verdict', e => { last = e.detail; drawChecks(); });
    $('.hx', pb).onclick = () => toggleHelp(false);
    $('.setup', pb).oninput = e => { spec.setup = e.target.value.trim(); save(); };
    $('.addc', pb).onclick = e => menu(e.currentTarget, [{ h: 'Que tipo de critério?' }, ...Object.entries(CRIT_KINDS).map(([k, x]) => ({ label: x.name, d: t('ui.pane.oracle.ex.645d99') + ' ' + x.ex, f: () => addCheck({ ...x.make(), description: t(x.title) }, true) }))], null, { alignRight: true });
    drawVerdict(c, $('.verdict', pb), last); drawChecks(); drawSugg(); drawDocs(files); loadSugg();
  };
  const toggleHelp = open => { helpOpen = open ?? !helpOpen; lsSet('oracleHelp', helpOpen); const hb = $('.howto', pb); if (hb) hb.style.display = helpOpen ? '' : 'none'; };
  $('.help', extra).onclick = () => toggleHelp();
  const addCheck = (k, focus) => { k.id = uid(k.description); spec.checks.push(k); save(); drawChecks(); if (focus) setTimeout(() => { const card = $$('.checks .crit', pb).pop(); (card && ($('.slot', card) || $('.ctitle', card)))?.focus(); }, 30); };
  const drawSugg = () => {
    const box = $('.sugg', pb); if (!box) return;
    if (!sugg.length) { box.innerHTML = ''; return; }
    box.innerHTML = `<div class="osect"><span class="grow">${t('ui.page.docs.sugestoes-para-este-projeto.c15490')}</span>${sugg.length > 1 ? `<button class="btn sm all">${t('ui.pane.oracle.adicionar-todas.e4a32e')}</button>` : ''}</div>${sugg.map((s, i) => `<div class="sug"><div class="grow"><b>${esc(t(s.title))}</b><div class="faint">${esc(s.why)}</div></div><button class="btn sm" data-i="${i}">${I.plus} ${t('ui.pane.oracle.adicionar.314398')}</button></div>`).join('')}`;
    const take = s => { if (s.setup && !spec.setup) { spec.setup = s.setup; const inp = $('.setup', pb); if (inp) inp.value = s.setup; } addCheck({ ...s.check, description: t(s.title) }); };
    $$('[data-i]', box).forEach(b => b.onclick = () => { take(sugg[+b.dataset.i]); sugg.splice(+b.dataset.i, 1); drawSugg(); });
    const all = $('.all', box); if (all) all.onclick = () => { sugg.forEach(take); sugg = []; drawSugg(); };
  };
  const slot = (f, val, ph, mono = true) => `<input class="slot${mono ? ' mono' : ''}" data-f="${f}" value="${esc(val ?? '')}" placeholder="${esc(ph)}" spellcheck="false">`;
  const sentence = k => {
    const x = k.expect || (k.expect = {});
    const extra = x.contains != null || k._more ? ` ${t(k.kind === 'command' ? 'e que a saída contenha' : 'e que a resposta contenha')} ${slot('contains', x.contains, t('ui.pane.oracle.um-texto.6cbc02'), false)}` : '';
    if (k.kind === 'command') return `${t('ui.pane.oracle.rodar.d25c49')} ${slot('run', k.run, 'npm test')} ${t('ui.pane.oracle.e-esperar-que-termine.3e415e')}${extra}`;
    if (k.kind === 'http') return `${t('ui.sidebar.abrir.50d3cb')} ${slot('url', k.url, 'http://localhost:3000/')} ${t('ui.pane.oracle.e-esperar-a-resposta.bd8e98')} ${slot('status', x.status || 200, '200')}${extra}`;
    if (k.kind === 'maxLines') return `${t('ui.pane.oracle.nenhum-arquivo.2e3404')} ${slot('glob', k.glob, '**/*.ts')} ${t('ui.pane.oracle.passa-de.19a7f8')} ${slot('max', k.max || 400, '400')} ${t('ui.feed.linhas.2e043f')}`;
    const mode = x.exists === false ? 'absent' : x.contains != null ? 'contains' : 'exists';
    return `${t('ui.pane.oracle.o-arquivo.92437a')} ${slot('path', k.path, 'README.md')} <span class="chip fmode">${t({ exists: 'existe', absent: 'não existe', contains: 'contém' }[mode])} ${I.caret}</span>${mode === 'contains' ? ' ' + slot('contains', x.contains, t('ui.pane.oracle.um-texto.6cbc02'), false) : ''}`;
  };
  const drawChecks = () => {
    const box = $('.checks', pb); if (!box) return;
    const res = {}; (last?.checks || []).forEach(r => res[r.id] = r);
    box.innerHTML = spec.checks.length ? '' : `<div class="oempty">${t('ui.pane.oracle.nenhum-criterio-ainda-adicione.4c1e6e')}</div>`;
    spec.checks.forEach((k, i) => {
      const r = res[k.id], kind = CRIT_KINDS[k.kind] || CRIT_KINDS.command;
      const card = h(`<div class="crit"><div class="ch"><span class="cst ${r ? (r.passed ? 'ok' : 'bad') : ''}" title="${esc(r ? r.detail : t('ui.pane.oracle.ainda-nao-verificado.8172b9-2'))}">${r ? (r.passed ? '✓' : '✗') : '○'}</span><input class="ctitle" value="${esc(k.description || k.id)}" placeholder="${esc(t('ui.pane.oracle.nome-do-criterio.1ab8fb'))}" spellcheck="false"><span class="faint ckind">${esc(t(kind.name))}</span><button class="ib q" title="${esc(t('ui.pane.oracle.o-que-isto-faz.697de9'))}">?</button><button class="ib more" title="${esc(t('ui.page.connectors.mais.4f728e'))}">⋯</button></div><div class="cs">${sentence(k)}</div><div class="chelp faint" style="display:none">${esc(t(kind.help))}</div>${r && !r.passed ? `<div class="cres">${esc(r.detail)}</div>` : ''}</div>`);
      $('.ctitle', card).oninput = e => { k.description = e.target.value; save(); };
      $$('.slot', card).forEach(inp => inp.oninput = () => {
        const f = inp.dataset.f, val = inp.value; k.expect = k.expect || {};
        if (f === 'contains') { if (val) k.expect.contains = val; else delete k.expect.contains; k._more = true; }
        else if (f === 'status') k.expect.status = parseInt(val) || 200;
        else if (f === 'max') k.max = parseInt(val) || 400;
        else k[f] = val.trim();
        save();
      });
      $('.q', card).onclick = () => { const hp = $('.chelp', card); hp.style.display = hp.style.display ? '' : 'none'; };
      const fm = $('.fmode', card);
      if (fm) fm.onclick = e => menu(e.currentTarget, [['exists', 'existe'], ['absent', 'não existe'], ['contains', 'contém um texto']].map(([m, l]) => ({ label: l, f: () => { k.expect = m === 'absent' ? { exists: false } : m === 'contains' ? { contains: '' } : {}; save(); drawChecks(); } })));
      $('.more', card).onclick = e => menu(e.currentTarget, [
        (k.kind === 'command' || k.kind === 'http') && k.expect?.contains == null && { label: k.kind === 'command' ? 'Exigir um texto na saída' : 'Exigir um texto na resposta', f: () => { k._more = true; k.expect.contains = ''; drawChecks(); } },
        { label: 'Duplicar', f: () => addCheck(JSON.parse(JSON.stringify({ ...k, description: (k.description || k.id) + ' ' + t('ui.pane.oracle.copia.ea88bc') }))) },
        { label: 'Explicar este tipo', f: () => $('.q', card).click() },
        { sep: 1 },
        { label: 'Remover', danger: 1, f: () => { spec.checks.splice(i, 1); save(); drawChecks(); } },
      ], null, { alignRight: true });
      box.appendChild(card);
    });
  };
  const drawDocs = files => {
    const docs = $('.docs', pb);
    const prose = files.filter(f => f.name !== 'checks.json');
    (prose.length ? prose : [{ name: 'pronto.md', content: '' }]).forEach(f => {
      const d = h(`<div style="margin-bottom:10px">${prose.length > 1 ? `<div class="faint mono" style="margin-bottom:4px">${esc(f.name)}</div>` : ''}<textarea class="doc" placeholder="${esc(t('ui.pane.oracle.ex-o-login-aceita.331782'))}">${esc(f.content)}</textarea><div class="row"><button class="btn sm ex">${t('ui.page.hooks.inserir-exemplo.61c3bf')}</button><span class="faint st"></span></div></div>`);
      const ta = $('textarea', d), st = $('.st', d);
      const put = async () => { await api('/api/oracle', { method: 'PUT', body: { name: f.name, content: ta.value }, root: c.root }); st.textContent = t('ui.page.customize.salvo.7af5c9'); setTimeout(() => st.textContent = '', 1200); };
      ta.oninput = debounce(put, 500);
      $('.ex', d).onclick = () => { ta.value = (ta.value.trim() ? ta.value.trimEnd() + '\n\n' : '') + t(ORACLE_EXAMPLE); put(); ta.focus(); };
      docs.appendChild(d);
    });
  };
  draw(); return { refresh: () => loadSugg() };
}
