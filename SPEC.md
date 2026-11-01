# infuse — specification

> Water goes in pure and clear. It passes through the coffee filter and comes out dark. Always. Instantly.
> There is no path to the pot that does not go through the filter.

## 1. What it is

`infuse` is a **mandatory, instantaneous, agent-independent** interposition layer between a coding agent
(Claude Code, Codex CLI, Gemini CLI, any other) and the project.

Everything the agent **reads** arrives infused with the project's knowledge.
Everything the agent **writes or executes** is transformed by that same knowledge before it touches disk.
The agent does not consult the filter, call the filter, or configure the filter. It passes through it.

The filter is **not control**. The filter **is the knowledge**: the correct architecture, the project map,
the right way to search, code and fix, and the accumulated lessons.
The interposition layer is only the paper; the grounds are what give the color.

## 2. What it is NOT

- Not instructions to the agent (CLAUDE.md, AGENTS.md, prompts, rules). Text competes with habit and loses.
- Not blocking hooks. Walls get circumvented; terrain does not.
- Not dependent on the model API. No API proxy.
- Not dependent on the agents' native hooks. They are inconsistent across agents and break across versions;
  they are used only as redundancy where reliable.
- Not another LLM judging the LLM. Every transformation is deterministic; every verdict comes from execution.

## 3. Where the water passes (interposition points)

Every coding agent, in order to exist, does three things at the OS level: reads files, writes files,
runs commands. These are the only points that catch everything, in every agent.

| Point | Mechanism | Infused on the way in | Transformed on the way out |
|---|---|---|---|
| **Project directory** | mount (FUSE / WinFsp) when available; instant watcher always | directory map; native per-folder context files (`CLAUDE.md`/`AGENTS.md`/`GEMINI.md`, generated) | relocation to the base's place; oracle paths rejected; vice recorded |
| **Shell** | interposed shell on PATH, invoked by every agent's Bash tool | command output + oracle state + untouched dependents | heredoc → file write; vice patterns → remedy in the response |
| **Launcher** | `infuse claude` / `infuse codex` / `infuse gemini` | environment mounted before the agent opens | — |

What escapes: what the model thinks (touches nothing) and the final message it writes to the user
(not a verdict; the verdict comes from the executor).

## 4. The grounds (contents of the filter)

1. **Map** — the project as it is, generated from code, never hand-written, refreshed on every write.
   Level: feature → where it is split; module → responsibility; data → owner; "how X is done here" → real examples;
   intended vs. actual architecture.
2. **Bases** — reference architectures per project type, as executable scaffolds. Every base defines:
   canonical model and owner of each piece of data; declared states; layers and allowed dependencies;
   extension points (registries); config/tokens/data tables in one place; versioned persistence;
   built-in observability; observable surface for the oracle.
3. **Procedures and lessons** — how to search, code, fix, port; and what this project has already taught.
   Includes the vice catalog: deterministic signature + remedy, always together.
4. **Perception** — evidence about the agent's own code: dependents, execution cost, docs for the exact installed version.
5. **Oracle** — "done" defined before code, at the objective level (several dimensions fixed),
   readable by the agent, out of its reach.
6. **Executor** — runs the oracle on an immutable snapshot, produces evidence (not opinion), delivers the verdict to the user.
   "Not achieved, here is the evidence" is a legitimate first-class output.

## 5. Design principles (decided; do not reopen)

- The agent **sees the criteria**, **cannot reach the executor**. (Evidence: hiding the oracle cuts cheating but
  hurts honest performance; read-only prevents tampering.)
- The oracle measures the **objective**, never an isolated proxy. Isolated metrics get gamed.
- Architecture is **executable structure**, not rules. The correct path must be the cheapest.
- Verification is **by evidence**, never by the agent's report.
- Transformation is **deterministic and sub-second**. No LLM in the filter path.
- Every infuse artifact lives in the repository, versioned, readable by any agent.

## 6. Repository artifacts

```
.infuse/
  map/            # generated map (json + per directory)
  base.json       # which base was chosen, recorded
  oracle/         # done criteria (read-only for the agent)
  lessons.jsonl   # observed vices: signature, frequency, remedy
  journal.jsonl   # every write that passed through the filter
  evidence/       # executor outputs, per snapshot
```

## 7. Delivery slices

> Status (0.5): slice 1 done; the agent runs *inside* the filter, in a native window, with the oracle
> guarded in the executor for every tool. The oracle executor runs the user's criteria on an immutable
> snapshot at the end of every turn (including turns that changed nothing, reusing a content-hash
> verdict), the criteria are anchored in the user's store outside the project and the verdict carries an
> HMAC, so neither file can be forged inside the project. A shell/MCP/subagent write is judged by the
> same write rules before the turn can end — heredocs before they run, everything else at the turn
> boundary. Slice 2 is partially delivered: the starter bases (api, web, cli, interactive), the
> `.infuse/base.json` choice, out-of-base refusal and scaffolding (`infuse init <base>` or Customize >
> Base creates the folders, the base document and a generated map; the same surface applies a base and
> creates a project from one) exist. Slice 3 is delivered and anchored. Slice 5's vice catalog now
> ships as a data table (`internal/data/vices.json`)
> and lessons carry signature + remedy. Also delivered: tools, permissions, sessions, checkpoints,
> compaction, git, subagents with an independent reviewer, the MCP client, web search, the desktop
> bridges and the `infuse claude|codex|gemini` launcher. The behaviour policy is one versioned document
> edited in the Regras surface (permissions/prohibitions, work method, architecture, conventions, done),
> and multi-step work needs a user-confirmed Contract before the first write. OS containment is partial:
> every agent process runs in a Job Object on Windows, and Linux commands run under a Landlock ruleset;
> a deny-write OS sandbox for Git Bash on Windows is blocked by the MSYS2 runtime and remains the open
> item, together with the FUSE/WinFsp mount.


1. **Minimal filter** — launcher + interposed shell (heredoc → write) + map v0 + write watcher. ← this one
2. Bases v0 (API, web, CLI, interactive app) as scaffolds with a pre-filled map.
3. Oracle + executor (web via browser, CLI via process, API via requests).
4. Mount (FUSE/WinFsp) for pre-write interposition.
5. Full vice catalog (from the 32-item base catalog) + lessons.

## 8. Done for the tool itself (infuse's own oracle)

Slice 1 is done when, with no instruction given to the agent:
- a heredoc command emitted by the agent results in the file written correctly, with no heredoc reaching bash;
- the agent listing a directory receives that directory's map alongside;
- every agent write is journaled with path, size and detected vice (if any);
- the same behavior holds under Claude Code, Codex CLI and Gemini CLI, launched through `infuse`.
