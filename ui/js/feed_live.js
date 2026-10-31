'use strict';
// Live stream deltas and incremental patches.

// incomplete JSON (so the code shows as the model writes it).
function partialJSON(src) {
  try { return JSON.parse(src); } catch { }
  const out = { _streaming: true };
  const re = /"([A-Za-z_]+)"\s*:\s*/g; let m;
  while ((m = re.exec(src))) {
    const key = m[1]; let i = re.lastIndex;
    if (src[i] === '"') { // a string, possibly unfinished
      let s = '', j = i + 1, done = false;
      for (; j < src.length; j++) {
        const ch = src[j];
        if (ch === '\\') { const nx = src[j + 1]; if (nx === undefined) break; const map = { n: '\n', t: '\t', r: '\r', '"': '"', '\\': '\\', '/': '/', b: '\b', f: '\f' }; if (nx === 'u') { const hex = src.slice(j + 2, j + 6); if (hex.length < 4) break; s += String.fromCharCode(parseInt(hex, 16)); j += 5; } else { s += map[nx] ?? nx; j++; } continue; }
        if (ch === '"') { done = true; break; }
        s += ch;
      }
      out[key] = s; if (!done) out._open = key; re.lastIndex = j + 1;
    } else { const mm = /^(-?\d+(?:\.\d+)?|true|false|null)/.exec(src.slice(i)); if (mm) out[key] = JSON.parse(mm[1]); }
  }
  return out;
}
// Code being written: numbered lines, following the end while it grows.
function liveCodeHTML(text, cls) {
  const lines = String(text || '').split('\n');
  return `<div class="livecode ${cls || ''}">${lines.map((l, k) => `<div class="lc"><span class="no">${k + 1}</span><span class="tx">${esc(l) || ' '}</span></div>`).join('')}<span class="cursor"></span></div>`;
}
document.addEventListener('click', e => { if (e.target.closest('.agentsgo')) togglePane('agents', true); });
function renderLive(v) {
  if (!v.el) return; const el = $('.live', v.el); const L = v.sid && S.live[v.sid]; if (!el || !L) return;
  const parts = [];
  if (L.think) parts.push(['think', thinkBlock(`${L.id}:live`, L.think, true)]);
  if (L.text) parts.push(['text', `<div class="a"><div class="md sel">${md(L.text)}<span class="cursor"></span></div></div>`]);
  for (const [id, tl] of Object.entries(L.tools)) { const inp = partialJSON(tl.input); parts.push(['tool:' + id, toolBlock(id, tl.name, inp, null, L.running?.id === id)]); }
  let out = '';
  const working = (L.snap?.agents || []).filter(a => a.state === 'running');
  if (working.length && busy(L)) out += `<div class="status sub agentsgo" title="${esc(t('Abrir o painel Agentes'))}"><span class="spin"></span><span>${working.length === 1 ? t('1 subagente trabalhando') : working.length + ' ' + t('subagentes trabalhando')}: ${working.map(a => `<b>${esc(a.title)}</b> · ${a.steps} ${t('passos')}${a.tool ? ' · ' + esc(a.tool) : ''}`).join(' &nbsp;|&nbsp; ')}</span></div>`;
  const st = L.snap?.state;
  if (st === 'thinking' || st === 'tool' || st === 'compacting') out += `<div class="status"><span class="spin"></span><span>${st === 'tool' ? t('executando ') + esc(L.running?.name || '') : st === 'compacting' ? t('compactando contexto') : t('pensando')}</span><span class="faint el" data-el="${L.id}"></span></div>`;
  if (out) parts.push(['status', `<div>${out}</div>`]);
  patchParts(v, el, parts);
  if (v.stick !== false) toBottom(v); updateJump(v);
}
// Updates a container part by part (keyed): an unchanged part keeps its element, so nothing flashes,
// folds stay as they are and a code view keeps its scroll (following the end only if it was there).
function patchParts(v, el, parts) {
  const have = new Map([...el.children].map(c => [c.dataset.k, c]));
  let prev = null;
  for (const [key, html] of parts) {
    let node = have.get(key);
    if (!node || node._html !== html) {
      const n = document.createElement('div'); n.dataset.k = key; n.className = 'lk'; n.innerHTML = html; n._html = html;
      if (node) {
        const codes = $$('.livecode', node).map(c => ({ end: c.scrollHeight - c.scrollTop - c.clientHeight < 8, top: c.scrollTop }));
        node.replaceWith(n);
        $$('.livecode', n).forEach((c, i) => { const was = codes[i]; c.scrollTop = !was || was.end ? c.scrollHeight : was.top; });
      } else {
        if (prev) prev.after(n); else el.prepend(n);
        $$('.livecode', n).forEach(c => { c.scrollTop = c.scrollHeight; });
      }
      bindChat(v, n); node = n;
    }
    have.delete(key); prev = node;
  }
  have.forEach(n => n.remove());
}
setInterval(() => { for (const el of $$('[data-el]')) { const L = S.live[el.dataset.el]; if (L?.started) el.textContent = Math.round((Date.now() - L.started) / 1000) + 's'; } }, 500);
