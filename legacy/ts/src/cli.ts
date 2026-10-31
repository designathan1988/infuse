#!/usr/bin/env node
/**
 * infuse — install, launch, inspect.
 *
 *   infuse install          install the interposed shell for Claude Code (desktop or CLI). One time.
 *   infuse uninstall        remove it and restore Claude Code's own shell detection.
 *   infuse status           show what is installed and whether a watcher is alive here.
 *   infuse claude|codex|gemini [args]   launch an agent through the filter (terminal use, any OS)
 *   infuse run <cmd> [args] launch any program through the filter
 *   infuse map              (re)generate the project map
 *   infuse journal [n]      show the last n journaled events
 *   infuse lessons          show recorded vices
 *   infuse env              print the environment the filter installs (for manual shells)
 */
import { spawn } from "node:child_process";
import { chmodSync, copyFileSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { delimiter, dirname, join, resolve } from "node:path";
import { homedir } from "node:os";
import { fileURLToPath } from "node:url";
import { buildMap, writeMap } from "./map/index.js";
import { startWatcher } from "./watch.js";
import { infuseDir } from "./journal.js";
import { findRealBash, infuseHome, findRoot } from "./env.js";
import { loadRules } from "./rules.js";

const here = dirname(fileURLToPath(import.meta.url));
const pkgRoot = resolve(here, "..");
const root = findRoot(process.cwd());
const win = process.platform === "win32";
const [cmd, ...rest] = process.argv.slice(2);

switch (cmd) {
  case "install": install(); break;
  case "uninstall": uninstall(); break;
  case "status": status(); break;
  case "map": {
    const map = buildMap(root);
    const file = writeMap(root, map);
    console.log(`[infuse] map: ${Object.keys(map.files).length} files, ${Object.keys(map.dirs).length} dirs, ${map.hotspots.length} hotspot(s) → ${file}`);
    for (const h of map.hotspots) console.log(`  ⚠ ${h.path}: ${h.lines} lines`);
    break;
  }
  case "journal": tail(join(root, ".infuse", "journal.jsonl"), Number(rest[0] ?? 20), (e) => `${new Date(Number(e.ts)).toISOString()} ${String(e.kind).padEnd(9)} ${String(e.path ?? String(e.command ?? "").slice(0, 80))} ${String(e.detail ?? "")}`); break;
  case "rules": {
    const set = loadRules(root);
    console.log(`[infuse] rule files in effect: ${set.sources.length ? set.sources.join(", ") : "none (copy rules.example.json to .infuse/rules.json or ~/.infuse/rules.json)"}`);
    if (set.disabled.size) console.log(`[infuse] disabled built-ins: ${[...set.disabled].join(", ")}`);
    set.rules.forEach((r, i) => console.log(`  ${i + 1}. [${r.when}] ${r.id ?? "(no id)"}${r.match ? ` match=/${r.match}/` : ""}${r.replace !== undefined ? " replace" : ""}${r.say ? " say" : ""}${r.infuse ? " infuse" : ""}${r.infuseFile ? ` infuseFile=${r.infuseFile}` : ""}${r.vice ? ` vice=${r.vice}` : ""}`));
    break;
  }
  case "lessons": tail(join(root, ".infuse", "lessons.jsonl"), Number(rest[0] ?? 50), (e) => `${new Date(Number(e.ts)).toISOString()} ${String(e.vice)}: ${String(e.detail)}`); break;
  case "env": {
    const env = filterEnv();
    for (const k of ["INFUSE_REAL_BASH", "SHELL", "CLAUDE_CODE_SHELL", "CLAUDE_CODE_GIT_BASH_PATH", "PATH"]) if (env[k]) console.log(`export ${k}=${JSON.stringify(env[k])}`);
    break;
  }
  case "claude": case "codex": case "gemini": launch(cmd, rest); break;
  case "run": if (!rest.length) usage(1); launch(rest[0], rest.slice(1)); break;
  default: usage(cmd ? 1 : 0);
}

function usage(code: number): never {
  console.log(`usage: infuse install | uninstall | status | rules | <claude|codex|gemini> [args] | run <cmd> [args] | map | journal [n] | lessons | env`);
  process.exit(code);
}

function tail(file: string, n: number, fmt: (e: Record<string, unknown>) => string): void {
  if (!existsSync(file)) { console.log(`[infuse] nothing recorded yet (${file})`); return; }
  for (const l of readFileSync(file, "utf8").trim().split("\n").slice(-n)) console.log(fmt(JSON.parse(l)));
}

// ---------------------------------------------------------------------------------------------
// Shim installation (global, one time). After this, every Claude Code session on this machine
// runs its Bash tool through infuse — desktop app or CLI — with no launcher involved.
// ---------------------------------------------------------------------------------------------

function shimPaths() {
  const binDir = join(infuseHome(), "bin");
  const shim = join(binDir, win ? "bash.exe" : "bash");
  return { binDir, shim, config: join(binDir, "shim.json") };
}

function installShim(): string {
  const { binDir, shim, config } = shimPaths();
  mkdirSync(binDir, { recursive: true });
  const realBash = findRealBash(binDir);
  const fsh = join(here, "shell", "fsh.js");
  if (win) {
    copyFileSync(join(pkgRoot, "shim", "win32", "bash.exe"), shim);
  } else {
    writeFileSync(shim, `#!/bin/sh\nexport INFUSE_REAL_BASH=${JSON.stringify(realBash)}\nexec ${JSON.stringify(process.execPath)} ${JSON.stringify(fsh)} "$@"\n`);
    chmodSync(shim, 0o755);
    const sh = join(binDir, "sh");
    copyFileSync(shim, sh); chmodSync(sh, 0o755);
  }
  writeFileSync(config, JSON.stringify({ node: process.execPath, fsh, realBash }, null, 2));
  return shim;
}

function claudeSettingsPath(): string {
  return join(homedir(), ".claude", "settings.json");
}

function readSettings(): Record<string, any> {
  try { return JSON.parse(readFileSync(claudeSettingsPath(), "utf8")); } catch { return {}; }
}

function writeSettings(s: Record<string, any>): void {
  mkdirSync(dirname(claudeSettingsPath()), { recursive: true });
  writeFileSync(claudeSettingsPath(), JSON.stringify(s, null, 2) + "\n");
}

function install(): void {
  const shim = installShim();
  const s = readSettings();
  s.env = { ...(s.env ?? {}) };
  if (win) s.env.CLAUDE_CODE_GIT_BASH_PATH = shim;   // Windows: Claude Code always uses Git Bash and finds it here
  else s.env.CLAUDE_CODE_SHELL = shim;               // macOS/Linux: explicit shell binary for the Bash tool
  writeSettings(s);
  console.log(`[infuse] interposed shell installed: ${shim}`);
  console.log(`[infuse] real bash: ${JSON.parse(readFileSync(shimPaths().config, "utf8")).realBash}`);
  console.log(`[infuse] Claude Code pointed at it via ${claudeSettingsPath()} (${win ? "CLAUDE_CODE_GIT_BASH_PATH" : "CLAUDE_CODE_SHELL"})`);
  console.log(`[infuse] restart Claude Code. From then on every Bash tool command passes through the filter; run 'infuse status' inside a project to confirm.`);
}

function uninstall(): void {
  const s = readSettings();
  if (s.env) { delete s.env.CLAUDE_CODE_GIT_BASH_PATH; delete s.env.CLAUDE_CODE_SHELL; if (!Object.keys(s.env).length) delete s.env; }
  writeSettings(s);
  rmSync(shimPaths().binDir, { recursive: true, force: true });
  console.log(`[infuse] removed. Claude Code is back to its own shell detection (restart it).`);
}

function status(): void {
  const { shim, config } = shimPaths();
  console.log(`[infuse] shim: ${existsSync(shim) ? shim : "not installed"}`);
  if (existsSync(config)) console.log(`[infuse] shim config: ${readFileSync(config, "utf8").trim()}`);
  const s = readSettings();
  console.log(`[infuse] claude settings env: ${JSON.stringify(s.env ?? {})}`);
  console.log(`[infuse] project root here: ${root}`);
  const pidFile = join(root, ".infuse", "watcher.json");
  if (existsSync(pidFile)) {
    const { pid, started } = JSON.parse(readFileSync(pidFile, "utf8"));
    let alive = true; try { process.kill(pid, 0); } catch { alive = false; }
    console.log(`[infuse] watcher: pid ${pid}, started ${new Date(started).toISOString()}, ${alive ? "alive" : "dead"}`);
  } else console.log(`[infuse] watcher: not started here yet (starts on the first agent command)`);
  const journal = join(root, ".infuse", "journal.jsonl");
  if (existsSync(journal)) console.log(`[infuse] journal: ${readFileSync(journal, "utf8").trim().split("\n").length} events`);
}

// ---------------------------------------------------------------------------------------------
// Terminal launcher (any OS): same filter, scoped to one launched process.
// ---------------------------------------------------------------------------------------------

function filterEnv(): NodeJS.ProcessEnv {
  const shimDir = join(infuseDir(root), "bin");
  mkdirSync(shimDir, { recursive: true });
  const realBash = findRealBash(shimDir);
  const fsh = join(here, "shell", "fsh.js");
  let shim: string;
  if (win) {
    shim = join(shimDir, "bash.exe");
    copyFileSync(join(pkgRoot, "shim", "win32", "bash.exe"), shim);
    writeFileSync(join(shimDir, "shim.json"), JSON.stringify({ node: process.execPath, fsh, realBash }));
  } else {
    shim = join(shimDir, "bash");
    const body = `#!/bin/sh\nexec ${JSON.stringify(process.execPath)} ${JSON.stringify(fsh)} "$@"\n`;
    for (const name of ["bash", "sh"]) { const p = join(shimDir, name); writeFileSync(p, body); chmodSync(p, 0o755); }
  }
  return {
    ...process.env,
    INFUSE_ROOT: root,
    INFUSE_REAL_BASH: realBash,
    SHELL: shim,                                    // Codex CLI resolves its shell from $SHELL
    PATH: `${shimDir}${delimiter}${process.env.PATH ?? ""}`, // Gemini CLI spawns `bash -c` through PATH
    CLAUDE_CODE_SHELL: shim,                        // Claude Code (macOS/Linux)
    ...(win ? { CLAUDE_CODE_GIT_BASH_PATH: shim } : {}), // Claude Code (Windows)
  };
}

function launch(program: string, args: string[]): void {
  const env = filterEnv();
  try { writeMap(root, buildMap(root)); } catch (e) { console.error(`[infuse] map failed: ${(e as Error).message}`); }
  const watcher = startWatcher(root);
  console.error(`[infuse] ${program} is running through the filter (root: ${root})`);
  const child = spawn(program, args, { stdio: "inherit", env, shell: win });
  child.on("error", (e) => { console.error(`[infuse] cannot start ${program}: ${e.message}`); watcher.stop().finally(() => process.exit(127)); });
  child.on("exit", (code, signal) => { watcher.stop().finally(() => process.exit(code ?? (signal ? 1 : 0))); });
  for (const sig of ["SIGINT", "SIGTERM"] as const) process.on(sig, () => child.kill(sig));
}
