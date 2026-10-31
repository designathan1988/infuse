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
5. Data that describes behaviour is a table: provider presets (`internal/data/providers.json`), tool
   registration (`internal/agent/tool_register.go`), route groups (`internal/api/appGroups.go`,
   `studioGroups.go`), pane registry (`ui/js/panes/core.js`).
6. User-facing text comes from catalogs, never from literals in handlers: Go through `core.M`, the UI
   through `t('ui.…')`. Both read the same embedded catalog (`internal/core/catalog.json` for the interface,
   `core.RegisterMessages` catalogs for Go), with stable ids and English for the model.

## Packages

```
cmd/infuse        role dispatch, CLI and composition

internal/
  core            wire types, messages and catalogs, paths, JSON/text helpers
  data            embedded declarative tables (provider presets)
  config          settings, secret vault, projects, usage ledger, pricing, migrations
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

## i18n

- Go: `core.Msg` is an identifier plus named arguments; `internal/core/catalog.json` carries English
  and pt-BR. The model reads English; the user reads their configured language.
- UI: messages have stable ids (`ui.<area>.<slug>.<hash>`) in `internal/core/catalog.json`, embedded into
  `core` and served by `/api/i18n`; `t(id, args)` renders the current language and falls back to the
  Portuguese source only for labels that arrive as data. Static markup carries `data-i18n`,
  `data-i18n-title` or `data-i18n-placeholder`, applied once at boot and again when the language changes.
  There is no `MutationObserver`; switching language re-renders the interface in place.
- `TestUICatalogIsComplete` and `TestUIMessagesUseCatalogIDs` fail the build on a missing language,
  divergent placeholders, a literal sentence used as a key, or an id absent from the catalog.

## Frontend

- `ui/index.html` is the shell: CSS links in order, classic scripts in dependency order, `boot.js` last.
- `ui/js/store.js` is the state container: `dispatch` is the only writer, state is deep-frozen after
  every action, and modules subscribe to receive the new `S`. `ui/js/state.js` holds the reducer,
  action creators and selectors; server data is a cache fed by the API and SSE, and live-session
  events are folded in by the reducer.
- `ui/js/runtime.js` holds what is not application state: DOM nodes per view, scroll/fold/attachment
  runtime, and the SSE connections. That keeps the store plain and freezable.
- `ui/js/core.js` holds shared helpers, the API client and the native bridge.
- One file per area: `sidebar`, `session`, `chat`, `feed_live`, `feed_tools`, `feed_cards`,
  `markdown`, `composer`, `rail`, `overlays`, `boot`.
- One file per pane (`ui/js/panes/*`) and per page (`ui/js/pages/*`); the registry in
  `panes/core.js` is loaded after the pane implementations it names.
- `ui/styles/tokens.css` is the only file with colour literals; every other rule reads tokens.
  `ui/styles/utilities.css` holds the one-off layout classes that used to be inline declarations.
- Dynamic geometry travels as CSS custom properties (`--w`, `--pane-width`); the architecture test
  refuses a static `style="…"` declaration and any colour literal outside the token file.

## Known debt

- None in the architecture rules: no Go function is over 60 lines (`TestFunctionsStaySmall` in
  `internal/architecture`), no frontend function or file is (checked by `tools/check-frontend.mjs`),
  and every rule above is enforced by a test or a CI step.
- Compatibility route families (`/api/session/<id>` and `/api/sessions/<id>`) are kept so older pages
  and external callers keep working; both dispatch into the same domain methods.

## Verification

- `go vet ./...`, `go test ./...` and `go test ./internal/architecture`.
- Cross builds: `GOOS=windows CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 GOOS=linux go build ./...`,
  `CGO_ENABLED=0 GOOS=darwin go build ./...`.
- Frontend: `node --check` on every `ui/js` file, `tools/check-frontend.mjs` for file/function size,
  inline styles and colour literals, and every screen/pane exercised in the preview.
- CI runs all of the above (`.github/workflows/ci.yml`).
