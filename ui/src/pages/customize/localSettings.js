// customize/localSettings — the layers and the form of .infuse/settings.local.json, with the project's
// environment variables. The server never sends a saved value; this screen never holds one.
import { h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { I } from '../../core/icons.js';
import { api } from '../../core/api.js';
import { cache } from '../../core/store.js';
import { setRow, selectEl, inputEl } from '../shell.js';
import { toast } from '../../components/toast.js';
import { localValues, localPayload } from './localValues.js';

const LAYER_ORDER = ['user', 'project', 'local', 'managed'];
const MODES = [['', 'local.inherit'], ['plan', 'mode.plan'], ['default', 'mode.ask'], ['acceptEdits', 'mode.acceptEdits'], ['bypass', 'mode.bypass']];
const PERMS = [['', 'local.inherit'], ['ask', 'perm.ask'], ['allow', 'perm.allow'], ['deny', 'perm.deny']];

const loadLayers = root => api('/api/settings/layers', { root });
const putLocal = (root, settings) => api('/api/settings/local', { method: 'PUT', body: { settings }, root }).catch(e => ({ error: String(e) }));

// One change to the local file, keeping everything else in it.
export async function patchLocalSettings(root, patch) {
  const v = localValues(await loadLayers(root));
  return putLocal(root, { ...localPayload(v), ...patch });
}

function layerRow(l) {
  const vars = Object.keys(l.settings?.env || {}).length;
  const facts = [l.settings?.model, l.settings?.outputStyle, vars && t('local.varsCount', { count: vars })].filter(Boolean).map(esc).join(' · ');
  return `<div class="layer-row"><span class="conn-dot ${l.exists ? 'ok' : ''}"></span><b>${t('local.layer.' + l.name)}</b><span class="faint">${l.exists ? facts : t('local.missing')}</span><span class="grow"></span><span class="mono faint trunc layer-file" title="${esc(l.file)}">${esc(l.file)}</span></div>`;
}

function envRow(v, row, redraw) {
  const r = h(`<div class="env-row"><input class="slot mono en" placeholder="${esc(t('local.envName'))}" aria-label="${esc(t('local.envName'))}" value="${esc(row.name)}" spellcheck="false" ${row.stored ? 'readonly' : ''}><input class="slot mono ev" type="password" autocomplete="new-password" placeholder="${esc(t(row.stored ? 'local.envKept' : 'local.envValue'))}" aria-label="${esc(t('local.envValue'))}"><button class="icon-btn del" aria-label="${esc(t('common.remove'))}">${I.trash}</button></div>`);
  r.querySelector('.en').oninput = e => { row.name = e.target.value; };
  r.querySelector('.ev').oninput = e => { row.value = e.target.value; };
  r.querySelector('.del').onclick = () => { v.env.splice(v.env.indexOf(row), 1); redraw(); };
  return r;
}

function envBlock(v) {
  const box = h(`<div><h2>${t('local.env')}</h2><p class="faint field-note">${t('local.envDesc')}</p><div class="env-list"></div><button class="btn sm eadd">${I.plus} ${t('local.envAdd')}</button></div>`);
  const list = box.querySelector('.env-list');
  const draw = () => {
    list.innerHTML = v.env.length ? '' : `<div class="faint">${t('local.envNone')}</div>`;
    for (const row of v.env) list.append(envRow(v, row, draw));
  };
  box.querySelector('.eadd').onclick = () => { v.env.push({ name: '', value: '', stored: false }); draw(); list.querySelector('.env-row:last-child .en').focus(); };
  draw();
  return box;
}

function formBlock(v) {
  const box = h(`<div><h2>${t('local.edit')}</h2></div>`);
  box.append(setRow('local.model', 'local.modelDesc', inputEl(v.model, x => { v.model = x; })));
  box.append(setRow('local.mode', 'local.modeDesc', selectEl(MODES, v.mode, x => { v.mode = x; })));
  box.append(setRow('local.protected', 'local.protectedDesc', inputEl(v.protectedPaths, x => { v.protectedPaths = x; })));
  box.append(h(`<h3 class="local-sub">${t('local.permissions')}</h3>`));
  for (const tool of Object.keys(cache.settings.get().permissions || {}).sort()) {
    box.append(setRow(tool, '', selectEl(PERMS, v.permissions[tool] || '', x => { v.permissions[tool] = x; })));
  }
  return box;
}

export async function localSection(el, root, reopen) {
  el.append(h(`<h1>${t('cust.local')}</h1>`), h(`<p class="lead">${t('cust.localDesc')}</p>`));
  const layers = await loadLayers(root).catch(e => { toast(String(e)); return []; });
  const rows = LAYER_ORDER.map(n => layers.find(l => l.name === n)).filter(Boolean).map(layerRow).join('');
  el.append(h(`<div><h2>${t('local.layers')}</h2><p class="faint field-note">${t('local.layersDesc')}</p><div class="layer-list">${rows}</div></div>`));
  const v = localValues(layers);
  el.append(formBlock(v), envBlock(v));
  const save = h(`<div class="row local-save"><span class="grow"></span><button class="btn pri sv">${t('common.save')}</button></div>`);
  save.querySelector('.sv').onclick = async () => {
    document.activeElement?.blur(); // a text field commits on change
    const r = await putLocal(root, localPayload(v));
    if (r.error) return toast(r.error);
    toast(t('local.saved'));
    reopen();
  };
  el.append(save);
}
