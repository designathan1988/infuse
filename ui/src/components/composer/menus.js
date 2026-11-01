// composer menus — plus/attachments, project folder, mode, model and effort.
import { t } from '../../core/i18n.js';
import { api } from '../../core/api.js';
import { cache, pref, liveOf, setSessionOpts, reloadSessions, ensureModels } from '../../core/store.js';
import { menu } from '../menu.js';
import { toast } from '../toast.js';
import { MODE_INFO, EFFORT_INFO, vopts } from './options.js';
import { attachFiles } from './attachments.js';
import { renderHome } from './effect.js';

export function plusMenu(ctx, anchor) {
  const P = ctx.P;
  menu(anchor, [
    { label: t('composer.addFiles'), f: () => attachFiles(ctx) },
    { label: t('composer.mentionFile'), d: '@', f: () => { const p = P.selectionStart; P.value = P.value.slice(0, p) + '@' + P.value.slice(p); P.selectionStart = P.selectionEnd = p + 1; P.focus(); ctx.ac.autocomplete(); } },
    { label: t('composer.slashCommands'), d: '/', f: () => { if (!P.value.startsWith('/')) P.value = '/' + P.value; P.focus(); ctx.ac.autocomplete(); } },
    { sep: 1 },
    { label: t('side.title'), d: 'Ctrl ;', f: () => import('../view.js').then(m => m.openSideChat(ctx.v)) },
  ], null, { above: true });
}

export function folderMenu(ctx, anchor) {
  const items = [
    { h: t('sidebar.projects') },
    ...cache.projects.get().map(p => ({
      label: p.label || p.root.replace(/^.*[\\/]/, ''), d: p.root, check: p.root === ctx.v.draft.root,
      f: () => {
        ctx.v.draft.root = p.root; pref('lastRoot', p.root);
        renderHome(ctx);
        api('/api/projectmode', { root: p.root }).then(r => { if (r.mode) { ctx.v.draft.mode = r.mode; ctx.renderUnder(); } }).catch(() => { });
        ctx.renderUnder();
      }
    })),
    { sep: 1 },
    { label: t('env.openFolder'), d: t('env.explorer'), f: () => pickFolder(ctx) }
  ];
  menu(anchor, items, null, { above: true });
}

export async function pickFolder(ctx) {
  const p = window.infuse ? await infuse.call('pickFolder') : (await api('/api/dialog/folder', { body: {}, root: '' }).catch(() => null))?.path;
  if (p) {
    await reloadSessions();
    ctx.v.draft.root = p; pref('lastRoot', p);
    renderHome(ctx);
    ctx.renderUnder();
  }
}

export function modeMenu(ctx, anchor) {
  const o = vopts(ctx.v);
  menu(anchor, Object.entries(MODE_INFO).map(([k, [l, d]]) => ({
    label: t(l), sub: t(d), check: o.mode === k,
    f: () => {
      if (ctx.v.sid) setSessionOpts(liveOf(ctx.v.sid), { mode: k }); else { ctx.v.draft.mode = k; ctx.renderUnder(); }
      toast(t('mode.changedTo') + t(l));
    }
  })), null, { above: true, numbers: true });
}

export async function modelMenu(ctx, anchor, effortOnly) {
  const o = vopts(ctx.v);
  const effort = [
    { h: t('model.effort') },
    ...Object.entries(EFFORT_INFO).map(([k, l]) => ({
      label: t(l), check: o.effort === k,
      sub: t({ low: 'model.effortLow', medium: 'model.effortMedium', high: 'model.effortHigh', max: 'model.effortMax' }[k]),
      f: () => { if (ctx.v.sid) setSessionOpts(liveOf(ctx.v.sid), { effort: k }); else { ctx.v.draft.effort = k; ctx.renderUnder(); } }
    }))
  ];
  if (effortOnly) return menu(anchor, effort, null, { above: true, numbers: true });
  await ensureModels();
  const models = cache.models.get();
  const byProv = {};
  for (const m of models) (byProv[m.provider] = byProv[m.provider] || { label: m.label, items: [] }).items.push(m);
  const items = [];
  for (const [pid, g] of Object.entries(byProv)) {
    items.push({ h: g.label });
    g.items.slice(0, 40).forEach(m => items.push({
      label: m.model, check: o.model === m.model && o.provider === pid,
      f: () => { if (ctx.v.sid) setSessionOpts(liveOf(ctx.v.sid), { provider: pid, model: m.model }); else { ctx.v.draft.provider = pid; ctx.v.draft.model = m.model; ctx.renderUnder(); } }
    }));
  }
  if (!items.length) items.push({ label: t('model.connectProvider'), f: () => import('../../pages/settings.js').then(p => p.openSettings('providers')) });
  items.push({ sep: 1 }, ...effort, { sep: 1 }, { label: t('model.manage'), f: () => import('../../pages/settings.js').then(p => p.openSettings('providers')) });
  menu(anchor, items, null, { above: true });
}

