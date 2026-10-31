import { appendFileSync, mkdirSync, existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

export const INFUSE_DIR = ".infuse";

export interface JournalEntry {
  ts: number;
  kind: "write" | "unlink" | "command" | "transform";
  path?: string;
  bytes?: number;
  command?: string;
  detail?: string;
  vice?: string;
}

export function infuseDir(root: string): string {
  const dir = join(root, INFUSE_DIR);
  mkdirSync(dir, { recursive: true });
  return dir;
}

export function journal(root: string, entry: Omit<JournalEntry, "ts">): void {
  appendFileSync(join(infuseDir(root), "journal.jsonl"), JSON.stringify({ ts: Date.now(), ...entry }) + "\n");
}

export function lesson(root: string, vice: string, detail: string): void {
  appendFileSync(join(infuseDir(root), "lessons.jsonl"), JSON.stringify({ ts: Date.now(), vice, detail }) + "\n");
}

export function readJournalSince(root: string, since: number): JournalEntry[] {
  const file = join(root, INFUSE_DIR, "journal.jsonl");
  if (!existsSync(file)) return [];
  return readFileSync(file, "utf8")
    .split("\n")
    .filter(Boolean)
    .map((l) => JSON.parse(l) as JournalEntry)
    .filter((e) => e.ts >= since);
}
