// settings/about — version, build identity and the shortcut hint.
import { h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { cache } from '../../core/store.js';

export function renderAbout({ el }) {
  el.append(h('<h1>infuse</h1>'), h(`<p class="lead">${t('settings.version')} ${esc(cache.version || '')}</p>`), h(`<p class="faint">${t('palette.shortcuts')}: <span class="kbd">Ctrl /</span></p>`));
}
