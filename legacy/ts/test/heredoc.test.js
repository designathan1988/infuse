import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { execFileSync } from "node:child_process";
import { findHeredocs, detectFileWrite } from "../dist/shell/heredoc.js";
import { transformCommand } from "../dist/shell/transform.js";

const ctx = () => ({ cwd: mkdtempSync(join(tmpdir(), "infuse-t-")), realBash: "/bin/bash" });
const run = (cmd, cwd) => execFileSync("/bin/bash", ["-c", cmd], { cwd, encoding: "utf8" });

test("finds a heredoc with quoted delimiter and ignores << inside quotes", () => {
  const hs = findHeredocs(`echo "a <<b"; cat <<'EOF'\nline\nEOF\n`);
  assert.equal(hs.length, 1);
  assert.equal(hs[0].delimiter, "EOF");
  assert.equal(hs[0].quoted, true);
  assert.equal(hs[0].body, "line\n");
  assert.equal(hs[0].terminatorFound, true);
});

test("ignores here-strings (<<<)", () => {
  assert.equal(findHeredocs(`cat <<< "x"`).length, 0);
});

test("tolerates indented terminator with trailing spaces and CRLF", () => {
  const hs = findHeredocs("cat > f <<EOF\r\na\r\n   EOF  \r\necho next");
  assert.equal(hs[0].body, "a\n");
  assert.equal(hs[0].terminatorFound, true);
});

test("missing terminator: end of command closes the body", () => {
  const hs = findHeredocs("cat > f <<EOF\na\nb");
  assert.equal(hs[0].body, "a\nb\n");
  assert.equal(hs[0].terminatorFound, false);
});

test("<<- strips leading tabs", () => {
  const hs = findHeredocs("cat <<-EOF\n\tx\n\tEOF\n");
  assert.equal(hs[0].body, "x\n");
});

test("two heredocs in one command keep their own bodies", () => {
  const hs = findHeredocs("cat <<A > a; cat <<B > b\n1\nA\n2\nB\n");
  assert.equal(hs.length, 2);
  assert.equal(hs[0].body, "1\n");
  assert.equal(hs[1].body, "2\n");
});

test("detects file-write intent in the common forms", () => {
  assert.deepEqual(detectFileWrite("cat > a.txt <<EOF"), { path: "a.txt", append: false });
  assert.deepEqual(detectFileWrite("cat >> 'a b.txt' <<'EOF'"), { path: "a b.txt", append: true });
  assert.deepEqual(detectFileWrite("cat <<EOF > out/x.ts"), { path: "out/x.ts", append: false });
  assert.deepEqual(detectFileWrite("tee -a log.txt <<EOF"), { path: "log.txt", append: true });
  assert.deepEqual(detectFileWrite("sudo tee /etc/x <<EOF > /dev/null"), { path: "/etc/x", append: false });
  assert.equal(detectFileWrite("python3 <<EOF"), null);
});

test("quoted heredoc file write: exact bytes, no heredoc left in the command", () => {
  const c = ctx();
  const cmd = "cat > a.ts <<'EOF'\nconst s = `x${y}` // $(no) \\\" \"q\"\nEOF\necho ok";
  const r = transformCommand(cmd, c);
  assert.ok(!r.command.includes("<<"));
  assert.equal(readFileSync(join(c.cwd, "a.ts"), "utf8"), "const s = `x${y}` // $(no) \\\" \"q\"\n");
  assert.equal(run(r.command, c.cwd).trim(), "ok");
});

test("unquoted heredoc containing code is written literally and recorded as a vice", () => {
  const c = ctx();
  const r = transformCommand("cat > a.js <<EOF\nconsole.log(`hi ${name}`)\nEOF", c);
  assert.equal(readFileSync(join(c.cwd, "a.js"), "utf8"), "console.log(`hi ${name}`)\n");
  assert.ok(r.actions.some((a) => a.vice === "unquoted-heredoc-with-code"));
});

test("unquoted heredoc plain text keeps same-shell expansion", () => {
  const c = ctx();
  const r = transformCommand("N=world; cat > h.txt <<EOF\nhello $N $(echo X) \\$lit \\\"q\\\"\nEOF", c);
  assert.ok(!r.command.includes("<<"));
  run(r.command, c.cwd);
  assert.equal(readFileSync(join(c.cwd, "h.txt"), "utf8"), 'hello world X $lit \\"q\\"\n');
});

test("heredoc feeding a program: quoted goes through a temp file, unquoted keeps expansion", () => {
  const c = ctx();
  const r1 = transformCommand("python3 <<'PY'\nprint('a'+'b')\nPY", c);
  assert.ok(!r1.command.includes("<<"));
  assert.equal(run(r1.command, c.cwd).trim(), "ab");
  const r2 = transformCommand("V=3; python3 <<PY\nprint($V*2)\nPY", c);
  assert.ok(!r2.command.includes("<<"));
  assert.equal(run(r2.command, c.cwd).trim(), "6");
});

test("missing terminator still writes the file and records the vice", () => {
  const c = ctx();
  const r = transformCommand("cat > n.md <<'EOF'\n# t\nbody", c);
  assert.equal(existsSync(join(c.cwd, "n.md")), true);
  assert.ok(r.actions.some((a) => a.vice === "heredoc-missing-terminator"));
});

test("commands without heredocs pass through untouched", () => {
  const cmd = "ls -la && echo '<<not a heredoc>>'";
  assert.equal(transformCommand(cmd, ctx()).command, cmd);
});
