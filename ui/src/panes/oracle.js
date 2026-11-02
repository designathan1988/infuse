// oracle — "Done?" criteria editor + executor. Criteria read as sentences with inline value slots;
// the project's own scripts become one-click suggestions; running executes them on a frozen copy.
import { $, $$, h, esc, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { fmtTime } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { api } from '../core/api.js';
import { pref, ui, liveOf, refreshChanges } from '../core/store.js';
import { menu } from '../components/menu.js';
import { toast } from '../components/toast.js';

const CRIT_KINDS = {
  command: { name: 'crit.command.name', ex: 'npm test · go test ./... · python -m pytest', help: 'crit.command.help', make: () => ({ kind: 'command', run: '', expect: { exit: 0 } }), title: 'crit.command.title' },
  http: { name: 'crit.http.name', ex: 'http://localhost:3000/login → 200 "Sign in"', help: 'crit.http.help', make: () => ({ kind: 'http', url: 'http://localhost:3000/', expect: { status: 200 } }), title: 'crit.http.title' },
  browser: { name: 'crit.browser.name', ex: 'http://localhost:3000/ → #root > * "Sign in"', help: 'crit.browser.help', make: () => ({ kind: 'browser', url: 'http://localhost:3000/', expect: { selector: '' } }), title: 'crit.browser.title' },
  file: { name: 'crit.file.name', ex: 'README.md contains "Installation" · .env does not exist', help: 'crit.file.help', make: () => ({ kind: 'file', path: '', expect: {} }), title: 'crit.file.title' },
  maxLines: { name: 'crit.maxLines.name', ex: 'src/**/*.ts with at most 400 lines', help: 'crit.maxLines.help', make: () => ({ kind: 'maxLines', glob: '**/*.ts', max: 400 }), title: 'crit.maxLines.title' },
};
const ORACLE_EXAMPLE_KEY = 'crit.example';

export const oracleSpecs = {}; // root → spec last loaded (verdict titles)
const critTitle = (root, id) => (oracleSpecs[root]?.checks || []).find(k => k.id === id)?.description || id;

export function drawVerdict(c, out, v, err) {
  if (!v || !v.run) {
    out.innerHTML = err ? `<div class="errbox" role="alert">${esc(err)}</div>` : `<div class="verdict-box idle"><b>${t('verdict.notChecked')}</b><span>${t('verdict.runHint')}</span></div>`;
    return;
  }
  const checks = v.checks || [], passed = checks.filter(k => k.passed).length;
  const heads = {
    aprovado: ['verdict.passed', t('verdict.passedCount', { passed, total: checks.length }), 'ok'],
    reprovado: ['verdict.failed', t('verdict.failedCount', { failed: checks.length - passed, total: checks.length }), 'bad'],
    'sem-criterios': ['verdict.nothing', 'verdict.addCriteria', 'warn'],
  };
  const [hk, hs, cls] = heads[v.verdict] || ['verdict.couldNot', v.summary || err || '', 'bad'];
  out.innerHTML = `<div class="verdict-box ${cls}" role="status"><b>${esc(t(hk))}</b><span>${esc(t(hs) === hs ? hs : t(hs) || hs)}</span><span class="grow"></span><span class="faint" title="${esc(t('verdict.snapshotHint'))}">${fmtTime(v.finished || v.started)}</span></div>
    ${checks.map(k => `<div class="verdict-row"><span class="${k.passed ? 'add' : 'del'}" aria-hidden="true">${k.passed ? '✓' : '✗'}</span><span class="vt">${esc(critTitle(c.root, k.id))}</span><span class="faint grow trunc" title="${esc(k.detail)}">${esc(k.detail)}</span><span class="faint">${k.millis < 1000 ? k.millis + ' ms' : (k.millis / 1000).toFixed(1) + ' s'}</span>${k.evidence ? `<button class="btn sm" data-ev="${esc(k.evidence)}">${t('verdict.viewOutput')}</button>` : ''}</div>`).join('')}
    <div class="evbox"></div>`;
  $$('[data-ev]', out).forEach(b => b.onclick = async () => {
    const box = out.querySelector('.evbox');
    if (box.dataset.f === b.dataset.ev) { box.innerHTML = ''; box.dataset.f = ''; return; }
    const e = await api(`/api/oracle/evidence?run=${encodeURIComponent(v.run)}&file=${encodeURIComponent(b.dataset.ev)}`, { root: c.root }).catch(() => ({}));
    box.dataset.f = b.dataset.ev;
    box.innerHTML = `<pre class="ev-pre sel">${esc(e.content || e.error || t('verdict.emptyOutput'))}</pre>`;
  });
}

async function runOracle(c, out) {
  out.innerHTML = `<div class="verdict-box"><span class="spin"></span>${t('verdict.running')}</div>`;
  const r = await api('/api/oracle/run', { body: { session: liveOfCur(c)?.id || '' }, root: c.root, dir: c.dir }).catch(e => ({ error: String(e) }));
  const v = r.verdict || {};
  drawVerdict(c, out, v, r.error);
  const L = liveOfCur(c);
  if (L) { L.verdict.set(v); refreshChanges(L); }
  out.dispatchEvent(new CustomEvent('verdict', { detail: v, bubbles: true }));
}
const liveOfCur = c => {
  for (const v of ui.views.get()) {
    const L = v?.sid ? liveOf(v.sid) : null;
    if (L && L.root === c.root) return L;
  }
  return ui.views.get().map(v => v?.sid ? liveOf(v.sid) : null).find(Boolean) || null;
};

function shellHTML(o, files) {
  return `<div class="pane-list oracle-pane">
      <div class="ohead"><button class="btn pri run">${t('rail.doneBtn')}</button><span class="grow">${t('oracle.explainer')}</span></div>
      <div class="howto" ${o.helpOpen ? '' : 'hidden'}>
        <div class="row"><b class="grow">${t('common.howItWorks')}</b><button class="icon-btn hx" aria-label="${esc(t('common.close'))}">${I.close}</button></div>
        <ol>
          <li><b>${t('oracle.step1a')}</b> ${t('oracle.step1b')}</li>
          <li><b>${t('oracle.step2a')}</b> ${t('oracle.step2b')}</li>
          <li><b>${t('oracle.step3a')}</b> ${t('oracle.step3b')}</li>
        </ol>
        <div class="kinds">${Object.entries(CRIT_KINDS).map(([k, x]) => `<div><b>${t(x.name)}</b><span class="faint">${t('common.eg')} ${esc(x.ex)}</span></div>`).join('')}</div>
      </div>
      <div class="verdict-host"></div>
      <div class="oracle-sect"><span class="grow">${t('oracle.needsToBeTrue')}</span><button class="btn sm addc">${I.plus} ${t('oracle.addCriterion')}</button></div>
      <div class="checks"></div>
      <label class="row rules-toggle"><span class="switch ${o.spec.requireAcceptance ? 'on' : ''} acc" role="switch" aria-checked="${!!o.spec.requireAcceptance}" tabindex="0"></span><span class="grow">${t('oracle.requireAcceptance')}<span class="faint"> — ${t('oracle.requireAcceptanceDesc')}</span></span></label>
      <div class="sugg"></div>
      <details class="details-fold"${o.spec.setup || o.spec.server ? ' open' : ''}><summary>${t('oracle.setupTitle')}</summary>
        <div class="crit-sentence">${t('oracle.setupRun')} <input class="slot mono setup" value="${esc(o.spec.setup || '')}" placeholder="npm ci" spellcheck="false"> ${t('oracle.setupHint')}</div>
        <div class="faint crit-help">${t('oracle.setupHelp')}</div>
        <div class="crit-sentence">${t('oracle.serverRun')} <input class="slot mono server" value="${esc(o.spec.server || '')}" placeholder="npm run dev" spellcheck="false"> ${t('oracle.serverHint')}</div>
      </details>
      <details class="details-fold"${files.some?.(f => f.name !== 'checks.json' && f.content.trim()) ? ' open' : ''}><summary>${t('oracle.proseTitle')}</summary>
        <div class="faint crit-help">${t('oracle.proseHelp')}</div>
        <div class="docs"></div>
      </details></div>`;
}

function wireShell(o) {
  o.pb.querySelector('.run').onclick = () => runOracle(o.c, o.pb.querySelector('.verdict-host'));
  o.pb.addEventListener('verdict', e => { o.last = e.detail; drawChecks(o); });
  o.pb.querySelector('.hx').onclick = () => toggleHelp(o, false);
  o.pb.querySelector('.setup').oninput = e => { o.spec.setup = e.target.value.trim(); o.save(); };
  o.pb.querySelector('.server').oninput = e => { o.spec.server = e.target.value.trim(); o.save(); };
  o.pb.querySelector('.acc').onclick = e => {
    const on = !e.currentTarget.classList.contains('on');
    e.currentTarget.classList.toggle('on', on);
    e.currentTarget.setAttribute('aria-checked', String(on));
    o.spec.requireAcceptance = on;
    o.save();
  };
  o.pb.querySelector('.addc').onclick = e => menu(e.currentTarget, [
    { h: t('oracle.whichKind') },
    ...Object.entries(CRIT_KINDS).map(([k, x]) => ({
      label: t(x.name), sub: t('common.eg') + ' ' + x.ex,
      f: () => addCheck(o, { ...x.make(), description: t(x.title) }, true)
    }))
  ], null, { alignRight: true });
}

function toggleHelp(o, open) {
  o.helpOpen = open ?? !o.helpOpen;
  pref('oracleHelp', o.helpOpen);
  const hb = o.pb.querySelector('.howto');
  if (hb) hb.hidden = !o.helpOpen;
}

function uid(o, title) {
  const base = (title || 'criterio').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'criterio';
  let id = base, n = 2;
  while (o.spec.checks.some(k => k.id === id)) id = base + '-' + n++;
  return id;
}

function addCheck(o, k, focus) {
  k.id = uid(o, k.description);
  o.spec.checks.push(k);
  o.save(); drawChecks(o);
  if (focus) setTimeout(() => {
    const card = $$('.checks .criterion', o.pb).pop();
    (card && (card.querySelector('.slot') || card.querySelector('.ctitle')))?.focus();
  }, 30);
}

function takeSuggestion(o, s) {
  if (s.setup && !o.spec.setup) {
    o.spec.setup = s.setup;
    const inp = o.pb.querySelector('.setup');
    if (inp) inp.value = s.setup;
  }
  if (s.server && !o.spec.server) {
    o.spec.server = s.server;
    const inp = o.pb.querySelector('.server');
    if (inp) inp.value = s.server;
  }
  addCheck(o, { ...s.check, description: t(s.title) });
}

function drawSugg(o) {
  const box = o.pb.querySelector('.sugg');
  if (!box) return;
  if (!o.sugg.length) { box.innerHTML = ''; return; }
  box.innerHTML = `<div class="oracle-sect"><span class="grow">${t('oracle.suggestions')}</span>${o.sugg.length > 1 ? `<button class="btn sm all">${t('oracle.addAll')}</button>` : ''}</div>${o.sugg.map((s, i) => `<div class="suggestion"><div class="grow"><b>${esc(t(s.title))}</b><div class="faint">${esc(s.why)}</div></div><button class="btn sm" data-i="${i}">${I.plus} ${t('oracle.add')}</button></div>`).join('')}`;
  $$('[data-i]', box).forEach(b => b.onclick = () => { takeSuggestion(o, o.sugg[+b.dataset.i]); o.sugg.splice(+b.dataset.i, 1); drawSugg(o); });
  box.querySelector('.all')?.addEventListener('click', () => { o.sugg.forEach(s => takeSuggestion(o, s)); o.sugg = []; drawSugg(o); });
}

const slot = (f, val, ph, mono = true) => `<input class="slot${mono ? ' mono' : ''}" data-f="${f}" value="${esc(val ?? '')}" placeholder="${esc(ph)}" spellcheck="false">`;

function sentence(k) {
  const x = k.expect || (k.expect = {});
  const extraS = x.contains != null || k._more ? ` ${t(k.kind === 'command' ? 'crit.andOutputContains' : 'crit.andResponseContains')} ${slot('contains', x.contains, t('crit.someText'), false)}` : '';
  if (k.kind === 'command') return `${t('crit.run')} ${slot('run', k.run, 'npm test')} ${t('crit.expectNoError')}${extraS}`;
  if (k.kind === 'http') return `${t('crit.open')} ${slot('url', k.url, 'http://localhost:3000/')} ${t('crit.expectResponse')} ${slot('status', x.status || 200, '200')}${extraS}`;
  if (k.kind === 'browser') return `${t('crit.open')} ${slot('url', k.url, 'http://localhost:3000/')} ${t('crit.browser.expect')} ${slot('selector', x.selector, '#root > *')}${extraS}`;
  if (k.kind === 'maxLines') return `${t('crit.noFile')} ${slot('glob', k.glob, '**/*.ts')} ${t('crit.exceeds')} ${slot('max', k.max || 400, '400')} ${t('common.lines')}`;
  const mode = x.exists === false ? 'absent' : x.contains != null ? 'contains' : 'exists';
  return `${t('crit.theFile')} ${slot('path', k.path, 'README.md')} <span class="chip fmode">${t({ exists: 'crit.exists', absent: 'crit.absent', contains: 'crit.contains' }[mode])} ${I.caret}</span>${mode === 'contains' ? ' ' + slot('contains', x.contains, t('crit.someText'), false) : ''}`;
}

function bindCriterion(o, k, i, card) {
  card.querySelector('.ctitle').oninput = e => { k.description = e.target.value; o.save(); };
  $$('.slot', card).forEach(inp => inp.oninput = () => {
    const f = inp.dataset.f, val = inp.value;
    k.expect = k.expect || {};
    if (f === 'contains') { if (val) k.expect.contains = val; else delete k.expect.contains; k._more = true; }
    else if (f === 'selector') { if (val) k.expect.selector = val; else delete k.expect.selector; }
    else if (f === 'status') k.expect.status = parseInt(val) || 200;
    else if (f === 'max') k.max = parseInt(val) || 400;
    else k[f] = val.trim();
    o.save();
  });
  card.querySelector('.q').onclick = () => {
    const hp = card.querySelector('.crit-help');
    hp.hidden = !hp.hidden;
  };
  const fm = card.querySelector('.fmode');
  if (fm) fm.onclick = e => menu(e.currentTarget, [['exists', 'crit.exists'], ['absent', 'crit.absent'], ['contains', 'crit.containsText']].map(([m, l]) => ({
    label: t(l), f: () => { k.expect = m === 'absent' ? { exists: false } : m === 'contains' ? { contains: '' } : {}; o.save(); drawChecks(o); }
  })));
  card.querySelector('.more').onclick = e => menu(e.currentTarget, [
    (k.kind === 'command' || k.kind === 'http') && k.expect?.contains == null && { label: t(k.kind === 'command' ? 'crit.requireOutputText' : 'crit.requireResponseText'), f: () => { k._more = true; k.expect.contains = ''; drawChecks(o); } },
    { label: t('common.duplicate'), f: () => addCheck(o, JSON.parse(JSON.stringify({ ...k, description: (k.description || k.id) + ' ' + t('common.copyTag') }))) },
    { label: t('crit.explainKind'), f: () => card.querySelector('.q').click() },
    { sep: 1 },
    { label: t('common.remove'), danger: 1, f: () => { o.spec.checks.splice(i, 1); o.save(); drawChecks(o); } },
  ].filter(Boolean), null, { alignRight: true });
}

function drawChecks(o) {
  const box = o.pb.querySelector('.checks');
  if (!box) return;
  const res = {};
  (o.last?.checks || []).forEach(r => res[r.id] = r);
  box.innerHTML = o.spec.checks.length ? '' : `<div class="empty">${t('oracle.noCriteria')}</div>`;
  o.spec.checks.forEach((k, i) => {
    const r = res[k.id], kind = CRIT_KINDS[k.kind] || CRIT_KINDS.command;
    const card = h(`<div class="criterion"><div class="ch"><span class="crit-state ${r ? (r.passed ? 'ok' : 'bad') : ''}" title="${esc(r ? r.detail : t('crit.notCheckedYet'))}">${r ? (r.passed ? '✓' : '✗') : '○'}</span><input class="ctitle" value="${esc(k.description || k.id)}" placeholder="${esc(t('crit.namePlaceholder'))}" spellcheck="false" aria-label="${esc(t('crit.namePlaceholder'))}"><span class="faint ckind">${esc(t(kind.name))}</span><button class="icon-btn q" aria-label="${esc(t('crit.whatDoes'))}">?</button><button class="icon-btn more" aria-label="${esc(t('common.more'))}">⋯</button></div><div class="crit-sentence">${sentence(k)}</div><div class="crit-help faint" hidden>${esc(t(kind.help))}</div>${r && !r.passed ? `<div class="crit-result">${esc(r.detail)}</div>` : ''}</div>`);
    bindCriterion(o, k, i, card);
    box.appendChild(card);
  });
}

function drawDocs(o, files) {
  const docs = o.pb.querySelector('.docs');
  if (!docs) return;
  const prose = (files || []).filter(f => f.name !== 'checks.json');
  (prose.length ? prose : [{ name: 'ready.md', content: '' }]).forEach(f => {
    const d = h(`<div class="mb-5">${prose.length > 1 ? `<div class="faint mono mb-2">${esc(f.name)}</div>` : ''}<textarea class="doc-editor" placeholder="${esc(t('oracle.prosePlaceholder'))}">${esc(f.content)}</textarea><div class="row"><button class="btn sm ex">${t('oracle.insertExample')}</button><span class="faint st" aria-live="polite"></span></div></div>`);
    const ta = d.querySelector('textarea'), st = d.querySelector('.st');
    const put = async () => {
      await api('/api/oracle', { method: 'PUT', body: { name: f.name, content: ta.value }, root: o.c.root }).catch(() => { });
      st.textContent = t('common.saved');
      setTimeout(() => st.textContent = '', 1200);
    };
    ta.oninput = debounce(put, 500);
    d.querySelector('.ex').onclick = () => {
      ta.value = (ta.value.trim() ? ta.value.trimEnd() + '\n\n' : '') + t(ORACLE_EXAMPLE_KEY);
      put(); ta.focus();
    };
    docs.appendChild(d);
  });
}

async function loadSugg(o) {
  o.sugg = await api('/api/oracle/suggest', { root: o.c.root }).catch(() => []);
  if (!Array.isArray(o.sugg)) o.sugg = [];
  drawSugg(o);
}

async function drawOracle(o) {
  const [files, specR, hist] = await Promise.all([
    api('/api/oracle', { root: o.c.root }).catch(() => []),
    api('/api/oracle/spec', { root: o.c.root }).catch(() => ({})),
    api('/api/oracle/verdicts', { root: o.c.root }).catch(() => [])
  ]);
  o.spec = specR.spec || { checks: [] };
  o.spec.checks = o.spec.checks || [];
  oracleSpecs[o.c.root] = o.spec;
  o.last = hist[0];
  if (o.helpOpen === null || o.helpOpen === undefined) o.helpOpen = !o.spec.checks.length;
  o.pb.innerHTML = shellHTML(o, files);
  wireShell(o);
  drawVerdict(o.c, o.pb.querySelector('.verdict-host'), o.last);
  drawChecks(o); drawSugg(o); drawDocs(o, files);
  loadSugg(o);
}

export function mount(pb, extra, c) {
  extra.innerHTML = `<button class="icon-btn help" title="${esc(t('common.howItWorks'))}" aria-label="${esc(t('common.howItWorks'))}">?</button>`;
  const o = { pb, extra, c, spec: { checks: [] }, sugg: [], last: null, helpOpen: pref('oracleHelp') };
  o.save = debounce(() => api('/api/oracle/spec', { method: 'PUT', body: { spec: o.spec }, root: c.root }).then(() => loadSugg(o)).catch(() => { }), 400);
  extra.querySelector('.help').onclick = () => toggleHelp(o);
  o.draw = () => drawOracle(o);
  drawOracle(o);
  return {
    refresh: () => loadSugg(o),
    run: out => runOracle(c, out || pb.querySelector('.verdict-host'))
  };
}
