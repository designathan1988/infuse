# infuse

A mandatory, instant, agent-independent interposition layer between a coding agent and the project.
One executable, no runtime, no dependencies. Read [SPEC.md](./SPEC.md).

**Water goes in clear and comes out dark.** Everything the agent reads arrives infused with the
project's knowledge; everything it writes or runs is transformed by that knowledge before it touches
disk. The agent does not consult it, call it, or configure it. It passes through it.

## 0.5: the agent lives inside the filter

`infuse.exe` is now a complete AI coding environment in its own native window (no browser, no console):
Anthropic (direct, Amazon Bedrock and Google Cloud), OpenAI (and compatible endpoints), Google Gemini and
Ollama providers with streaming and tool use, each tested against a fake server emitting its wire format;
the tools Bash, Read, Write, Edit,
MultiEdit, Glob, Grep, LS, WebFetch, WebSearch, TodoWrite, AskUserQuestion, Memory, Skill, Diagnosis,
Report, Contract, Impasse, Agent, Browser, BashOutput and KillShell, plus MCP connector tools; permission
modes (ask / accept edits / plan / bypass) with per-tool rules and "always for this session";
persistent sessions (several in parallel, in tabs); per-turn checkpoints with revert; a guard that keeps
`.infuse/oracle/**`, `.infuse/evidence/**`, the project's settings, hooks, MCP configuration and infuse's
own records unreachable through every tool — Write/Edit at the path gate, Bash heredoc/redirection/
rm/mv/tee/sed -i/mklink/curl -o/wget -O/powershell -EncodedCommand before anything runs — every attempt
journaled as `blocked`; native folder/file dialogs (`IFileOpenDialog`, NSOpenPanel, GTK) and drag & drop;
automatic context compaction; cost tracking with an editable price table; layered instructions
(CLAUDE.md, AGENTS.md, GEMINI.md, .cursorrules, copilot instructions); a project memory; a read-only
oracle anchored outside the project and signed with an HMAC the project cannot produce; an architecture
base (`base.json`) enforced on every write; an interposed launcher (`infuse claude|codex|gemini`); git
status, diff, discard and commit; a `mock` provider to try everything without a key. See
`docs/PROMPT-infuse-studio.md` for the full target and `windows/LEIA-ME.txt` for the user guide
(Portuguese).

## One binary, three roles

| Invoked as | Role |
|---|---|
| `infuse.exe` (double-click) / `infuse ui` | the application window (native WebView2 / WKWebView / WebKitGTK): projects and sessions, chat with streaming and inline approvals, files/diff/git/terminal, Pronto, Regras, Jornal, Vícios, Mapa, Config |
| `infuse serve [port]` | the same UI served on 127.0.0.1 without a window (development) |
| `bash.exe` (the same file, copied by install) | the interposed shell every agent command passes through |
| `infuse <command>` | CLI: `claude`, `codex`, `gemini`, `init <base> [dir]`, `install`, `uninstall`, `status`, `map`, `journal`, `lessons`, `rules`, `watch` |

How external agents are caught: `install` points Claude Code at the copied shell through
`CLAUDE_CODE_GIT_BASH_PATH` on Windows and `CLAUDE_CODE_SHELL` on macOS/Linux. The launcher
(`infuse claude|codex|gemini`) installs the shim and starts the CLI with `SHELL`/`PATH` pointed at it
for that process, without touching the user's global settings; the Studio's own "open agent" action
always goes through this launcher. The bridge providers (`claudedesk`/`codexdesk`) keep the model as a
tool-less pipe and execute every tool inside infuse.

## What passes through the filter (slice 1)

- No heredoc ever reaches bash (file writes become direct writes, program feeds become redirections;
  quoted keeps exact bytes, unquoted keeps same-shell expansion; tolerant of indented/missing terminators, CRLF, Git Bash paths).
- A heredoc body is judged by the write policy before it is written (plan, size, base, bans, large
  removal, suppressors); everything else the shell changes is judged at the end of the turn.
- Command output comes back with what was written, who depends on it, the map of what was listed, and whatever your rules inject.
- A per-project watcher starts on first use and journals every write by any tool within milliseconds.
- Project map (exports, imports, dependents, hotspots), generated from code only; Go and Rust imports
  resolve to the file they name, so dependents are real for those languages too.
- Lessons carry the deterministic signature and the remedy from the base vice catalog; Glob/Grep
  results carry the same directory map as LS.
- User rules (`~/.infuse/rules.json`, `<project>/.infuse/rules.json`): command / write / list / always → replace, say, infuse, infuseFile, vice.
- An empty `.infuse/base.json` means no base. `{"base":"web"}` chooses a starter (api, web, cli,
  interactive); a full object declares custom folders; code written outside the chosen folders is refused.
  `infuse init <base> [dir]` scaffolds a starter's folders and writes its base document.
- Hook and MCP approvals are signed with a key kept outside the project: an approval file the agent
  edits no longer approves anything.

## Build

```sh
go build -ldflags="-s -w -H windowsgui" -o build/infuse.exe ./cmd/infuse   # Windows: no console window
go build -o build/infuse ./cmd/infuse                                      # macOS/Linux (CGO_ENABLED=1 for the native window)
go vet ./...
go test ./...
node --experimental-vm-modules scripts/check-ui.mjs && node tools/check-frontend.mjs  # frontend syntax and architecture
node scripts/check-i18n.mjs && node scripts/check-language.mjs             # catalog coverage and English-only
node scripts/gen-tokens.mjs                                                # must reproduce ui/css/tokens.css
```

The last audit and its evidence live in `docs/AUDIT-REPORT.md` (`docs/audit/` holds the 2× Chrome
screenshots and the console/network logs).

## Repository layout

`cmd/infuse` is role dispatch and wiring only. The domains live in `internal/`: `core` (types,
messages, paths), `config`/`connections`/`llm`, `filter` (the interposed shell), `knowledge`,
`browser`, `mcp`, `oracle`, `workspace`, `terminal`, `scheduler`, `agent` (session loop, gates,
tools), `api` (HTTP/SSE), `desktop` (window, WebView2, dialogs). The frontend is `ui/src` (ES modules:
core, components, one module per pane/page), `ui/css` with the DTCG sources in `ui/tokens`, and
`internal/core/catalog.json` (the single message catalog used by Go and the UI). The architecture
rules are executable: `go test ./internal/architecture`.

`legacy/ts` holds the original TypeScript implementation of slice 1, kept for reference.
