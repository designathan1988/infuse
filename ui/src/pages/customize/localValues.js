// localValues — the local settings form as data. DOM-free, so the gate executes it against the payload
// of /api/settings/layers. A saved variable arrives as a mask and goes back as one unless it was retyped.
export const ENV_MASK = '…';

export const layerOf = (layers, name) => (layers || []).find(l => l.name === name) || { name, settings: {} };

export function localValues(layers) {
  const s = layerOf(layers, 'local').settings || {};
  return {
    settings: s,
    model: s.model || '', mode: s.mode || '', permissions: { ...s.permissions },
    protectedPaths: (s.protectedPaths || []).join(', '),
    env: Object.keys(s.env || {}).sort().map(name => ({ name, value: '', stored: true })),
  };
}

export function localPayload(v) {
  const env = {};
  for (const row of v.env) {
    const name = row.name.trim();
    if (name) env[name] = row.value || (row.stored ? ENV_MASK : '');
  }
  const permissions = Object.fromEntries(Object.entries(v.permissions).filter(([, p]) => p));
  return {
    ...v.settings, model: v.model.trim(), mode: v.mode, permissions, env,
    protectedPaths: v.protectedPaths.split(',').map(p => p.trim()).filter(Boolean),
  };
}
