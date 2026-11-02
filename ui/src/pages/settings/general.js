// settings/general — language, theme and the application's own behavior. The agent's policy switches
// live on the Rules page, where the rest of the policy is.
import { h } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { pref } from '../../core/store.js';
import { setRow, toggleEl, selectEl, inputEl } from '../shell.js';

export function renderGeneral({ el, s, save }) {
  const applyTheme = () => import('../../app.js').then(m => m.applyTheme());
  el.append(h(`<h1>${t('settings.general')}</h1>`), h(`<p class="lead">${t('settings.appliesOnChange')}</p>`));
  el.append(setRow('settings.language', 'settings.languageDesc', selectEl([['pt-BR', 'Português (Brasil)'], ['en', 'English']], s.language === 'en' ? 'en' : 'pt-BR', v => save({ language: v }).then(() => location.reload()))));
  el.append(setRow('settings.theme', 'settings.themeDesc', selectEl([['system', 'settings.themeSystem'], ['light', 'settings.themeLight'], ['dark', 'settings.themeDark']], s.theme || 'system', v => save({ theme: v }).then(applyTheme))));
  el.append(setRow('settings.viewMode', 'settings.viewModeDesc', selectEl([['normal', 'view.normal'], ['thinking', 'view.thinking'], ['verbose', 'view.verbose']], pref('viewMode'), v => pref('viewMode', v))));
  el.append(h(`<h2>${t('settings.behavior')}</h2>`), h(`<p class="lead settings-lede">${t('settings.behaviorDesc')}</p>`));
  el.append(setRow('settings.promptSuggestions', 'settings.promptSuggestionsDesc', toggleEl(s.promptSuggestions !== false, v => save({ promptSuggestions: v }))));
  el.append(setRow('settings.notify', 'settings.notifyDesc', toggleEl(s.notify !== false, v => save({ notify: v }))));
  el.append(setRow('settings.trayIcon', 'settings.trayIconDesc', toggleEl(!s.hideTray, v => save({ hideTray: !v }))));
  el.append(setRow('settings.closeToTray', 'settings.closeToTrayDesc', toggleEl(!!s.closeToTray, v => save({ closeToTray: v }))));
  el.append(setRow('settings.vimMode', 'settings.vimModeDesc', toggleEl(!!s.vim, v => save({ vim: v }))));
  const openPolicy = () => import('../rules.js').then(p => p.openRules('writes'));
  const btn = h(`<button class="btn sm line">${t('settings.openPolicy')}</button>`);
  btn.onclick = openPolicy;
  el.append(setRow('settings.agentProtocol', 'settings.agentProtocolDesc', btn));
}
