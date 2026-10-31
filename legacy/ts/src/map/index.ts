import { readdirSync, readFileSync, statSync, writeFileSync, mkdirSync } from "node:fs";
import { join, relative, dirname, extname, resolve, sep } from "node:path";
import { infuseDir } from "../journal.js";

/** Project map v0: files, exports, imports, dependents, hotspots — generated from code only. */

export interface FileInfo {
  path: string;          // relative, posix
  lang: string;
  lines: number;
  bytes: number;
  exports: string[];
  imports: string[];     // resolved relative paths (posix) when resolvable, else raw specifier
  dependents: string[];  // files that import this one
}

export interface DirInfo {
  path: string;          // relative, posix ("." for root)
  files: string[];
  subdirs: string[];
  lines: number;
}

export interface ProjectMap {
  generatedAt: number;
  root: string;
  files: Record<string, FileInfo>;
  dirs: Record<string, DirInfo>;
  hotspots: { path: string; lines: number; reason: string }[];
}

const IGNORED_DIRS = new Set(["node_modules", ".git", ".infuse", "dist", "build", "out", ".next", "coverage", "__pycache__", ".venv", "venv", "target", "vendor"]);
const LANG_BY_EXT: Record<string, string> = {
  ".ts": "ts", ".tsx": "ts", ".mts": "ts", ".cts": "ts",
  ".js": "js", ".jsx": "js", ".mjs": "js", ".cjs": "js",
  ".py": "py", ".go": "go", ".rs": "rs", ".java": "java", ".kt": "kotlin", ".cs": "cs",
  ".rb": "rb", ".php": "php", ".css": "css", ".scss": "css", ".html": "html",
  ".json": "json", ".md": "md", ".yml": "yaml", ".yaml": "yaml", ".toml": "toml", ".sh": "sh",
};
const HOTSPOT_LINES = 800;

export function buildMap(root: string): ProjectMap {
  const files: Record<string, FileInfo> = {};
  const dirs: Record<string, DirInfo> = {};
  walk(root, root, files, dirs);
  resolveImports(root, files);
  const hotspots = Object.values(files)
    .filter((f) => f.lines >= HOTSPOT_LINES)
    .map((f) => ({ path: f.path, lines: f.lines, reason: `file holds ${f.lines} lines; responsibilities are likely accumulating here` }))
    .sort((a, b) => b.lines - a.lines);
  return { generatedAt: Date.now(), root, files, dirs, hotspots };
}

function walk(root: string, dir: string, files: Record<string, FileInfo>, dirs: Record<string, DirInfo>): number {
  const rel = toPosix(relative(root, dir)) || ".";
  const info: DirInfo = { path: rel, files: [], subdirs: [], lines: 0 };
  dirs[rel] = info;
  let entries: string[] = [];
  try { entries = readdirSync(dir); } catch { return 0; }
  for (const name of entries.sort()) {
    const full = join(dir, name);
    let st; try { st = statSync(full); } catch { continue; }
    if (st.isDirectory()) {
      if (IGNORED_DIRS.has(name) || name.startsWith(".")) continue;
      info.subdirs.push(name);
      info.lines += walk(root, full, files, dirs);
    } else if (st.isFile()) {
      const lang = LANG_BY_EXT[extname(name).toLowerCase()];
      if (!lang || st.size > 2_000_000) continue;
      const relFile = toPosix(relative(root, full));
      const text = readFileSync(full, "utf8");
      const fi: FileInfo = {
        path: relFile, lang, bytes: st.size,
        lines: text.length === 0 ? 0 : text.split("\n").length,
        exports: extractExports(text, lang),
        imports: extractImports(text, lang),
        dependents: [],
      };
      files[relFile] = fi;
      info.files.push(name);
      info.lines += fi.lines;
    }
  }
  return info.lines;
}

function extractExports(text: string, lang: string): string[] {
  const out = new Set<string>();
  if (lang === "ts" || lang === "js") {
    for (const m of text.matchAll(/^export\s+(?:default\s+)?(?:async\s+)?(?:function\*?|class|const|let|var|interface|type|enum|abstract\s+class)\s+([A-Za-z_$][\w$]*)/gm)) out.add(m[1]);
    for (const m of text.matchAll(/^export\s*\{([^}]*)\}/gm)) for (const n of m[1].split(",")) { const t = n.trim().split(/\s+as\s+/).pop()?.trim(); if (t) out.add(t); }
    if (/^export\s+default\b/m.test(text)) out.add("default");
    for (const m of text.matchAll(/^module\.exports\s*=\s*\{([^}]*)\}/gm)) for (const n of m[1].split(",")) { const t = n.trim().split(":")[0].trim(); if (t) out.add(t); }
  } else if (lang === "py") {
    for (const m of text.matchAll(/^(?:def|class)\s+([A-Za-z_]\w*)/gm)) if (!m[1].startsWith("_")) out.add(m[1]);
  } else if (lang === "go") {
    for (const m of text.matchAll(/^func\s+(?:\([^)]*\)\s*)?([A-Z]\w*)/gm)) out.add(m[1]);
    for (const m of text.matchAll(/^type\s+([A-Z]\w*)/gm)) out.add(m[1]);
  } else if (lang === "rs") {
    for (const m of text.matchAll(/^\s*pub\s+(?:async\s+)?(?:fn|struct|enum|trait|type|mod|const|static)\s+([A-Za-z_]\w*)/gm)) out.add(m[1]);
  } else if (lang === "cs" || lang === "java" || lang === "kotlin") {
    for (const m of text.matchAll(/^\s*(?:public|internal|open|data|sealed)\s+(?:\w+\s+)*?(?:class|interface|record|enum|struct|object)\s+([A-Za-z_]\w*)/gm)) out.add(m[1]);
  }
  return [...out];
}

function extractImports(text: string, lang: string): string[] {
  const out = new Set<string>();
  if (lang === "ts" || lang === "js") {
    for (const m of text.matchAll(/^\s*(?:import|export)\s+(?:[^'"]*?\s+from\s+)?['"]([^'"]+)['"]/gm)) out.add(m[1]);
    for (const m of text.matchAll(/\brequire\(\s*['"]([^'"]+)['"]\s*\)/g)) out.add(m[1]);
    for (const m of text.matchAll(/\bimport\(\s*['"]([^'"]+)['"]\s*\)/g)) out.add(m[1]);
  } else if (lang === "py") {
    for (const m of text.matchAll(/^\s*from\s+([\w.]+)\s+import/gm)) out.add(m[1]);
    for (const m of text.matchAll(/^\s*import\s+([\w.]+)/gm)) out.add(m[1]);
  } else if (lang === "go") {
    for (const m of text.matchAll(/^\s*"([^"]+)"\s*$/gm)) out.add(m[1]);
  } else if (lang === "rs") {
    for (const m of text.matchAll(/^\s*(?:pub\s+)?use\s+([\w:]+)/gm)) out.add(m[1]);
    for (const m of text.matchAll(/^\s*mod\s+(\w+)\s*;/gm)) out.add(m[1]);
  }
  return [...out];
}

const TS_EXTS = [".ts", ".tsx", ".mts", ".js", ".jsx", ".mjs", ".cjs", "/index.ts", "/index.tsx", "/index.js"];

function resolveImports(root: string, files: Record<string, FileInfo>): void {
  for (const f of Object.values(files)) {
    f.imports = f.imports.map((spec) => {
      if (f.lang === "ts" || f.lang === "js") {
        if (!spec.startsWith(".") && !spec.startsWith("/")) return spec;
        const base = resolve(root, dirname(f.path), spec);
        const stripped = base.replace(/\.js$/, "");
        for (const cand of [base, ...TS_EXTS.map((e) => stripped + e), ...TS_EXTS.map((e) => base + e)]) {
          const rel = toPosix(relative(root, cand));
          if (files[rel]) return rel;
        }
        return spec;
      }
      if (f.lang === "py") {
        const rel = spec.replace(/^\.+/, (d) => d.length > 1 ? "../".repeat(d.length - 1) : "./").replace(/\./g, "/");
        const base = resolve(root, spec.startsWith(".") ? dirname(f.path) : ".", rel);
        for (const cand of [base + ".py", join(base, "__init__.py")]) {
          const r = toPosix(relative(root, cand));
          if (files[r]) return r;
        }
        return spec;
      }
      return spec;
    });
  }
  for (const f of Object.values(files)) for (const imp of f.imports) if (files[imp]) files[imp].dependents.push(f.path);
}

function toPosix(p: string): string {
  return p.split(sep).join("/");
}

/** Short, agent-facing description of one directory — what is infused into `ls`/`cd` output. */
export function describeDir(map: ProjectMap, relDir: string): string {
  const d = map.dirs[relDir || "."];
  if (!d) return "";
  const lines: string[] = [];
  lines.push(`[infuse] ${d.path === "." ? "project root" : d.path}: ${d.files.length} files, ${d.subdirs.length} dirs, ${d.lines} lines`);
  for (const name of d.files) {
    const f = map.files[d.path === "." ? name : `${d.path}/${name}`];
    if (!f) continue;
    const ex = f.exports.length ? ` exports ${f.exports.slice(0, 8).join(", ")}${f.exports.length > 8 ? "…" : ""}` : "";
    const dep = f.dependents.length ? ` ← used by ${f.dependents.length}` : "";
    const hot = f.lines >= HOTSPOT_LINES ? " ⚠ hotspot" : "";
    lines.push(`  ${name} (${f.lines}l)${ex}${dep}${hot}`);
  }
  for (const s of d.subdirs) {
    const sd = map.dirs[d.path === "." ? s : `${d.path}/${s}`];
    if (sd) lines.push(`  ${s}/ (${sd.lines}l, ${sd.files.length} files)`);
  }
  return lines.join("\n");
}

/** Who depends on a file — infused when the agent edits it. */
export function describeDependents(map: ProjectMap, relFile: string): string {
  const f = map.files[relFile];
  if (!f) return "";
  if (f.dependents.length === 0) return `[infuse] ${relFile}: no dependents in the project`;
  return `[infuse] ${relFile} is used by ${f.dependents.length}: ${f.dependents.slice(0, 12).join(", ")}${f.dependents.length > 12 ? "…" : ""}`;
}

export function writeMap(root: string, map: ProjectMap): string {
  const dir = join(infuseDir(root), "map");
  mkdirSync(dir, { recursive: true });
  const file = join(dir, "map.json");
  writeFileSync(file, JSON.stringify(map, null, 1));
  const md = [
    `# Project map (generated ${new Date(map.generatedAt).toISOString()})`,
    "",
    ...Object.keys(map.dirs).sort().map((d) => describeDir(map, d)),
    "",
    map.hotspots.length ? "## Hotspots" : "",
    ...map.hotspots.map((h) => `- ${h.path}: ${h.reason}`),
  ].join("\n");
  writeFileSync(join(dir, "MAP.md"), md);
  return file;
}

export function loadMap(root: string): ProjectMap | null {
  try { return JSON.parse(readFileSync(join(root, ".infuse", "map", "map.json"), "utf8")) as ProjectMap; } catch { return null; }
}
