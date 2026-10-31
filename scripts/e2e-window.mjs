// End-to-end check of the real window (infuse.exe), never shown on screen. Run:
//   INFUSE_WINDOW_HIDDEN=1 WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9333 build/infuse.exe &
//   node scripts/e2e-window.mjs
// Needs a project at .../test-infuse with a session and a connected provider.
// End-to-end check of the real infuse.exe window (started hidden, with WebView2 remote debugging).
const PORT = process.env.CDP_PORT || 9333;
const sleep = ms => new Promise(r => setTimeout(r, ms));
let target;
for (let i = 0; i < 60 && !target; i++) {
  try {
    const list = await (await fetch(`http://127.0.0.1:${PORT}/json/list`)).json();
    target = list.find(t => t.type === 'page' && /^http:\/\/(127\.0\.0\.1|localhost)/.test(t.url));
  } catch { }
  if (!target) await sleep(500);
}
if (!target) { console.log(JSON.stringify({ fatal: 'app page not found over CDP' })); process.exit(1); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });
let seq = 0; const pending = {};
ws.onmessage = m => { const d = JSON.parse(m.data); if (d.id && pending[d.id]) { pending[d.id](d); delete pending[d.id]; } };
const send = (method, params = {}) => new Promise(r => { const id = ++seq; pending[id] = r; ws.send(JSON.stringify({ id, method, params })); });
const ev = async (expr, timeout = 60000) => {
  const r = await send('Runtime.evaluate', { expression: `(async () => { ${expr} })()`, awaitPromise: true, returnByValue: true, timeout });
  if (r.result?.exceptionDetails) return { __error: r.result.exceptionDetails.exception?.description || r.result.exceptionDetails.text };
  return r.result?.result?.value;
};
const results = [];
const check = (name, ok, detail) => { results.push({ name, ok: !!ok, detail }); };

await sleep(2500);
const boot = await ev(`return { native: !!(window.infuse && infuse.native), sessions: S.sessions.length, projects: S.projects.length, title: document.title }`);
check('app carregou na janela nativa', boot && boot.native && boot.sessions >= 0, boot);

const open = await ev(`const s = S.sessions.find(x => /test-infuse$/i.test(x.root)); if (!s) return 'sem sessão em test-infuse'; await openSession(s.root, s.id); await new Promise(r => setTimeout(r, 1200)); return { title: curL()?.snap?.title, msgs: curL()?.snap?.messages?.length }`);
check('abre sessão de test-infuse', open && open.msgs > 0, open);

// native browser: a site that refuses iframes, then a search, then back
await ev(`window.__bev = []; infuse.on('browser', e => __bev.push(e)); P2.open.slice().forEach(closePane); togglePane('browser', true); await new Promise(r => setTimeout(r, 800)); paneCall('browser', 'go', 'github.com'); return true`);
const waitEv = async (re, ms = 25000) => { for (let t = 0; t < ms; t += 500) { const hit = await ev(`return (__bev || []).filter(e => !e.loading && ${re}.test(e.url || '')).pop() || null`); if (hit) return hit; await sleep(500); } return null; };
const gh = await waitEv('/github\\.com/');
check('navegador nativo abre github.com (recusa iframe)', gh, gh);
const pane = await ev(`const f = document.querySelector('.pane[data-id="browser"]'); return { iframes: f.querySelectorAll('iframe').length, url: f.querySelector('.url').value, tab: f.querySelector('.ptab')?.innerText }`);
check('aba e barra de endereço atualizadas', pane && pane.iframes === 0 && /github/.test(pane.url), pane);
await ev(`paneCall('browser', 'go', 'como usar fetch em javascript'); return true`);
const g = await waitEv('/google\\.[a-z.]+\\/search/');
check('pesquisa pela barra de endereço', g, g && { url: g.url.slice(0, 80), title: g.title });
await ev(`document.querySelector('.pane[data-id="browser"] .back').click(); return true`);
const back = await waitEv('/github\\.com/');
check('voltar retorna ao github', back, back && back.url);
await ev(`document.querySelector('.pane[data-id="browser"] .nt').click(); await new Promise(r => setTimeout(r, 300)); paneCall('browser', 'go', 'https://developer.mozilla.org/pt-BR/'); return true`);
const mdn = await waitEv('/developer\\.mozilla\\.org/');
check('segunda aba abre MDN', mdn, mdn && mdn.title);

// Pronto: run the criteria in a snapshot
const pronto = await ev(`closePane('browser'); togglePane('oracle', true); await new Promise(r => setTimeout(r, 1500)); const out = document.querySelector('.pane[data-id="oracle"] .verdict'); await runOracleUI({ root: curRoot(), dir: curDir() }, out); return out.innerText.split('\\n').slice(0, 4).join(' | ')`);
check('Pronto? roda os critérios', typeof pronto === 'string' && /Pronto|Ainda não|Nada para verificar/.test(pronto), pronto);
const sugg = await ev(`return { cards: document.querySelectorAll('.pane[data-id="oracle"] .crit').length, help: !!document.querySelector('.pane[data-id="oracle"] .help') }`);
check('critérios em frases e botão de ajuda', sugg && sugg.cards >= 1 && sugg.help, sugg);

// Memory pane
const mem = await ev(`closePane('oracle'); togglePane('memory', true); await new Promise(r => setTimeout(r, 1200)); return { items: [...document.querySelectorAll('.pane[data-id="memory"] .mitem textarea')].map(x => x.value) }`);
check('painel de memória lista itens', mem && mem.items.length >= 1, mem);

// a real turn: time, duration, model and tokens under the answer; turn summary
const turn = await ev(`closePane('memory'); const v = V(); const P = v.el.querySelector('.prompt'); const L = curL(); const n0 = L.snap.messages.length; P.value = 'Sem usar ferramentas, responda apenas: ok'; send(v); for (let i = 0; i < 180; i++) { await new Promise(r => setTimeout(r, 500)); const m = L.snap.messages; if (m.length > n0 + 1 && L.snap.state === 'idle' && m[m.length - 1]?.role === 'assistant') break; } await new Promise(r => setTimeout(r, 800)); const last = L.snap.messages[L.snap.messages.length - 1]; return { state: L.snap.state, text: msgText(last), foot: [...v.el.querySelectorAll('.afoot .meta')].pop()?.innerText.replace(/\\n/g, ' '), sum: [...v.el.querySelectorAll('.turnsum')].pop()?.innerText.replace(/\\n/g, ' ') }`, 90000);
check('turno real com o modelo (eventos ao vivo chegam à janela)', turn && turn.state === 'idle' && /^ok\b/i.test(turn.text || ''), turn);
check('horário, duração, modelo e tokens na resposta', turn && /\d\d:\d\d/.test(turn.foot || '') && /tokens/.test(turn.foot || ''), turn && turn.foot);
check('resumo do turno com Copiar', turn && /Trabalhou por/.test(turn.sum || ''), turn && turn.sum);
let ghost;
for (let i = 0; i < 30; i++) { ghost = await ev(`const P = V().el.querySelector('.prompt'); return { ghost: P.dataset.ghost || '' }`); if (ghost.ghost) break; await sleep(500); }
const tab = ghost.ghost ? await ev(`const v = V(); const P = v.el.querySelector('.prompt'); P.focus(); P.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true })); const val = P.value; P.value = ''; setGhost(v, ''); return val`) : '';
check('sugestão da próxima mensagem; Tab aceita', ghost.ghost && tab === ghost.ghost, { ghost: ghost.ghost, tab });

// sidebar search inside conversations
const search = await ev(`const s = document.querySelector('#search'); s.value = 'commonjs'; s.dispatchEvent(new Event('input')); await new Promise(r => setTimeout(r, 1500)); const hits = document.querySelectorAll('.srow.hit').length; s.value = ''; s.dispatchEvent(new Event('input')); return hits`);
check('busca dentro das conversas', search >= 1, search);

// English
const en = await ev(`const before = S.lang; return { lang: before, hasEN: typeof EN === 'object' && EN['Pensou por'] === 'Thought for' }`);
check('tabela de inglês carregada', en && en.hasEN, en);

const errs = await ev(`return (window.__errs || []).length`);
console.log(JSON.stringify({ results, errs }, null, 1));
ws.close();
process.exit(0);
