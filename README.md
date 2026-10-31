# infuse

A mandatory, instant, agent-independent interposition layer between a coding agent and the project.
One executable, no runtime, no dependencies. Read [SPEC.md](./SPEC.md).

**Water goes in clear and comes out dark.** Everything the agent reads arrives infused with the
project's knowledge; everything it writes or runs is transformed by that knowledge before it touches
disk. The agent does not consult it, call it, or configure it. It passes through it.

## 0.5: the agent lives inside the filter

`infuse.exe` is now a complete AI coding environment in its own native window (no browser, no console):
Anthropic, OpenAI (and compatible endpoints), Google Gemini and Ollama providers with streaming and tool
use, each tested against a fake server emitting its wire format; tools Bash, Read, Write, Edit, Glob, Grep, LS, WebFetch, TodoWrite, AskUserQuestion; permission
modes (ask / accept edits / plan / bypass) with per-tool rules and "always for this session";
persistent sessions (several in parallel, in tabs); per-turn checkpoints with revert; a guard that keeps
`.infuse/oracle/**` and protected paths unreachable through every tool — Write/Edit at the path gate, Bash
heredoc/redirection/rm/mv/tee/sed -i before anything runs — every attempt journaled as `blocked`;
native folder/file dialogs (`IFileOpenDialog`, NSOpenPanel, GTK) and drag & drop; automatic context compaction; cost tracking
with an editable price table; layered instructions (compatible with CLAUDE.md / AGENTS.md); a project
memory; a read-only oracle (`.infuse/oracle/**`) enforced in the tool executor; git status, diff,
discard and commit; a `mock` provider to try everything without a key. See `docs/PROMPT-infuse-studio.md`
for the full target and `windows/LEIA-ME.txt` for the user guide (Portuguese).

## One binary, three roles

| Invoked as | Role |
|---|---|
| `infuse.exe` (double-click) / `infuse ui` | the application window (native WebView2 / WKWebView / WebKitGTK): projects and sessions, chat with streaming and inline approvals, files/diff/git/terminal, Pronto, Regras, Jornal, Vícios, Mapa, Config |
| `infuse serve [port]` | the same UI served on 127.0.0.1 without a window (development) |
| `bash.exe` (the same file, copied by install) | the interposed shell every agent command passes through |
| `infuse <command>` | CLI: `install`, `uninstall`, `status`, `map`, `journal`, `lessons`, `rules`, `watch` |

How Claude Code is caught (documented channels): `CLAUDE_CODE_GIT_BASH_PATH` on Windows,
`CLAUDE_CODE_SHELL` on macOS/Linux, both written to `~/.claude/settings.json` by `install`.
Codex CLI resolves its shell from `$SHELL`; Gemini CLI spawns `bash -c` through `PATH` — the same
binary serves them when placed accordingly (launcher for those comes next).

## What passes through the filter (slice 1)

- No heredoc ever reaches bash (file writes become direct writes, program feeds become redirections;
  quoted keeps exact bytes, unquoted keeps same-shell expansion; tolerant of indented/missing terminators, CRLF, Git Bash paths).
- Command output comes back with what was written, who depends on it, the map of what was listed, and whatever your rules inject.
- A per-project watcher starts on first use and journals every write by any tool within milliseconds.
- Project map (exports, imports, dependents, hotspots), generated from code only.
- User rules (`~/.infuse/rules.json`, `<project>/.infuse/rules.json`): command / write / list / always → replace, say, infuse, infuseFile, vice.

## Build

```sh
go build -ldflags="-s -w -H windowsgui" -o build/infuse.exe ./cmd/infuse   # Windows: no console window
go build -o build/infuse ./cmd/infuse                                      # macOS/Linux (CGO_ENABLED=1 for the native window)
go test ./...
go vet ./...
```

## Repository layout

`cmd/infuse` is role dispatch and wiring only. The domains live in `internal/`: `core` (types,
messages, paths), `config`/`connections`/`llm`, `filter` (the interposed shell), `knowledge`,
`browser`, `mcp`, `oracle`, `workspace`, `terminal`, `scheduler`, `agent` (session loop, gates,
tools), `api` (HTTP/SSE), `desktop` (window, WebView2, dialogs). The frontend is `ui/js` (one file
per area and pane), `ui/styles` (tokens and area CSS) and `ui/locales` (JSON translations). The
architecture rules are executable: `go test ./internal/architecture`.

`legacy/ts` holds the original TypeScript implementation of slice 1, kept for reference.
