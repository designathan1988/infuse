import { mkdirSync, writeFileSync, appendFileSync, mkdtempSync } from "node:fs";
import { dirname, resolve, join } from "node:path";
import { tmpdir } from "node:os";
import { findHeredocs, detectFileWrite, commandLineOf, looksLikeLiteralIntent, type Heredoc } from "./heredoc.js";
import { fromMsysPath } from "../env.js";

export interface TransformAction {
  kind: "heredoc-to-file" | "heredoc-to-stdin" | "lesson";
  detail: string;
  path?: string;
  bytes?: number;
  vice?: string;
}

export interface TransformResult {
  command: string;
  actions: TransformAction[];
}

export interface TransformContext {
  cwd: string;
  realBash: string;
}

/**
 * Rewrites a shell command so that no heredoc ever reaches bash.
 *
 *  - `cat > file <<EOF ... EOF` and `tee file <<EOF ...` become direct file writes
 *    performed here; the command line is replaced by a no-op that preserves exit status.
 *  - Any other heredoc (`python3 <<EOF`, `psql <<EOF`...) has its body written to a temp file
 *    and the operator replaced by `< tmpfile`.
 *
 * Unquoted delimiters keep bash semantics (expansion) by running the expansion in a
 * script file under our control, where quoting cannot break. Exception: a file write
 * whose body contains backticks/`$(`/`${` with an unquoted delimiter is treated as literal
 * — the agent was writing code, not a shell template — and the vice is recorded.
 */
export function transformCommand(cmd: string, ctx: TransformContext): TransformResult {
  const heredocs = findHeredocs(cmd);
  if (heredocs.length === 0) return { command: cmd, actions: [] };

  const actions: TransformAction[] = [];
  let out = cmd;
  // Process from last to first so earlier offsets stay valid.
  for (const h of [...heredocs].reverse()) {
    const line = commandLineOf(out, h);
    const intent = detectFileWrite(line.text);
    let body = h.body;
    let literal = h.quoted;

    if (!h.quoted && intent && looksLikeLiteralIntent(body)) {
      literal = true;
      actions.push({
        kind: "lesson",
        vice: "unquoted-heredoc-with-code",
        detail: `heredoc for ${intent.path} had an unquoted delimiter but contains \`/$(/\${; treated as literal`,
      });
    }
    if (!h.terminatorFound) {
      actions.push({
        kind: "lesson",
        vice: "heredoc-missing-terminator",
        detail: `heredoc "${h.delimiter}" never terminated; end of command used as terminator`,
      });
    }
    if (intent && literal) {
      // Exact bytes, written here. No shell involved.
      const target = resolve(ctx.cwd, fromMsysPath(intent.path));
      mkdirSync(dirname(target), { recursive: true });
      if (intent.append) appendFileSync(target, body);
      else writeFileSync(target, body);
      actions.push({
        kind: "heredoc-to-file",
        path: target,
        bytes: Buffer.byteLength(body),
        detail: `${intent.append ? "appended" : "wrote"} ${intent.path} (${Buffer.byteLength(body)} bytes) without heredoc`,
      });
      out = out.slice(0, line.start) + ":\n" + out.slice(h.blockEnd);
    } else if (intent) {
      // Template intent: expansion must happen in the same shell, with the same variables.
      // A double-quoted string has the same expansion rules as an unquoted heredoc.
      const printf = `printf '%s' ${doubleQuote(body)} ${intent.append ? ">>" : ">"} ${shellQuote(intent.path)}`;
      actions.push({
        kind: "heredoc-to-file",
        path: resolve(ctx.cwd, fromMsysPath(intent.path)),
        bytes: Buffer.byteLength(body),
        detail: `${intent.append ? "append to" : "write"} ${intent.path} via printf (shell expansion kept) without heredoc`,
      });
      out = out.slice(0, line.start) + printf + "\n" + out.slice(h.blockEnd);
    } else if (literal) {
      // Feeds another program; exact bytes through a temp file.
      const dir = mkdtempSync(join(tmpdir(), "infuse-hd-"));
      const tmp = join(dir, "stdin");
      writeFileSync(tmp, body);
      actions.push({ kind: "heredoc-to-stdin", path: tmp, bytes: Buffer.byteLength(body), detail: `heredoc body routed through ${tmp}` });
      const afterOp = out.slice(h.opEnd, line.end);
      out = out.slice(0, h.opStart) + `< ${shellQuote(tmp)}` + afterOp + "\n" + out.slice(h.blockEnd);
    } else {
      // Feeds another program with expansion: process substitution in the same shell.
      actions.push({ kind: "heredoc-to-stdin", bytes: Buffer.byteLength(body), detail: "heredoc body routed through process substitution (shell expansion kept)" });
      const afterOp = out.slice(h.opEnd, line.end);
      out = out.slice(0, h.opStart) + `< <(printf '%s' ${doubleQuote(body)})` + afterOp + "\n" + out.slice(h.blockEnd);
    }
  }
  return { command: out, actions };
}

/**
 * Wraps a heredoc body in double quotes preserving unquoted-heredoc semantics:
 * `$`, backticks and `\` behave the same; only `"` needs escaping, and a pre-existing `\"`
 * (literal in a heredoc) must survive as `\"`.
 */
function doubleQuote(body: string): string {
  return `"${body.replace(/\\"|"/g, (m) => (m === '\\"' ? '\\\\\\"' : '\\"'))}"`;
}

function shellQuote(s: string): string {
  return `'${s.replace(/'/g, `'\\''`)}'`;
}

export type { Heredoc };
