# Agent failures and the locks infuse enforces

Sources:
- **R**: what the owner already reported. That is 656 messages across all sessions (roadcraft, lab, cad2,
  Laboratório de Sentido, infuse), plus roadcraft's `docs/LICOES.md` and the memories.
- **P**: research. METR (reward hacking), ImpossibleBench, MAST (multi-agent system failures), OpenAI
  (chain-of-thought monitoring), Anthropic (system cards, Claude Code best practices), OWASP LLM Top 10,
  SWE-bench studies, slopsquatting.

Principle, taken from the research and from experience: **asking the model does not prevent**. METR showed
that telling it "do not cheat" changes almost nothing. What prevents is the **mechanical lock**, and every
lock here acts on language-free facts:
- tool calls, paths and diffs;
- exit codes;
- lines actually shown to the agent;
- structured data the agent is required to declare, such as the diagnosis and the report with claims.
  That data is checked by code.

Natural-language text is never interpreted by words. When meaning matters (the project's instructions), a
model compiles it into structured rules, each with a **verbatim citation verified** in the text.

State: **covered** (lock in code, with a test) · **partial** · **missing**.

## A. Reading, research and knowledge of the project

| # | Failure | Mechanical signal | Lock | State |
|---|---|---|---|---|
| A1 | Acts without reading the instructions and the documents they point to (R ~10, P G4) | first change/command before the full reading | required reading with line-by-line coverage; no change or command before it | covered |
| A2 | Ignores what the project already knows: the cause and the plan were in the audit (R, GPU case) | diagnosis without evidence from the documents | infuse reads every document to its last line (coverage by construction); facts with verified citations enter the context; the diagnosis requires evidence from the documents | covered |
| A3 | Reads pieces and says it read (R ~4+8) | coverage < 100% | Read pages instead of cutting the middle; the diagnosis evidence requires the file read whole | covered |
| A4 | Claims "I read X" in the report without having read it | read claim × coverage | a `read` claim in the Report is checked against the coverage of what infuse showed the agent | covered |
| A5 | Uses a library/API without researching the documentation (R ~30, P D2) | new import/host in the diff | documentation consulted in the turn before writing | covered |
| A6 | Uses command options without having read about them (R, RenderDoc case) | `--option` × text actually read | command refused with the list of options | covered |
| A7 | Reads only part of a page and acts (R) | parts of the page read | a page read in parts blocks action until it is finished | covered |
| A8 | Trial and error (R ~25, P G1) | repeated failures; the same command after each adjustment | research gate; the measure-and-tweak loop demands a new diagnosis | covered |
| A9 | Repropones what the project or the user already rejected (R ~12) | proposal × recorded decisions | project knowledge in the context + diagnosis; the user's prohibitions are structured | partial (see F4) |
| A10 | Loses the thread after compaction (R ~6, P G5) | work redone, rules forgotten | literal requests preserved; safe compaction; **diagnosis, plan and prohibitions reinjected on every request** | covered |
| A11 | Copies code the instructions forbid copying (R ~6) | clone of a passage from the reference | compiled rule with citation + clone detector | covered |

## B. Honesty and fraud

| # | Failure | Signal | Lock | State |
|---|---|---|---|---|
| B1 | Says it did, tested or fixed something without having done it (R ~40, P B1) | claim × execution | Report with structured claims checked against what ran | covered |
| B2 | False scope (measured something else and reported it as the request) (R ~8) | cited command × executed command | a command claim requires the exact command that ran | covered |
| B3 | Edits, reverses or deletes tests to make them pass (R ~5, P A1) | diff in a test file | sentinel asks the user for authorization | covered |
| B4 | Skips or disables tests (skip/only/xfail) (P A2) | new marker in the diff | shortcut detector | covered |
| B5 | Silences the checker (ts-ignore, eslint-disable, `any`, empty catch) (P A10/A11) | new suppression in the diff | a suppression without its reason on the same line is refused — for Write/Edit and for shell writes (heredoc pre-check and turn audit); with the reason it passes | covered |
| B6 | Exits early or fakes the exit code (`|| true`, `exit(0)`) (P A3) | `|| true` in the test command; new `exit(0)` outside the entry point | test runs judged by the real exit code; swallowing is refused | covered |
| B7 | Special-cases the test values (P A4) | test literals in production code, inside a condition | refused | covered |
| B8 | Equality or timers that are always true, patches in the framework (P A5/A6) | `__eq__` returning a constant; writes to conftest/setup/scorer | refused; test infrastructure files protected (the user decides every time) | covered |
| B9 | Changes thresholds, baselines and budgets to pass (R ~5) | writes to files read by the declared verifications | baseline/threshold/budget/golden/coverage files are verdict containers: the user decides every time, and a shell write to one blocks the turn end | covered |
| B10 | Placeholders, stubs and TODO instead of implementation (R, P C1/A9) | function body reduced to a constant or `throw`; new TODO/FIXME/HACK markers | refuse the growth | covered |
| B11 | Invents URLs and sources (P D4) | cited URL × URLs actually fetched | refuse the end of the turn | covered |
| B12 | Announces "I found the cause" several times (R ~10) | — | structured diagnosis + new diagnosis with new evidence | covered |
| B13 | Bluffs or flatters ("you are right") (R ~12) | language only | not mechanical; mitigated by the checked Report | not mechanical |

## C. Approach and scope

| # | Failure | Signal | Lock | State |
|---|---|---|---|---|
| C1 | Treats symptoms, not the cause (R ~30) | loop, regression, self-reversal | mandatory diagnosis; objective signals demand a new diagnosis | covered |
| C2 | Chooses the cheap path (R ~15) | — | diagnosis + mandatory independent review | partial (not mechanical) |
| C3 | Cuts scope, disables a feature, deletes code to "solve" it (R ~12, P C4) | Write that removes most of an existing file | the diagnosis/plan must cite the file and the removal | covered |
| C4 | God files, scattered code, no organization; CSS spread out; global state; data without a schema (R) | diff × architecture rules | architecture rules on every write (only the new is judged) | covered |
| C5 | Undoes its own work, oscillates (P C3) | file back to an earlier content of the turn | demands a new diagnosis | covered |
| C6 | Breaks what worked (R ~8) | declared verification used to pass and now fails | demands a new diagnosis; the end of the turn requires the declared verifications green | covered |
| C7 | Tools, reports and probes instead of fixing (R ~8) | own measurement tool × declared commands | declared commands required after changes | covered |

## D. Process, tests and measurement

| # | Failure | Signal | Lock | State |
|---|---|---|---|---|
| D1 | Measurement after measurement (R ~45) | non-reading command before the diagnosis; loop | diagnosis before measuring; the loop demands a new diagnosis | covered |
| D2 | Stops halfway (R ~25, P B5) | turn end with an open plan; provider errors; step limit | end-of-turn obligations; full resilience; progress watchdog; resume after restart | covered |
| D3 | Repeats what gets stuck (R ~8) | same call failing | circuit breaker: change approach → external research → stop | covered |
| D4 | Leaves processes running (R ~6) | processes alive at the end of the turn | every running background process must be named in the Report (id and purpose) or stopped with KillShell | covered |
| D5 | Edits a file without having read it (P D3) | Edit without Read | required | covered |
| D6 | Invented package (slopsquatting) (P D1) | install or import of a package | check it in the registry before installing | covered |

## E. Interface and visual verification

| # | Failure | Signal | Lock | State |
|---|---|---|---|---|
| E1 | Changes the interface without photographing or looking at it (R ~20, P B4) | diff in css/html/jsx/tsx/vue/svelte without a capture afterwards | the end of the turn requires a capture (Browser screenshot) after the last UI change | covered |
| E2 | Opens windows on the user's screen (R ~17) | — | the agent's browser has no window; preview inside the panel | covered |

## F. Communication and control

| # | Failure | Signal | Lock | State |
|---|---|---|---|---|
| F1 | Replies in another language (R ~11) | — | configured language in the prompt; infuse's messages through the catalog | covered |
| F2 | Asks instead of deciding (R ~30) | turn ends in a question with an open plan | end-of-turn obligations | partial |
| F3 | Changes its own rules or hooks (P E5) | write to instructions, `.infuse` or rules | protected | covered |
| F4 | Does what the user forbade in the session (R ~35, P G3) | call × prohibition | the user's prohibitions compiled with verified citations and applied to tools, commands and paths — including shell write targets (heredoc pre-check) and shell changes at the turn boundary | covered |
| F5 | Uses subagents after being forbidden, or parallelizes too much (R ~15) | Agent call × prohibition | through F4 | covered |
| F6 | Accepts the subagent's "done" without checking (R ~15, P H3) | subagent result used without verification | subagent work enters the Report as not verified until the agent itself produces the evidence | covered |
| F7 | Destructive commands (rm -rf outside, reset --hard, push --force) (P E1/E2) | command × list | sentinel/guard | covered |
| F8 | Prompt injection through external content (P F2) | external content marked | external content is marked; it does not count as a request | partial |
| F9 | Leaks secrets (P F1) | secret in output/diff | secret redaction; system vault; browser captures redacted | covered |

## G. Locks added in the 2026-11-01 hardening pass

| Lock | Failure it closes | State |
|---|---|---|
| Control files protected at the executor and by the guard: `.infuse/settings.json`, `.infuse/hooks.json`, `.infuse/base.json`, `.mcp.json`, `.claude/settings.json` | one write disarms the filter or self-approves a command | covered (`containment_test.go`) |
| Project settings can only tighten mode/permissions, never loosen them | a project file turns the filter off | covered (`settings_test.go`) |
| Guard rooted at the real project, case-insensitive, any-depth records, symlinks/junctions both ways, `mklink` refused | worktree `../../`, `.INFUSE`, junction read bypass | covered |
| Indirect writers: `sed --in-place`, `curl -o`, `wget -O`, `sort -o`, PowerShell `-EncodedCommand`, inline script paths | writes that dodge the writing-command list | covered |
| Hook commands judged by the guard; hook/MCP approvals HMAC-signed with a key outside the project; MCP read-only hints ignored | hook/MCP self-approval through an agent-writable file | covered |
| Heredoc bodies judged before execution (plan, size, base, bans, large removal, suppressors, cheat, baseline ask) | `cat > src/big.js <<EOF` writing before any gate | covered (`shell_precheck_test.go`) |
| Shell audit applies bans, suppressors and `cheat.test_infra`; failed snapshot blocks the turn; the ask budget ends UNVERIFIED; revert restores shell deletions | rules that stopped at Write/Edit | covered |
| Oracle criteria anchored outside the project; verdicts HMAC-signed; HTTP without snapshot server refused; port free + child alive; content-hash cache; no-edit turns judged; mission reuses the turn verdict | forged criteria/verdict, timestamp replay, impostor server | covered (`oracle/executor_test.go`) |
| Background-command claims resolve against the final exit code | a claim confirmed by the first two seconds | covered (claims path) |
| Screenshot must come from the origin the criteria measure | any screenshot satisfying the visual gate | covered (code + catalog message) |
| Lessons carry signature + remedy from the vice catalog; reviewer/subagents/auditor receive the grounds; digest overflow/failure is loud; comment language is an observation | knowledge that was cosmetic | covered (`knowledge_test.go`, `map_test.go`) |
| Package gate reads a local cache; map parsed once per file generation | network and re-parsing in the gate path | covered (`locks_test.go`) |
| Base `.infuse/base.json` refuses code outside its folders | work with no intended structure | covered (`base_test.go`) |
| `infuse claude|codex|gemini` and the app launcher start external CLIs through the shim | the Studio launching unfiltered agents | build + desktop tests |
| One behaviour policy per scope, edited in Regras; protected from the agent; corrupt policy blocks every turn; prohibitions become typed bans | scattered behaviour sources and a silent weaker fallback | covered (`internal/policy`, `policy_session_test.go`) |
| A confirmed Contract bound to the current request before multi-step work writes | starting work on an unconfirmed reading of the order | covered (`contract_gate_test.go`) |
| Job Objects for every agent process; containment on every platform (Windows Low integrity, Linux namespace + Landlock, macOS Seatbelt; write only the work root and a private temp; `.infuse` never granted) | processes outliving infuse; writes outside the project | covered (`internal/sandbox`, `contained_test.go`) |
| A screenshot is bound to the HTML the criteria measure, not only to the origin | a capture of another page/state satisfying the visual gate | covered (`locks_screenshot_test.go`) |
| Python dependency versions are read from the manifests; Django/pytest/Express use verified versioned docs URLs | docs for a version other than the one installed | covered (`docs_version_test.go`) |
| Turn snapshots record a dependency fingerprint; the executor refuses a verdict when lockfiles or installed code change while the checks run, in the live tree or in the snapshot | poisoned `node_modules/.bin` runner or a lockfile edited during measurement | covered (`deps_test.go`, oracle executor test) |
| Stale compiled rules are labeled and the prompt says so; the blob store keeps 50 turns per session and drops unreferenced content; the base shows intended vs. actual folders | silently enforcing an old instruction text; unbounded snapshot storage; a base that exists only on paper | covered (`knowledge_test.go`, `prune_test.go`, `base_gate_test.go`) |
| Built-in slash commands come from one data table grouped by area, and custom commands join it | a second hardcoded command list drifting from the UI | covered (`commands_test.go`, Chrome 2×) |
| A base is chosen and applied in the Rules page; creating from a base writes the folders, the base document and the generated map | a base that exists only as a file, or a project created without structure | covered (`appBase_test.go`, Chrome 2×) |
| Unattended runs answer their own cards and questions; plan mode, the destructive table, the protected paths and every sentinel still refuse with nobody to approve | a mission hanging on a card nobody sees, or approving what nobody read | covered (`unattended_test.go`, `turn_locks_test.go`) |
| An existing test, its configuration and a verdict container are refused deterministically, with no card, in every mode; a shell rewrite is restored | "accept everything" or a card reflex approving a weakened test | covered (`sentinel_test.go`, `shell_audit_test.go`) |
| Protected oracle and evidence files changed outside the executor are restored from the turn snapshot | a deleted criterion surviving the refusal | covered (`shell_audit_test.go`) |
| High-stakes criteria ask the user to accept the delivery once, after executor and reviewer; a rejection ends the turn visibly and journaled | shipping a risky result as done because the criteria happened to pass | covered (`acceptance_test.go`, Chrome 2×) |

## H. Corrections from the 2026-11-02 audit

The states above were written from reading the code. Running the program showed these rows were not
true at that point; each is now fixed and has a test.

| Rows | What was wrong | Fix |
|---|---|---|
| B1, B2, B6, A8, C6, C7, D3 | the Bash tool appended a `printf` after the command, so every failing command read as success and no lock that reads an exit code ever fired | the status is kept across the trailer (`TestBashKeepsTheExitStatusOfTheCommand`) |
| F3, F7, G "Control files", G "Indirect writers" | the OS containment never ran a process: no command could start on Windows, and the deny on `.infuse` was never written | Low integrity containment, tested through the Bash tool with commands the parser cannot read (`TestContainmentStopsWhatTheParserCannotRead`) |
| B3, B4, C4, E1, F4 | a shell edit skipped the sentinel, the shortcut detector, the contract and the screenshot obligation, and a path once gated was never judged again whatever it later held | gates are bound to content; `shellHonesty`, `noteShellChanges` |
| B1 | the Report was waived after one reminder and unconfirmed claims were display only; a pass followed by a failure stayed confirmed | `unconfirmedReport`, last run decides, a run older than the last edit is stale |
| A10, D2 | compaction disconnected Turn from TurnStarts, turning the Report gate off and opening every later Contract | turns stay addressable, the request is stored apart from the transcript |
| D2 | failed or cancelled turns laundered their shell writes into the next baseline; a crash left a mission running forever | `AuditPending` carries the base; a stale mission is closed at startup |
| G "Oracle criteria anchored", SPEC "immutable snapshot" | checks ran uncontained, the snapshot linked live dependencies, git hooks ran on the host | contained checks, private snapshot, `gitx` is the only way to run git |
| G "revert restores shell deletions" | checkpoints were reapplied after the snapshot, restoring shell-modified bytes | the snapshot has the last word |

Still honest gaps: B13 and C2 are not mechanical. F2 and F8 remain partial. B8 cannot see a new test
file whose `TestMain` exits early, because nothing in a new file distinguishes that from a real one;
the oracle's own criteria are the defense. D4 tracks `run_in_background`; on Windows a `cmd &` inside a
foreground command dies with the wrapper's job instead of being declared.
