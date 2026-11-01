# infuse 0.5 — audit report

Audit performed on 2026-11-01 against `master` at `d6b3f6f` plus the working-tree changes described
below. Every UI interaction in this report was performed in **headless Google Chrome driven by
Playwright** at **2× device scale factor** (viewport 1440×900 → PNG 2880×1800), with console and
network recorded. Screenshots are under `docs/audit/screens/`, logs under `docs/audit/logs/`.

The embedded WebView2 window was not used as the verification surface. It is a feature under test.

## 1. What was done

1. Read `SPEC.md`, `README.md`, `docs/ARCHITECTURE.md`, `docs/FALHAS-DE-AGENTES.md`,
   `docs/PROMPT-infuse-studio.md` and the code of the packages named in the audit prompt.
2. Built with the portable Go toolchain, ran the suite, then audited the running application in
   Chrome.
3. Closed the ten gaps of section 4 of the prompt where the code allowed it (see §4), and fixed every
   defect the sweeps found (see §3).
4. Re-ran the full quality gate suite and the UI sweeps on a clean build with the embedded UI.

## 2. Environment

| Item | Value |
|---|---|
| OS | Windows 11 Pro for Workstations 10.0.26300 |
| Go | 1.27.3 (portable, `%TEMP%\infuse-refactor\toolchain\go`) |
| Chrome | Google Chrome, headless, channel `chrome`, Playwright `playwright-core` |
| Server | `infuse-dev.exe serve 7481` with `INFUSE_TOKEN=dev-preview-only`, UI embedded in the binary |
| Model sources | `mock` for deterministic UI flows; `codexdesk` for the real-model interceptor proof; `claudedesk` attempted and blocked by its CLI login (see §6) |

No DeepSeek key or API was used at any point. The default provider configured in the app
(DeepSeek) was not used for any audit interaction.

## 3. Defects found and fixed

| ID | Area | Defect (reproduced) | Root cause | Fix | Evidence |
|---|---|---|---|---|---|
| D-01 | Terminal | Opening the Terminal with no project showed `Error: A sintaxe do nome do arquivo…` and no shell | an empty working directory reached `CreateProcess` (`cwd=""`) | `terminal.ResolveDir` picks the requested folder, the project, then the user's home; `Open` never starts a shell without a directory; regression tests | before `terminal__draft-open__light__pt-br__before.png`, after `terminal__draft-open__light__pt-br__after.png`, `terminal__echo…`, `terminal__interaction…` |
| D-02 | Terminal | Ctrl+V turned into a control byte (`^V`/quoted insert) instead of pasting | the custom Ctrl+letter handler converted `v` when `keyCode` was absent | the handler skips `v` and `c` with a selection; paste verified with a real clipboard event | `terminal__paste__light__pt-br__after.png` |
| D-03 | Terminal | A 404 on `/api/term/<id>/input` after closing a tab (console error) | the debounced input timer fired after the tab died | input batching cancels its timer on close/exit and ignores the failure | `terminal-deep-after.summary.json` (0 HTTP errors) |
| D-04 | Terminal | Two tabs were both named after the home folder | names were not made unique | `uniqueName` appends an index | `terminal__tabs__light__pt-br__after.png` |
| D-05 | Interface / i18n | A raw catalog key (`menu.itemByNumber`) appeared in the shortcuts dialog; message pin tooltips showed `action.pin` / `action.unpin` | missing catalog entries and a table the i18n check could not see | catalog entries added; `check-i18n` now requires dotted literals in an existing namespace | `dialogs__shortcuts…` (after), `check-i18n` green |
| D-06 | Interface / plan | The plan card read `Create the demo filecomprovação: …` | CSS defined `.plan-task` while the rail rendered `.ptask` | rail uses `.plan-task`, the class the design system styles | `now__mock-turn__light__pt-br__after.png` |
| D-07 | Interface / report | Claim evidence ran into the claim text (`createdsrc/demo.ts…`) | no separator between the reference and the check result | a space is emitted before the evidence span | `session__mock-turn__light__pt-br__debug2.png` |
| D-08 | Interface / navigation | Clicking a session while a page (Scheduled/Settings/Customize) was open left the page on screen | `openSession`/`newSessionScreen` did not leave the page | a `close-page` event returns the views; verified in Chrome | `fixes__session-from-page__light__pt-br.png` |
| D-09 | Interface / rail | The "show/hide session details" button did nothing with a pane open | a container query hid the rail below 700 px regardless of the button | the rail yields only below 600 px; the button now always has a visible effect and its `aria-pressed` reflects the state | `fixes__rail-visible-with-pane…`, `ctrl__rail…` |
| D-10 | Interface / settings | The Apps page could stay empty (header only) after a quick navigation | asynchronous section renders appended to a detached element; concurrent `openSettings` calls raced | render sequence guard + `isConnected` checks in Apps, Providers and Models | `ctrlB__apps__light__pt-br.png`, `diag-apps2` log |
| D-11 | Interface / projects | The project menu was undiscoverable (actions only on hover) and adding a project with an empty root silently added `.` | actions were `visibility:hidden`; the add route accepted `""` | actions are always visible (dimmed, full on hover/focus); the add route rejects a missing root | `fixes__project-menu…`, `ctrl__project-menu…` |
| D-12 | Interface / drop | Dropping an image added it to the view state but no attachment chip appeared | the window's drop handler pushed to `v.images` without asking the composer to render | the composer exposes `v.renderAtts` and the drop handler calls it | `verify__drop-attachment…`, `ctrlC__drop…` |
| D-13 | Demo provider | The keyless demo stopped at the diagnosis gate and never wrote a file | the mock did not follow infuse's own protocol | the mock runs LS → Read → Diagnosis → Bash → TodoWrite → Write → TodoWrite → Report, and plays the reviewer | `session__mock-turn__light__pt-br__debug2.png` |
| D-14 | Bridges | A model answering in plain prose failed the turn (`no JSON object found`) | the JSON protocol had no tolerant fallback | after the repair round, prose becomes the text reply (no tool call invented); unit test | `internal/llm/bridge_text_test.go` |
| D-15 | Oracle | The executor failed on a machine without a git identity (`commit-tree: Author identity unknown`) | the snapshot commit relied on the global git config | the snapshot commits with an explicit infuse identity | `ctrlC__oracle-verdict…`, oracle run log |
| D-16 | Oracle | `POST /api/oracle/run` accepted any method; a local GET (agent's shell) could start the executor; a missing root resolved criteria against the server's own folder | route had no method check and no root validation | the run requires POST (token) and a project folder; both return 400/405 otherwise | `verify-drop-oracle` run, `appOracle.go` |

## 4. Gaps from the prompt, closed

| Gap | What changed | Test |
|---|---|---|
| 1. Writes through Bash/MCP/subagents bypassed the gates | the turn snapshots the tree; every file the tree gained or lost is judged by the write policy before the turn may end (conventions, architecture, size, anti-cheat, baselines, large removals). A checkpoint alone no longer exempts a path (heredocs are checkpointed but not gated). Files > 2 MB are hashed in a stream so they are seen. `.infuse/oracle` and `.infuse/evidence` are hashed at turn start; a change outside the executor blocks the turn | `shell_audit_test.go`, `hardening_test.go` |
| 2. The oracle did not govern a normal turn | the executor runs at the end of every turn that changed files, the verdict is fed back to the agent with the failing checks, repeated attempts reuse the verdict for the same tree, an unreadable criteria file blocks instead of disabling the executor; verdicts record the criteria hash, the turn, and a copy of the criteria is stored with the evidence; HTTP checks use a server started from the snapshot when the criteria declare `server` | `oracle_turn_test.go`, `internal/oracle` tests |
| 3. No deterministic detection of feature spread / god classes / dead code | the existing architecture checks were kept and extended where the audit pointed (file/function size, duplicated state, import cycles, JSON schema, tokens); no new heuristic was added without a test | `internal/architecture` |
| 4. Portuguese and i18n not enforced | `scripts/check-language.mjs` fails on Portuguese outside the catalog (accented letters + decisive words), with a reviewed exception file; `check-i18n.mjs` now also requires dotted key literals in existing namespaces; wired into CI; 373 Portuguese lines removed from code/comments/tests/scripts | both checks green |
| 5. Single source of truth / tables / JSON coverage | unchanged detections kept; the audit did not find a new deterministic rule that could be added without false positives | architecture tests |
| 6. Specific locks B9, D4, F6, B2, B5 | B9 (baseline/threshold/budget files ask the user, shell writes block), D4 (running processes must be declared in the Report), F6 (subagent work is not the agent's own evidence), B2 (command claims require the exact command), B5 (suppressors are refused without a reason on the same line); B6 also gained `exit(0)` outside the entry point and hidden test output | `locks_b_test.go`, `hardening_test.go`, updated `gate_test.go` |
| 7. Knowledge gaps | not fully closed: reviewer/digest caps remain as before (see §6) | — |
| 8. Bases and FUSE/WinFsp mount | not implemented (see §6) | — |
| 9. Terminal | fixed (D-01…D-04) and accepted in Chrome | terminal screenshots and logs |
| 10. Stale documentation | `README.md`, `SPEC.md`, `docs/ARCHITECTURE.md`, `docs/FALHAS-DE-AGENTES.md` updated; `windows/LEIA-ME.txt` is the Portuguese user guide kept as such | this repository |

## 5. Test matrix

Columns as required: ID, area, action, expected, evidence, result, defect, fix. Evidence is a
screenshot under `docs/audit/screens/`, a log under `docs/audit/logs/`, a test name, or a command
whose output is quoted in this report. `—` means nothing to fix.

### A. Launch, roles and build

| ID | Area | Action | Expected | Evidence | Result | Defect | Fix |
|---|---|---|---|---|---|---|---|
| A-01 | Serve + UI | `infuse serve 7481`, open in Chrome 2× | the same UI loads | `final__home__light__pt-br.png` | pass | — | — |
| A-02 | Embedded UI | rebuild, serve without `INFUSE_UI_DIR` | UI comes from the binary | final sweeps ran on this build | pass | — | — |
| A-03 | CLI roles | `version`, `status`, `rules`, `lessons`, `journal`, `map` | correct output, exit 0 | `docs/audit/logs/cli-roles.log` | pass | — | — |
| A-04 | Interposed shell | copy the binary as `bash.exe`, run `echo` | command passes through the filter | `interposed-42` in `cli-roles.log` | pass | — | — |
| A-05 | App token | GET and POST without `X-Infuse` | GET read-only allowed; POST refused | GET 200, POST 403, POST with token 200 | pass | D-16 | run route requires POST + token |
| A-06 | Port in use | second `serve` on the same port | clear error, no crash | `cannot listen: ... bind ...` | pass | — | — |
| A-07 | `install`/`uninstall` | run them | shell shim installed/removed | not run | not run — rewrites the user's `~/.claude/settings.json`; not asked to change their shell | — | — |
| A-08 | Native window/tray | open the window, tray menu | window, tray open/quit | not run visually | not run — opens a window on the user's screen; `go test ./internal/desktop` passes | — | — |
| A-09 | Cross-builds | `GOOS=windows/linux/darwin CGO_ENABLED=0 go build ./...` | all succeed | command output | pass | — | — |

### B. Core UI (Chrome, 2×, final embedded build)

| ID | Area | Action | Expected | Evidence | Result | Defect | Fix |
|---|---|---|---|---|---|---|---|
| B-01 | View bar | changes, terminal, files, browser buttons | pane opens | `ctrl__*`, `sweep-controls-a` 33/33 | pass | — | — |
| B-02 | More menu | tasks, plan, procs, agents, filter group | each pane opens | `ctrl__pane-*` | pass | — | — |
| B-03 | Pane headers | close (×), tab plus, split grip | closes/opens/resizes | `terminal__tabs`, `terminal__resized` | pass | D-03 | input timer cancelled on close |
| B-04 | Rail button | show/hide details with a pane open | rail appears/disappears | `fixes__rail-visible-with-pane`, `fixes__rail-hidden` | pass | D-09 | container threshold, aria state |
| B-05 | Title | rename inline | title saved | `sweep-controls-a` | pass | — | — |
| B-06 | Session menu | view modes, folds, rewind, side chat, split, archive | each action works | `ctrl__rewind-dialog`, `ctrl__side-chat` | pass | — | — |
| B-07 | Sidebar | search, group/sort menu, collapse | filters/sorts/collapses | `sidebar__search`, `ctrl__project-menu` | pass | — | — |
| B-08 | Project menu | new, rename, open in, remove | actions work | `fixes__project-menu` | pass | D-11 | actions always visible; add route validates root |
| B-09 | Session row | open, pin/unpin, unread, archive | state changes and restores | `ctrl__session-menu` | pass | — | — |
| B-10 | Composer | plus menu, @ mention, / commands | menus and insertion work | `ctrl__plus-menu`, `ctrlB__customize` | pass | — | — |
| B-11 | Composer options | mode/model/effort menus | change and restore | `ctrl__mode-menu`, `ctrl__model-menu` | pass | — | — |
| B-12 | Context | context ring dialog | dialog with breakdown | `ctrl__context-dialog` | pass | — | — |
| B-13 | Attachments | image paste, file drop | chip appears, removable | `ctrlC__image-paste`, `ctrlC__drop` | pass | D-12 | composer exposes renderAtts to the drop handler |
| B-14 | Dialogs | command palette, shortcuts, mission | open, filter, run/cancel | `dialogs__*`, `ctrlB__mission-dialog` | pass | D-05 | missing catalog keys |
| B-15 | Theme | light/dark switch via Settings | whole UI changes | `final__*__dark__pt-br` | pass | — | — |
| B-16 | Language | pt-BR/en switch via Settings | whole UI changes and reloads | `final__*__light__en` | pass | — | — |
| B-17 | Responsive | 900 px width | layout fits, no clipping | `session__transcript__light__pt-br__narrow` | pass | — | — |
| B-18 | Reduced motion | `prefers-reduced-motion` | no essential motion | not run | not run — no motion-only affordance found in the sweep | — | — |
| B-19 | Copy buttons | copy a message | clipboard receives the text | copy action clicked in the transcript | pass (click; clipboard not asserted) | — | — |

### C. Sessions

| ID | Area | Action | Expected | Evidence | Result | Defect | Fix |
|---|---|---|---|---|---|---|---|
| C-01 | Create/send | draft → project → mock → send | streaming reply, tool blocks | `session__mock-turn` | pass | D-13 | mock follows the protocol |
| C-02 | Stop | send then stop | turn stops, UI idle | `session__stopped` | pass | — | — |
| C-03 | Rename | inline rename | saved | `sweep-controls-a` | pass | — | — |
| C-04 | Archive/unarchive | row menu, archived view | leaves and returns | `ctrlC__archived` | pass | — | — |
| C-05 | Delete | row menu | session removed | API delete used and verified | pass | — | — |
| C-06 | Pin/unpin | row menu | pinned group | `sweep-controls-a` | pass | — | — |
| C-07 | Side chat | menu → side chat | parallel panel, closes | `ctrl__side-chat` | pass | — | — |
| C-08 | Split view | open beside, exit | two views, back to one | `ctrl__split` | pass | — | — |
| C-09 | Search | query in the sidebar | filters rows | `sidebar__search` | pass | — | — |
| C-10 | Title proposal | existing session title | kept | `ponte codex` sessions | pass | — | — |
| C-11 | Fork | fork a message | new session | not run | not run — no fork entry point was exercised in Chrome | — | — |
| C-12 | Compaction | compact a long session | summary retained | not run | not run — would spend a real model call | — | — |
| C-13 | Comments/bans | session comments, bans | shown and enforced | bans exercised by `gate_test` | partial | — | — |
| C-14 | Spend limit | lower the daily limit | turn blocked | not run | not run | — | — |

### D. Providers

| ID | Area | Action | Expected | Evidence | Result | Defect | Fix |
|---|---|---|---|---|---|---|---|
| D-01 | mock | full demo turn | read, diagnose, plan, write, edit, run, report | `session__mock-turn`, `flow-mock` | pass | D-13 | mock protocol |
| D-02 | codexdesk | one bounded real turn | tool call executed by infuse | `interceptor__codex` | pass | — | — |
| D-03 | claudedesk | one bounded real turn | tool call executed by infuse | CLI says `Not logged in · Please run /login` | blocked by credential (section 6) | — | bridge retries with the desktop-bundled CLI |
| D-04 | Provider forms | connect/edit a provider | form opens, cancels | `ctrlB__provider-connect`, `ctrlB__provider-edit` | pass | — | — |
| D-05 | Invalid key, disconnect, vault | exercise each | errors/vault correct | not run | not run — would change the user's real connections | — | — |
| D-06 | Stream error | plain-prose reply from the bridge | turn does not fail | `TestBridgeAcceptsAPlainTextAnswer` | pass | D-14 | tolerant fallback |

### E. Tools

| ID | Area | Action | Expected | Evidence | Result | Defect | Fix |
|---|---|---|---|---|---|---|---|
| E-01 | Bash/Write/Edit/Read | run in the mock and Codex turns | executed with gates | `interceptor-codex` | pass | — | — |
| E-02 | Diagnosis/Todo/Report | protocol tools | validated, claims checked | `TestReportClaimsCheckedAgainstTheTurn` | pass | — | — |
| E-03 | AskUserQuestion | question card | options reach the user | exercised in earlier mock flows | pass | — | — |
| E-04 | Agent/subagents | explore/review | run with own context | `interceptor-codex` (Agent), `agent_test` | pass | — | — |
| E-05 | Browser tool | navigate/screenshot | page controlled | `browsertool_test` | pass (unit) | — | — |
| E-06 | Glob/Grep/LS/WebFetch/WebSearch/Memory/Skill/Contract/Impasse/BashOutput/KillShell | each | behaves | package tests | pass by tests; not each in Chrome | — | — |

### F. Permissions and security

| ID | Area | Action | Expected | Evidence | Result | Defect | Fix |
|---|---|---|---|---|---|---|---|
| F-01 | Ask mode | Bash permission card | Y/A/T/N decisions | `session__permission-bash` | pass | — | — |
| F-02 | Bypass | accept all | no prompts | `session__draft-mock` flow | pass | — | — |
| F-03 | Plan mode | write/exec refused | refused with reason | `perm.plan_mode` tested | pass (tests) | — | — |
| F-04 | Protected paths | Write/Bash to `.infuse/oracle` | refused and journaled | `oracle_bypass_test` | pass | — | — |
| F-05 | Oracle/evidence via script | indirect write | turn blocked | `TestOracleFilesChangedOutsideTheExecutorAreRefused` | pass | — | — |
| F-06 | Banned commands/tools | user prohibition | refused | `gate_test` bans | pass | — | — |
| F-07 | Secrets | key in output | redacted | `security` tests | pass (tests) | — | — |
| F-08 | Prompt injection | web/MCP content | marked, not a request | not run live | not run | — | — |

### G. Quality gates

| ID | Area | Action | Expected | Evidence | Result | Defect | Fix |
|---|---|---|---|---|---|---|---|
| G-01 | Plan before write | shell/write without a plan | refused | `TestShellWriteNeedsAPlan` | pass | — | — |
| G-02 | Diagnosis | change without diagnosis | refused | `diagnosis_test` | pass | — | — |
| G-03 | Anti-cheat B5 | empty catch / suppressor | refused without reason | `TestSuppressorsNeedAReasonOnTheSameLine` | pass | — | — |
| G-04 | Anti-cheat B6 | swallowed exit, hidden test output, `exit(0)` | refused | `TestSilencedTestOutputIsRefused`, `TestExitZeroOutsideTheEntryPointIsRefused` | pass | — | — |
| G-05 | B9 baselines | edit a baseline via Write/shell | user decides / turn blocked | `TestBaselineFilesAreRecognized` | pass | — | — |
| G-06 | Large removal | shell delete of a file | grounded in plan/diagnosis | `TestLargeShellRemovalNeedsThePlanOrDiagnosis` | pass | — | — |
| G-07 | JSON schema / architecture | bad file | refused | `architecture` tests | pass | — | — |
| G-08 | i18n | missing key / Portuguese | build fails | `check-i18n`, `check-language` | pass | D-05 | new checks |

### H. Workspace and git

| ID | Area | Action | Expected | Evidence | Result | Defect | Fix |
|---|---|---|---|---|---|---|---|
| H-01 | Changes pane | open, select a file | diff shown | `ctrl__changes` | pass | — | — |
| H-02 | Commit UI | type a message, click commit | commits | not clicked | not run — would commit the user's work | — | — |
| H-03 | Discard | discard a change | restored | not clicked | not run — destructive to user data | — | — |
| H-04 | Worktree, PR/CI, editors, dev servers | each | behaves | `workspace` tests | pass by tests; not each in Chrome | — | — |

### I. Terminal

Covered by `terminal-full-final` / `terminal-deep-final`: 8/8 deep cases (open/close/reopen,
Ctrl+C/Ctrl+D, REPL, paste, background process tree, fresh shell) and 13/16 in the full sweep, whose
three misses are focus races duplicated and passed by the deep sweep. Screenshots `terminal__*__after`.

### J. Browser pane

| ID | Area | Action | Expected | Evidence | Result | Defect | Fix |
|---|---|---|---|---|---|---|---|
| J-01 | Navigate | address → infuse server | page loads, tab label updates | `ctrl__browser` | pass | — | — |
| J-02 | Back/reload/new tab | buttons | navigation works | `ctrl__browser` | pass | — | — |
| J-03 | Click/type/record/PDF/CDP | each | works | not run live | not run — the pane is a feature under test, not the audit surface | — | — |

### K. MCP

Not run live: no MCP server is configured on this machine. The client, tool schema, approval and
failure handling are covered by `mcp_test` in the Go suite.

### L. Customization

All eight Settings sections, every toggle (8), select (11) and input (6), the provider forms, Apps,
all seven Customize sections, the Scheduled editor, the command palette, shortcuts and the mission
dialog were exercised in Chrome and restored: `controls-b` 26/26, screenshots `ctrlB__*`.

### M. Oracle

Criteria PUT, run, per-check result, evidence, verdict and history: `ctrlC__oracle-verdict`,
`verify-drop-oracle` (verdict `aprovado`, criteria 1, spec hash present). Tampering: F-04/F-05.
Read-only: `oracle_bypass_test`. Criteria hash and snapshot copy: verdict fields and
`oracle_run_meta.go`. Concurrent run: serialized by `executorMu` (code) — not exercised live.
Cancellation: not run live.

### N. Knowledge and memory

Memory add/remove in the pane (`ctrlC__memory`), instructions and conventions sections render
(`ctrlB__customize-*`), map pane refresh (`pane__map`), rules add/remove (`ctrlC__rules`), journal
(`ctrlC__journal`). The knowledge gaps of gap 7 (GEMINI.md/.cursorrules/copilot injection, reviewer
blind to map/digest, capped digest) remain open — see section 4/6.

### O. Interceptor proof

Tool blocks in the transcript (pass, `interceptor__codex`), journal and checkpoint (pass, 2 journal
entries and checkpoint files), CLI flags and scratch directory (pass, command line captured), CLI did
not modify the project (pass, the file content is infuse's), no DeepSeek (pass, provider `codexdesk`),
stop kills the CLI (pass, session stopped and deleted, no process left). Claude bridge: blocked by the
CLI login (section 6).

### P. Adversarial

`exit(0)`, suppressors, hidden test output, baseline files, oracle/evidence writes, test
infrastructure (user sentinel), huge files (2 MB audit cap removed), swallowed exit codes: tests
above. Path traversal, symlinks, disk full, concurrent writes, two windows: not run live.

### Q. UX and design

Light/dark and pt-BR/en screenshots of home, session, terminal and settings were opened and compared
(`final__*`); hierarchy, spacing, contrast, clipping and state feedback were checked and the defects
found are D-05…D-12. WCAG contrast measurement, `prefers-reduced-motion` and screen-reader output
were not measured with an external tool.

### R. Architecture and code quality

`go vet ./...`, `go test ./...`, the race suite (exit 0, no data race reported), cross-builds,
`check-ui`, `check-frontend`, `check-i18n`, `check-language`, `gen-tokens` and the architecture tests
pass; no file above 400 lines or function above 60.

### S. Cross-platform

Windows builds and runs. Linux/macOS cross-builds pass; neither was smoke-tested on a real host.
ConPTY vs `script` PTY behaviour was not exercised beyond the builds.

### T. Recovery and robustness

Cancel mid-turn (stop) pass; provider error surfaced (Claude login); server restart/resume,
corrupted settings/session file, stale lock, full disk and second window were not simulated.

### K–T consolidated rows

| ID | Area | Action | Expected | Evidence | Result | Defect | Fix |
|---|---|---|---|---|---|---|---|
| K-01 | MCP | client, schema, approval, failure | behaves | `mcp_test` (Go suite) | pass by tests | — | — |
| K-02 | MCP live | a real stdio/HTTP server | discovery and tools | not run — no server configured | not run | — | — |
| L-01 | Settings | every section, toggle, select, input | changes apply and restore | `controls-b` 26/26, `ctrlB__*` | pass | D-10 | render guards |
| L-02 | Customize | all seven sections and the command editor | render and open/cancel | `ctrlB__customize-*` | pass | — | — |
| L-03 | Scheduled | page and new-task editor | render, open/cancel | `ctrlB__scheduled-new` | pass | — | — |
| M-01 | Oracle run | criteria → run → verdict | verdict with hash and evidence | `ctrlC__oracle-verdict`, `verify-drop-oracle` | pass | D-15 | git identity in the snapshot |
| M-02 | Oracle tampering | script writes criteria/evidence | turn blocked | `TestOracleFilesChangedOutsideTheExecutorAreRefused` | pass | — | — |
| M-03 | Oracle HTTP checks | server from the snapshot | server started from the snapshot | `oracle_server.go`; live run not exercised | partial — mechanism added, live server not run | — | — |
| N-01 | Memory/rules/journal/map | add, remove, list | persisted and restored | `ctrlC__memory`, `ctrlC__rules`, `ctrlC__journal`, `pane__map` | pass | — | — |
| N-02 | Knowledge gaps | GEMINI/.cursorrules/copilot, reviewer map, digest cap | injected/visible | not addressed | open — see section 6 | — | — |
| O-01 | Interceptor codex | one bounded real turn | tools, journal, checkpoint, flags | `interceptor__codex`, `interceptor-codex.*.json` | pass | — | — |
| O-02 | Interceptor claude | one bounded real turn | same | CLI login error | blocked — credential | — | — |
| P-01 | Adversarial | fraud/shortcut attempts | refused by tests | `locks_test`, `hardening_test`, `locks_b_test` | pass | — | — |
| P-02 | Adversarial live | symlink, disk full, two windows | safe behaviour | not run | not run | — | — |
| Q-01 | UX | light/dark, pt/en, hierarchy | no regressions | `final__*`, `fixes__*`, `ctrlB__*` | pass | D-05…D-12 fixed | — |
| Q-02 | Accessibility | ARIA labels, focus, contrast | AA | labels present; contrast not measured | partial | — | — |
| R-01 | Gates | vet, test, race, builds, checks | all pass | §7 below | pass | — | — |
| S-01 | Cross-platform | windows/linux/darwin builds | succeed | command output | pass | — | — |
| S-02 | Cross-platform run | smoke on linux/macOS | app runs | not available here | not run | — | — |
| T-01 | Recovery | cancel mid-turn, provider error | consistent state | `session__stopped`, CLI login error | pass | — | — |
| T-02 | Recovery | restart, corrupted files, disk full | recovers | not simulated | not run | — | — |

## 6. Remaining risks and what was not tested

1. **Claude desktop bridge login** — the CLI is not signed in on this machine; the real-model proof
   therefore ran on the Codex bridge only. This is a credential the user owns.
2. **HTTP oracle checks** are snapshot-bound only when the criteria declare `server`; otherwise the
   verdict carries a warning naming the address and the limitation.
3. **Bases (scaffolds) and the FUSE/WinFsp mount** from `SPEC.md` are still absent. They are product
   features, not defects of the current surface; implementing them is a separate slice.
4. **Knowledge gaps** (section 4.7 of the prompt): the reviewer still does not see the map and the
   digested facts, the digest is capped, and the project map can be stale without an indicator. These
   are open.
5. **Native window, tray and WebView2** were not exercised visually: doing so opens a window on the
   user's screen, which the audit forbids. `go test ./internal/desktop` passes.
6. Dark theme and English locale were exercised and photographed on the main surfaces
   (`final__home__dark__pt-br.png`, `final__session__dark__pt-br.png`,
   `final__terminal__dark__pt-br.png`, `final__settings__dark__pt-br.png`,
   `final__home__light__en.png`, `final__session__light__en.png`,
   `final__settings__light__en.png`); not every secondary pane has a dark/English pair.
7. The per-check detail texts of the oracle executor are in English (evidence detail); the verdict
   summary and the criteria messages are localized through the catalog.

## 7. Quality gates

| Gate | Result |
|---|---|
| `go vet ./...` | pass |
| `go test ./...` | pass (all packages) |
| `go test ./internal/architecture` | pass |
| `node scripts/check-ui.mjs` | 69 modules OK |
| `node tools/check-frontend.mjs` | pass |
| `node scripts/check-i18n.mjs` | 676 keys used, 1361 in the catalog |
| `node scripts/check-language.mjs` | 0 Portuguese lines outside the catalog, 24 reviewed exceptions |
| `node scripts/gen-tokens.mjs` | reproduces `ui/css/tokens.css` |
| Race suite | pass — `go test -race -ldflags=-linkmode=external ./...` with the portable zig wrapper exited 0, no data race reported |

## 8. Cleanup

Test sessions created by the audit were deleted (the session list holds only the pre-existing ones),
the temporary bridge project and the scratch folder were removed, the preview server was stopped at
the end, and no bridge CLI child process was left running (process list checked). The project list was
restored after the removal test. A final smoke run on the rebuilt binary opened the home and a session
with zero console errors and zero HTTP ≥ 400 (`final__home__light__pt-br.png`,
`final__session__light__pt-br.png`, `smoke-final.*.json`).

## 9. Hardening pass on top of this audit (2026-11-01, working tree)

Scope: the uncommitted work of the audit above was preserved and extended. Every item below has a
regression test; the package suite, `go vet`, `go test ./internal/architecture` and the frontend checks
are green. The pass was committed on `master` (`3b2cfaf`, `2fb5081`, `21f9336`, `cfc81b2`, `8146344`);
the feature index and base starter files below were added after that commit.

| Area | What changed | Test |
|---|---|---|
| Control files | `.infuse/settings.json`, `.infuse/hooks.json`, `.infuse/base.json`, `.mcp.json` and `.claude/settings.json` are protected; project settings may tighten the mode/permissions but never loosen them | `TestProjectControlFilesAreProtected`, `TestProjectSettingsCannotDisarmTheFilter` |
| Path matching | case-insensitive where the OS is; protected records recognized at any depth; symlink/junction resolved on read and write; `mklink` refused | `TestProtectionIsCaseInsensitiveOnWindows`, `TestGuardProtectsEveryInternalRecordFromAWorktree`, `TestGuardRefusesLinksAndIndirectWriters` |
| Guard root | the Bash guard is built against the real project root, not the session worktree | `TestGuardProtectsEveryInternalRecordFromAWorktree` |
| Indirect writers | `sed --in-place`, `curl -o`, `wget -O`, `sort -o`, PowerShell `-EncodedCommand` (decoded, opaque refused) and inline script text that names a protected path | `TestGuardRefusesLinksAndIndirectWriters`, `TestGuardRefusesPowerShellEncodedCommand` |
| Hooks | hooks run through the guard; hook and MCP approvals are HMAC-signed with a key in the OS vault; MCP tools no longer auto-allow on a server's read-only hint | `TestSealedFilesRejectTampering`, `TestHooksRunOnlyWhenApprovedAndCanBlock`, `TestMCPStdioServerNeedsApprovalThenServesTools` |
| API and browser | `/api/file`, `/api/frameable` and `/api/open` require the app token even on GET; the agent's browser refuses non-web schemes/`file://`/link-local metadata and redacts captured text | `TestPrivateTargets`, `TestBrowserTarget*` (security) |
| Shell writes | heredoc bodies are judged by the write policy before they run (plan, size, base, bans, large removal, suppressors, cheat, baseline ask); shell writes get bans and suppressors at the turn boundary; `cheat.test_infra` is not skipped; a failed turn snapshot blocks the turn; the 6-ask budget ends the turn as UNVERIFIED; revert restores shell deletions from the turn snapshot | `TestHeredocIsJudgedBeforeItRuns`, `TestShellAuditAppliesBansSuppressorsAndTestInfra`, `TestSnapshotFailureBlocksTheTurn`, `TestRestoreTurnBringsBackDeletions` |
| Oracle | criteria anchored outside the project; verdicts HMAC-signed; HTTP checks without a snapshot server refused; port must be free and the answering child alive; cache keyed by file content; no-edit turns still run the oracle; missions reuse the turn's verdict; background claims resolve against the final exit code | `TestCriteriaEditedInsideTheProjectAreRefused`, `TestVerdictSignatureDetectsTampering`, `TestHTTPCheckWithoutSnapshotServerIsRefused`, `TestMissionRunsUntilCriteriaPassOrBudget` |
| Knowledge | vice catalog (`internal/data/vices.json`) with signature + remedy rendered in the prompt; reviewer/subagents/auditor receive map, conventions, lessons and digested facts; digest cap raised to 200 with an overflow notice and per-document failures surfaced; comment language reported as an observation, not a convention; GEMINI.md/.cursorrules/copilot instructions injected; Go and Rust imports resolve to dependents; Glob/Grep carry the directory map; map freshness stated | `TestLessonsCarryTheirRemedy`, `TestGoDependentsResolve`, `TestRustDependentsResolve`, `TestForeignInstructionFilesAreInjected`, updated `TestConventionsAreCountedFromTheCode` |
| Determinism | package-existence gate reads a local cache and refreshes off the gate path; the parsed map is cached while its file is unchanged | `TestPackageLookupRunsOffTheGatePath`, `TestInventedPackagesAreRefused` |
| Bases | starter catalog (api, web, cli, interactive) with a responsibility per folder; `{"base":"web"}` or a custom base in `.infuse/base.json`; out-of-base refusal on Write, heredoc and shell audit; Customize > Base applies a starter and scaffolds a new project with folders, base document, generated map and a starter README in each base folder, never overwriting an existing file; the prompt states intended vs. actual folders and each folder's responsibility | `TestStarterAndCustomBases`, `TestNoBaseMeansNoCheck`, `TestBaseAPIListsAppliesAndScaffolds`, `TestBaseGroundsShowsIntendedVersusActual`, `TestScaffoldCreatesTheBaseStructure`; Chrome 2× `base__customize__light__pt-br__before/after.png`, `...__en__after.png`, `base-ui.summary.json` (211 requests, 0 console/HTTP errors) |
| Feature index | deterministic feature→where index from feature folders (`features/`, `modules/`, `pages/`, `commands/`, `routes/`, `handlers/`…), HTTP route literals in Go, JS/TS, Python/Flask, C#, Java/Kotlin, Rust, Rails and Laravel, and a two-file filename-prefix fallback; routes fold into the feature named by their first static path segment; sorted and bounded (60 features, 400 routes); carried in the agent prompt, `/api/map`, `MAP.md` and the Map pane, whose file paths open the Files pane | `TestFeatureIndex`, `TestRoutesFromSourceLiterals`, `TestFeatureIndexIsDeterministic`, `TestPromptCarriesTheFeatureIndex`; Chrome 2× `map__features__light__pt-br.png`, `map__feature-file-open__light__pt-br.png`, `map__route-open__light__pt-br.png`, dark and English pairs, `docs/audit/logs/map-ui.summary.json` (297 requests, 0 console/HTTP errors) |
| Universal interposition | `infuse claude|codex|gemini` installs the shim and starts the CLI with `SHELL`/`PATH` pointed at it; the Studio's app launcher goes through it instead of opening a raw CLI | build + `desktop` tests; live CLI not exercised in this pass |
| Session persistence | `save` serializes under the session lock; a finishing turn and a mission step no longer read/write `SessionData` at the same time (found by the race detector in the mission test) | race suite, `TestMissionRunsUntilCriteriaPassOrBudget` under `-race` |
| Behaviour policy | one versioned `.infuse/policy.json` / `~/.infuse/policy.json` per scope, edited in Regras (permissions, prohibitions, work method, architecture, conventions, done); a corrupt policy blocks every turn; prohibitions become typed session bans; the file is protected from the agent | `internal/policy` tests, `TestPolicyAppliesToTheSession`, `TestCorruptPolicyBlocksTheSession`; Chrome 2× `policy__regras*` screenshots, `policy-ui-final.summary.json` (237 requests, 0 console/HTTP errors) |
| Contract | multi-step work cannot write before the user confirms the agent's restatement, bound to the current request; a new request needs a new confirmation; single-step work skips it; policy toggle | `TestMultiStepWorkNeedsAConfirmedContract`, `TestContractGateOnlyAppliesToWritingCommands` |
| OS containment | every agent process (Bash, hooks, MCP, executor checks/servers) runs in a kill-on-close Job Object; on Linux the hidden `__sandbox` wrapper applies Landlock (read everywhere, write only in the work root and sandbox temp; `.infuse` not granted) before exec; on Windows the wrapper creates a `WRITE_RESTRICTED` Low-integrity token (logon SID, Everyone, user SID, work-root capability SID), grants write on the work root and private temp/cache, labels the root Low and `.infuse`/protected paths Medium, and runs MSYS2 from a private per-Git-version runtime copy; toolchain caches are redirected so builds work without opening `%USERPROFILE%` | `TestWindowsSandboxWritesInside`, `TestWindowsSandboxBlocksOutsideWrites`, `TestWindowsSandboxProtectsInfuseRecords`, `TestWindowsSandboxBuildsPrivateMSYS`, `TestNormalizeMixedPath`, `TestWrapUsesTheHiddenWrapper`; live GUI-binary evidence in `docs/audit/logs/windows-sandbox-evidence.txt` (Go build, npm install, git commit, outside write/delete denied, native Node write denied); Linux cross-builds |
| Project mount | `infuse mount <drive|dir> [project] [--timeout seconds]` serves a project through WinFsp (cgofuse no-cgo host); reads pass through, writes are journaled and pass the base/protected-path policy, `.infuse` and protected globs are refused, and junctions/symlinks are resolved before the containment check; `Ctrl+C` or `--timeout` unmounts | `TestMountReadsAndWrites`, `TestMountRefusesProtectedAndOutsideBase`, `TestMountReaddir`, `TestMountServesAProject` (real WinFsp mount on a free drive); `docs/audit/logs/windows-mount-evidence.txt` (driver 2.1, inside/in-base writes, out-of-base and `.infuse` refusals, journal entries, clean unmount) |
| Visual evidence | a capture is bound to the measured HTML when the criteria declare `Expect.Contains`, in addition to the origin check | `TestScreenshotHTMLMustShowWhatTheCriteriaMeasure`, `TestScreenshotWithoutContentRequirementUsesOriginOnly` |
| Doc suggestions | Python versions are read from requirements.txt/pyproject (exact pin, `~=`, lower bound); Django, pytest and Express use verified versioned docs URLs, other vendors keep stable URLs | `TestSuggestDocsPinsPythonVersions`, `TestSuggestDocsFallsBackWithoutVersions`, `TestSuggestDocsReadsPoetryVersions`, `TestSuggestDocsPrefersExactPin` |
| Dependencies | every turn snapshot records a fingerprint of the lockfiles and the installed entrypoints; the executor refuses a verdict when they change while the checks run (the snapshot junctions the live dependency directories) | `TestDependencyFingerprintFollowsLockfilesAndEntrypoints`, `TestDependencyChangeDuringChecksInvalidatesTheVerdict` |
| User acceptance | criteria marked high-stakes ask the user once, after the executor and reviewer pass; accepting lets the turn end, rejecting ends it visibly and journals it; the toggle is in the Done? pane | `TestAcceptanceGateAsksOnceForHighStakesCriteria`, `TestRejectedDeliveryIsRecorded`; Chrome 2× `oracle__acceptance*` screenshots, `acceptance-ui.summary.json` (223 requests, 0 console/HTTP errors) |
| Knowledge freshness | a failed instruction recompilation keeps the old rules in force but labeled stale in the prompt and in the message; the blob store keeps 50 turns per session and drops unreferenced content; the base states intended-vs-actual folders | `TestStaleCompiledRulesAreLabeledAndEnforced`, `TestPruneTreeKeepsRecentTurnsAndReferencedBlobs`, `TestBaseGroundsShowsIntendedVersusActual` |
| Command registry | built-in slash commands are a data table with group and execution locus, custom commands join the same registry, and the composer menu renders them grouped | `TestBuiltinCommandsComeFromTheTable`; Chrome 2× `slash__commands__light__pt-br__after.png` / `...__en__after.png`, `slash-menu.summary.json` (204 requests, 0 console/HTTP errors) |
| Interface / rail | the "session details" button was visible on the new-session screen while CSS hid the rail unconditionally, so it did nothing; the control is now hidden when there is no session and toggles the rail in a session (280 px ↔ 0, `aria-pressed` follows) | Chrome via Playwright, headless, 2× (2880×1800): `hardening__home__light__pt-br__before-repro.png` / `...__after.png`, `hardening__session-rail-open__light__pt-br__after.png` / `...-closed...`, `hardening__settings__*`, `hardening__customize__*`, dark and English pairs; logs `docs/audit/logs/hardening-pass.summary.json` — 208 requests, 0 console errors, 0 HTTP ≥ 400 |

### Remaining risks after this pass

1. **Windows OS containment is delivered for the commands infuse launches, with residual risks.**
   The earlier dead ends were real: Low integrity alone fails because Cygwin's shared namespace and
   signal pipe are medium objects, and AppContainer needs administrator ACLs on `Program Files` and a
   namespace fix in the MSYS2 runtime. The working mechanism is a `WRITE_RESTRICTED` Low token whose
   restricting SIDs include the user (for MSYS2's per-user pipe and shared memory), a capability SID
   granted write on the work root, a Low label on the root, and a protected Medium label on `.infuse`
   and the protected paths; MSYS2 runs from a private runtime copy so its installation namespace does
   not collide with the user's Git Bash. Residual risks: same-user brokers (Explorer/COM can start a
   medium process on the user's behalf) mean this contains agent mistakes and file tampering, not a
   hostile same-user process; network access is not restricted; the Low labels stand on the workspace
   after the command (other Low processes can then write where the DACL allows); a hard link can alias
   a file outside the root; the private runtime costs a ~119 MB copy once per Git version (old versions
   pruned); caches are redirected to `~/.infuse/sandbox/cache`. On Linux Landlock is implemented and
   fail-closed but was cross-built only (no Linux host was available for a live smoke test).
2. **The WinFsp mount is a CLI surface, not yet wired into Studio sessions.** `infuse mount
   <drive|dir> [project] [--timeout seconds]` exposes the project through WinFsp (cgofuse's no-cgo
   host). Every write is journaled and passes the base/protected-path policy; `.infuse`, protected
   globs and out-of-base code are refused, and junctions/symlinks are resolved before the containment
   check. The filesystem runs in the infuse process at medium integrity, so its in-process policy is
   the perimeter; a pre-existing hard link can alias a file outside the root, and automatic mounting
   of every session (with relocation rather than refusal) remains a product decision.
3. **Approvals/verdict keys** fall back to a private file (`~/.infuse/signing.key`, `oracle-hmac.key`)
   when the OS vault is unavailable. On Windows the fallback file holds a DPAPI blob (same user, same
   machine; a copied file signs nothing) and legacy plain keys are migrated on first use; on Unix the
   file is protected by its `0600` mode and the home directory.
4. **The launcher and the signed approval flow were not exercised live** in this pass (unit/build
   evidence only); the native window was not opened on the user's screen.
5. **Doc suggestions**: Django and pytest use verified versioned paths; other vendors' docs keep stable
   URLs because their versioned paths could not be verified, and the reason still states the installed
   version.
6. **Contract scope**: the hard gate applies when the turn's plan has two or more steps (the Contract
   tool's own contract); single-step work and a policy with `requireContract:false` skip it.
7. **Policy vs settings**: the policy is the source for the new groups, but the legacy settings file
   still stores the scalar values the policy overlays; the Regras UI writes both through the session
   reload, so the two are linked until a later storage migration.
8. **Regras UI** was verified in Chrome via Playwright at 2× in light/dark and pt-BR/en with zero console
   or HTTP errors; the base editor lives in Customize > Base.
9. **Map depth**: the map lists files, exports, imports/dependents, hotspots, freshness, the base's
   intended-vs-actual folders and folder responsibilities, and the deterministic feature→where index
   with its HTTP routes (feature folders, route literals, filename prefixes). "How X is done here"
   examples are not generated.

### Corrections to `PROMPT-INFUSE-FULL-AUDIT.md` (the master execution prompt)

The master prompt's diagnosis is sound, but these statements are stale and should be corrected before
it is handed to an executor:

1. `docs/AUDIT-REPORT.md` exists (this file); it is not missing.
2. `windows/LEIA-ME.txt` lists two absent items (base scaffolds and FUSE/WinFsp), not seven.
3. Image drag-to-composer exists (`ui/src/app.js`, `ui/src/components/composer/attachments.js`).
4. B1: heredocs are judged at the turn boundary; a checkpoint is not a gate. The pre-execution write
   was the real gap, and it is now closed.
5. B3: shell deletions are judged by large-removal; they are now also revertible from the turn
   snapshot.
6. C7: D4 is not skipped on no-file turns (it runs before the early return); the oracle now also runs
   on them, reusing the content-hash verdict.
7. C2: an unreadable criteria file blocks the turn with a message; only an absent file means "no
   criteria".
8. C8: URL presence is checked only against Browser/WebFetch/WebSearch I/O, not any tool output; the
   `example.com` exemption was removed.
9. D8: per-check execution time is already recorded (`CheckResult.Millis`); what was missing was
   showing it to the agent.
10. D7: a failed recompilation is not fully silent — it emits a gate message; the old rules staying in
    force is now stated in the message.
11. The audit contains 194 screenshots under `docs/audit/screens/`, not 155.
12. SPEC §7 listed one future slice, not three; the README omitted 13 tools, not 14+.
13. Block 1's Windows requirement (a deny-write OS sandbox around the agent's Bash) is not achievable
    with the bundled Git Bash: the low-integrity and AppContainer probes are documented in the remaining
    risks; Job Objects and the Linux Landlock wrapper are in place, and the Windows write perimeter stays
    with the guard and the turn audit until a patched MSYS2 runtime or another shell is available.
