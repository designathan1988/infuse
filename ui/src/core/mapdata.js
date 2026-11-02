// mapdata — how the panes read /api/map. No DOM here, so scripts/check-ui.mjs can exercise it.

// Every file with what it offers and who needs it; the most depended-on first, then the largest.
export function fileRows(m) {
  return Object.entries(m?.files || {}).map(([path, f]) => ({
    path, lines: f.lines || 0,
    exports: f.exports || [], dependents: f.dependents || [],
    nExports: (f.exports || []).length, nDependents: (f.dependents || []).length,
  })).sort((a, b) => b.nDependents - a.nDependents || b.lines - a.lines || a.path.localeCompare(b.path));
}

// The files others lean on most: where a change reaches furthest.
export const mostUsed = (m, limit = 5) => fileRows(m).filter(r => r.nDependents > 0).slice(0, limit);

// path → what the Files pane marks on the tree. Only files with something to say are listed.
export function fileOverlay(m) {
  const out = {};
  for (const r of fileRows(m)) if (r.nDependents) out[r.path] = { dependents: r.nDependents, lines: r.lines };
  for (const h of m?.hotspots || []) out[h.path] = { dependents: out[h.path]?.dependents || 0, lines: h.lines, hotspot: true };
  return out;
}
