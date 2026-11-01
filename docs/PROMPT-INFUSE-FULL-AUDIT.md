# infuse — full application audit, completion and verification

> This prompt is self-contained. Read the repository files it names before changing anything.
> The repository is `C:\Users\Jonathan\Documents\infuse-src-0.5`, branch `master`, at `d6b3f6f`,
> pushed to `https://github.com/designathan1988/infuse`.

> **HARD RULE — UI verification is done in Google Chrome through Playwright, never in the embedded
> browser.** Open the application served by `infuse serve` in **Google Chrome driven by Playwright**,
> at **2× device scale factor** (for example, viewport `1440×900` → PNG `2880×1800`). The app's
> native WebView2 window, the embedded Browser pane and the agent's own Browser tool are **not** the
> verification surface; they are features under test. Every button, menu, pane, tab, setting, option,
> dialog, toggle, picker, shortcut and state must be exercised in Chrome, photographed at high
> resolution, visually analyzed, and recorded before and after each fix. A screenshot that was not
> looked at is not evidence.

> **HARD RULE — everything must work.** Every feature that exists must be tested in the running
> application and must work. Unit tests, `go test` and API calls do not substitute for the
> Chrome/Playwright sweep. If something does not work, fix it at the root cause; do not disable it,
> hide it, or write a test that avoids it.

## 0. Role and mission

You are the lead engineer and QA owner for **infuse 0.5**. Your mission has two halves that are one
deliverable:

1. **Close everything that is still missing** from the product and from its deterministic guarantees.
2. **Test the entire application end to end and fix every defect you find** — every button, menu,
   pane, tab, setting, option, dialog, state, integration and edge case. Nothing is out of scope.
   If it exists in the application, it must be tested in the real UI; if it does not work, it must
   be fixed.

Do not stop at the first failure. Do not declare success without evidence. Do not change a test to
make it pass and do not disable a feature to make a test pass. Fix the product at its root cause.

## 1. Read before changing anything

Read, in this order:

- `SPEC.md` — what infuse is, what it is not, and the design principles that must not be reopened.
- `README.md` — the current 0.5 surface and repository layout.
- `docs/ARCHITECTURE.md` — package boundaries, state ownership, i18n and frontend rules.
- `docs/FALHAS-DE-AGENTES.md` — the catalogue of agent failures and the mechanical locks. Some rows
  are stale; verify each in code.
- `docs/PROMPT-infuse-studio.md` — the full product target.
- `internal/agent`, `internal/oracle`, `internal/filter`, `internal/api`, `internal/llm`,
  `internal/desktop`, `internal/workspace`, `ui/src` — the code you will change.
- `git status`, `git log --oneline -15`, `git branch -vv`.

Then build and run the application before judging any behavior.

## 2. What infuse is — the objective you must preserve

infuse is a **mandatory, instantaneous, agent-independent interposition layer** between a coding
agent and the project. Water goes in clear and comes out dark:

- Everything the agent **reads** arrives infused with the project's knowledge: map, conventions,
  layered instructions, memory, lessons, rules and the definition of done.
- Everything the agent **writes or runs** is judged by that knowledge before it touches disk.
- The agent **sees** the criteria but **cannot reach** the executor; verification is by evidence,
  never by the agent's report.
- The filter is **deterministic and sub-second**. There is no second LLM in the filter path.
- It is **one executable**, with no runtime and no dependency; the agent does not call it, configure
  it or opt into it. It passes through it.

The application must also guarantee, mechanically wherever possible:

- **No fraud**: no claiming work that was not done, no editing tests to pass, no swallowed exit
  codes, no hollow stubs, no invented URLs or packages, no fake evidence.
- **No shortcuts**: the cheapest path is not acceptable when it is not the correct path; the agent
  must diagnose before measuring, read before acting and verify before declaring done.
- **Clean architecture**: one responsibility per file, the correct folder structure, no god files,
  no feature spread across many files, one owner per state, one source of truth, tables when a
  literal branch chain would be worse, JSON with schema when data needs a contract, i18n through
  the single catalog when text is user-facing.
- **No dirty code**: no dead code, no duplicated implementations, no suppressed errors, no
  placeholders, no language mixing.
- **English only**: code, identifiers, comments, commit messages, tests and documentation are in
  English. Portuguese exists only as the `pt-BR` translation inside the shared UI catalog.
- **Evidence over claims**: every "done" must be reproducible from files, logs, high-resolution
  screenshots, test output, journal entries or a verdict.

## 3. Non-negotiable constraints

### 3.1 Model access — use the installed desktop apps, never DeepSeek

The AI that executes this audit must run on the **Claude Code desktop** and/or the
**ChatGPT/Codex desktop** installed on this computer, through the existing bridge providers
`claudedesk` and `codexdesk` (`internal/llm/bridge.go`, `bridge_cli.go`, `bridge_proto.go`). These
are the model sources for planning, coding, testing and reviewing.

- **Do not use the DeepSeek API**, a DeepSeek key, or any other external API as the model source for
  this task. DeepSeek must not be used to reason, plan, write code, run tests or review.
- When the application needs a model to exercise a flow, use `mock` for deterministic automation, or
  the `claudedesk`/`codexdesk` bridges for real-model integration. The provider screen may be
  exercised with its own configured providers as part of the test matrix, but the audit itself must
  not depend on DeepSeek or any external API.
- The CLIs are **only a pipe to the model**. They must not execute tools or touch the project:
  - Claude Code runs with `--tools ""`, `--strict-mcp-config`, `--setting-sources ""`, in a scratch
    directory.
  - Codex runs with `exec --json --sandbox read-only --skip-git-repo-check`, in a scratch directory.
- **All tool execution happens inside infuse** — through its own gates, journal, checkpoints and
  permission model. Verify this, do not assume it.
- If neither CLI is available, stop and report it. Do not silently substitute another provider.

### 3.2 Never leave AI running

- Use the `mock` provider for deterministic UI and flow automation.
- Use the real bridges only for bounded integration tests, with a timeout, one session at a time.
- At the end of every test, stop the session, kill child processes and confirm no model request is
  still in flight. Never spend the user's credits unattended.

### 3.3 UI verification protocol — Chrome + Playwright, high resolution (hard rule)

This is not optional and is not satisfied by the embedded browser.

- **Open the application in Google Chrome through Playwright**, against the server started with
  `infuse serve <port>` and the app token. Use Playwright's Chrome channel or the installed Chrome
  executable; do not use WebView2, the app's native window, the app's Browser pane or the agent's
  Browser tool for the audit.
- **Run headless by default** so no window pops up on the user's screen. A headed Chrome window is
  allowed only if the human explicitly asks for it; the embedded browser is never the verification
  surface.
- **Use 2× device scale factor** (and larger when useful): for example, `viewport: {width: 1440,
  height: 900}` with `deviceScaleFactor: 2` produces a `2880×1800` PNG. Never downscale before
  analysis; keep the original resolution.
- **Capture every screen and state**: each page, pane, tab, menu, dialog, context menu, tooltip,
  empty/loading/error/disabled state, long text, responsive width, light/dark theme and pt-BR/en
  language. Take a full-page and a viewport screenshot where each is relevant.
- **Look at every screenshot**. Open the PNG and analyze it visually for hierarchy, spacing,
  typography, contrast, alignment, clipping, overflow, missing controls, wrong states and
  regressions. Saving a screenshot without reading it does not count as evidence.
- **Before/after every fix**: reproduce the defect, capture the before shot, fix it, capture the
  after shot at the same viewport, theme and language, and compare them.
- **Record console and network**: fail the sweep on any console error or any HTTP response ≥ 400.
  Save the console log and the network summary next to the screenshots.
- **Exercise every control**: click every button, open every menu, toggle every switch, change every
  option, open every pane and tab, run every setting, test keyboard and mouse paths, and verify the
  resulting state in the UI and in the backend.
- **Screenshots are deliverables**: save them under `docs/audit/screens/` with stable names
  (`<area>__<state>__<theme>__<language>__<before|after>.png`) and link them from
  `docs/AUDIT-REPORT.md`.
- If Playwright or Chrome is missing, install Playwright locally in a scratch directory with npm;
  do not install it system-wide without asking.

### 3.4 No popups, no windows on the user's screen

- Preview the application on `127.0.0.1` with the app token.
- Use headless Chrome through Playwright for the audit; do not open the agent's browser as a visible
  window on the user's desktop.
- Do not add popups, modal interruptions, telemetry prompts or startup dialogs.

### 3.5 Evidence over claims

Every item in the test matrix below needs evidence: a high-resolution screenshot taken in Chrome via
Playwright, a console/network log, a test name that fails before and passes after, a journal entry, a
verdict file, or a recorded HTTP response. A prose statement is not evidence.

### 3.6 Language policy

- All code, identifiers, comments, test names, commit messages and documentation in **English**.
- The only Portuguese in the repository is the `pt-BR` value inside `internal/core/catalog.json`
  (and end-user documentation explicitly meant for the user).
- No mixed-language comments, no Portuguese identifiers, no Portuguese runtime error strings that
  bypass the catalog.
- Add a deterministic check that fails the build when new non-English comments/identifiers/UI
  literals appear; this is one of the gaps to close.

### 3.7 Architecture and code quality rules

- No source file above 400 lines; no function above 60 lines (the repository already enforces this
  in Go and in the frontend; keep it true).
- One responsibility per file; one state owner; one catalog; one store; no duplicated domain state.
- Dependencies point down (`cmd` → `api`/`desktop` → `agent` → capabilities → `config`/`llm`/`core`).
- Behavioural data belongs in tables (`internal/data/providers.json`, tool registration, route
  groups, pane registry), not in scattered conditionals.
- Data with a contract gets a JSON schema; user-facing text goes through the catalog; colours and
  sizes go through DTCG tokens; static styles and colour literals are rejected.
- Do not touch other worktrees or branches. The `ui-rebuild` branch is a historical reference: its
  UI was adapted into `master` in `1f4b5a2`, then visually adjusted in `87ba3ba`. Do not merge the
  branch wholesale and do not delete the user's work.

### 3.8 Git

- Do not commit or push unless the human explicitly asks in this session.
- If asked, create focused commits (one theme per commit), never force-push, never rewrite published
  history, never commit secrets.
- Review `git status` before any destructive git operation and stash/commit user work first.

## 4. Current state and known gaps (verify each; do not trust the docs)

The repository is at `d6b3f6f`, `origin/master` is synchronized, and the full Go suite, `check-ui`,
`check-i18n` and `check-frontend` passed at that commit. The UI from `ui-rebuild` was converted into
`master`; the branch remains as a reference.

The last audit found these gaps. Re-verify each in code before fixing; some documentation is more
pessimistic than the implementation.

1. **Bash/MCP/subagent write bypass.** Every write-time gate (architecture, file size, conventions,
   anti-cheat, JSON schema, i18n) runs only for tools of category `write` (`Write`, `Edit`,
   `MultiEdit`). `Bash` is `exec`; heredocs, `>`, `tee`, `sed -i`, `node -e`, script files, MCP
   tools and subagent Bash can write without any gate. The filter guard only protects
   `.infuse/oracle` and listed paths. Close this at the executor level (audit every changed file,
   including files created by shell commands, before the turn can end; refuse or journal what
   violates the rules). This is the highest-priority architectural gap because it nullifies the rest.
2. **The oracle does not govern a normal turn.** Only missions run the executor automatically.
   "Pronto?" is a manual button and its result is not fed back to the agent. `/api/oracle/run`
   accepts any method; a local GET needs no token; `checks.json` and `verdict.json` are forgeable by
   a script; HTTP checks run against whatever server is already running, not against the snapshot;
   there is no criteria hash, no link to the checkpoint and no browser check. Harden the executor and
   make the verdict part of the definition of done for every turn.
3. **No deterministic detection** of feature spread, god classes, dead code, or duplicated domain
   state outside the current JS regexes. Add real checks; do not rely only on the independent
   reviewer.
4. **English-only and i18n are not enforced.** Portuguese literals, enums, provider labels, journal
   strings, mission/oracle states and fallback HTML bypass the catalog. `check-i18n.mjs` only scans
   literal `t('…')`, `setRow`, `toast` and `data-i18n`; it does not scan Go `M()` ids, registry keys
   or Python/Go literals, and it never rejects Portuguese. The convention learner even blesses the
   existing comment language. Fix both the literals and the checks.
5. **Single source of truth, tables and JSON coverage are partial.** State-owner detection is a JS
   regex; table detection only catches literal `if`/`case` chains; JSON schema validation is
   bypassed by Bash and the "any `schemas/` directory requires `$schema` on every JSON" rule is
   over-broad. Extend coverage without blocking legitimate data files.
6. **Specific locks still missing:** B9 (baseline/threshold/budget files), D4 (background processes
   still running at the end of a turn), F6 (subagent claims without provenance in the Report), B2
   (command claim matched by substring instead of exact execution), B5 (suppressors warn but do not
   block). Close them with tests.
7. **Knowledge gaps:** `GEMINI.md`, `.cursorrules` and `.github/copilot-instructions.md` are not
   injected; stale compiled rules survive a failed recompilation; the project map can be stale
   without an indicator; lessons reach the prompt as names and counts but without their remedy; the
   reviewer is blind to the map and the digested facts; the digest caps at 80 documents and proceeds
   silently on failure.
8. **Spec items truly absent:** architecture bases (scaffolds per project type) and the FUSE/WinFsp
   mount. Everything else in `SPEC.md` is implemented.
9. **The command terminal is reported non-functional.** Reproduce it, find the root cause and fix it
   completely (see section 6).
10. **Documentation is stale.** `README.md`, `SPEC.md` and `windows/LEIA-ME.txt` list as "next"
    features that already exist. `docs/FALHAS-DE-AGENTES.md` marks as "missing" locks that are
    implemented with tests. Update the docs to the truth in English.

## 5. Test scope — everything

Build a test matrix with these columns: `ID`, `area`, `action`, `expected`, `evidence`, `result`,
`defect`, `fix`. Keep it in `docs/AUDIT-REPORT.md` and keep it current. A row is only "pass" with
evidence attached: a high-resolution Chrome/Playwright screenshot, a console/network log, a test
name, a journal entry or a verdict file.

At minimum, cover:

### A. Launch, roles and build

- Double-click / no arguments opens the native window (WebView2 on Windows); `infuse ui` and
  `infuse panel`/`window` do the same.
- `infuse serve [port]` serves the same UI on 127.0.0.1; the embedded UI and the disk UI both work.
- The same binary copied as `bash.exe` acts as the interposed shell; `install`, `uninstall`,
  `status`, `map`, `journal`, `lessons`, `rules`, `watch`, `version` behave correctly.
- Tray icon (open/quit), single instance, window placement/theme persistence, close-to-tray.
- Native folder/file dialogs and drag & drop; the app token and same-origin protection.
- The native window and the Browser pane are tested as features in their own right, but the audit
  interactions and screenshots come from Chrome via Playwright.
- Cross-builds: Windows, Linux, macOS. Run what can run on this machine; state exactly what could
  not be run and why.

### B. Core UI

Every visible control is clicked, opened, toggled and verified in Chrome via Playwright: sidebar,
groups, pins, archive, search, session rows, new session, project picker, folder picker, split view,
rail, command palette, keyboard shortcuts, toasts, menus, dialogs, context menus, tabs, pane headers,
grippers, drag-to-reorder, focus rings, disabled states, empty states, loading states, error states,
long text, truncation, tooltips, copy buttons, attachments, image paste, drag & drop, file mentions,
model/effort pickers, permission selector, cost/context indicators, theme switch, language switch,
responsive widths (chat minimum, pane minimum, stacked panes), `prefers-reduced-motion`.

For each control: capture a 2× screenshot of its default state, opened/expanded state, disabled
state where applicable, and the state after the action. Analyze each image; do not just save it.

### C. Sessions

Create, resume, rename, archive, unarchive, delete, pin, fork, side chat, review, mission, suggest,
comments, bans, processes, search, split view, rewind/checkpoints, compaction, cost and spend limit,
title proposal, session list across projects, session created elsewhere (scheduler/another window),
restart and reload, stale frontend cache, concurrent sessions.

### D. Providers

Anthropic, OpenAI-compatible (OpenRouter/Groq/Together/Mistral/xAI/LM Studio/custom), Google Gemini,
Ollama, mock, and the bridges `claudedesk`/`codexdesk`. For each: connect, test, list models, select
model, disconnect, invalid key, missing CLI, login failure, timeout, cancellation, retry, streaming,
tool use, usage/cost accounting, key storage in the OS vault, no key in logs or UI. **DeepSeek must
not be used as the model source for this task.** The OpenAI-compatible preset may exist in the UI;
do not use it with a real DeepSeek key for this audit.

### E. Tools

Bash (foreground/background, timeout, cancel, cwd, exit code, streaming, large output), Read (text,
ranges, images, PDF, notebooks), Write, Edit, MultiEdit, Glob, Grep, LS, WebFetch, WebSearch, Todo,
AskUserQuestion, Memory, Skill, Diagnosis, Report, Contract, Impasse, Agent (explore/review/general),
Browser, BashOutput, KillShell, and MCP tools. For every tool: success, failure, permission denied,
malformed input, huge input, Unicode, paths outside the project, protected paths, cancellation.

### F. Permissions and security

Ask / accept edits / plan / bypass; "always for this session"; per-tool rules; Bash prefix rules;
plan mode forbids writing and execution; bypass warning; destructive command sentinel; protected
`.infuse/oracle` and `.infuse/evidence`; path traversal; symlink escape; secret redaction; spend
limit; prompt injection from web/MCP content; banned tool/command/path/action from the user;
subagent permissions; hooks cannot be used to bypass the filter.

### G. Quality gates

Plan before write, docs-before-use, reading coverage, diagnosis evidence, report claims, test
sentinel, anti-cheat locks (B1–B13), large removal, unknown packages, URL claims, screenshots after
UI changes, conventions, architecture, file size, function size, tables, tokens, store, constants,
duplication, cycles, JSON schema, i18n. Test each with a case that must pass and a case that must
be refused with the correct reason and a way out.

### H. Workspace and git

Status, diff per file, discard, commit, branch, worktree, PR/CI, review comments, failed-log repair,
editors, dev servers, snapshots, local diff, treesnap, file modes, ignored files, binary files,
large files, merge conflicts, detached HEAD, no git repository.

### I. Terminal

See section 6. Every terminal behavior is in scope, exercised in Chrome via Playwright and captured
at 2×.

### J. Browser

CDP connection, tabs, navigation, content extraction, inspect, click/type/scroll, screenshots,
gestures, recording, downloads, errors, page crashes, multiple tabs, native browser pane, headless
vs visible, screenshots served by `/api/shot`. Test the Browser pane as a feature; do not use it as
the audit surface.

### K. MCP

stdio/HTTP/SSE, discovery of tools/resources/prompts, approval, enable/disable per project, status,
OAuth if supported, malformed server, server crash, restart, tool schema, large output, untrusted
content marking.

### L. Customization

Settings pages (general, providers, models, permissions, code, apps, integration, about),
Customize (commands, skills, connectors, hooks, docs, conventions, instructions), Scheduled tasks,
themes, output styles, status line, notifications, environment variables per project, language.

### M. Oracle

Criteria editor, suggestions, run, per-check results, evidence, verdict history, pass/fail/error/
no-criteria, mission integration, read-only protection, tampering attempts, criteria hash, snapshot
immutability, setup/command/http/file/maxLines checks, cancellation, concurrent run.

### N. Knowledge and memory

Project map freshness, convention learning, instruction layers (CLAUDE.md/AGENTS.md/GEMINI.md/
.cursorrules/copilot), imports, deleted instruction files from git HEAD, memory (session/project/
global), docs pinning/digest, skills index, lessons/vices, user rules, compiled rules with citations.

### O. Interceptor proof — coding must happen in infuse

Run one bounded session with `claudedesk` and one with `codexdesk`. Ask the model to create a file,
edit it and run a command. Prove:

- the model's tool calls appear as infuse tool blocks, are journaled, and produce a checkpoint;
- the file is written by infuse's Write/Edit and the command runs through infuse's Bash/filter;
- the CLI child process command line contains `--tools ""` (Claude) or `--sandbox read-only`
  (Codex) and runs in a scratch directory, not the project;
- the CLI did not modify the project, spawn a shell, or use its own tools;
- no request went to DeepSeek or any API other than the CLI's own authenticated backend;
- stopping the session kills the CLI process and leaves no orphan.

### P. Adversarial and negative tests

Fraud attempts, oracle/evidence writes, baseline/threshold edits, test edits, `|| true`, `exit(0)`,
stubs, TODO, test literals in production, swallowed errors, prompt injection, malicious MCP/web
content, path traversal, symlinks, secrets in output, huge files, CRLF/LF, Unicode, paths with
spaces, missing permissions, network failures, provider 5xx, timeouts, cancellation mid-stream,
server restart, app crash, disk full (simulated), concurrent writes, two windows.

### Q. UI/UX, usability, design and hedonic UX

Evaluate both **pragmatic quality** (usability, clarity, efficiency, errors) and **hedonic quality**
(stimulation, identification, evocation, trust, premium feel). Check: visual hierarchy, spacing,
typography, colour and contrast (WCAG AA), consistency, feedback, latency perception, motion,
states, copy, discoverability, cognitive load, keyboard-first flows, focus management, ARIA, screen
reader labels, language switching, responsive layout, dark/light theme, no flicker/layout shift, no
popups, no hidden controls, no duplicated projects, no stale lists, no console errors.

Every one of these checks is done in **Chrome via Playwright at 2×**, in light and dark, in pt-BR
and en, at desktop and narrow widths. Take before/after screenshots, open each PNG, and describe
what is wrong and what changed. Compare with the classic interface and the `ui-rebuild` reference;
do not accept a regression in polish, spacing or clarity.

### R. Architecture and code quality

Run `go vet ./...`, `go test ./...`, the race suite, cross-builds, `check-ui`, `check-i18n`,
`check-frontend`, `gen-tokens` reproducibility, and the architecture tests. Add regression tests for
every defect. Ensure no file above 400 lines, no function above 60, no static style, no colour
literal outside tokens, no missing catalog key, no dead code, no duplicated feature, no mixed
language.

### S. Cross-platform

Windows is the primary runtime. Build and smoke-test Linux and macOS where possible; state what was
not run. Check path separators, case sensitivity, ConPTY vs script PTY, dialogs, tray, WebView.

### T. Recovery and robustness

Cancel mid-turn, kill the server, reopen the app, resume a session, restore a checkpoint, recover
from a provider error, from a failed tool, from a corrupted settings/session file, from a full disk,
from a locked file, from a stale lock, from a second instance, from a port already in use.

## 6. Terminal — dedicated acceptance criteria

The user reports the command terminal does not work. Fix it completely and prove it in Chrome via
Playwright at 2×. Relevant code: `internal/terminal/*` (`terminal.go`, `terminal_windows.go`,
`terminal_unix.go`), `internal/api/terminal.go`, `appTerminal.go`, `studio_terminal.go`,
`ui/src/panes/terminal.js`, `ui/src/core/api.js`, `internal/desktop/*`.

Test at least:

- Open/close the Terminal pane; multiple tabs; tab switch; reopen after close; session switch.
- Prompt appears; `echo hello` prints; `cd` changes the working directory; `pwd` matches.
- ANSI colours, cursor movement, clear screen, long lines, wrapping, scrollback.
- Resize the pane and the window; the PTY size follows; no corrupted output.
- Ctrl+C interrupts; Ctrl+D exits; exit codes are correct.
- Interactive programs (a REPL, `python`, `node`, `vim` if available) receive input and redraw.
- Copy/paste; selection; keyboard shortcuts; Unicode and CRLF; paths with spaces.
- Background command, long-running command, close while running, kill process tree.
- No extra console window on Windows; no orphan processes; no leaked handles.
- Terminal theme follows light/dark tokens; contrast is correct.
- The terminal is part of the session's workspace and does not bypass the filter/protected paths
  where the product requires protection.
- Add automated tests where possible and provide 2× Chrome/Playwright screenshots and logs for the
  manual cases.

## 7. Method

1. Plan the work with phases and keep the plan current.
2. Build and run the app. Use the portable Go toolchain; do not install a system-wide Go without
   asking. The known path is
   `C:\Users\Jonathan\AppData\Local\Temp\infuse-refactor\toolchain\go\bin\go.exe`; verify it exists.
3. Start the server with `infuse serve <port>` and the app token. Open it in **Google Chrome via
   Playwright**, headless by default, `deviceScaleFactor: 2` (for example `1440×900` → `2880×1800`
   PNG). Do **not** use the embedded WebView2 window, the app's Browser pane or the agent's Browser
   tool for the audit. Capture console and network logs and fail on any console error or HTTP ≥ 400.
4. For each area: reproduce in Chrome, capture the before screenshot at 2×, read the image, find the
   root cause, fix it, capture the after screenshot at the same viewport/theme/language, add a
   regression test, re-run the affected checks.
5. Batch independent fixes; after each batch run `go vet`, `go test`, the frontend checks and the
   architecture tests.
6. Keep a defect log: ID, severity, reproduction, root cause, fix, test, screenshot paths, status.
7. At the end, run the full matrix again from a clean build, a clean profile and headless Chrome.
8. Do not leave test sessions, projects, hooks, approvals, credentials, Chrome profiles or
   processes behind.

## 8. Definition of done

- Every row of the test matrix has evidence and passes; no open functional defects.
- The full UI was exercised in **Google Chrome via Playwright**, at 2×, not in the embedded browser;
  every screen/state has before/after 2× screenshots that were visually analyzed, linked from the
  report.
- The UI sweep has zero console errors and zero HTTP responses ≥ 400.
- The terminal passes every acceptance case in section 6, with 2× screenshots and logs.
- The interceptor proof passes for both `claudedesk` and `codexdesk`; all coding happens in infuse;
  the CLIs are tools-disabled/read-only pipes; no DeepSeek API is used.
- Every quality gate passes: `go vet`, `go test ./...`, race suite, cross-builds, `check-ui`,
  `check-i18n`, `check-frontend`, `gen-tokens`, architecture tests.
- The new deterministic checks for the gaps in section 4 are implemented and tested.
- No AI, model request, server, terminal, Chrome profile or background process is left running.
- No secret is printed, logged or committed.
- All code, comments, identifiers, tests, commits and documentation are in English; the only
  Portuguese is the `pt-BR` catalog translation and end-user docs.
- `docs/AUDIT-REPORT.md` contains the matrix, the defects, the fixes, the screenshot links, the
  console/network logs and the remaining risks. If something could not be tested in this
  environment, it says exactly why and what evidence was obtained instead.
- The repository builds from a clean checkout and the app opens and works from the built binary.

## 9. Deliverables

- The code fixes and the regression tests.
- `docs/AUDIT-REPORT.md` with the complete matrix and evidence.
- `docs/audit/screens/` with the high-resolution Chrome/Playwright screenshots (before/after, all
  themes and languages used), linked from the report.
- `docs/audit/logs/` with the console and network logs for the sweeps.
- Updated `README.md`, `SPEC.md`, `docs/ARCHITECTURE.md`, `docs/FALHAS-DE-AGENTES.md` and
  `windows/LEIA-ME.txt` so they match reality, in English (the LEIA-ME may remain the user-facing
  Portuguese guide if the project keeps it that way).
- If the human explicitly asks: focused commits and a push to
  `https://github.com/designathan1988/infuse`.

## 10. Failure protocol

- If a test cannot be run in this environment (missing platform, missing credential, missing
  service), stop that test, state exactly why, and provide the closest evidence. Never fake it.
- If Chrome or Playwright cannot run, stop the UI audit and report it; do not silently fall back to
  the embedded browser.
- If a fix requires a product decision, present the options with evidence and ask the human.
- If the application is blocked by an external dependency, report it and continue with everything
  that is not blocked.
- If you find a contradiction between documentation and code, trust the code and update the docs.
- Never declare "everything works" without the evidence required by section 8.
