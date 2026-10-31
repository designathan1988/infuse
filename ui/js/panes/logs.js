'use strict';
// logs
// Journal, vices and project map panes.

function mountRules(pb, extra, c) {
  const draw = async () => {
    const scope = lsGet('rulesScope', 'project'); const r = await api(`/api/rules?scope=${scope}`, { root: c.root }); const st = { rules: r.rules || [], disable: r.disable || [] };
    const actOf = x => x.replace != null ? 'replace' : x.say ? 'say' : x.infuse ? 'infuse' : x.infuseFile ? 'infuseFile' : x.vice ? 'vice' : 'say';
    pb.innerHTML = `<div class="plist"><div class="row u-mb8px"><span class="chip sc">${t(scope === 'project' ? 'este projeto' : 'todos os projetos')} ${I.caret}</span><span class="faint mono trunc grow">${esc(r.file)}</span></div>
      <label class="row u-mb8px-curpointer"><span class="sw ${st.disable.includes('heredoc') ? 'on' : ''} hd"></span>${t('ui.pane.logs.desligar-a-transformacao-de.d6df19')}</label>
      <div class="rule faint u-fs105-tt-uppercase"><span></span><span>${t('ui.pane.logs.quando.a484f2')}</span><span>${t('ui.pane.logs.casa-com-regex.fc35f6')}</span><span>${t('ui.pane.logs.acao.64b518')}</span><span>${t('ui.pane.logs.valor.0a3c0d')}</span><span></span></div><div class="rows"></div>
      <button class="btn sm add u-mt6px">${I.plus} ${t('ui.pane.logs.regra.b75728')}</button>
      <div class="u-mt16px faint">${t('ui.pane.logs.testar-um-comando.37df75')}</div><input class="tc mono logs-input" placeholder="cat > a.ts <<EOF …"><pre class="tout hidden logs-out"></pre></div>`;
    const rows = $('.rows', pb);
    const save = debounce(async () => { const res = await api(`/api/rules?scope=${scope}`, { method: 'PUT', body: { rules: st.rules, disable: st.disable }, root: c.root }); if (res.error) toast(res.error); }, 400);
    const drawRows = () => { rows.innerHTML = ''; st.rules.forEach((x, i) => { const row = h(`<div class="rule"><span class="sw ${x.enabled === false ? '' : 'on'}" data-k="en"></span><select data-k="when">${['command', 'write', 'list', 'always'].map(w => `<option ${x.when === w ? 'selected' : ''}>${w}</option>`).join('')}</select><input data-k="match" class="mono" value="${esc(x.match || '')}"><select data-k="act">${['say', 'infuse', 'infuseFile', 'replace', 'vice'].map(a => `<option ${actOf(x) === a ? 'selected' : ''}>${a}</option>`).join('')}</select><input data-k="val" value="${esc(x.replace ?? x.say ?? x.infuse ?? x.infuseFile ?? x.vice ?? '')}"><button class="ib u-w22px-h22px" data-k="del">${I.close}</button></div>`);
      row.oninput = row.onchange = () => { x.when = $('[data-k=when]', row).value; x.match = $('[data-k=match]', row).value; const act = $('[data-k=act]', row).value, val = $('[data-k=val]', row).value; delete x.replace; delete x.say; delete x.infuse; delete x.infuseFile; delete x.vice; x[act] = val; save(); };
      $('[data-k=en]', row).onclick = e => { x.enabled = x.enabled === false; e.target.classList.toggle('on', x.enabled !== false); save(); };
      $('[data-k=del]', row).onclick = () => { st.rules.splice(i, 1); drawRows(); save(); }; rows.appendChild(row); }); };
    drawRows();
    $('.add', pb).onclick = () => { st.rules.push({ id: 'r' + Date.now(), when: 'command', match: '', say: '' }); drawRows(); save(); };
    $('.hd', pb).onclick = e => { const on = !e.target.classList.contains('on'); e.target.classList.toggle('on', on); st.disable = on ? ['heredoc'] : []; save(); };
    $('.sc', pb).onclick = e => menu(e.currentTarget, [{ label: 'este projeto', check: scope === 'project', f: () => { lsSet('rulesScope', 'project'); draw(); } }, { label: 'todos os projetos', check: scope === 'global', f: () => { lsSet('rulesScope', 'global'); draw(); } }]);
    $('.tc', pb).onkeydown = async e => { if (e.key !== 'Enter') return; const res = await api('/api/rules/test', { body: { command: e.target.value }, root: c.root }); const o = $('.tout', pb); o.classList.remove('hidden'); o.textContent = `${t('ui.pane.logs.comando-final.c9fbd5')}:\n${res.finalCommand}\n\n${t('ui.pane.logs.o-agente-recebe.e8b79b')}:\n${(res.lines || []).join('\n') || t('ui.pane.logs.apenas-a-saida-normal.1fa6d9')}`; };
  };
  draw(); return {};
}
function mountJournal(pb, extra, c) { const draw = async () => { const l = await api('/api/journal', { root: c.root }); pb.innerHTML = `<div class="plist">${l.length ? `<table><tr><th>${t('ui.pane.logs.quando.a484f2')}</th><th>${t('ui.pane.logs.tipo.db78ff')}</th><th>${t('ui.pane.logs.o-que.d21346')}</th></tr>${l.map(e => `<tr><td class="faint u-wsnowrap">${new Date(e.ts).toLocaleString()}</td><td><span class="tag ${e.kind === 'blocked' ? 'bad' : ''}">${esc(e.kind)}</span></td><td class="mono sel u-wbbreak-all">${esc(e.path || e.command || '')}${e.detail ? ` <span class="faint">${esc(e.detail)}</span>` : ''}</td></tr>`).join('')}</table>` : `<div class="empty">${t('ui.pane.logs.nenhuma-escrita-passou-pelo.35b964')}</div>`}</div>`; }; draw(); return { refresh: debounce(draw, 800) }; }
function mountVices(pb, extra, c) { const draw = async () => { const l = await api('/api/lessons', { root: c.root }); pb.innerHTML = `<div class="plist">${l.length ? `<table><tr><th>${t('ui.pane.logs.quando.a484f2')}</th><th>${t('ui.pane.logs.vicio.376c10')}</th><th>${t('ui.pane.logs.detalhe.cac645')}</th></tr>${l.map(e => `<tr><td class="faint u-wsnowrap">${new Date(e.ts).toLocaleString()}</td><td><span class="tag warn">${esc(e.vice)}</span></td><td class="sel">${esc(e.detail)}</td></tr>`).join('')}</table>` : `<div class="empty">${t('ui.pane.logs.nenhum-vicio-observado.fb13b5')}</div>`}</div>`; }; draw(); return { refresh: draw }; }
function mountMap(pb, extra, c) {
  extra.innerHTML = `<button class="ib" title="${esc(t('ui.pane.logs.regerar.777df1'))}">${I.reload}</button>`;
  const draw = async (refresh) => { const m = await api('/api/map' + (refresh ? '?refresh=1' : ''), { root: c.dir }); const dirs = Object.entries(m.dirs || {}).sort(); pb.innerHTML = `<div class="plist"><div class="faint u-mb8px">${Object.keys(m.files || {}).length} ${t('ui.rail.arquivos.0dd06b')} · ${new Date(m.generatedAt).toLocaleString()}</div>${(m.hotspots || []).length ? `<div class="card u-mb10px"><b>${t('ui.pane.logs.arquivos-grandes-demais.ecc54b')}</b>${m.hotspots.map(x => `<div class="mono path" data-path="${esc(x.path)}">${esc(x.path)} — ${x.lines} ${t('ui.feed.linhas.2e043f')}</div>`).join('')}</div>` : ''}<table><tr><th>${t('ui.composer.pasta.46139b')}</th><th>${t('ui.rail.arquivos.0dd06b')}</th><th>${t('ui.feed.linhas.2e043f')}</th></tr>${dirs.map(([p, d]) => `<tr><td class="mono">${esc(p)}</td><td>${(d.files || []).length}</td><td>${d.lines || 0}</td></tr>`).join('')}</table></div>`; $$('[data-path]', pb).forEach(x => x.onclick = () => openFileInPane(x.dataset.path)); };
  $('button', extra).onclick = () => draw(true); draw(); return { refresh: debounce(() => draw(), 800) };
}

// Agent CLIs (Claude Code, Codex, Gemini) run in a terminal tab inside the window, in the session's folder.
