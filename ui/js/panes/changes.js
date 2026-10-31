'use strict';
// changes
// Changes pane: file list, diff with line comments, commit, discard.

// ---------------------------------------------------------------- Changes: file list, diff with line comments, commit, discard
function mountChanges(pb, extra, c) {
  pb.innerHTML = `<div class="chg2"><div class="fl"></div><div class="dv"><div class="empty">${t('ui.pane.changes.selecione-um-arquivo.398a34')}</div></div></div><div class="pfoot"><span class="faint ncom"></span><button class="btn sm line sendc hidden">${t('ui.pane.changes.enviar-comentarios.e7f36f')} <span class="kbd">Ctrl Enter</span></button><span class="grow"></span><input class="cmsg" placeholder="${esc(t('ui.pane.changes.mensagem-do-commit.a734a6'))}"><button class="btn sm pri commit">${t('ui.pane.changes.commit.4015b5')}</button></div>`;
  extra.innerHTML = `<span class="scopeslot"></span><button class="ib" title="${esc(t('ui.pane.changes.atualizar.f89062'))}">${I.reload}</button>`;
  const fl = $('.fl', pb), dv = $('.dv', pb); let sel = null, files = [];
  const q = () => '&session=' + encodeURIComponent(curL()?.id || '') + '&scope=' + chgScope();
  const comments = []; // {path, line, code, text}
  const updC = () => { $('.ncom', pb).textContent = comments.length ? `${comments.length} ${t('ui.pane.changes.comentario-s.197658')}` : ''; $('.sendc', pb).classList.toggle('hidden', !comments.length); };
  const load = async () => {
    const sid = curL()?.id || '';
    const r = await api('/api/changes?session=' + encodeURIComponent(sid) + '&scope=' + chgScope(), { root: c.root, dir: c.dir });
    files = r.files || [];
    $('.scopeslot', extra).innerHTML = r.local ? scopeSwitch() : ''; bindScope(extra);
    $$('.cmsg, .commit', pb).forEach(x => x.classList.toggle('hidden', !!r.local)); // no git: nothing to commit
    fl.innerHTML = files.length ? '' : `<div class="empty">${t('ui.rail.nenhuma-alteracao.63c618')}</div>`;
    files.forEach(f => { const it = h(`<div class="fli${f.path === sel ? ' on' : ''}${f.status === 'D' ? ' gone' : ''}" title="${esc(f.path)}"><span class="n">${esc(f.path)}</span>${chgTag(f)}<span class="add">+${f.added}</span><span class="del">−${f.removed}</span></div>`); it.onclick = () => select(f.path); it.oncontextmenu = e => { e.preventDefault(); menu(null, [{ label: 'Abrir no painel Arquivos', f: () => openFileInPane(f.path) }, ...openInItems(c.dir.replace(/[\\/]+$/, '') + '/' + f.path), { sep: 1 }, { label: 'Descartar alterações', danger: 1, f: () => discard(f.path) }], e); }; fl.appendChild(it); });
    if (sel && files.find(f => f.path === sel)) select(sel); else if (files[0]) select(files[0].path); else dv.innerHTML = `<div class="empty">${t('ui.pane.changes.nada-para-revisar.e658bb')}</div>`;
  };
  const discard = async p => { if (!confirm(t('ui.pane.changes.descartar-as-alteracoes-em.b32da4') + p + '?')) return; await fetch(`/api/git?root=${encodeURIComponent(c.dir)}${q()}&project=${encodeURIComponent(c.root)}`, { method: 'POST', headers: { 'content-type': 'application/json', 'X-Infuse': API_TOKEN || readToken() }, body: JSON.stringify({ action: 'discard', path: p }) }); load(); const L = curL(); if (L) refreshChanges(L); };
  const select = async p => {
    sel = p; $$('.fli', fl).forEach(x => x.classList.toggle('on', x.title === p));
    const r = await api('/api/diff?path=' + encodeURIComponent(p) + q() + '&project=' + encodeURIComponent(c.root), { root: c.dir });
    const lines = (r.diff || '').replace(/\n$/, '').split('\n'); let nNew = 0; const out = h('<div class="rdiff"></div>');
    out.appendChild(h(`<div class="row u-p6px10px-bord1pxsolid--border-ff--sans"><span class="mono trunc grow">${esc(p)}</span><button class="btn sm danger">${t('ui.pane.changes.descartar.c88fe9')}</button></div>`));
    $('.btn', out).onclick = () => discard(p);
    for (const l of lines) {
      if (/^(diff |index |--- |\+\+\+ )/.test(l)) continue;
      const m = l.match(/^@@ -\d+(?:,\d+)? \+(\d+)/); if (m) { nNew = +m[1]; out.appendChild(h(`<div class="ln hh"><span class="no"></span><span class="tx">${esc(l)}</span></div>`)); continue; }
      const cls = l.startsWith('+') ? 'a1' : l.startsWith('-') ? 'd1' : ''; const lineNo = l.startsWith('-') ? '' : nNew;
      const row = h(`<div class="ln ${cls}"><span class="no">${lineNo}</span><span class="tx">${esc(l) || ' '}</span>${lineNo ? '<span class="cadd" title="Comentar">+</span>' : ''}</div>`);
      if (lineNo) $('.cadd', row).onclick = () => addComment(row, p, lineNo, l.slice(1));
      out.appendChild(row);
      for (const cm of comments.filter(x => x.path === p && x.line === lineNo && lineNo)) out.appendChild(noteEl(cm));
      if (!l.startsWith('-')) nNew++;
    }
    dv.innerHTML = ''; dv.appendChild(out);
  };
  const noteEl = cm => { const n = h(`<div class="cnote"><span class="grow">${esc(cm.text)}</span><button class="ib u-w20px-h20px">${I.close}</button></div>`); $('button', n).onclick = () => { comments.splice(comments.indexOf(cm), 1); n.remove(); updC(); }; return n; };
  const addComment = (row, path, line, code) => {
    const box = h(`<div class="cbox"><textarea placeholder="${esc(t('ui.pane.changes.comentario-para-o-agente.a30ab0'))}"></textarea></div>`); row.after(box); const ta = $('textarea', box); ta.focus();
    ta.onkeydown = e => { e.stopPropagation(); if (e.key === 'Escape') box.remove(); if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); if (ta.value.trim()) { comments.push({ path, line, code, text: ta.value.trim() }); } box.remove(); sendComments(); return; } if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); if (ta.value.trim()) { const cm = { path, line, code, text: ta.value.trim() }; comments.push(cm); box.replaceWith(noteEl(cm)); updC(); } } };
  };
  const sendComments = async () => {
    const L = curL(); if (!L) return toast('Abra a sessão para enviar os comentários');
    if (!comments.length) return; const r = await api(`/api/session/${L.id}/comments`, { body: { comments }, root: L.root });
    if (r.error) return toast(r.error); comments.length = 0; updC(); toast('Comentários enviados ao agente'); select(sel);
  };
  $('.sendc', pb).onclick = sendComments;
  pb.addEventListener('keydown', e => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && comments.length) { e.preventDefault(); sendComments(); } });
  const commit = async () => { const msg = $('.cmsg', pb).value.trim(); if (!msg) return toast('Escreva a mensagem do commit'); const r = await api('/api/git', { body: { action: 'commit', message: msg }, root: c.dir }); toast(r.ok ? 'Commit feito' : t('ui.pane.changes.falhou.2b04dd') + (r.output || r.error)); if (r.ok) { $('.cmsg', pb).value = ''; load(); const L = curL(); if (L) refreshChanges(L); } };
  $('.commit', pb).onclick = commit; $('.cmsg', pb).onkeydown = e => { if (e.key === 'Enter') commit(); };
  $('.ib', extra).onclick = load;
  load();
  return { refresh: debounce(load, 300), select };
}
