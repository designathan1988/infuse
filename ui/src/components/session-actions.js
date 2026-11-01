// session-actions — the one place that archives, deletes, renames, pins or unarchives a session.
// Both the sidebar and the view header call these; no feature duplicates the server contract.
import { api } from '../core/api.js';
import { t } from '../core/i18n.js';
import { ui, cache, closeLive, liveOf, reloadSessions } from '../core/store.js';
import { toast } from './toast.js';

export function closeSessionViews(id, root) {
  let open = false;
  for (const v of ui.views.get()) {
    if (v.sid !== id) continue;
    v.sid = '';
    v.draft.root = root;
    open = true;
  }
  if (open) {
    closeLive(id);
    ui.views.set([...ui.views.get()]);
  }
}

export async function archiveSession(L, force = false) {
  const r = await api(`/api/session/${L.id}/archive`, { body: { force }, root: L.root }).catch(e => ({ error: String(e) }));
  if (r.dirty) {
    if (confirm(t('sidebar.archiveDirtyConfirm'))) return archiveSession(L, true);
    return false;
  }
  if (r.error) { toast(r.error); return false; }
  closeSessionViews(L.id, L.root);
  await reloadSessions();
  toast(t('sidebar.archived'));
  return true;
}

export async function deleteSession(L) {
  if (!confirm(t('sidebar.deleteConfirm'))) return false;
  const r = await api(`/api/sessions/${L.id}`, { method: 'DELETE', root: L.root }).catch(e => ({ error: String(e) }));
  if (r?.error) { toast(r.error); return false; }
  closeSessionViews(L.id, L.root);
  await reloadSessions();
  toast(t('sidebar.deleted'));
  return true;
}

export async function renameSession(L, title) {
  const r = await api(`/api/sessions/${L.id}`, { method: 'PUT', body: { title }, root: L.root }).catch(e => ({ error: String(e) }));
  if (r?.error) { toast(r.error); return false; }
  const live = liveOf(L.id);
  if (live?.snap.get()) live.snap.set({ ...live.snap.get(), title });
  await reloadSessions();
  return true;
}

export async function setPinned(L, pinned) {
  await api(`/api/session/${L.id}/pin`, { body: { pinned }, root: L.root }).catch(() => { });
  await reloadSessions();
}

export async function unarchiveSession(L) {
  await api(`/api/session/${L.id}/unarchive`, { body: {}, root: L.root }).catch(() => { });
  cache.archived.set(await api('/api/sessions/all?archived=1', { root: '' }).catch(() => []));
  await reloadSessions();
}
