'use strict';
// Permission and question cards.

function permCard(p) {
  let inp = {}; try { inp = typeof p.input === 'string' ? JSON.parse(p.input) : p.input || {}; } catch { }
  const verb = p.category === 'exec' ? 'executar um comando' : p.category === 'write' ? 'alterar um arquivo' : p.category === 'net' ? 'acessar a web' : p.category === 'mcp' ? 'usar um conector (roda fora do filtro)' : p.category === 'memory' ? 'registrar na memória' : 'usar uma ferramenta';
  const detail = p.tool === 'Bash' ? `<pre class="cmd">${esc(inp.command || '')}</pre>` : p.tool === 'Edit' ? `<div class="mono u-mt8px">${esc(inp.path)}</div>` + diffHTML((inp.old_string || '').split('\n').map(l => '-' + l).join('\n') + '\n' + (inp.new_string || '').split('\n').map(l => '+' + l).join('\n')) : p.tool === 'Write' ? `<div class="mono u-mt8px">${esc(inp.path)}</div>` + diffHTML((inp.content || '').split('\n').slice(0, 200).map(l => '+' + l).join('\n')) : p.tool === 'WebFetch' ? `<pre class="cmd">${esc(inp.url || '')}</pre>` : `<pre class="cmd">${esc(JSON.stringify(inp, null, 2))}</pre>`;
  return `<div class="pcard"><div class="hd"><b>${esc(p.tool)}</b><span class="muted">${t('ui.feed.quer.589029')} ${t(verb)}</span></div>${p.reason ? `<div class="sentinel">${t('ui.feed.sentinela.1de605')} ${esc(t(p.reason))}. ${t('ui.feed.o-infuse-pergunta-mesmo.3d076d')}</div>` : ''}${detail}
  <div class="opts"><div class="opt" data-perm="allow" data-id="${p.id}">${t('ui.feed.sim.813221')}<span class="kbd">Y</span></div><div class="opt" data-perm="always" data-id="${p.id}">${t('ui.feed.sim-e-nao-perguntar.4078dd')}${p.tool === 'Bash' ? ` <span class="faint">(${esc((inp.command || '').trim().split(/\s+/).slice(0, 2).join(' '))})</span>` : ''}<span class="kbd">A</span></div><div class="opt" data-perm="all" data-id="${p.id}">${t('ui.feed.sim-e-aceitar-tudo.026675')}<span class="kbd">T</span></div><div class="opt" data-perm="deny" data-id="${p.id}">${t('ui.feed.nao-e-dizer-ao.7ae7c5')}<span class="kbd">N</span></div></div></div>`;
}
function questionCard(q) {
  return `<div class="pcard"><div class="hd"><b>${esc(q.question)}</b></div><div class="opts">${(q.options || []).map((o, i) => `<div class="opt" data-ans="${esc(o)}" data-id="${q.id}">${esc(o)}<span class="kbd">${i + 1}</span></div>`).join('')}</div>
  <div class="free"><input data-id="${q.id}" placeholder="${esc(t('ui.feed.ou-responda-com-suas.ea37eb'))}"></div></div>`;
}
async function decide(v, id, d) {
  if (d === 'all') { await setSessionOpts(v, { mode: 'bypass' }); d = 'allow'; toast(t('ui.composer.modo.3d33be') + t('ui.composer.aceitar-tudo.4986c1')); }
  const L = S.live[v.sid]; await api(`/api/sessions/${v.sid}/permission`, { body: { id, decision: d }, root: L.root }); if (L.snap) Live.snap(L.id, { pendingPermission: null }); renderChat(v); if (d === 'deny') setTimeout(() => $('.prompt', elOf(v)).focus(), 50); }
async function answer(v, id, a) { const L = S.live[v.sid]; await api(`/api/sessions/${v.sid}/answer`, { body: { id, answer: a }, root: L.root }); if (L.snap) Live.snap(L.id, { pendingQuestion: null }); renderChat(v); }

// markdown (small, safe): code blocks, inline code (paths become links), headings, lists, quotes, tables, links
// Lists, nested by indentation: a sub-item stays inside its item; ordered lists keep their numbers.
