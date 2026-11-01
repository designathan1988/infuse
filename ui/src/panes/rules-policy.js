// rules-policy — the behaviour policy section of the Rules pane: one typed document per scope that
// shapes how the AI works (method, architecture, conventions, done, permissions, prohibitions).
import { h, esc, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { api } from '../core/api.js';
import { setRow, toggleEl, selectEl, inputEl } from '../pages/shell.js';

const TOOL_NAMES = ['Bash', 'Write', 'Edit', 'MultiEdit', 'WebFetch', 'Agent', 'MCP'];
const MODES = [['default', 'policy.modeDefault'], ['acceptEdits', 'policy.modeAccept'], ['plan', 'policy.modePlan'], ['bypass', 'policy.modeBypass']];
const ACTIONS = ['commit', 'push', 'install'];

function normalize(raw) {
  const p = {
    permissions: { mode: '', tools: {}, protectedPaths: [] },
    prohibitions: { commands: [], paths: [], tools: [], actions: [] },
    method: { plan: true, diagnosis: true, docsBeforeUse: true, readingCoverage: true, askBudget: 6 },
    architecture: { maxFileLines: 400, enforce: true },
    conventions: { enforce: true },
    done: { review: true, requireContract: true },
  };
  const r = raw || {};
  for (const g of ['permissions', 'prohibitions', 'method', 'architecture', 'conventions', 'done']) {
    Object.assign(p[g], r[g] || {});
  }
  p.permissions.tools = Object.assign({}, r.permissions?.tools || {});
  p.permissions.protectedPaths = [...(r.permissions?.protectedPaths || [])];
  for (const g of ['prohibitions']) {
    for (const k of ['commands', 'paths', 'tools', 'actions']) p[g][k] = [...(r[g]?.[k] || [])];
  }
  return p;
}

const lines = text => text.split('\n').map(x => x.trim()).filter(Boolean);
const setIn = (o, path, v) => { const [g, k] = path.split('.'); o[g][k] = v; };

function head(key) {
  return h(`<div class="rules-help-title"><b>${esc(t(key))}</b></div>`);
}

function toggleRow(s, group, field, labelKey, descKey) {
  const key = `${group}.${field}`;
  const sw = toggleEl(!!s.policy[group][field], v => { setIn(s.policy, key, v); s.save(); });
  sw.dataset.k = key;
  return setRow(labelKey, descKey, sw);
}

function numberRow(s, group, field, labelKey, descKey) {
  const key = `${group}.${field}`;
  const input = inputEl(s.policy[group][field], v => { setIn(s.policy, key, v); s.save(); }, 'number');
  input.dataset.k = key;
  return setRow(labelKey, descKey, input);
}

function selectRow(s, path, labelKey, descKey, options) {
  const [group, field] = path.split('.');
  const sel = selectEl(options, s.policy[group][field] ?? '', v => { setIn(s.policy, path, v); s.save(); });
  sel.dataset.k = path;
  return setRow(labelKey, descKey, sel);
}

function areaField(s, group, field, labelKey, descKey, kind = 'list') {
  const key = `${group}.${field}`;
  const box = h(`<div class="field-my"><div class="l"><b>${esc(t(labelKey))}</b><div class="faint">${esc(t(descKey))}</div></div><textarea class="doc-editor policy-textarea" data-k="${esc(key)}" aria-label="${esc(t(labelKey))}"></textarea></div>`);
  const ta = box.querySelector('textarea');
  ta.value = (s.policy[group][field] || []).join('\n');
  ta.oninput = () => {
    const values = lines(ta.value);
    s.policy[group][field] = kind === 'actions' ? values.map(v => v.toLowerCase()) : values;
    s.save();
  };
  return box;
}

function methodRows(s) {
  return [
    toggleRow(s, 'method', 'plan', 'policy.plan', 'policy.planDesc'),
    toggleRow(s, 'method', 'diagnosis', 'policy.diagnosis', 'policy.diagnosisDesc'),
    toggleRow(s, 'method', 'docsBeforeUse', 'policy.docs', 'policy.docsDesc'),
    toggleRow(s, 'method', 'readingCoverage', 'policy.reading', 'policy.readingDesc'),
    numberRow(s, 'method', 'askBudget', 'policy.askBudget', 'policy.askBudgetDesc'),
  ];
}

function architectureRows(s) {
  return [
    toggleRow(s, 'architecture', 'enforce', 'policy.enforceArchitecture', 'policy.enforceArchitectureDesc'),
    numberRow(s, 'architecture', 'maxFileLines', 'policy.maxFileLines', 'policy.maxFileLinesDesc'),
  ];
}

function permissionRows(s) {
  const rows = [selectRow(s, 'permissions.mode', 'policy.mode', 'policy.modeDesc', MODES)];
  for (const name of TOOL_NAMES) {
    const key = `permissions.tools.${name}`;
    const sel = selectEl([['ask', 'policy.toolAsk'], ['allow', 'policy.toolAllow'], ['deny', 'policy.toolDeny']], s.policy.permissions.tools[name] || 'ask', v => {
      s.policy.permissions.tools[name] = v;
      s.save();
    });
    sel.dataset.k = key;
    rows.push(setRow(name, 'policy.toolsDesc', sel));
  }
  rows.push(areaField(s, 'permissions', 'protectedPaths', 'policy.protectedPaths', 'policy.protectedPathsDesc'));
  return rows;
}

function prohibitionRows(s) {
  return [
    areaField(s, 'prohibitions', 'commands', 'policy.commands', 'policy.commandsDesc'),
    areaField(s, 'prohibitions', 'paths', 'policy.paths', 'policy.pathsDesc'),
    areaField(s, 'prohibitions', 'tools', 'policy.prohibitedTools', 'policy.prohibitedToolsDesc'),
    areaField(s, 'prohibitions', 'actions', 'policy.actions', 'policy.actionsDesc', 'actions'),
  ];
}

function validate(p) {
  if (p.permissions.mode && !MODES.some(([v]) => v === p.permissions.mode)) return 'policy.errorMode';
  const budget = Number(p.method.askBudget);
  if (!(budget >= 1 && budget <= 20)) return 'policy.errorBudget';
  const max = Number(p.architecture.maxFileLines);
  if (max !== -1 && !(max >= 20 && max <= 5000)) return 'policy.errorMaxLines';
  if ((p.prohibitions.actions || []).some(a => !ACTIONS.includes(a))) return 'policy.errorActions';
  return '';
}

async function save(s) {
  const problem = validate(s.policy);
  const error = s.host.querySelector('.policy-error');
  if (problem) {
    error.textContent = t(problem);
    error.hidden = false;
    s.status.textContent = t('policy.saveFailed');
    return;
  }
  error.hidden = true;
  s.status.textContent = t('policy.saving');
  const res = await api(`/api/policy?scope=${s.scope}`, { method: 'PUT', body: s.policy, root: s.root }).catch(e => ({ error: String(e) }));
  if (res.error) {
    error.textContent = res.error;
    error.hidden = false;
    s.status.textContent = t('policy.saveFailed');
    return;
  }
  if (res.file) s.host.querySelector('.policy-file').textContent = res.file;
  s.status.textContent = t('policy.saved');
}

// mountPolicy draws the section for the pane's current scope and re-draws with it.
export async function mountPolicy(host, c, scope) {
  if (!host) return;
  const res = await api(`/api/policy?scope=${scope === 'global' ? 'global' : 'project'}`, { root: c.root }).catch(e => ({ error: String(e) }));
  host.innerHTML = '';
  if (res.error) {
    host.append(h(`<p class="faint field-note">${esc(t('policy.unavailable'))}</p>`));
    return;
  }
  const s = { c, scope, root: c.root, host, policy: normalize(res.policy), file: res.file || '' };
  host.append(head('policy.title'));
  host.append(h(`<div class="row mb-4"><span class="faint mono trunc grow policy-file">${esc(s.file)}</span><span class="faint policy-status" aria-live="polite"></span></div>`));
  host.append(h(`<p class="faint field-note"><b>${esc(t('policy.noteTitle'))}</b> ${esc(t('policy.note'))}</p>`));
  s.status = host.querySelector('.policy-status');
  const sections = [
    ['policy.method', methodRows],
    ['policy.architecture', architectureRows],
    ['policy.conventions', s2 => [toggleRow(s2, 'conventions', 'enforce', 'policy.enforceConventions', 'policy.enforceConventionsDesc')]],
    ['policy.done', s2 => [toggleRow(s2, 'done', 'review', 'policy.review', 'policy.reviewDesc'), toggleRow(s2, 'done', 'requireContract', 'policy.contract', 'policy.contractDesc')]],
    ['policy.permissions', permissionRows],
    ['policy.prohibitions', prohibitionRows],
  ];
  for (const [title, build] of sections) {
    host.append(head(title));
    for (const row of build(s)) host.append(row);
  }
  const error = h('<p class="faint field-note policy-error" hidden></p>');
  host.append(error);
  s.save = debounce(() => save(s), 450);
}
