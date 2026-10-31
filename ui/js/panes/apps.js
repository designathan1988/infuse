'use strict';
// apps
// Open an external app in the terminal.

async function runAppInTerminal(id, install) {
  const r = await api('/api/apps/command', { body: { id, install: install ? '1' : '' }, root: '' });
  if (!r.ok) return toast(r.error);
  showPage(null); togglePane('terminal', true);
  setTimeout(() => paneCall('terminal', 'openAt', curDir(), r.command, r.name), 80);
}
