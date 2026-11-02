// pages/rules — the unified Rules (policy) page: the project's architecture base (choose, create,
// edit), the behaviour policy, the filter rules, the write policy and the project's prohibitions, in
// one place.
import { h, esc } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { api } from '../core/api.js';
import { cache, projectLabel, curRoot, saveSettings, pref } from '../core/store.js';
import { pageShell, setRow, toggleEl, inputEl } from './shell.js';
import { WRITE_POLICY, ALWAYS_ON, writePolicyValues } from './writePolicy.js';
import { mountFilterRules } from '../components/policy/filterRules.js';
import { mountPolicy } from '../components/policy/behaviour.js';
import { toast } from '../components/toast.js';

const SECTIONS = [['base', 'policy.base'], ['behaviour', 'policy.title'], ['filter', 'policy.filter'], ['writes', 'policy.writes'], ['bans', 'policy.bans']];

export async function openRules(section = 'base') {
  const el = pageShell('rules', SECTIONS, section, openRules);
  const root = curRoot() || cache.projects.get()[0]?.root || '';
  if (!root) { el.append(h(`<div class="empty">${t('cust.needProject')}</div>`)); return; }
  el.append(h(`<p class="faint field-note">${esc(projectLabel(root))} · ${esc(root)}</p>`));
  if (section === 'base') return baseSection(el, root);
  if (section === 'behaviour') return behaviourSection(el, root);
  if (section === 'filter') return filterSection(el, root);
  if (section === 'writes') return writesSection(el, root);
  if (section === 'bans') return bansSection(el, root);
}

// ---------------------------------------------------------------- base: choose, create, edit

async function baseSection(el, root) {
  const { catalog = [], current = null, file = '', folders = [] } = await api('/api/base', { root }).catch(() => ({}));
  el.append(h(`<h1>${t('policy.base')}</h1>`), h(`<p class="lead">${t('policy.baseDesc')}</p>`));
  el.append(h(`<div class="doc-editor-box field-my">
      <div class="rules-help-title">${t('base.current')}</div>
      <p class="lead">${esc(current?.name || t('base.none'))}</p>
      <div class="faint mono">${esc(file)}</div>
      ${folders.length ? `<div class="faint mt-4">${esc(t('base.folders'))}: ${esc(folders.join(', '))}</div>` : ''}
    </div>`));
  el.append(h(`<div class="base-rows"></div>`));
  const rows = el.querySelector('.base-rows');
  for (const b of catalog) {
    rows.append(baseRow(root, b, current));
  }
  el.append(h(`<h2 class="mt-6">${t('policy.baseCustom')}</h2>`), h(`<p class="lead">${t('policy.baseCustomDesc')}</p>`));
  customBaseEditor(el, root, current, catalog);
  newProjectFromBase(el, catalog);
}

// A new project in a folder the user picks: the folders, the base document and the generated map.
function newProjectFromBase(el, catalog) {
  const options = catalog.map(b => `<option value="${esc(b.name)}">${esc(b.name)} — ${esc(t('base.desc.' + b.name))}</option>`).join('');
  const make = h(`<div class="doc-editor-box field-my mt-7 base-create">
      <div class="rules-help-title">${t('base.createTitle')}</div>
      <p class="lead">${t('base.createDesc')}</p>
      <div class="row mt-4"><input class="field-select name" placeholder="${esc(t('base.name'))}" aria-label="${esc(t('base.name'))}"><select class="field-select starter" aria-label="${esc(t('base.choose'))}">${options}</select><button class="btn line choose">${t('base.chooseFolder')}</button></div>
      <div class="faint status mt-3"></div>
    </div>`);
  el.append(make);
  make.querySelector('.choose').onclick = async () => {
    const pick = await api('/api/dialog/folder').catch(e => ({ error: String(e) }));
    if (pick.error) return toast(pick.error);
    if (!pick.path) return;
    const name = make.querySelector('.name').value.trim();
    const r = await api('/api/base/scaffold', { method: 'POST', body: { parent: pick.path, name, base: make.querySelector('.starter').value } }).catch(e => ({ error: String(e) }));
    if (r.error) return toast(r.error);
    make.querySelector('.status').textContent = t('base.created', { path: r.root });
    const projects = await api('/api/projects').catch(() => null);
    if (projects) cache.projects.set(projects);
  };
}

function baseRow(root, b, current) {
  const chosen = current?.name === b.name;
  const row = h(`<div class="base-row${chosen ? ' on' : ''}">
    <div class="top"><span class="nm">${esc(b.name)}</span>${chosen ? `<span class="tag ok">${t('policy.baseChosen')}</span>` : ''}<span class="grow"></span>
      <button class="btn sm pick">${chosen ? t('policy.baseRechoose') : t('policy.basePick')}</button>
      <button class="btn sm line create">${t('policy.baseCreate')}</button></div>
    <p class="faint">${esc(t('base.desc.' + b.name))}</p>
    <div class="faint mono trunc">${esc(b.folders.join(' · '))}</div>
    <div class="faint res" aria-live="polite"></div>
  </div>`);
  row.querySelector('.pick').onclick = async () => {
    const r = await api('/api/base', { method: 'PUT', body: { base: b.name }, root }).catch(e => ({ error: String(e) }));
    if (r.error) return toast(r.error);
    toast(t('policy.baseSaved'));
    openRules('base');
  };
  row.querySelector('.create').onclick = async () => {
    const out = row.querySelector('.res');
    out.textContent = t('policy.baseCreating');
    const r = await api('/api/base/scaffold', { body: { base: b.name }, root }).catch(e => ({ error: String(e) }));
    if (r.error || !r.ok) { out.textContent = r.error || 'error'; return; }
    out.textContent = t('policy.baseCreated');
    toast(t('policy.baseCreated'));
    openRules('base');
  };
  return row;
}

function customBaseEditor(el, root, current, catalog) {
  const isCustom = current && current.name && !catalog.some(b => b.name === current.name);
  const d = h(`<div class="doc-editor-box field-my">
    <div class="row"><input class="bname field-select" placeholder="${esc(t('policy.baseName'))}" value="${esc(isCustom ? current.name : '')}"><input class="bmax" type="number" min="0" value="${current?.maxFileLines || 400}" aria-label="${esc(t('settings.maxFileLines'))}"></div>
    <textarea class="bfolders mono" rows="5" placeholder="${esc(t('policy.baseFoldersPh'))}" aria-label="${esc(t('policy.baseFolders'))}">${esc(isCustom ? (current.folders || []).join('\n') : '')}</textarea>
    <div class="row"><input class="bdata field-select" placeholder="${esc(t('policy.baseDataDir'))}" value="${esc(isCustom ? current.dataDir || '' : '')}"><input class="btokens field-select" placeholder="${esc(t('policy.baseTokens'))}" value="${esc(isCustom ? current.tokens || '' : '')}"></div>
    <div class="row mt-4"><span class="grow"></span><button class="btn pri save">${t('common.save')}</button></div>
  </div>`);
  d.querySelector('.save').onclick = async () => {
    const folders = d.querySelector('.bfolders').value.split('\n').map(x => x.trim()).filter(Boolean);
    const r = await api('/api/base', { method: 'PUT', body: {
      name: d.querySelector('.bname').value.trim() || 'custom',
      folders,
      dataDir: d.querySelector('.bdata').value.trim(),
      tokens: d.querySelector('.btokens').value.trim(),
      maxFileLines: Number(d.querySelector('.bmax').value) || 400,
    }, root }).catch(e => ({ error: String(e) }));
    if (r.error || (r && r.ok === false)) return toast(r.error || 'error');
    toast(t('policy.baseSaved'));
    openRules('base');
  };
  el.append(d);
}

// ---------------------------------------------------------------- filter, writes, bans

// One document per scope: the project's, or the user's for every project.
async function behaviourSection(el, root) {
  const scope = pref('rulesScope') === 'global' ? 'global' : 'project';
  el.append(h(`<h1>${t('policy.title')}</h1>`));
  const seg = h(`<div class="seg-group mb-4" role="group">${[['project', 'rules.thisProject'], ['global', 'rules.allProjects']].map(([k, l]) => `<button class="${k === scope ? 'on' : ''}" data-scope="${k}">${t(l)}</button>`).join('')}</div>`);
  seg.onclick = e => {
    const b = e.target.closest('[data-scope]');
    if (b) { pref('rulesScope', b.dataset.scope); openRules('behaviour'); }
  };
  const box = h('<div class="policy-mount"></div>');
  el.append(seg, box);
  await mountPolicy(box, { root }, scope);
}

async function filterSection(el, root) {
  el.append(h(`<h1>${t('policy.filter')}</h1>`), h(`<p class="lead">${t('policy.filterDesc')}</p>`));
  const box = h('<div></div>');
  el.append(box);
  mountFilterRules(box, { root });
}

async function writesSection(el, root) {
  el.append(h(`<h1>${t('policy.writes')}</h1>`), h(`<p class="lead">${t('policy.writesDesc')}</p>`));
  const vals = writePolicyValues(await api('/api/settings/full', { root: '' }).catch(() => null));
  if (!vals) { el.append(h(`<div class="empty">${t('policy.loadFailed')}</div>`)); return; }
  const save = patch => saveSettings(patch).catch(e => toast(e.message));
  for (const [key, label, desc] of WRITE_POLICY) {
    el.append(setRow(label, desc, toggleEl(vals.toggles[key], v => save({ [key]: v }))));
  }
  for (const [label, desc] of ALWAYS_ON) {
    el.append(setRow(label, desc, h(`<span class="tag ok">${t('policy.alwaysOn')}</span>`)));
  }
  el.append(setRow('settings.maxFileLines', 'settings.maxFileLinesDesc', inputEl(vals.maxFileLines, v => save({ maxFileLines: v }), 'number')));
  el.append(setRow('settings.protectedPaths', 'settings.protectedPathsDesc', inputEl(vals.protectedPaths, v => save({ protectedPaths: v.split(',').map(x => x.trim()).filter(Boolean) }))));
}

const BAN_KIND = { tool: 'ban.kindTool', command: 'ban.kindCommand', path: 'ban.kindPath', action: 'ban.kindAction' };

async function bansSection(el, root) {
  el.append(h(`<h1>${t('policy.bans')}</h1>`), h(`<p class="lead">${t('policy.bansDesc')}</p>`));
  const { sessions } = await api('/api/bans', { root }).catch(() => ({ sessions: [] }));
  if (!sessions.length) { el.append(h(`<div class="empty">${t('policy.bansEmpty')}</div>`)); return; }
  for (const se of sessions) {
    el.append(h(`<h2 class="mt-6">${esc(se.title || se.session)}</h2>`));
    for (const b of se.bans) {
      const off = b.off ? ` <span class="chip">${t('policy.banLifted')}</span>` : '';
      el.append(h(`<div class="settings-row"><div class="l"><b>${esc(t(BAN_KIND[b.kind] || 'ban.kindAction'))}: ${esc(b.value)}</b>${b.quote ? `<span>“${esc(b.quote)}”</span>` : ''}</div><span class="grow"></span>${off}</div>`));
    }
  }
}
