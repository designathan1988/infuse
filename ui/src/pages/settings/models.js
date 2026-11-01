// settings/models — default model and effort, usage summary, and the editable price table.
import { $$, h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { money, ktok } from '../../core/fmt.js';
import { I } from '../../core/icons.js';
import { api } from '../../core/api.js';
import { cache, ensureModels, projectLabel } from '../../core/store.js';
import { setRow, selectEl, inputEl } from '../shell.js';

export async function renderModels({ el, s, save, open }) {
  el.append(h(`<h1>${t('settings.models')}</h1>`));
  await ensureModels();
  const models = cache.models.get();
  const opts = models.map(m => [m.provider + '|' + m.model, `${m.label} · ${m.model}`]);
  el.append(setRow('settings.defaultModel', 'settings.defaultModelDesc', selectEl(opts.length ? opts : [['', 'settings.connectFirst']], s.provider + '|' + s.model, v => { const [p, m] = v.split('|'); save({ provider: p, model: m }); })));
  el.append(setRow('settings.fastModel', 'settings.fastModelDesc', selectEl(opts.length ? opts : [['', '—']], s.provider + '|' + s.fastModel, v => save({ fastModel: v.split('|')[1] }))));
  el.append(setRow('settings.defaultEffort', 'settings.defaultEffortDesc', selectEl([['low', 'effort.low'], ['medium', 'effort.medium'], ['high', 'effort.high'], ['max', 'effort.max']], s.effort || 'medium', v => save({ effort: v }))));
  el.append(setRow('settings.maxTokens', 'settings.maxTokensDesc', inputEl(s.maxTokens, v => save({ maxTokens: v }), 'number')));
  el.append(setRow('settings.contextLimit', 'settings.contextLimitDesc', inputEl(s.contextLimit || '', v => save({ contextLimit: +v || 0 }), 'number')));
  el.append(setRow('settings.dailyLimit', 'settings.dailyLimitDesc', inputEl(s.dailyLimitUsd, v => save({ dailyLimitUsd: v }), 'number')));
  await renderUsage(el);
  renderPricing(el, s, save, open);
}

async function renderUsage(el) {
  const u = await api('/api/usage', { root: '' }).catch(() => ({}));
  el.append(h(`<h2>${t('usage.title')}</h2>`), h(`<div class="row settings-usage"><div><div class="faint">${t('usage.today')}</div><div class="big-number">${money(u.today)}</div></div><div><div class="faint">${t('usage.month')}</div><div class="big-number">${money(u.month)}</div></div><div><div class="faint">${t('usage.tokens')}</div><div class="big-number">${ktok(u.tokens?.input)} / ${ktok(u.tokens?.output)}</div></div></div>`));
  const proj = Object.entries(u.byProject || {}).sort((a, b) => b[1] - a[1]);
  if (proj.length) el.append(h(`<table class="price-table"><tr><th>${t('usage.project')}</th><th>${t('common.total')}</th></tr>${proj.map(([r, cst]) => `<tr><td>${esc(projectLabel(r))} <span class="faint">${esc(r)}</span></td><td class="mono">${money(cst)}</td></tr>`).join('')}</table>`));
  if ((u.byDay || []).length) el.append(h(`<table class="price-table mt-6"><tr><th>${t('usage.day')}</th><th>${t('common.total')}</th></tr>${u.byDay.slice(0, 14).map(d => `<tr><td>${esc(d.day)}</td><td class="mono">${money(d.cost)}</td></tr>`).join('')}</table>`));
  const sess = cache.sessions.get().filter(x => x.cost > 0).sort((a, b) => b.cost - a.cost).slice(0, 10);
  if (sess.length) el.append(h(`<table class="price-table mt-6"><tr><th>${t('usage.session')}</th><th>${t('common.total')}</th></tr>${sess.map(x => `<tr><td>${esc(x.title)} <span class="faint">${esc(projectLabel(x.root))}</span></td><td class="mono">${money(x.cost)}</td></tr>`).join('')}</table>`));
}

function renderPricing(el, s, save, open) {
  el.append(h(`<h2>${t('settings.priceTable')}</h2>`), h(`<p class="lead">${t('settings.priceTableDesc')}</p>`));
  const tbl = h(`<table class="price-table"><tr><th>${t('settings.modelPrefix')}</th><th>${t('usage.input')}</th><th>${t('usage.output')}</th><th></th></tr></table>`);
  const savePrices = () => {
    const pricing = {};
    $$('tr[data-k]', tbl).forEach(tr => pricing[tr.dataset.k] = [+tr.querySelector('[data-i="0"]').value, +tr.querySelector('[data-i="1"]').value]);
    return save({ pricing });
  };
  Object.entries(s.pricing || {}).sort().forEach(([k, v]) => {
    const tr = h(`<tr data-k="${esc(k)}"><td class="mono">${esc(k)}</td><td><input data-i="0" value="${v[0]}" aria-label="${t('usage.input')}"></td><td><input data-i="1" value="${v[1]}" aria-label="${t('usage.output')}"></td><td><button class="icon-btn icon-22" aria-label="${esc(t('common.remove'))}">${I.close}</button></td></tr>`);
    $$('input', tr).forEach(i => i.onchange = savePrices);
    tr.querySelector('button').onclick = () => { tr.remove(); savePrices(); };
    tbl.appendChild(tr);
  });
  appendPricingRow(tbl, s, save, open);
  el.append(tbl);
}

function appendPricingRow(tbl, s, save, open) {
  const add = h(`<tr><td><input class="np w-full" placeholder="${esc(t('settings.newPrefix'))}"></td><td><input class="ni" placeholder="0"></td><td><input class="no" placeholder="0"></td><td></td></tr>`);
  $$('input', add).forEach(i => i.onkeydown = async e => {
    if (e.key === 'Enter' && add.querySelector('.np').value.trim()) {
      await save({ pricing: { ...s.pricing, [add.querySelector('.np').value.trim()]: [+add.querySelector('.ni').value || 0, +add.querySelector('.no').value || 0] } });
      open('models');
    }
  });
  tbl.appendChild(add);
}
