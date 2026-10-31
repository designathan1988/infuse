'use strict';
// scheduled
// Scheduled tasks page.

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
