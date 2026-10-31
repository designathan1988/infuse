/**
 * User-defined rules — the part of the filter the user owns.
 *
 * Two files, merged in this order (global first, then project; later rules run later):
 *   <home>/.infuse/rules.json      applies to every project on this machine
 *   <project>/.infuse/rules.json   applies to this project only
 *
 * A rule says WHEN it applies and WHAT happens:
 *
 *   when: "command"   match  = regex tested against the command the agent is about to run
 *                     replace = replacement for the matched text ($1... allowed); the agent's command is changed
 *   when: "write"     match  = regex tested against the CONTENT of every file written during the command
 *                     replace = replacement applied to the file content (the file is rewritten)
 *   when: "list"      match  = regex tested against the directory path the agent is listing (posix, relative)
 *   when: "always"    applies to every command
 *
 *   say        text appended to the command output the agent reads, prefixed with [infuse]
 *   infuse     text appended verbatim (your own instruction, at the moment it matters)
 *   infuseFile path (relative to the project, or absolute) whose contents are appended
 *   vice       name recorded in lessons.jsonl when the rule matches
 *   disable    for built-in behaviours: "heredoc" turns off the heredoc transformation
 *
 * Rules are plain JSON so no tooling is needed to edit them. See rules.example.json.
 */
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join, isAbsolute } from "node:path";
import { infuseHome } from "./env.js";
import { lesson } from "./journal.js";

export interface Rule {
  id?: string;
  when: "command" | "write" | "list" | "always";
  match?: string;
  flags?: string;
  replace?: string;
  say?: string;
  infuse?: string;
  infuseFile?: string;
  vice?: string;
  disable?: "heredoc";
}

export interface RuleSet {
  rules: Rule[];
  disabled: Set<string>;
  sources: string[];
}

export function loadRules(root: string): RuleSet {
  const sources = [join(infuseHome(), "rules.json"), join(root, ".infuse", "rules.json")];
  const rules: Rule[] = [];
  const disabled = new Set<string>();
  const used: string[] = [];
  for (const file of sources) {
    if (!existsSync(file)) continue;
    try {
      const parsed = JSON.parse(readFileSync(file, "utf8")) as { rules?: Rule[]; disable?: string[] };
      for (const r of parsed.rules ?? []) { if (r.disable) disabled.add(r.disable); else rules.push(r); }
      for (const d of parsed.disable ?? []) disabled.add(d);
      used.push(file);
    } catch (e) {
      rules.push({ when: "always", say: `rules file ${file} is not valid JSON: ${(e as Error).message}` });
    }
  }
  return { rules, disabled, sources: used };
}

function regex(r: Rule): RegExp | null {
  if (!r.match) return null;
  try { return new RegExp(r.match, r.flags ?? "m"); } catch { return null; }
}

function expandText(root: string, r: Rule): string[] {
  const out: string[] = [];
  if (r.say) out.push(`[infuse] ${r.say}`);
  if (r.infuse) out.push(r.infuse);
  if (r.infuseFile) {
    const p = isAbsolute(r.infuseFile) ? r.infuseFile : join(root, r.infuseFile);
    if (existsSync(p)) out.push(readFileSync(p, "utf8").trimEnd());
    else out.push(`[infuse] rule ${r.id ?? "?"}: file not found: ${r.infuseFile}`);
  }
  return out;
}

/** Applies command rules: may rewrite the command; returns the trailer lines they produce. */
export function applyCommandRules(root: string, set: RuleSet, command: string): { command: string; lines: string[] } {
  const lines: string[] = [];
  let cmd = command;
  for (const r of set.rules) {
    if (r.when !== "command" && r.when !== "always") continue;
    const re = regex(r);
    if (r.when === "command" && (!re || !re.test(cmd))) continue;
    if (re && r.replace !== undefined) cmd = cmd.replace(re, r.replace);
    if (r.vice) lesson(root, r.vice, `rule ${r.id ?? "?"} matched command: ${command.slice(0, 120)}`);
    lines.push(...expandText(root, r));
  }
  return { command: cmd, lines };
}

/** Applies write rules to the files written during a command (absolute paths). */
export function applyWriteRules(root: string, set: RuleSet, files: string[]): string[] {
  const lines: string[] = [];
  const writeRules = set.rules.filter((r) => r.when === "write");
  if (!writeRules.length) return lines;
  for (const file of files) {
    let content: string;
    try { content = readFileSync(file, "utf8"); } catch { continue; }
    for (const r of writeRules) {
      const re = regex(r);
      if (!re || !re.test(content)) continue;
      if (r.replace !== undefined) {
        const next = content.replace(new RegExp(re.source, re.flags.includes("g") ? re.flags : re.flags + "g"), r.replace);
        if (next !== content) { writeFileSync(file, next); content = next; lines.push(`[infuse] rule ${r.id ?? "?"} rewrote ${file}`); }
      }
      if (r.vice) lesson(root, r.vice, `rule ${r.id ?? "?"} matched in ${file}`);
      lines.push(...expandText(root, r).map((l) => l.replace("[infuse] ", `[infuse] ${file}: `)));
    }
  }
  return lines;
}

/** Applies list rules to a directory the agent listed (posix path relative to root). */
export function applyListRules(root: string, set: RuleSet, dir: string): string[] {
  const lines: string[] = [];
  for (const r of set.rules) {
    if (r.when !== "list") continue;
    const re = regex(r);
    if (re && !re.test(dir)) continue;
    lines.push(...expandText(root, r));
  }
  return lines;
}
