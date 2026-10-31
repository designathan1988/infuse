'use strict';
// Permission and question cards.

function permCard(p) {
  let inp = {}; try { inp = typeof p.input === 'string' ? JSON.parse(p.input) : p.input || {}; } catch { }
  const verb = p.category === 'exec' ? 'executar um comando' : p.category === 'write' ? 'alterar um arquivo' : p.category === 'net' ? 'acessar a web' : p.category === 'mcp' ? 'usar um conector (roda fora do filtro)' : p.category === 'memory' ? 'registrar na memória' : 'usar uma ferramenta';
  const detail = p.tool === 'Bash' ? `<pre class="cmd">${esc(inp.command || '')}</pre>` : p.tool === 'Edit' ? `<div class="mono" style="margin-top:8px">${esc(inp.path)}</div>` + diffHTML((inp.old_string || '').split('\n').map(l => '-' + l).join('\n') + '\n' + (inp.new_string || '').split('\n').map(l => '+' + l).join('\n')) : p.tool === 'Write' ? `<div class="mono" style="margin-top:8px">${esc(inp.path)}</div>` + diffHTML((inp.content || '').split('\n').slice(0, 200).map(l => '+' + l).join('\n')) : p.tool === 'WebFetch' ? `<pre class="cmd">${esc(inp.url || '')}</pre>` : `<pre class="cmd">${esc(JSON.stringify(inp, null, 2))}</pre>`;
  return `<div class="pcard"><div class="hd"><b>${esc(p.tool)}</b><span class="muted">${t('quer')} ${t(verb)}</span></div>${p.reason ? `<div class="sentinel">${t('Sentinela:')} ${esc(t(p.reason))}. ${t('O infuse pergunta mesmo no modo "aceitar tudo": enfraquecer o teste faz ele passar sem resolver o problema.')}</div>` : ''}${detail}
  <div class="opts"><div class="opt" data-perm="allow" data-id="${p.id}">${t('Sim')}<span class="kbd">Y</span></div><div class="opt" data-perm="always" data-id="${p.id}">${t('Sim, e não perguntar de novo nesta sessão')}${p.tool === 'Bash' ? ` <span class="faint">(${esc((inp.command || '').trim().split(/\s+/).slice(0, 2).join(' '))})</span>` : ''}<span class="kbd">A</span></div><div class="opt" data-perm="all" data-id="${p.id}">${t('Sim, e aceitar tudo daqui em diante')}<span class="kbd">T</span></div><div class="opt" data-perm="deny" data-id="${p.id}">${t('Não, e dizer ao agente o que fazer diferente')}<span class="kbd">N</span></div></div></div>`;
}
function questionCard(q) {
  return `<div class="pcard"><div class="hd"><b>${esc(q.question)}</b></div><div class="opts">${(q.options || []).map((o, i) => `<div class="opt" data-ans="${esc(o)}" data-id="${q.id}">${esc(o)}<span class="kbd">${i + 1}</span></div>`).join('')}</div>
  <div class="free"><input data-id="${q.id}" placeholder="${esc(t('ou responda com suas palavras…'))}"></div></div>`;
}
async function decide(v, id, d) {
  if (d === 'all') { await setSessionOpts(v, { mode: 'bypass' }); d = 'allow'; toast(t('Modo: ') + t('Aceitar tudo')); }
  const L = S.live[v.sid]; await api(`/api/sessions/${v.sid}/permission`, { body: { id, decision: d }, root: L.root }); if (L.snap) L.snap.pendingPermission = null; renderChat(v); if (d === 'deny') setTimeout(() => $('.prompt', v.el).focus(), 50); }
async function answer(v, id, a) { const L = S.live[v.sid]; await api(`/api/sessions/${v.sid}/answer`, { body: { id, answer: a }, root: L.root }); if (L.snap) L.snap.pendingQuestion = null; renderChat(v); }

// markdown (small, safe): code blocks, inline code (paths become links), headings, lists, quotes, tables, links
// Lists, nested by indentation: a sub-item stays inside its item; ordered lists keep their numbers.
