/**
 * Tolerant heredoc parser for shell commands emitted by coding agents.
 *
 * Deliberate tolerances (these are exactly where agents break):
 *  - indented terminator, or terminator with trailing whitespace
 *  - CRLF line endings
 *  - missing terminator at the end of the command (end of text closes the heredoc)
 *  - quoted or unquoted delimiter
 */

export interface Heredoc {
  /** position of `<<` in the original command */
  opStart: number;
  /** position right after the delimiter token */
  opEnd: number;
  /** start of the body (first byte after the newline of the command line) */
  bodyStart: number;
  /** end of the body (byte before the terminator line, or end of text) */
  bodyEnd: number;
  /** end of the whole block, including the terminator line */
  blockEnd: number;
  delimiter: string;
  quoted: boolean;
  stripTabs: boolean;
  body: string;
  terminatorFound: boolean;
}

/** Scans the command outside quotes and finds `<<` / `<<-` operators (not `<<<`). */
export function findHeredocs(cmd: string): Heredoc[] {
  const ops: Pick<Heredoc, "opStart" | "opEnd" | "delimiter" | "quoted" | "stripTabs">[] = [];
  let i = 0;
  let inSingle = false;
  let inDouble = false;
  while (i < cmd.length) {
    const c = cmd[i];
    if (inSingle) {
      if (c === "'") inSingle = false;
      i++;
      continue;
    }
    if (inDouble) {
      if (c === "\\") { i += 2; continue; }
      if (c === '"') inDouble = false;
      i++;
      continue;
    }
    if (c === "\\") { i += 2; continue; }
    if (c === "'") { inSingle = true; i++; continue; }
    if (c === '"') { inDouble = true; i++; continue; }
    if (c === "#") {
      while (i < cmd.length && cmd[i] !== "\n") i++;
      continue;
    }
    if (c === "<" && cmd[i + 1] === "<" && cmd[i + 2] !== "<" && cmd[i - 1] !== "<") {
      const opStart = i;
      i += 2;
      let stripTabs = false;
      if (cmd[i] === "-") { stripTabs = true; i++; }
      while (cmd[i] === " " || cmd[i] === "\t") i++;
      let quoted = false;
      let delimiter = "";
      if (cmd[i] === "'" || cmd[i] === '"') {
        const q = cmd[i];
        quoted = true;
        i++;
        while (i < cmd.length && cmd[i] !== q) delimiter += cmd[i++];
        i++;
      } else {
        while (i < cmd.length && !/[\s;|&<>)]/.test(cmd[i])) {
          if (cmd[i] === "\\") { quoted = true; i++; }
          delimiter += cmd[i++];
        }
      }
      if (delimiter.length === 0) continue;
      ops.push({ opStart, opEnd: i, delimiter, quoted, stripTabs });
      continue;
    }
    i++;
  }
  if (ops.length === 0) return [];

  // Bodies start after the newline of the line holding the operator(s), in order of appearance.
  const result: Heredoc[] = [];
  let cursor = 0;        // end of the previous heredoc block
  let opsLineEnd = -2;   // newline ending the command line of the previous operator
  for (const op of ops) {
    let bodyStart: number;
    if (op.opStart < opsLineEnd) {
      // Same command line as the previous operator: this body starts where the previous block ended.
      bodyStart = cursor;
    } else {
      opsLineEnd = cmd.indexOf("\n", op.opEnd);
      if (opsLineEnd === -1) {
        result.push({ ...op, bodyStart: cmd.length, bodyEnd: cmd.length, blockEnd: cmd.length, body: "", terminatorFound: false });
        cursor = cmd.length;
        opsLineEnd = cmd.length;
        continue;
      }
      bodyStart = opsLineEnd + 1;
    }
    const lines: string[] = [];
    let pos = bodyStart;
    let terminatorFound = false;
    let blockEnd = cmd.length;
    let bodyEnd = cmd.length;
    while (pos <= cmd.length) {
      const nl = cmd.indexOf("\n", pos);
      const rawLine = nl === -1 ? cmd.slice(pos) : cmd.slice(pos, nl);
      const line = rawLine.replace(/\r$/, "");
      const probe = op.stripTabs ? line.replace(/^\t+/, "") : line;
      if (probe.trim() === op.delimiter) {
        terminatorFound = true;
        bodyEnd = pos;
        blockEnd = nl === -1 ? cmd.length : nl + 1;
        break;
      }
      lines.push(op.stripTabs ? line.replace(/^\t+/, "") : line);
      if (nl === -1) { pos = cmd.length + 1; break; }
      pos = nl + 1;
    }
    const body = lines.length ? lines.join("\n") + "\n" : "";
    result.push({ ...op, bodyStart, bodyEnd, blockEnd, body, terminatorFound });
    cursor = blockEnd;
  }
  return result;
}

export interface FileWriteIntent {
  path: string;
  append: boolean;
}

/**
 * Recognizes, on the command line holding the heredoc, the intent to write a file:
 *   cat > PATH <<EOF      cat >> PATH <<EOF      cat <<EOF > PATH
 *   tee PATH <<EOF        tee -a PATH <<EOF      sudo tee ...
 * Returns null when the heredoc feeds something else (python, psql, node...).
 */
export function detectFileWrite(commandLine: string): FileWriteIntent | null {
  const line = commandLine.trim();
  const P = String.raw`('[^']*'|"[^"]*"|\S+)`;
  let m = line.match(new RegExp(String.raw`^cat(?:\s+-[A-Za-z]+)*\s*(>>?)\s*${P}\s*<<.*$`));
  if (m) return { path: unquote(m[2]), append: m[1] === ">>" };
  m = line.match(new RegExp(String.raw`^cat(?:\s+-[A-Za-z]+)*\s*<<-?\s*['"\\]?\w+['"]?\s*(>>?)\s*${P}\s*$`));
  if (m) return { path: unquote(m[2]), append: m[1] === ">>" };
  m = line.match(new RegExp(String.raw`^(?:sudo\s+)?tee\s+(-a\s+)?${P}\s*(?:>\s*/dev/null\s*)?<<.*$`));
  if (m) return { path: unquote(m[2]), append: Boolean(m[1]) };
  return null;
}

function unquote(s: string): string {
  if ((s.startsWith("'") && s.endsWith("'")) || (s.startsWith('"') && s.endsWith('"'))) return s.slice(1, -1);
  return s;
}

/** The command line (line start to newline) on which the operator appears. */
export function commandLineOf(cmd: string, h: Heredoc): { start: number; end: number; text: string } {
  const start = cmd.lastIndexOf("\n", h.opStart) + 1;
  let end = cmd.indexOf("\n", h.opEnd);
  if (end === -1) end = cmd.length;
  return { start, end, text: cmd.slice(start, end) };
}

/** Signs that the agent wanted literal content but left the delimiter unquoted. */
export function looksLikeLiteralIntent(body: string): boolean {
  return /`|\$\(|\$\{/.test(body);
}
