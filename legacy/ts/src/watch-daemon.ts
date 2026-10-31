/**
 * Detached watcher for one project. Started by the interposed shell on first use; exits after
 * a period with no writes, so nothing lingers once the session is over.
 */
import { rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { startWatcher } from "./watch.js";
import { buildMap, writeMap } from "./map/index.js";

const root = process.argv[2] || process.env.INFUSE_ROOT || process.cwd();
const IDLE_MS = Number(process.env.INFUSE_WATCHER_IDLE_MS || 45 * 60_000);
const pidFile = join(root, ".infuse", "watcher.json");

writeFileSync(pidFile, JSON.stringify({ pid: process.pid, started: Date.now() }));
try { writeMap(root, buildMap(root)); } catch { /* map is best effort */ }

let idle: NodeJS.Timeout;
const touch = () => { clearTimeout(idle); idle = setTimeout(quit, IDLE_MS); };
const watcher = startWatcher(root, touch);
touch();

function quit(): void {
  watcher.stop().finally(() => { rmSync(pidFile, { force: true }); process.exit(0); });
}
for (const sig of ["SIGINT", "SIGTERM"] as const) process.on(sig, quit);
