'use strict';
// logs
// Journal, vices and project map panes.

function mountRules(pb, extra, c) {
  const draw = async () => {
    const scope = lsGet('rulesScope', 'project'); const r = await api(`/api/rules?scope=${scope}`, { root: c.root }); const st = { rules: r.rules || [], disable: r.disable || [] };
    const actOf = x => x.replace != null ? 'replace' : x.say ? 'say' : x.infuse ? 'infuse' : x.infuseFile ? 'infuseFile' : x.vice ? 'vice' : 'say';
    pb.innerHTML = `<div class="plist"><div class="row" style="margin-bottom:8px"><span class="chip sc">${t(scope === 'project' ? 'este projeto' : 'todos os projetos')} ${I.caret}</span><span class="faint mono trunc grow">${esc(r.file)}</span></div>
      <label class="row" style="margin-bottom:8px;cursor:pointer"><span class="sw ${st.disable.includes('heredoc') ? 'on' : ''} hd"></span>${t('desligar a transformação de heredoc')}</label>
      <div class="rule faint" style="font-size:10.5px;text-transform:uppercase"><span></span><span>${t('quando')}</span><span>${t('casa com (regex)')}</span><span>${t('ação')}</span><span>${t('valor')}</span><span></span></div><div class="rows"></div>
      <button class="btn sm add" style="margin-top:6px">${I.plus} ${t('regra')}</button>
      <div style="margin-top:16px" class="faint">${t('Testar um comando')}</div><input class="tc mono" style="width:100%;padding:6px 10px;border-radius:7px;background:var(--s2);margin-top:4px" placeholder="cat > a.ts <<EOF …"><pre class="tout" style="display:none;white-space:pre-wrap;font-family:var(--mono);font-size:12px;background:var(--s2);border-radius:7px;padding:8px;margin-top:6px"></pre></div>`;
    const rows = $('.rows', pb);
    const save = debounce(async () => { const res = await api(`/api/rules?scope=${scope}`, { method: 'PUT', body: { rules: st.rules, disable: st.disable }, root: c.root }); if (res.error) toast(res.error); }, 400);
    const drawRows = () => { rows.innerHTML = ''; st.rules.forEach((x, i) => { const row = h(`<div class="rule"><span class="sw ${x.enabled === false ? '' : 'on'}" data-k="en"></span><select data-k="when">${['command', 'write', 'list', 'always'].map(w => `<option ${x.when === w ? 'selected' : ''}>${w}</option>`).join('')}</select><input data-k="match" class="mono" value="${esc(x.match || '')}"><select data-k="act">${['say', 'infuse', 'infuseFile', 'replace', 'vice'].map(a => `<option ${actOf(x) === a ? 'selected' : ''}>${a}</option>`).join('')}</select><input data-k="val" value="${esc(x.replace ?? x.say ?? x.infuse ?? x.infuseFile ?? x.vice ?? '')}"><button class="ib" data-k="del" style="width:22px;height:22px">${I.close}</button></div>`);
      row.oninput = row.onchange = () => { x.when = $('[data-k=when]', row).value; x.match = $('[data-k=match]', row).value; const act = $('[data-k=act]', row).value, val = $('[data-k=val]', row).value; delete x.replace; delete x.say; delete x.infuse; delete x.infuseFile; delete x.vice; x[act] = val; save(); };
      $('[data-k=en]', row).onclick = e => { x.enabled = x.enabled === false; e.target.classList.toggle('on', x.enabled !== false); save(); };
      $('[data-k=del]', row).onclick = () => { st.rules.splice(i, 1); drawRows(); save(); }; rows.appendChild(row); }); };
    drawRows();
    $('.add', pb).onclick = () => { st.rules.push({ id: 'r' + Date.now(), when: 'command', match: '', say: '' }); drawRows(); save(); };
    $('.hd', pb).onclick = e => { const on = !e.target.classList.contains('on'); e.target.classList.toggle('on', on); st.disable = on ? ['heredoc'] : []; save(); };
    $('.sc', pb).onclick = e => menu(e.currentTarget, [{ label: 'este projeto', check: scope === 'project', f: () => { lsSet('rulesScope', 'project'); draw(); } }, { label: 'todos os projetos', check: scope === 'global', f: () => { lsSet('rulesScope', 'global'); draw(); } }]);
    $('.tc', pb).onkeydown = async e => { if (e.key !== 'Enter') return; const res = await api('/api/rules/test', { body: { command: e.target.value }, root: c.root }); const o = $('.tout', pb); o.style.display = ''; o.textContent = `${t('COMANDO FINAL')}:\n${res.finalCommand}\n\n${t('O AGENTE RECEBE')}:\n${(res.lines || []).join('\n') || t('(apenas a saída normal)')}`; };
  };
  draw(); return {};
}
function mountJournal(pb, extra, c) { const draw = async () => { const l = await api('/api/journal', { root: c.root }); pb.innerHTML = `<div class="plist">${l.length ? `<table><tr><th>${t('quando')}</th><th>${t('tipo')}</th><th>${t('o quê')}</th></tr>${l.map(e => `<tr><td class="faint" style="white-space:nowrap">${new Date(e.ts).toLocaleString()}</td><td><span class="tag ${e.kind === 'blocked' ? 'bad' : ''}">${esc(e.kind)}</span></td><td class="mono sel" style="word-break:break-all">${esc(e.path || e.command || '')}${e.detail ? ` <span class="faint">${esc(e.detail)}</span>` : ''}</td></tr>`).join('')}</table>` : `<div class="empty">${t('Nenhuma escrita passou pelo filtro ainda.')}</div>`}</div>`; }; draw(); return { refresh: debounce(draw, 800) }; }
function mountVices(pb, extra, c) { const draw = async () => { const l = await api('/api/lessons', { root: c.root }); pb.innerHTML = `<div class="plist">${l.length ? `<table><tr><th>${t('quando')}</th><th>${t('vício')}</th><th>${t('detalhe')}</th></tr>${l.map(e => `<tr><td class="faint" style="white-space:nowrap">${new Date(e.ts).toLocaleString()}</td><td><span class="tag warn">${esc(e.vice)}</span></td><td class="sel">${esc(e.detail)}</td></tr>`).join('')}</table>` : `<div class="empty">${t('Nenhum vício observado.')}</div>`}</div>`; }; draw(); return { refresh: draw }; }
function mountMap(pb, extra, c) {
  extra.innerHTML = `<button class="ib" title="${esc(t('Regerar'))}">${I.reload}</button>`;
  const draw = async (refresh) => { const m = await api('/api/map' + (refresh ? '?refresh=1' : ''), { root: c.dir }); const dirs = Object.entries(m.dirs || {}).sort(); pb.innerHTML = `<div class="plist"><div class="faint" style="margin-bottom:8px">${Object.keys(m.files || {}).length} ${t('arquivos')} · ${new Date(m.generatedAt).toLocaleString()}</div>${(m.hotspots || []).length ? `<div class="card" style="margin-bottom:10px"><b>${t('Arquivos grandes demais')}</b>${m.hotspots.map(x => `<div class="mono path" data-path="${esc(x.path)}">${esc(x.path)} — ${x.lines} ${t('linhas')}</div>`).join('')}</div>` : ''}<table><tr><th>${t('pasta')}</th><th>${t('arquivos')}</th><th>${t('linhas')}</th></tr>${dirs.map(([p, d]) => `<tr><td class="mono">${esc(p)}</td><td>${(d.files || []).length}</td><td>${d.lines || 0}</td></tr>`).join('')}</table></div>`; $$('[data-path]', pb).forEach(x => x.onclick = () => openFileInPane(x.dataset.path)); };
  $('button', extra).onclick = () => draw(true); draw(); return { refresh: debounce(() => draw(), 800) };
}

// Agent CLIs (Claude Code, Codex, Gemini) run in a terminal tab inside the window, in the session's folder.
