// open-in — the editor/explorer list shared by the view header and the sidebar menus.
import { t } from '../core/i18n.js';
import { api } from '../core/api.js';
import { cache } from '../core/store.js';

export const openInItems = path => [
  { label: t('view.showInExplorer'), f: () => api('/api/open', { body: { path, app: 'explorer' }, root: '' }) },
  ...cache.editors.get().map(e => ({ label: t('view.openInEditor') + e.name, f: () => api('/api/open', { body: { path, app: e.id }, root: '' }) })),
];
