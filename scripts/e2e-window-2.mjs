// Second end-to-end pass on the real window: the features added after the first pass.
const PORT = 9333; const sleep = ms => new Promise(r => setTimeout(r, ms));
let target;
for (let i = 0; i < 60 && !target; i++) { try { const list = await (await fetch(`http://127.0.0.1:${PORT}/json/list`)).json(); target = list.find(t => t.type === 'page' && /^http:\/\/(127\.0\.0\.1|localhost)/.test(t.url)); } catch { } if (!target) await sleep(500); }
const ws = new WebSocket(target.webSocketDebuggerUrl); await new Promise(r => ws.onopen = r);
let seq = 0; const pending = {}; ws.onmessage = m => { const d = JSON.parse(m.data); if (d.id && pending[d.id]) { pending[d.id](d); delete pending[d.id]; } };
const ev = (expr, timeout = 120000) => new Promise(r => { const id = ++seq; pending[id] = d => r(d.result?.exceptionDetails ? { __error: d.result.exceptionDetails.exception?.description } : d.result?.result?.value); ws.send(JSON.stringify({ id, method: 'Runtime.evaluate', params: { expression: `(async () => { ${expr} })()`, awaitPromise: true, returnByValue: true, timeout } })); });
const results = []; const check = (name, ok, detail) => results.push({ name, ok: !!ok, detail });
await sleep(2500);
await ev(`const s = S.sessions.find(x => /test-infuse$/i.test(x.root)); await openSession(s.root, s.id); await new Promise(r => setTimeout(r, 1000)); return true`);

for (const [sec, sel] of [['skills', '.mcpcard, .oempty'], ['connectors', '.mcppre'], ['hooks', '.mcppre'], ['docs', '.mcpcustom'], ['conventions', 'textarea.doc']]) {
  const r = await ev(`await openCustomize('${sec}'); await new Promise(r => setTimeout(r, 1200)); return { n: document.querySelectorAll('${sel}').length, h1: document.querySelector('.pcontent h1')?.innerText }`);
  check('Personalizar › ' + sec, r && r.n > 0, r);
}
const conv = await ev(`await openCustomize('conventions'); await new Promise(r => setTimeout(r, 800)); return document.querySelector('textarea.doc').value.slice(0, 300)`);
check('convenções detectadas do código', /Indentação|Módulos/.test(conv || ''), conv);
await ev(`showPage(null); return true`);

const rail = await ev(`renderRail(V()); await new Promise(r => setTimeout(r, 500)); const el = V().el.querySelector('.rail'); return { mission: !!el.querySelector('.mnew, .mission'), review: !!el.querySelector('.revgo'), pronto: !!el.querySelector('.oraclego') }`);
check('trilho: missão, revisar, Pronto?', rail && rail.mission && rail.review && rail.pronto, rail);

// a real turn where the agent uses its own headless browser and takes a screenshot
const turn = await ev(`const v = V(); const L = curL(); const n0 = L.snap.messages.length; v.el.querySelector('.prompt').value = 'Use a ferramenta Browser: abra https://example.com, tire uma captura de tela e diga em uma frase o título da página. Não use outras ferramentas.'; send(v);
  for (let i = 0; i < 240; i++) { await new Promise(r => setTimeout(r, 500)); const m = L.snap.messages; if (L.snap.pendingPermission) decide(v, L.snap.pendingPermission.id, 'allow'); if (m.length > n0 + 1 && L.snap.state === 'idle' && m[m.length - 1].role === 'assistant') break; }
  await new Promise(r => setTimeout(r, 1500));
  const m = L.snap.messages.slice(n0); const img = [...v.el.querySelectorAll('img.shot')].pop();
  return { tools: m.flatMap(x => x.content.filter(b => b.type === 'tool_use').map(b => b.name + ':' + (JSON.parse(JSON.stringify(b.input)).action || ''))), answer: msgText(m[m.length - 1]).slice(0, 200), shot: img ? { w: img.naturalWidth, h: img.naturalHeight, src: img.src.slice(0, 80) } : null }`, 200000);
check('agente usa o navegador próprio (navigate + screenshot)', turn && (turn.tools || []).some(x => x.startsWith('Browser:navigate')) && (turn.tools || []).some(x => x.startsWith('Browser:screenshot')), turn && turn.tools);
check('captura aparece na conversa', turn && turn.shot && turn.shot.w > 100, turn && turn.shot);
check('resposta cita o título (Example Domain)', turn && /Example Domain/i.test(turn.answer || ''), turn && turn.answer);

const memory = await ev(`P2.open.slice().forEach(closePane); togglePane('memory', true); await new Promise(r => setTimeout(r, 1200)); return document.querySelectorAll('.pane[data-id="memory"] .mitem').length`);
check('painel de memória', memory >= 1, memory);

const search = await ev(`const s = document.querySelector('#search'); s.value = 'despedir'; s.dispatchEvent(new Event('input')); await new Promise(r => setTimeout(r, 1500)); const n = document.querySelectorAll('.srow.hit, .srow').length; s.value = ''; s.dispatchEvent(new Event('input')); return n`);
check('busca nas conversas', search >= 1, search);

console.log(JSON.stringify({ results }, null, 1));
ws.close(); process.exit(0);
