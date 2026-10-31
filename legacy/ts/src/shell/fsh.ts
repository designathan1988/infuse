#!/usr/bin/env node
/**
 * fsh — the interposed shell.
 *
 * Every agent's Bash tool invokes `bash -c "<command>"` (sometimes `-lc`, `-ec`...). The launcher puts
 * this program first on PATH under the names `bash` and `sh`, so the command passes through here:
 *   1. the command is transformed (no heredoc reaches bash; vices become remedies);
 *   2. the real bash runs it with the same options;
 *   3. the output goes back infused: what was written, who depends on it, the map of what was listed.
 * Without `-c` (interactive/login use) it simply execs the real bash.
 */
import { spawn } from "node:child_process";
import { existsSync, statSync } from "node:fs";
import { resolve, relative, sep } from "node:path";
import { transformCommand } from "./transform.js";
import { journal, lesson, readJournalSince } from "../journal.js";
import { loadMap, describeDir, describeDependents, buildMap, writeMap } from "../map/index.js";
import { findRoot, findRealBash, ensureWatcher, fromMsysPath, rememberProject } from "../env.js";
import { loadRules, applyCommandRules, applyWriteRules, applyListRules } from "../rules.js";

const realBash = findRealBash();
const root = findRoot(process.cwd());
const argv = process.argv.slice(2);

const cIndex = argv.findIndex((a) => a === "-c" || (/^-[a-zA-Z]+$/.test(a) && a.includes("c")));
if (cIndex === -1 || cIndex + 1 >= argv.length) {
  const child = spawn(realBash, argv, { stdio: "inherit", env: process.env });
  child.on("exit", (code, signal) => process.exit(code ?? (signal ? 1 : 0)));
} else {
  const started = Date.now();
  const original = argv[cIndex + 1];
  try { ensureWatcher(root); rememberProject(root); } catch { /* the shell must never fail because of the watcher */ }
  const rules = loadRules(root);
  const transformed = rules.disabled.has("heredoc") ? { command: original, actions: [] } : transformCommand(original, { cwd: process.cwd(), realBash });
  const actions = transformed.actions;
  const ruled = applyCommandRules(root, rules, transformed.command);
  const command = ruled.command;

  journal(root, { kind: "command", command: original, detail: actions.length ? `${actions.length} transformation(s)` : undefined });
  for (const a of actions) {
    if (a.kind === "lesson" && a.vice) lesson(root, a.vice, a.detail);
    else journal(root, { kind: "transform", path: a.path, bytes: a.bytes, detail: a.detail });
  }

  const args = [...argv];
  args[cIndex + 1] = command;
  const child = spawn(realBash, args, { stdio: ["inherit", "pipe", "pipe"], env: process.env });
  child.stdout.pipe(process.stdout, { end: false });
  child.stderr.pipe(process.stderr, { end: false });
  child.on("close", (code, signal) => {
    const trailer = buildTrailer(original, actions, started, rules, ruled.lines);
    if (trailer) process.stdout.write((trailer.startsWith("\n") ? "" : "\n") + trailer + "\n");
    process.exit(code ?? (signal ? 1 : 0));
  });
}

function buildTrailer(original: string, actions: ReturnType<typeof transformCommand>["actions"], started: number, rules: ReturnType<typeof loadRules>, ruleLines: string[]): string {
  const lines: string[] = [...ruleLines];
  for (const a of actions) {
    if (a.kind === "heredoc-to-file") lines.push(`[infuse] ${a.detail}`);
    else if (a.kind === "lesson") lines.push(`[infuse] vice ${a.vice}: ${a.detail}`);
  }

  let map = loadMap(root);
  if (!map) { try { map = buildMap(root); writeMap(root, map); } catch { map = null; } }
  const written = new Set<string>();
  for (const e of readJournalSince(root, started)) if (e.kind === "write" && e.path) written.add(e.path);
  for (const a of actions) if (a.kind === "heredoc-to-file" && a.path) written.add(toRel(a.path));
  if (written.size) {
    lines.push(`[infuse] written during this command: ${[...written].slice(0, 20).join(", ")}${written.size > 20 ? "…" : ""}`);
    if (map) for (const w of [...written].slice(0, 8)) { const d = describeDependents(map, w); if (d && !d.endsWith("no dependents in the project")) lines.push(d); }
    lines.push(...applyWriteRules(root, rules, [...written].map((w) => resolve(root, w))));
  }

  if (map) {
    const listed = listedDirectory(original);
    if (listed !== null) {
      const abs = resolve(process.cwd(), fromMsysPath(listed));
      if (existsSync(abs) && statSync(abs).isDirectory()) {
        const d = describeDir(map, toRel(abs) || ".");
        if (d) lines.push(d);
        lines.push(...applyListRules(root, rules, toRel(abs) || "."));
      }
    }
  }
  return lines.join("\n");
}

/** Returns the directory an `ls`/`tree`/`find -maxdepth 1` command is listing, or null. */
function listedDirectory(cmd: string): string | null {
  const segments = cmd.split(/&&|\|\||;|\||\r?\n/).map((s) => s.trim());
  let cwdPrefix = "";
  for (const seg of segments) {
    const cd = seg.match(/^cd\s+(\S+)/);
    if (cd) { cwdPrefix = cd[1].replace(/^['"]|['"]$/g, ""); continue; }
    const m = seg.match(/^(?:ls|tree|dir)\b(.*)$/);
    if (m) {
      const args = m[1].trim().split(/\s+/).filter((a) => a && !a.startsWith("-"));
      const target = args[0] ?? ".";
      return cwdPrefix ? resolve(cwdPrefix, target.replace(/^['"]|['"]$/g, "")) : target.replace(/^['"]|['"]$/g, "");
    }
  }
  return null;
}

function toRel(abs: string): string {
  const r = relative(root, abs);
  return r.split(sep).join("/");
}
