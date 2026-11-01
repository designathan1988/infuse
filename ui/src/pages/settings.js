// settings — the page shell and one renderer per section; each section owns its controls and saves.
import { cache, reloadSettings, saveSettings } from '../core/store.js';
import { pageShell } from './shell.js';
import { toast } from '../components/toast.js';
import { renderGeneral } from './settings/general.js';
import { renderProviders } from './settings/providers.js';
import { renderModels } from './settings/models.js';
import { renderPermissions } from './settings/permissions.js';
import { renderCode } from './settings/code.js';
import { renderApps } from './settings/apps.js';
import { renderIntegration } from './settings/integration.js';
import { renderAbout } from './settings/about.js';

const SECTIONS = [
  ['general', 'settings.general'], ['providers', 'settings.providers'], ['models', 'settings.models'],
  ['permissions', 'settings.permissions'], ['code', 'settings.code'], ['apps', 'settings.apps'],
  ['integration', 'settings.integration'], ['about', 'settings.about']
];

const RENDER = {
  general: renderGeneral, providers: renderProviders, models: renderModels,
  permissions: renderPermissions, code: renderCode, apps: renderApps,
  integration: renderIntegration, about: renderAbout
};

export async function openSettings(section = 'general') {
  await reloadSettings();
  const el = pageShell('settings', SECTIONS, section, openSettings);
  const s = cache.settings.get();
  const save = patch => saveSettings(patch).catch(e => toast(String(e)));
  const render = RENDER[section];
  if (render) await render({ el, s, save, open: openSettings });
}
