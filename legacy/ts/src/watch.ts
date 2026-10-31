import chokidar from "chokidar";
import { statSync } from "node:fs";
import { relative, sep } from "node:path";
import { journal } from "./journal.js";
import { buildMap, writeMap } from "./map/index.js";

/**
 * Instant write watcher — the always-available half of the filesystem interposition.
 * Every write by any tool (native Write/Edit, apply_patch, write_file, or a shell command)
 * is journaled within milliseconds and the map is refreshed.
 * Where a mount (FUSE/WinFsp) is available, interposition happens before the write instead;
 * this watcher still runs as the journal of record.
 */
export function startWatcher(root: string, onEvent?: () => void): { stop: () => Promise<void> } {
  let timer: NodeJS.Timeout | null = null;
  const refresh = () => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => { try { writeMap(root, buildMap(root)); } catch { /* keep watching */ } }, 300);
  };
  const toRel = (p: string) => relative(root, p).split(sep).join("/");
  const watcher = chokidar.watch(root, {
    ignored: (p) => /(^|[\\/])(node_modules|\.git|\.infuse|dist|build|coverage|\.venv|venv|target)([\\/]|$)/.test(p),
    ignoreInitial: true,
    persistent: true,
  });
  watcher
    .on("add", (p) => { journal(root, { kind: "write", path: toRel(p), bytes: size(p) }); refresh(); onEvent?.(); })
    .on("change", (p) => { journal(root, { kind: "write", path: toRel(p), bytes: size(p) }); refresh(); onEvent?.(); })
    .on("unlink", (p) => { journal(root, { kind: "unlink", path: toRel(p) }); refresh(); onEvent?.(); });
  return { stop: () => watcher.close() };
}

function size(p: string): number | undefined {
  try { return statSync(p).size; } catch { return undefined; }
}
