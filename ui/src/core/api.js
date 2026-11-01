// api — the single HTTP client. Errors are EVENTS, not silence: network failure and HTTP >= 400 throw
// ApiError (callers decide how to show them); nothing is ever swallowed into an empty object.
// The token lives in sessionStorage (set once from the URL fragment, which never travels in requests).
export class ApiError extends Error {
  constructor(message, { status = 0, body = null } = {}) { super(message); this.status = status; this.body = body; }
}

let TOKEN = '';
export function readToken() {
  const m = location.hash.match(/token=([A-Za-z0-9_-]+)/);
  try {
    if (m) { sessionStorage.setItem('infuseToken', m[1]); history.replaceState(null, '', location.pathname); }
    TOKEN = m ? m[1] : (sessionStorage.getItem('infuseToken') || TOKEN);
  } catch { if (m) TOKEN = m[1]; }
  return TOKEN;
}
addEventListener('hashchange', readToken);

function urlWith(url, opts) {
  const root = opts.root ?? '';
  const dir = opts.dir;
  if (root && !/[?&]root=/.test(url)) url += (url.includes('?') ? '&' : '?') + 'root=' + encodeURIComponent(root);
  if (dir && !/[?&]dir=/.test(url)) url += '&dir=' + encodeURIComponent(dir);
  return url;
}

async function request(url, opts = {}) {
  const r = await fetch(urlWith(url, opts), {
    method: opts.method || (opts.body !== undefined ? 'POST' : 'GET'),
    headers: { 'content-type': 'application/json', 'X-Infuse': TOKEN || readToken() },
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    signal: opts.signal
  }).catch(e => { throw new ApiError(e.message || 'network error'); });
  let body = null;
  try { body = await r.json(); } catch { /* empty body is fine */ }
  if (!r.ok) throw new ApiError(body?.error || `HTTP ${r.status}`, { status: r.status, body });
  return body ?? {};
}

// api(url, opts) → parsed body (throws on error); apiOk(...) → {ok:true, data} | {ok:false, error}
// for call sites that prefer branching over try/catch.
export const api = (url, opts) => request(url, opts);
export async function apiOk(url, opts) {
  try { return { ok: true, data: await request(url, opts) }; }
  catch (e) { return { ok: false, error: e }; }
}
export const tokenHeader = () => ({ 'content-type': 'application/json', 'X-Infuse': TOKEN || readToken() });
export const sseUrl = (url, root) => urlWith(url, { root });
export { urlWith };
