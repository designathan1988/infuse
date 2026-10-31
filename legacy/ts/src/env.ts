import { existsSync, readFileSync, writeFileSync, mkdirSync, rmSync } from "node:fs";
import { dirname, join, resolve, parse } from "node:path";
import { homedir } from "node:os";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

/** Where infuse keeps its global state (shim, config). */
export function infuseHome(): string {
  return process.env.INFUSE_HOME || join(homedir(), ".infuse");
}

/** The project root for a given cwd: nearest ancestor holding .infuse/, .git/ or package.json. */
export function findRoot(cwd: string): string {
  if (process.env.INFUSE_ROOT) return process.env.INFUSE_ROOT;
  let dir = resolve(cwd);
  const stop = parse(dir).root;
  let candidate: string | null = null;
  while (true) {
    if (existsSync(join(dir, ".infuse"))) return dir;
    if (!candidate && (existsSync(join(dir, ".git")) || existsSync(join(dir, "package.json")))) candidate = dir;
    if (dir === stop) break;
    dir = dirname(dir);
  }
  return candidate ?? resolve(cwd);
}

/** The real bash this machine has, never the shim. */
export function findRealBash(excludeDir?: string): string {
  if (process.env.INFUSE_REAL_BASH) return process.env.INFUSE_REAL_BASH;
  const candidates = process.platform === "win32"
    ? [
        "C:\\Program Files\\Git\\bin\\bash.exe",
        "C:\\Program Files\\Git\\usr\\bin\\bash.exe",
        "C:\\Program Files (x86)\\Git\\bin\\bash.exe",
        join(process.env.LOCALAPPDATA ?? "", "Programs", "Git", "bin", "bash.exe"),
      ]
    : ["/bin/bash", "/usr/bin/bash", "/usr/local/bin/bash", "/opt/homebrew/bin/bash"];
  for (const c of candidates) if (c && existsSync(c)) return c;
  const sep = process.platform === "win32" ? ";" : ":";
  for (const dir of (process.env.PATH ?? "").split(sep)) {
    if (!dir || (excludeDir && resolve(dir) === resolve(excludeDir))) continue;
    const p = join(dir, process.platform === "win32" ? "bash.exe" : "bash");
    if (existsSync(p) && !p.includes(join(".infuse", "bin"))) return p;
  }
  return process.platform === "win32" ? "bash.exe" : "/bin/bash";
}

/** Git Bash (MSYS) path → Windows path, on Windows only. `/c/Users/x` → `C:\Users\x`. */
export function fromMsysPath(p: string): string {
  if (process.platform !== "win32") return p;
  const m = p.match(/^\/([a-zA-Z])\/(.*)$/);
  if (m) return `${m[1].toUpperCase()}:\\${m[2].replace(/\//g, "\\")}`;
  if (p.startsWith("~/")) return join(homedir(), p.slice(2));
  return p;
}

const distDir = () => dirname(fileURLToPath(import.meta.url));

/**
 * Makes sure a watcher daemon is alive for this project; starts one (detached) if not.
 * The shell is invoked on every agent command, so the first command of a session boots the watcher.
 */
export function ensureWatcher(root: string): void {
  const dir = join(root, ".infuse");
  mkdirSync(dir, { recursive: true });
  const pidFile = join(dir, "watcher.json");
  try {
    const { pid } = JSON.parse(readFileSync(pidFile, "utf8")) as { pid: number };
    process.kill(pid, 0); // throws if not alive
    return;
  } catch { /* not running */ }
  const child = spawn(process.execPath, [join(distDir(), "watch-daemon.js"), root], {
    detached: true,
    stdio: "ignore",
    windowsHide: true,
    env: { ...process.env, INFUSE_ROOT: root },
  });
  child.unref();
  writeFileSync(pidFile, JSON.stringify({ pid: child.pid, started: Date.now() }));
}

export function stopWatcher(root: string): boolean {
  const pidFile = join(root, ".infuse", "watcher.json");
  try {
    const { pid } = JSON.parse(readFileSync(pidFile, "utf8")) as { pid: number };
    process.kill(pid);
    rmSync(pidFile, { force: true });
    return true;
  } catch { return false; }
}

export { spawnSync };

/** Remembers projects the filter has seen, for the panel. */
export function rememberProject(root: string): void {
  try {
    const file = join(infuseHome(), "projects.json");
    mkdirSync(infuseHome(), { recursive: true });
    let list: { root: string; lastSeen: number }[] = [];
    try { list = JSON.parse(readFileSync(file, "utf8")); } catch { /* first time */ }
    list = list.filter((p) => p.root !== root);
    list.unshift({ root, lastSeen: Date.now() });
    writeFileSync(file, JSON.stringify(list.slice(0, 50), null, 2));
  } catch { /* never fail the shell for this */ }
}
