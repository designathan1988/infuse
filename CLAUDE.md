# infuse — working instructions

These override any default behavior. Read them before touching the project.

## UI verification is Chrome + Playwright, always (hard rule)

Every UI change, fix or regression check is verified in **Google Chrome driven by Playwright**, headless,
at **2× device scale** (viewport 1440×900 → PNG 2880×1800). The embedded Browser pane, the native
WebView2 window and the agent's Browser tool are **not** verification surfaces; they are features under
test.

- Capture every affected screen and state, in light/dark and pt-BR/en where they apply, with stable
  names under `docs/audit/screens/`; console and network logs under `docs/audit/logs/`.
- **Open and analyze every PNG**; a screenshot that was not looked at is not evidence.
- A fix needs before/after captures at the same viewport, theme and language.
- The sweep fails on any console error or any HTTP status ≥ 400.
- Harness: `C:\Users\Jonathan\AppData\Local\Temp\infuse-audit\lib.mjs` (`launch`, `open`, `shot`,
  `saveLogs`, `close`), Chrome at `C:\Program Files\Google\Chrome\Application\chrome.exe`.
- Server for the sweep: `build/infuse-dev2.exe serve 7471` with `INFUSE_UI_DIR=ui` and
  `INFUSE_TOKEN=dev-preview-only`; open `http://127.0.0.1:7471/#token=dev-preview-only`.
- Stop the server and leave no process running when the sweep ends.

## Communication

- Every message to Jonathan is in **Brazilian Portuguese**. Code, identifiers, comments, tests, commits
  and documentation stay in **English**; the only Portuguese is the `pt-BR` catalog and end-user guides.
- Do not stop to ask which item to do next: decide and continue.

## Work and evidence

- "Done" requires execution evidence: tests, verdicts, logs or high-resolution screenshots. Never claim
  success from prose.
- After changing code, rebuild the binaries in `build/` (development and release) and exercise the
  changed feature.
- No windows or popups on the user's screen; prefer the local server and headless Chrome.
- Never leave an AI/model/session running: use `mock` for deterministic flows and stop every process you
  started.
- Never weaken a test or disable a feature to make a check pass; fix the root cause and add a regression
  test.
- Uncommitted work in the tree is the user's: inspect `git status` before anything destructive, never
  discard it, and do not commit or push unless explicitly asked.
- Use the portable Go toolchain at
  `C:\Users\Jonathan\AppData\Local\Temp\infuse-refactor\toolchain\go` (no system Go).

## Architecture rules (enforced by tests)

- One responsibility per file; no file over 400 lines; no function over 60 lines.
- One owner per state; one message catalog (`internal/core/catalog.json`); design tokens as the only
  color/size source; behaviour data in tables; contracts in JSON schemas.
- Dependencies point down: `cmd` → `api`/`desktop` → `agent` → capabilities → `config`/`llm`/`core`.
- `go vet ./...`, `go test ./...`, `go test ./internal/architecture`, the race suite, cross-builds,
  `node --experimental-vm-modules scripts/check-ui.mjs`, `node tools/check-frontend.mjs`,
  `node scripts/check-i18n.mjs`, `node scripts/check-language.mjs` and `node scripts/gen-tokens.mjs`
  (must reproduce `ui/css/tokens.css`) are the quality gate.
