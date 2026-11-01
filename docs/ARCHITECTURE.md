# infuse — architecture

This document describes the structure the code follows after the 0.5 refactor, the rules that keep it
that way, and the known debt that remains.

## Rules

1. One domain per package; one responsibility per file. No source file above 400 lines.
2. Dependencies point down: `cmd` → `api`/`desktop` → `agent` → capabilities (`browser`, `mcp`,
   `knowledge`, `filter`, `workspace`, …) → `config`/`llm`/`core`/`data`. `filter` never imports the
   agent or the knowledge layer (it receives a `Policy`).
3. `package main` exists only in `cmd/infuse`. The module root is not a package.
4. State has one owner: a running conversation is an `agent.Session`; the UI snapshot is a projection
   (`agent.Session.Snapshot`), never a second source of truth. The server is the source of truth for the
   browser UI; `ui` keeps only a cache.
5. Data that describes behaviour is a table: provider presets (`internal/data/providers.json`), built-in
   slash commands (`internal/data/commands.json`, with group and execution locus; custom
   `.infuse/commands/*.md` join the same registry), tool registration (`internal/agent/tool_register.go`),
   route groups (`internal/api/appGroups.go`, `studioGroups.go`), pane registry (`ui/src/panes/index.js`).
6. User-facing text comes from one catalog, never from literals in handlers: Go through `core.M`, the UI
   through `t('area.subject')`. Both read `internal/core/catalog.json`; the UI receives it from
   `/api/i18n` at boot, so there is no second translation table to drift.

## Packages

```
cmd/infuse        role dispatch, CLI and composition

internal/
  core            wire types, messages and catalogs, paths, JSON/text helpers
  data            embedded declarative tables (provider presets, vice catalog, starter bases)
  base            the project's chosen architecture base: folders, data/token homes, out-of-base check
  policy          the user's behaviour policy: permissions, prohibitions, method, architecture,
                  conventions and done, per global/project scope
  sandbox         OS containment: Job Objects (Windows) and the Landlock wrapper (Linux)
  config          settings, secret vault, signed approval files, projects, usage ledger, pricing, migrations
  connections     provider presets as a service, connect/disconnect, model discovery
  llm             Provider interface, Anthropic/OpenAI/Gemini/Ollama/mock, SSE, retry, limits
  filter          the interposed shell role: guard, heredoc, transform, rules, watcher, install
  knowledge       instructions and compiled rules, document digests, docs, memory, skills, conventions
  browser         CDP WebSocket, browser pool, agent page, streamed tabs, Browser tool
  mcp             MCP configuration, JSON-RPC client, per-project manager
  oracle          criteria, snapshot, executor, verdicts, suggested checks
  workspace       git, worktrees, local diffs, snapshots, PR/CI, editors, dev servers
  terminal        ConPTY/script PTY sessions
  scheduler       scheduled-task storage and timing
  agent           Session runtime (state, events, permissions, checkpoints, loop, tools, gates),
                  subagents, missions, hooks, background processes
  api             HTTP server, route groups, SSE, terminals, local security
  desktop         Win32 window, tray, theme, placement, WebView2, native browser pane, dialogs, launchers
  platform/process  process groups, kill trees, console handling
  lang, projectmap, journal, security, fs helpers
  architecture    executable structure tests (file size, main, import direction, cycles)
```

## State, events and persistence

- `agent.Session` embeds the persisted `SessionData` and owns every field under one mutex. Only Session
  methods mutate it; the API calls exported methods (`Save`, `Snapshot`, `Send`, `Stop`, missions,
  bans, processes). `SessionData` is the serialized projection.
- The turn starts with a tree snapshot (`workspace.SnapshotTurn`). Write and Edit are judged as they
  run (`agent.writeGuards`); every other change to the tree — a shell command, an MCP tool, a
  generator, a subagent — is judged at the end of the turn by the same policy (`agent/shell_audit.go`).
  `.infuse/oracle` and `.infuse/evidence` are hashed at turn start; changing them outside the executor
  blocks the turn.
- `agent.Registry` (the `sessions` map) owns live sessions; `NewSession`/`LoadSession`/`ListSessions`
  are the only entry points.
- Events are emitted after a mutation with a monotonically increasing sequence (`Event.Seq`). The UI
  uses snapshots plus these events; snapshots always carry the current state.
- Writes are atomic (`core.WriteJSON`: temporary file, fsync, rename). The terminal state of a turn
  (`idle`/`error`) becomes observable only after the document is saved.
- Every infuse-owned document carries `schemaVersion`: settings, project settings, sessions, scheduled
  tasks, architecture rules, oracle criteria, rules files, hooks, memory, projects and the pinned-doc
  index. `store.Versioned.Upgrade` runs the migration chain from the version found on disk (0 for
  files written before versioning) to `store.Version`; documents in list form use an
  `{schemaVersion, items}` envelope while still reading the legacy bare array. Files shared with other
  tools (`.mcp.json`, `.claude/settings.json`) keep their external formats and are not stamped.
- The browser UI is a cache: server snapshots + SSE events. It never writes shared state directly
  (pending sends live in the view, not in the transcript).

## Security anchors

- The agent's shell guard is built against the real project root even when a session works in a
  worktree, so `../../` cannot reach the project's own records. Path matching is case-insensitive where
  the filesystem is, protected records are recognized at any depth, symlinks/junctions are resolved on
  both the read and the write side, and `mklink` is treated as a writer.
- `.infuse/settings.json`, `.infuse/hooks.json`, `.infuse/base.json`, `.mcp.json` and
  `.claude/settings.json` are protected: a project file can tighten the policy, never loosen it
  (`EffectiveSettings` ignores a looser mode or permission).
- Hooks run through the same guard as any shell command. Hook and MCP approvals are stored with an
  HMAC whose key lives in the OS vault; the fallback file is DPAPI-encrypted on Windows (legacy plain
  keys migrate on first use) and `0600` on Unix. An unsigned or edited approval file approves nothing.
  MCP tools always go through the permission policy; a server's own read-only hint is not evidence.
- The oracle's criteria hash is anchored in `~/.infuse/oracle/`, outside the project, and every verdict
  carries an HMAC over its fields. A criteria/verdict pair edited inside the project no longer verifies.
  HTTP checks require a server started from the snapshot on a port that was free; the verdict records
  the child pid.
- Read-only API routes that read a caller-chosen file, fetch a URL or open a path also require the
  application token. The agent's browser refuses non-web schemes and `file://`; text captured from a
  page passes secret redaction.
- The package-existence check reads a local cache (`~/.infuse/packages.json`); a miss is looked up in
  the background, never inside the gate path.
- Folder-level instruction files (CLAUDE.md, AGENTS.md, GEMINI.md, .cursorrules) load once per session
  when a Read or Write enters their folder, never as a repeated prefix.
- `.infuse/policy.json` and `~/.infuse/policy.json` are the behaviour policy: the Regras surface edits
  them, the agent may not write them, and a corrupt policy blocks every turn instead of falling back to
  weaker defaults. Prohibitions become typed session bans (tool/command/path/action) enforced by the
  same banCheck used for the user's own prohibitions. Multi-step work additionally needs a
  user-confirmed Contract bound to the current request before the first write.
- Criteria can be marked as needing the user's acceptance: after the executor and the reviewer pass, the
  turn asks the user once (accept/reject); a rejection ends the turn visibly and is journaled, never
  "done". The toggle is in the Done? criteria pane.
- `.infuse/policy.json` and `.infuse/base.json` are protected by the guard; a project policy may only
  add protections, never remove the filter's own gates.
- Every turn snapshot records a dependency fingerprint (lockfiles plus the installed entrypoints under
  `node_modules/.bin`/`.venv/bin`). The executor compares it before and after the checks, because the
  snapshot junctions the live dependency directories: a manifest or runner that changes while the
  checks run invalidates the verdict instead of approving a tree it no longer measured.
- The blob store is pruned at every snapshot: each session keeps its newest 50 turn manifests and the
  content they reference; older blobs (and orphans) are removed.
- A failed instruction recompilation keeps the previous compiled rules in force but marks them stale
  (`project-rules.json` `stale:true`); the prompt labels them and the compile-failure message says so.
- The base states intended vs. actual: the prompt lists declared folders that do not exist yet and
  code folders that sit outside the base, alongside the base's rules. Each folder also carries its
  responsibility (entry point, transport, domain, data owner, tokens…), so the map answers "what lives
  here" and not only "how large is it".
- The map also answers "where is feature X": `projectmap` builds a deterministic feature index from
  feature folders (`features/`, `modules/`, `pages/`, `commands/`, `routes/`, `handlers/`…), HTTP route
  literals in every supported language, and a filename-prefix fallback (two or more files sharing a
  prefix). Routes fold into the feature named by their first static path segment. The index is sorted
  and bounded (60 features, 400 routes), reaches the agent prompt, `/api/map` and `MAP.md`, and the Map
  pane renders it with clickable files.

## i18n

- One catalog for the whole application: `internal/core/catalog.json` carries English and pt-BR under
  stable semantic ids (`common.cancel`, `chat.send`, `panes.changes.title`). Go registers the same data
  through `core.RegisterMessages`; the UI loads it once from `/api/i18n`.
- The model reads English; the user reads their configured language. Messages use a small ICU subset
  (`{name}`, `plural`, `select`, `number`, `date`) rendered with `Intl.PluralRules` and
  `Intl.DateTimeFormat` in the browser, so categories are real CLDR categories.
- Static markup carries `data-i18n`, `data-i18n-ph`, `data-i18n-title` or `data-i18n-aria`, applied at
  boot and on every language switch. There is no `MutationObserver` and no per-string catalog file.
- `TestCatalogIsComplete` and `TestUIMessagesUseCatalogIDs` fail the build on a missing language,
  divergent placeholders, or a translation call whose id is absent from the catalog;
  `scripts/check-i18n.mjs` checks the same contract from the UI side, including dynamic key families.
- `scripts/check-language.mjs` fails the build on Portuguese outside the catalog (accented letters and
  decisive words), with a reviewed exception list in `scripts/language-allowlist.txt` for stored
  protocol values and language-detection tables.

## Frontend

- `ui/index.html` is the shell: the CSS cascade in a fixed order and one ES-module entry point
  (`/src/app.js`). There is no bundler and no runtime dependency.
- `ui/src/core/` owns the shared primitives: `api.js` is the only HTTP client, `signals.js` the reactive
  primitive, `store.js` the single store (server cache, live sessions, UI preferences), `i18n.js` the
  catalog renderer, and `dom.js`/`markdown.js`/`fmt.js` the small utilities.
- `ui/src/components/` holds the reusable interface (sidebar, view, transcript, composer, rail, menus,
  dialogs, data table). `ui/src/panes/` and `ui/src/pages/` expose one module per pane/page behind
  registries in `panes/index.js` and the page modules; large pages are split into section modules.
- State has one direction: server snapshots and SSE events update signals; effects re-render only the
  affected subtree by keyed reconciliation. Components never mutate server state directly; actions in
  the store call the API.
- Design tokens are DTCG JSON in `ui/tokens/` (primitives, light/dark colours, sizes). `scripts/gen-tokens.mjs`
  generates `ui/css/tokens.css`, the only file with colour literals. Components read tokens from the
  cascade; dynamic geometry travels as CSS custom properties (`--w`, pane sizes). Static `style="…"`
  declarations and colour literals elsewhere are rejected by `tools/check-frontend.mjs`.
- CSS uses cascade layers (`reset, tokens, base, layout, components, utilities`) with base,
  layout and component files linked in order. Shared classes live in the design system, not in feature
  modules.

## Known debt

- None in the architecture rules: no Go function is over 60 lines (`TestFunctionsStaySmall` in
  `internal/architecture`), no frontend function or file is (checked by `tools/check-frontend.mjs`),
  and every rule above is enforced by a test or a CI step.
- OS-level containment is delivered on both platforms for the commands infuse launches. Every agent
  process (Bash, hooks, MCP servers, executor checks and servers) runs in a Windows Job Object that
  ends the tree with infuse. On Linux the hidden `__sandbox` wrapper applies a Landlock ruleset (read
  everywhere; write only in the work root and a sandbox temp; `.infuse` and protected paths are not
  granted). On Windows the wrapper creates a `WRITE_RESTRICTED` token with the logon SID, Everyone,
  the user SID and a capability SID derived from the work root, lowers it to Low integrity, grants the
  capability write on the root and on a private temp/cache, and labels the root Low; `.infuse` and
  protected paths carry a protected Medium label, so the confined process cannot rewrite the records.
  Write access requires both the normal and the restricting access checks, and the Low label closes the
  parent-directory `FILE_DELETE_CHILD` path, so writes and deletes outside the root fail while reads
  stay allowed. MSYS2 is started from a private copy of the runtime (linked or copied once per Git
  version) so its per-installation shared memory and signal pipe do not collide with the user's Git
  Bash. Toolchain caches (Go modules/build, npm, pnpm, pip, Playwright and XDG) are redirected to the
  granted cache, so real builds work without opening `%USERPROFILE%`. Tests: `internal/sandbox`
  (`TestWindowsSandboxWritesInside`, `TestWindowsSandboxBlocksOutsideWrites`,
  `TestWindowsSandboxProtectsInfuseRecords`, `TestWindowsSandboxBuildsPrivateMSYS`). The residual risks
  are same-user brokers (Explorer/COM can start a medium process on the user's behalf), network access
  (not restricted), the standing Low labels on the workspace (other Low processes can then write where
  the DACL allows), hard links that alias files outside the root, and the private runtime copy per Git
  version; the evidence is in `docs/AUDIT-REPORT.md` §9.
- The behaviour policy and the Regras surface are delivered (groups for permissions/prohibitions,
  method, architecture, conventions and done, plus the filter rules in the same pane). The legacy
  settings file still stores scalars the policy overlays and writes back, so the two remain linked
  until a later migration; `.infuse/base.json` is chosen as a file or through the API, with no visual
  base editor yet.
- Base scaffolds create the folders, the base document, a generated map and a starter README in each
  base folder (`infuse init <base>` or Customize > Base, which also applies a base to the current
  project and creates a project from one). Re-scaffolding never overwrites an existing file. The
  FUSE/WinFsp mount remains absent.
- The Contract is a hard gate for work with more than one planned step; single-step work proceeds
  without it, and the gate can be turned off in Regras.
- Compatibility route families (`/api/session/<id>` and `/api/sessions/<id>`) are kept so older pages
  and external callers keep working; both dispatch into the same domain methods.

## Verification

- `go vet ./...`, `go test ./...` and `go test ./internal/architecture`.
- `node scripts/check-language.mjs` (English only) and `node scripts/check-i18n.mjs` (catalog).
- Cross builds: `GOOS=windows CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 GOOS=linux go build ./...`,
  `CGO_ENABLED=0 GOOS=darwin go build ./...`.
- Frontend: `node --experimental-vm-modules scripts/check-ui.mjs` parses every module,
  `tools/check-frontend.mjs` enforces file/function size and the no-inline/no-literal rules,
  `scripts/check-i18n.mjs` verifies catalog coverage, and `scripts/gen-tokens.mjs` must reproduce
  `ui/css/tokens.css`. Every screen and pane is exercised in the preview.
- CI runs all of the above (`.github/workflows/ci.yml`).
