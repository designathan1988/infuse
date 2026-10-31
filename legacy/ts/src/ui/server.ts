/**
 * Local panel: a small HTTP server on 127.0.0.1 serving ui/index.html and a JSON API.
 * Nothing leaves the machine. Started with `infuse ui` (or PAINEL.cmd on Windows).
 */
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawn, spawnSync } from "node:child_process";
import { infuseHome } from "../env.js";
import { loadRules, applyCommandRules, type Rule } from "../rules.js";
import { buildMap, writeMap, loadMap } from "../map/index.js";
import { transformCommand } from "../shell/transform.js";
import { findRealBash } from "../env.js";

const here = dirname(fileURLToPath(import.meta.url));
const pkgRoot = resolve(here, "..", "..");
const cli = join(pkgRoot, "dist", "cli.js");

export function startUi(port = 4711, open = true): void {
  const server = createServer((req, res) => handle(req, res).catch((e) => json(res, 500, { error: (e as Error).message })));
  server.listen(port, "127.0.0.1", () => {
    const url = `http://127.0.0.1:${port}`;
    console.log(`[infuse] panel at ${url}  (Ctrl+C to stop)`);
    if (open) openBrowser(url);
  });
}

async function handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
  const url = new URL(req.url ?? "/", "http://127.0.0.1");
  const root = url.searchParams.get("root") ?? "";
  if (url.pathname === "/" || url.pathname === "/index.html") {
    res.writeHead(200, { "content-type": "text/html; charset=utf-8" });
    res.end(readFileSync(join(pkgRoot, "ui", "index.html")));
    return;
  }
  if (!url.pathname.startsWith("/api/")) { res.writeHead(404); res.end(); return; }
  const body = req.method === "POST" || req.method === "PUT" ? JSON.parse((await readBody(req)) || "{}") : {};

  switch (url.pathname) {
    case "/api/status": return json(res, 200, status());
    case "/api/projects": return json(res, 200, readJson(join(infuseHome(), "projects.json"), []));
    case "/api/install": return json(res, 200, runCli(["install"]));
    case "/api/uninstall": return json(res, 200, runCli(["uninstall"]));
    case "/api/rules": {
      const file = rulesFile(url.searchParams.get("scope") ?? "project", root);
      if (req.method === "PUT") {
        mkdirSync(dirname(file), { recursive: true });
        writeFileSync(file, JSON.stringify({ rules: body.rules ?? [], disable: body.disable ?? [] }, null, 2) + "\n");
        return json(res, 200, { ok: true, file });
      }
      return json(res, 200, { file, exists: existsSync(file), ...readJson(file, { rules: [], disable: [] }) });
    }
    case "/api/rules/test": {
      // Dry run: what the filter would do with this command under the current rules (nothing is executed).
      const set = loadRules(root);
      const t = body.command?.includes("<<") && !set.disabled.has("heredoc") ? previewHeredoc(body.command) : { command: body.command, notes: [] as string[] };
      const r = applyCommandRules(root, set, t.command);
      return json(res, 200, { finalCommand: r.command, lines: [...t.notes, ...r.lines] });
    }
    case "/api/journal": return json(res, 200, readJsonl(join(root, ".infuse", "journal.jsonl")).slice(-200).reverse());
    case "/api/lessons": return json(res, 200, readJsonl(join(root, ".infuse", "lessons.jsonl")).slice(-200).reverse());
    case "/api/map": {
      let map = loadMap(root);
      if (!map || url.searchParams.get("refresh")) { map = buildMap(root); writeMap(root, map); }
      return json(res, 200, map);
    }
    case "/api/notes": {
      const file = join(root, ".infuse", "notes", String(url.searchParams.get("name") ?? "").replace(/[^\w.-]/g, "_"));
      if (req.method === "PUT") { mkdirSync(dirname(file), { recursive: true }); writeFileSync(file, body.content ?? ""); return json(res, 200, { ok: true, file }); }
      return json(res, 200, { file, content: existsSync(file) ? readFileSync(file, "utf8") : "" });
    }
    default: res.writeHead(404); res.end();
  }
}

function rulesFile(scope: string, root: string): string {
  return scope === "global" ? join(infuseHome(), "rules.json") : join(root, ".infuse", "rules.json");
}

function status() {
  const shim = join(infuseHome(), "bin", process.platform === "win32" ? "bash.exe" : "bash");
  const settingsFile = join(process.env.USERPROFILE ?? process.env.HOME ?? "", ".claude", "settings.json");
  const settings = readJson(settingsFile, {} as Record<string, any>);
  return {
    platform: process.platform,
    node: process.version,
    shim, shimInstalled: existsSync(shim),
    shimConfig: readJson(join(infuseHome(), "bin", "shim.json"), null),
    settingsFile, claudeEnv: settings.env ?? {},
    pointed: Boolean(settings.env?.CLAUDE_CODE_GIT_BASH_PATH === shim || settings.env?.CLAUDE_CODE_SHELL === shim),
    realBash: findRealBash(),
    home: infuseHome(),
  };
}

function previewHeredoc(command: string): { command: string; notes: string[] } {
  // transformCommand writes files for literal heredocs; for a preview we run it in a scratch dir.
  const scratch = join(infuseHome(), "preview");
  mkdirSync(scratch, { recursive: true });
  const r = transformCommand(command, { cwd: scratch, realBash: findRealBash() });
  return { command: r.command, notes: r.actions.map((a) => `[infuse] ${a.kind === "lesson" ? `vice ${a.vice}: ` : ""}${a.detail}`) };
}

function runCli(args: string[]) {
  const r = spawnSync(process.execPath, [cli, ...args], { encoding: "utf8", env: process.env });
  return { ok: r.status === 0, output: (r.stdout ?? "") + (r.stderr ?? "") };
}

function readJson<T>(file: string, fallback: T): T {
  try { return JSON.parse(readFileSync(file, "utf8")) as T; } catch { return fallback; }
}
function readJsonl(file: string): Record<string, unknown>[] {
  if (!existsSync(file)) return [];
  return readFileSync(file, "utf8").split("\n").filter(Boolean).map((l) => { try { return JSON.parse(l); } catch { return { raw: l }; } });
}
function json(res: ServerResponse, code: number, data: unknown): void {
  res.writeHead(code, { "content-type": "application/json; charset=utf-8" });
  res.end(JSON.stringify(data));
}
function readBody(req: IncomingMessage): Promise<string> {
  return new Promise((ok) => { let s = ""; req.on("data", (c) => (s += c)); req.on("end", () => ok(s)); });
}
function openBrowser(url: string): void {
  const cmd = process.platform === "win32" ? ["cmd", ["/c", "start", "", url]] as const
    : process.platform === "darwin" ? ["open", [url]] as const : ["xdg-open", [url]] as const;
  try { spawn(cmd[0], [...cmd[1]], { stdio: "ignore", detached: true }).unref(); } catch { /* user opens it by hand */ }
}

export type { Rule };
