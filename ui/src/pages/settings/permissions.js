// settings/permissions — default mode and per-tool policy. Protected paths and the write policy live
// on the Rules page.
import { h } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { setRow, selectEl } from '../shell.js';
import { MODE_INFO } from '../../components/composer.js';

export function renderPermissions({ el, s, save }) {
  el.append(h(`<h1>${t('settings.permissions')}</h1>`), h(`<p class="lead">${t('settings.permissionsDesc')}</p>`));
  el.append(setRow('settings.defaultMode', 'settings.defaultModeDesc', selectEl(Object.entries(MODE_INFO).map(([k, [l]]) => [k, l]), s.mode || 'default', v => save({ mode: v }))));
  const descriptions = { Bash: 'settings.permBashDesc', Write: 'settings.permWriteDesc', Edit: 'settings.permEditDesc', WebFetch: 'settings.permWebDesc' };
  for (const tool of ['Bash', 'Write', 'Edit', 'WebFetch']) {
    el.append(setRow(tool, descriptions[tool], selectEl([['ask', 'perm.ask'], ['allow', 'perm.allow'], ['deny', 'perm.deny']], (s.permissions || {})[tool] || 'ask', v => save({ permissions: { ...s.permissions, [tool]: v } }))));
  }
}
