<div align="center">

# zerrcode

**A human-in-the-loop AI coding agent for your terminal, written in Go.**

`zerrcode` turns a natural-language request into verified changes in your workspace.
It runs an LLM through [OpenRouter](https://openrouter.ai) and lets it call a small set of
workspace tools — read, search, patch, write, delete — inside a sandboxed project directory.

In **TUI mode** it goes further: every change the model proposes is computed as a **real
before/after diff** from the files on disk, and shown to you for approval **before anything
is written**. Nothing reaches your disk that you have not seen and approved.

[![Go](https://img.shields.io/badge/go-1.27%2B-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)
[![Tests](https://img.shields.io/badge/tests-71%20functions%20%2F%20190%20cases-success?style=flat-square&logo=go&logoColor=white)](internal)
[![TUI](https://img.shields.io/badge/TUI-Bubble%20Tea%20v2-FF7AB6?style=flat-square&logo=charm)](internal/tui)

</div>

---

## Table of Contents

- [Why zerrcode](#why-zerrcode)
- [Two Ways to Run It](#two-ways-to-run-it)
- [The TUI](#the-tui)
- [Features](#features)
- [Built-in Tools](#built-in-tools)
- [Requirements](#requirements)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Testing](#testing)
- [Code Quality](#code-quality)
- [Security Model](#security-model)
- [Known Limitations](#known-limitations)
- [Project Structure](#project-structure)
- [Architecture](#architecture)
- [Documentation](#documentation)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

---

## Why zerrcode

Most "AI coding assistant" demos hand the model a shell and hope for the best. `zerrcode`
takes a narrower, more defensible position:

1. **No shell. Ever.** The model can only act through seven explicit, schema-described
   tools. It cannot run `go test`, `npm install`, or `curl`. The system prompt states this
   outright and requires the agent to admit what it did *not* run.
2. **Humans approve every write.** Workspace-changing tools are *planners*: they validate
   the request and compute the complete result — including a real diff — in memory. The
   registry then shows that plan to an approval manager. In TUI mode that is you. If no
   approver is configured, the registry **fails closed**.
3. **The diff you review is the truth.** Approval diffs are computed from the file's actual
   current content versus the content that would be written — never from the model's own
   description of the change. If the model claims to have edited one line and actually
   truncated the file, the diff shows it immediately.
4. **Your edits are never clobbered.** Each plan snapshots the files it touches. If a file
   changed while you were reviewing the diff, the write is refused rather than silently
   overwriting your work.
5. **Verified completion, enforced in code.** After a mutating tool succeeds, the run is
   marked *unverified*. The agent loop will not let the model finish until it has actually
   called `git_diff` and seen the result.
6. **Workspace confinement by construction.** Every path — from any tool — passes through a
   single shared gate that rejects lexical `..` escapes *and* symlink escapes, before any
   filesystem call.

The result is an assistant that is deliberately quieter and more honest than a
shell-wielding agent, and that is safe to point at a real repository.

---

## Two Ways to Run It

One binary, two modes.

| | TUI mode | REPL mode |
|---|---|---|
| **Command** | `zerrcode -tui` | `zerrcode` |
| **Interface** | Full-screen Bubble Tea chat UI | Classic line-based prompt |
| **Change approval** | **Interactive** — real diff, `y` / `n` | Automatic (`AllowAll`) |
| **Tool activity** | Rendered in the chat as live events | Not printed (assistant text only) |
| **Cancel a running turn** | `Ctrl+C` / `Esc` | Not supported |
| **Best for** | Careful, review-driven work | Scripting, piping, quick edits |

> **Both modes require an OpenRouter API key.** `zerrcode` exits immediately with
> `OPENROUTER_API_KEY is not set` if the key is missing. See [Configuration](#configuration).

---

## The TUI

`-tui` launches a full-screen terminal interface built on [Bubble Tea v2](https://github.com/charmbracelet/bubbletea).

```
 zerrcode · myproject                                              ● ready
──────────────────────────────────────────────────────────────────────────────
                                       zerrcode
                                       I'll rename the variable and update the
                                       callers.

  ⚡ edit_file {"patch":"--- a/main.go..."}
    ↳ modified main.go
──────────────────────────────────────────────────────────────────────────────
 ❯ Send a message...▌
 esc quit  ·  enter send  ·  ↑/↓ scroll  ·  /help  ·  /clear
```

When the agent wants to change a file, the UI switches to an approval card showing the
actual diff:

```
 ⚠️  Proposed edit_file (2 file(s) changed):
  ➜ modified main.go
    created internal/tools/new.go

  --- Diff preview: main.go (File 1 of 2, [Tab] to switch) ---
  @@ -12,7 +12,7 @@
   func main() {
  -	x := 1
  +	count := 1

 ⚠ Approve changes? [y] Yes  ·  [n] No  ·  [d] Toggle Diff  ·  [Tab] Next File
```

### TUI keybindings

| Key | Context | Action |
|---|---|---|
| `Enter` | Composer | Send the message |
| `Esc` | Composer | Clear the input |
| `Esc` | Working | Cancel the running turn |
| `Esc` | Idle | Quit |
| `Ctrl+C` | Working | Cancel the running turn |
| `Ctrl+C` | Idle | Quit |
| `y` / `Y` | Approval | Approve the change |
| `n` / `N` | Approval | Reject the change (nothing is written) |
| `d` / `D` | Approval | Toggle the diff preview |
| `Tab` | Approval | Cycle to the next affected file |
| `↑` / `k` | Viewport | Scroll up (pauses auto-scroll) |
| `↓` / `j` | Viewport | Scroll down |
| `PgUp` / `PgDn` | Viewport | Scroll by five lines |

### Slash commands

| Command | Action |
|---|---|
| `/help` | Show the available commands |
| `/clear` | Clear the conversation view |
| `/exit`, `/quit` | Quit the application |

---

## Features

### Core agent

- **Conversational agent** — a REPL or TUI chat that keeps full conversation context across
  the session.
- **Tool calling** — seven workspace tools, each with its own JSON Schema, discovered
  dynamically from the registry.
- **Streaming responses** — model output is streamed to the terminal token by token.
- **Bounded agent loop** — hard cap of 25 model round-trips per turn (configurable), with
  `context` cancellation propagated through every layer.
- **Verified completion** — the agent is forced to read `git diff` before it can claim the
  work is done.
- **Honest reporting** — the system prompt separates *Done and observed* from
  *Not run / not verified*, and forbids claiming any command was run without a matching
  tool result.

### Interactive TUI

- **Full-screen Bubble Tea interface** — header bar, scrollable chat viewport, input
  composer, and a contextual footer.
- **Interactive change approval** — every workspace-changing call shows a real
  before/after diff for you to approve (`y`) or reject (`n`).
- **Multi-file diff review** — cycle through every affected file with `Tab`.
- **Real-time streaming** — assistant responses stream token by token into the chat.
- **Turn cancellation** — `Ctrl+C` or `Esc` cancels the in-flight turn.
- **Live event feed** — tool calls, results, rejections, and verification nudges render as
  they happen.
- **Auto-scroll** — follows new content; manual scrolling pauses it.

### Safety

- **Workspace sandbox** — lexical *and* symlink-aware path confinement on every operation.
- **Optimistic concurrency guards** — if a file changes while you are reviewing an
  approval, the write is refused rather than silently overwriting your edits.
- **Fail-closed approval** — with no approver configured, the registry refuses to change
  the workspace at all.
- **Strict input validation** — unknown JSON fields, trailing values, control characters,
  absolute paths, `..` segments, and `.git` components are all rejected.

---

## Built-in Tools

Tools describe themselves — name, description, and JSON Schema live next to the code that
parses the arguments, so the two cannot drift apart.

| Tool | Mutates | Description |
|---|:---:|---|
| `read_file` | — | Read the contents of a file within the workspace. |
| `list_files` | — | List files and directories in a workspace directory (sorted, non-recursive). |
| `search_code` | — | Literal-text search across files (wraps `grep -rnIF`, flag-safe). |
| `git_diff` | — | Read-only: changed files plus the diff against `HEAD`. |
| `write_file` | ✅ | Create a file, or overwrite an existing one. |
| `edit_file` | ✅ | Apply a multi-file unified diff — all-or-nothing. |
| `delete_file` | ✅ | Delete a single file within the workspace. |

The three mutating tools implement the `Planner` interface: `Plan` validates the request
and computes the complete result in memory, and `Apply` performs the write once — and only
once — the plan has been approved and re-verified against the filesystem.

See [`docs/tools.md`](docs/tools.md) for the exact input schema, validation rules, and
failure behaviour of each tool.

---

## Requirements

| Requirement | Needed for | Notes |
|---|---|---|
| **Go 1.27+** | Building | See `go.mod` for the pinned toolchain. |
| **OpenRouter API key** | **Both modes** | Set `OPENROUTER_API_KEY`. The program exits without it. |
| **git** | `git_diff` | Also required for the workspace to be verifiable. |
| **grep** | `search_code` | Literal search only. |
| **True-color terminal** | TUI mode | 24-bit color. REPL mode works anywhere. |

---

## Installation

```bash
git clone https://github.com/Aneeshie/zerrcode.git
cd zerrcode

go mod tidy             # install dependencies
go build -o zerrcode .  # build the binary
```

Or run it straight from source without building:

```bash
go run . -tui
```

---

## Configuration

Configuration is environment-based. Create a `.env` file in the project root (it is
git-ignored):

```env
OPENROUTER_API_KEY=your_openrouter_api_key_here
```

| Variable | Required | Description |
|---|:---:|---|
| `OPENROUTER_API_KEY` | ✅ | Authenticates every request to the OpenRouter API. **Required in both TUI and REPL mode** — `zerrcode` prints `OPENROUTER_API_KEY is not set` and exits if it is missing. |

The key may also be exported in your shell environment; `.env` is loaded first and the
process environment wins if both are set.

> **Never commit your API key.** `.env` is already listed in `.gitignore`.

The default model is `minimax/minimax-m3.1-flash:free`. It is set in
`internal/client/openrouter-client.go` (`convertToChatRequest`) and can be changed to any
model available on OpenRouter.

---

## Usage

Run the binary from the root of the project you want to work on — the workspace root is
the current working directory, and every tool is confined to it.

### TUI mode

```bash
./zerrcode -tui
```

Type a request and press <kbd>Enter</kbd>. When the agent proposes a change, review the
diff and press <kbd>y</kbd> to approve or <kbd>n</kbd> to reject.

### REPL mode

```bash
./zerrcode
```

```console
zerrcode — type 'exit' to quit.

zerrcode > list the files in the current directory
zerrcode > read main.go and explain what it does
zerrcode > create hello.go that prints Hello, World
zerrcode > rename the variable x to count in main.go
zerrcode > exit
Goodbye!
```

Press `Ctrl+D` to quit as well. Note that REPL mode prints only the assistant's text —
tool activity is visible in TUI mode.

---

## Testing

Testing is a first-class part of this project, not an afterthought. The suite is
**190 test cases across 71 test functions**, all of which pass.

### Coverage

| Package | Statement coverage | Test functions | Notes |
|---|:---:|:---:|---|
| `internal/diff` | **100.0%** | 5 | Line classification, binary handling, create/delete, no-newline-at-EOF, round-trip against `gitdiff`. |
| `internal/agent` | **95.0%** | 18 | Every branch of the loop, the verification gate, rejection handling, and the error paths. |
| `internal/tools` | **78.3%** | 40 | All tools, all path-confinement and argument-validation paths. |
| `internal/tui` | **38.0%** | 8 | Model interactions: typing, streaming, approval, rejection, slash commands, resize. |
| `internal/client` | 0.0% | — | Thin SDK adapter; exercised end-to-end against the live API. |
| `internal/permissions` | 0.0% | — | Contract only (`Manager`, `AllowAll`, `DenyAll`). |
| `main.go` | 0.0% | — | Flag parsing and REPL/TUI wiring. |
| **Total** | **56.7%** | **71** | Across the whole module. |

### Running the suite

```bash
go test ./...                    # run everything
go test ./... -v                 # verbose, with subtest names
go test ./... -race              # race detector
go test ./... -cover             # per-package coverage
go test ./... -coverprofile=cov.out && go tool cover -html=cov.out   # HTML report
```

### What the tests actually cover

The suite is written against **real behaviour, not mocks**, wherever the behaviour under
test is a filesystem or git behaviour.

**Every tool is tested for the failure modes that matter when code runs on model output:**

- path traversal (`..`, deep traversal, absolute paths) and **symlink escapes**
- missing files, directories passed where a file is expected, and vice versa
- empty and blank arguments
- malformed JSON, unknown fields, and multiple JSON values in one call
- assertions that a rejected call leaves the workspace **untouched**, including files
  *outside* it

**Tools are exercised against real temporary directories and real git repositories**,
because symlink resolution and file permissions are exactly what is being verified.

**The agent loop is tested with a scripted fake client and fake tools**, covering:

- the normal loop, and multiple tool calls in a single turn
- tool errors being forwarded to the model instead of aborting the run
- unknown tool names being reported back to the model
- the iteration cap (`ErrMaxIterations`) stopping a runaway tool-call loop
- context cancellation stopping the run mid-flight
- the verification gate: that it fires after a change, that it does *not* fire for
  read-only runs or for *failed* changes, that a later change makes an earlier
  verification stale, that it can be disabled, and that it gives up after the nudge limit
- **rejection handling** — a user-rejected change is reported to the model with a distinct
  status and never marked as a workspace change

**The TUI is tested as a state machine**, not by screenshotting: typing and backspace,
quit, agent-done, token streaming, the full approval flow, rejection, slash commands, and
window resize.

**The diff library is tested by round-trip property test.** Over 300 randomized
before/after pairs, the diff it produces is parsed and applied back by the independent
`go-gitdiff` parser, and the result must reproduce the intended content byte for byte —
which catches classification bugs a few hand-written cases would miss.

**The system prompt is tested as a contract.** `TestSystemPromptStatesKeyRules` asserts
that the prompt still contains each rule the product depends on (no shell, never claim
unverified work, keep the two lists separate, the `edit_file` retry rule, and the
`git_diff` verification rule).

**Tests degrade gracefully.** Tests that require `grep` or `git` skip themselves with a
clear message when the binary is unavailable, so the suite stays green on minimal
environments.

See [`docs/decisions.md`](docs/decisions.md#13-test-strategy) for the reasoning behind
this testing strategy.

---

## Code Quality

The codebase is kept clean by the standard Go toolchain, and the checks are part of the
normal workflow:

```bash
gofmt -l .     # formatting: no output == all files formatted
go vet ./...   # static analysis: no output == no findings
go test ./...  # tests: all packages pass
```

Current status: **`gofmt` clean, `go vet` clean, all tests passing.**

---

## Security Model

`zerrcode` can modify your files, so filesystem access is constrained in layers.

**Workspace confinement.** Every tool resolves paths through `resolveInWorkspace`, which
rejects paths that escape the root lexically (`../../etc/passwd`) *and* after resolving
symlinks — a symlink inside the workspace pointing outside it is refused. Only the
existing prefix of a path is resolved, so creating new files still works.

**Stricter validation for model-supplied paths.** `edit_file` (every path in a patch) and
`git_diff` additionally reject absolute paths, any `..` segment, any `.git` component,
control characters including NUL, and paths over 4096 bytes.

**Approval gate.** `Registry.Execute` is the only path from a model's tool call to a tool,
and it is where approval is enforced. A `Planner` tool is planned first, the resulting
changes (with real diffs) are handed to the `permissions.Manager`, and only an approved
plan is applied. **With no approver configured, the registry fails closed** with
`ErrNoApprover` — it never silently assumes consent.

**Real diffs, computed from disk.** Approval diffs come from the file's actual current
content and the content that would be written. The model's own description of the change
is never what you are shown.

**Optimistic concurrency.** Every plan snapshots the files it touches. `Plan.Apply`
re-checks those snapshots immediately before writing; if anything changed in the interim
the write is refused with a clear message.

**All-or-nothing edits.** `edit_file` validates and computes every file in memory before
writing anything, so a failed hunk changes nothing.

**No clobbering.** A create or rename may not overwrite an existing file, and a path may
appear only once per patch.

**Strict inputs.** JSON arguments are decoded with `DisallowUnknownFields` and trailing
values are rejected.

**Read-only git.** `git_diff` passes only fixed arguments to git — never raw model text —
and never writes to the repository.

**Bounded work.** The agent loop has an iteration cap, tool output is capped
(`search_code` 32 KB, `git_diff` 64 KB), and `edit_file` is limited to 50 files per patch.

---

## Known Limitations

This project is honest about its edges. The full list with reasoning is in
[`docs/decisions.md`](docs/decisions.md#known-limitations).

### Security

- **Not a hardened sandbox.** The symlink checks are time-of-check / time-of-use: a symlink
  could be swapped between the check and the write. Concurrent filesystem changes and
  filesystem permissions need careful consideration before running against untrusted
  workspaces.
- **`.git` protection is partial.** It applies to `edit_file` and `git_diff` only —
  `write_file` and `delete_file` do not apply it.
- **The approval gate protects writes, not reads.** Read-only tools (`read_file`,
  `search_code`) can read anything inside the workspace, including secrets you have not
  meant to expose to the model.
- **REPL mode has no approval gate.** It runs with `permissions.AllowAll{}`, so every
  change is written without review. Use `-tui` when the workspace matters.

### Filesystem and tools

- **`edit_file` phase 2 is not transactional.** Validation and computation happen
  all-or-nothing, but the write phase is sequential: a mid-write failure (permissions,
  disk full) leaves earlier files written. The error reports which ones.
- **`edit_file` format detection is per patch.** A patch mixing plain and `diff --git`
  sections is treated as git format throughout; in a plain diff with no `a/`/`b/` prefixes,
  a real top-level directory named `a` or `b` would be mis-stripped.
- **Binary patches are unsupported** by `edit_file`, and binary files render as an
  opaque "binary files differ" marker in the TUI rather than a line diff.
- **`write_file` is not atomic** (no temp-file-and-rename), and **`read_file` has no size
  limit** — very large files are returned in full.
- **`git_diff` does not show the contents of untracked files.** They appear in the changed
  file list, but `git diff` has nothing to show for them.
- **`search_code` requires `grep`** and is **literal-only** — it does not support regular
  expressions.

### Agent behaviour

- **Prompt rules are guidance.** Only the iteration cap and the verification nudge are
  enforced in code. The agent can be *required* to call `git_diff`, but it cannot be
  forced to judge the diff correctly.
- **Verification is best-effort.** After `MaxVerificationNudges` reminders (default 2) the
  agent returns anyway rather than looping forever.
- **The agent has no shell.** It cannot run tests, builds, linters, or formatters. If you
  want those run, you must run them — and the agent is instructed to say so rather than
  guess.
- **A model can still talk nonsense.** The prompt asks for honest reporting; it cannot
  guarantee it.

### TUI and UX

- **The TUI needs a true-color terminal.** The theme uses 24-bit ANSI. On terminals
  without it, colours degrade but the interface still functions.
- **An unanswered approval blocks the turn.** The approval prompt waits on the agent's
  tool goroutine until you answer or the turn is cancelled. There is no timeout.
- **REPL mode cannot interrupt a running request.** The REPL uses a background context
  with no cancellation path; only the TUI can cancel a turn mid-flight.
- **No session persistence.** Conversation history lives in memory and is lost on exit.
  There is no resume, no transcript export, and no checkpoint.
- **One workspace per process.** The workspace root is the current working directory.
  There is no way to point the agent at a different directory at runtime.
- **TUI coverage is the thinnest in the project** (38%). The state-machine transitions are
  well covered; the rendering layer is not.
- **The model is a compile-time constant.** Changing it means editing
  `internal/client/openrouter-client.go` and rebuilding — there is no env var or flag.
- **`internal/client` has no unit tests.** It is a thin adapter over the OpenRouter SDK
  and is covered end-to-end against the live API instead.

---

## Project Structure

```text
zerrcode/
├── main.go                          # CLI entry point: -tui flag, REPL/TUI wiring
├── go.mod / go.sum
├── LICENSE                          # MIT
├── README.md
├── docs/
│   ├── architecture.md              # components, agent loop, approval flow, security model
│   ├── decisions.md                 # design decisions, trade-offs, limitations, change log
│   └── tools.md                     # per-tool reference: inputs, validation, failures
└── internal/
    ├── agent/
    │   ├── agent.go                 # bounded agent loop, event system, verification gate
    │   ├── prompt.go                # SystemPrompt (honesty, editing, verification rules)
    │   ├── agent_test.go            # scripted fake client + fake tools
    │   └── prompt_test.go
    ├── client/
    │   ├── client.go                # LLMClient interface, Message / ToolCall types
    │   └── openrouter-client.go     # OpenRouter SDK adapter (streaming + tool calls)
    ├── diff/
    │   ├── diff.go                  # structured diff computation from real content
    │   └── diff_test.go
    ├── permissions/
    │   └── persmissions.go          # approval contract (Manager, FileChange, AllowAll)
    ├── tools/
    │   ├── tool.go                  # Tool, Mutator and Planner interfaces
    │   ├── plan.go                  # Plan/Planner pattern + optimistic concurrency guards
    │   ├── registry.go              # registry + the approval gate
    │   ├── path.go                  # resolveInWorkspace — shared symlink-aware path gate
    │   ├── read_file.go
    │   ├── list_files.go
    │   ├── search_code.go
    │   ├── git_diff.go
    │   ├── write_file.go            # Planner: approval-gated write
    │   ├── edit_file.go             # Planner + validatePath, multi-file patches
    │   ├── delete_file.go           # Planner: approval-gated delete
    │   └── *_test.go
    └── tui/
        ├── model.go                 # Bubble Tea Model (chat UI, approval card)
        ├── theme.go                 # ANSI true-color theme, diff rendering
        ├── approver.go              # TUIApprover — permissions.Manager bridge
        └── model_test.go
```

---

## Architecture

The agent loop, the tools, the approval contract, and the UI are separate packages behind
interfaces. Nothing in `internal/agent` knows about Bubble Tea; nothing in `internal/tools`
knows about the terminal.

| Layer | Responsibility | Knows about |
|---|---|---|
| `main.go` | Chooses a client, registers tools, wires approval, runs REPL or TUI. | Everything — the only place that wires things together. |
| `internal/tui` | Full-screen interface and the approval UI. | Agent events, `permissions.Manager`, diff rendering. |
| `internal/agent` | The loop: call the model, run tools, feed results back, enforce limits and verification, emit events. | The `LLMClient` interface and the `Registry`. Nothing provider-specific. |
| `internal/client` | Translate between internal types and the OpenRouter SDK. | The SDK, `Message`, `ToolDefinition`. |
| `internal/tools` | The capabilities, the Plan/Planner pattern, and the approval gate. | The filesystem, `grep`, `git`, `permissions`, `diff`. |
| `internal/permissions` | The approval contract. | `diff.FileDiff`. Nothing about tools, agent, or UI. |
| `internal/diff` | Structured diff computation. | `go-udiff`. Nothing else. |

### The agent loop

1. Build tool definitions from the registry and send the conversation to the model.
2. If the model requested tools, execute each one through the registry — which plans,
   diffs, requests approval, and applies. A tool error becomes
   `Tool call failed: <error>`; it never aborts the run.
3. If the model returned a plain answer, the run may end — **unless** the workspace was
   modified and `git_diff` has not been called since. In that case the agent appends a
   verification reminder and loops (up to `MaxVerificationNudges`, default 2).
4. `MaxIterations` (default 25) caps model round-trips per run.

The agent emits `Event`s (`EventAssistant`, `EventToolStart`, `EventToolResult`,
`EventVerify`) instead of printing, so the same loop drives the REPL, the TUI, or a
headless service.

Full details, including Mermaid diagrams of the request lifecycle, the agent loop, and
the approval sequence, are in [`docs/architecture.md`](docs/architecture.md).

---

## Documentation

| Document | Contents |
|---|---|
| [`docs/architecture.md`](docs/architecture.md) | Components, the request lifecycle, the agent loop, the approval flow, the system prompt, and the security model. Includes Mermaid diagrams. |
| [`docs/tools.md`](docs/tools.md) | Exactly how each tool works: inputs, validation, output, failure behaviour, and limitations. |
| [`docs/decisions.md`](docs/decisions.md) | Every significant design decision with its context, reasoning, and trade-offs; known limitations; a chronological change log. |

---

## Roadmap

- [x] OpenRouter client with streaming and tool calling
- [x] Tool registry with self-describing JSON Schemas
- [x] Symlink-aware workspace path confinement
- [x] Multi-file, all-or-nothing `edit_file`
- [x] Read-only `git_diff` and the enforced verification gate
- [x] Honest-reporting system prompt
- [x] Bounded, cancellable agent loop
- [x] Full-screen Bubble Tea TUI with interactive diff approval
- [x] Plan/Planner pattern with optimistic concurrency guards
- [x] Fail-closed approval gate in the registry
- [x] Structured diff computation from real file content
- [x] Comprehensive test suite
- [ ] Session persistence — resume a conversation across runs
- [ ] Configurable model and temperature via environment or flags
- [ ] Additional LLM providers behind the `LLMClient` interface
- [ ] Token / cost accounting per session
- [ ] Undo / rollback of an applied change without git
- [ ] Hardening the path gate against time-of-check / time-of-use races

---

## Contributing

Contributions, bug reports, and suggestions are welcome.

1. Fork the repository.
2. Create a branch for your change.
3. Add tests that cover it — the suite is expected to stay green, `gofmt`-clean, and
   `go vet`-clean.
4. Update the documentation if you change behaviour.
5. Open a pull request describing the change and the reasoning behind it.

Design decisions and their trade-offs are documented in
[`docs/decisions.md`](docs/decisions.md); if you are about to make a significant
architectural change, that file is the right place to add the rationale.

---

## License

Released under the [MIT License](LICENSE).

Copyright © 2026 Aneesh.
