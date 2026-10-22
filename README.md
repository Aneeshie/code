<div align="center">

# zerrcode

**An interactive, terminal-native coding agent written in Go.**

`zerrcode` turns a natural-language request into verified changes in your workspace.
It runs a local REPL, streams an LLM's answer from [OpenRouter](https://openrouter.ai),
and lets the model call a small set of workspace tools — read, search, patch, write,
delete — inside a sandboxed project directory.

After it changes anything, `zerrcode` does not take the model's word for it: the agent
is **required to inspect the real `git diff`** before it is allowed to report completion.

[![Go](https://img.shields.io/badge/go-1.27%2B-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)
[![Tests](https://img.shields.io/badge/tests-58%20functions%20%2F%20177%20cases-success?style=flat-square&logo=go&logoColor=white)](internal)
[![Coverage](https://img.shields.io/badge/coverage-100%25%20agent%20%7C%2083.4%25%20tools-success?style=flat-square)](internal)

</div>

---

## Table of Contents

- [Why zerrcode](#why-zerrcode)
- [Highlights](#highlights)
- [How It Works](#how-it-works)
- [Built-in Tools](#built-in-tools)
- [Requirements](#requirements)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Testing](#testing)
- [Code Quality](#code-quality)
- [Security Model](#security-model)
- [Project Structure](#project-structure)
- [Documentation](#documentation)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

---

## Why zerrcode

Most "AI coding assistant" demos hand the model a shell and hope for the best. `zerrcode`
takes a narrower, more defensible position:

1. **No shell. Ever.** The model can only act through seven explicit, schema-described
   tools. It cannot run `go test`, `npm install`, or `curl`. The system prompt states
   this outright and requires the agent to admit what it did *not* run.
2. **Verified completion, enforced in code.** After a mutating tool succeeds, the run is
   marked *unverified*. The agent loop will not let the model finish until it has actually
   called `git_diff` and seen the result. This is enforced by the loop, not just requested
   in a prompt.
3. **All-or-nothing edits.** `edit_file` takes a unified diff and validates *and* computes
   every resulting file in memory before a single byte is written. One bad hunk means
   nothing changed.
4. **Workspace confinement by construction.** Every path — from any tool — passes through a
   single shared gate that rejects lexical `..` escapes *and* symlink escapes, before any
   filesystem call.

The result is an assistant that is deliberately quieter and more honest than a
shell-wielding agent, and that is safe to point at a real repository.

---

## Highlights

| | |
|---|---|
| **Conversational agent** | A REPL that keeps full conversation context across your session. |
| **Tool calling** | Seven workspace tools, each with its own JSON Schema, discovered dynamically from the registry. |
| **Streaming responses** | Model output is streamed to the terminal token by token. |
| **Workspace sandbox** | Lexical **and** symlink-aware path confinement on every filesystem operation. |
| **Multi-file patches** | `edit_file` applies a git-format unified diff across many files, atomically. |
| **Verified completion** | The agent is forced to read `git diff` before it can claim the work is done. |
| **Honest reporting** | The system prompt separates *Done and observed* from *Not run / not verified*. |
| **Bounded & cancellable** | Hard iteration cap per turn; `context` cancellation propagates through every layer. |
| **Defensive input handling** | Unknown fields, trailing JSON values, control characters, and path escapes are all rejected loudly. |
| **Well tested** | 58 test functions / 177 test cases. 100% statement coverage on the agent loop. |

---

## How It Works

```
┌──────────┐
│   User   │  natural language
└────┬─────┘
     ▼
┌──────────────┐   registers tools    ┌────────────────────┐
│  main.go     │─────────────────────▶│  tools.Registry    │
│  (REPL)      │                      │  7 tools + schemas │
└────┬─────────┘                      └─────────┬──────────┘
     │ history                                 │ dispatch
     ▼                                         ▼
┌──────────────┐   ToolDefinition[]   ┌────────────────────┐
│ agent.Agent  │──────────────────────▶│  Tool.Execute     │
│ bounded loop │                       │  strict JSON args  │
│ verify gate  │◀──────────────────────│  path confinement  │
└────┬─────────┘   results / errors    └─────────┬──────────┘
     │                                         ▼
     ▼                              ┌────────────────────┐
┌──────────────┐                   │  Workspace / git   │
│ client.LLM   │◀─────────────────▶│  grep              │
│ OpenRouter   │   streaming SSE   └────────────────────┘
└──────────────┘
```

The agent runs a bounded loop:

1. Build tool definitions from the registry and send the conversation to the model.
2. If the model requested tools, execute each one and append the result to the history.
   A tool error becomes `Tool call failed: <error>` — it never aborts the run.
3. If the model returned a plain answer, the run may end — **unless** the workspace was
   modified and `git_diff` has not been called since. In that case the agent appends a
   verification reminder and loops (up to `MaxVerificationNudges`, default 2).
4. `MaxIterations` (default 25) caps model round-trips per run.

Full details, including Mermaid diagrams of the request lifecycle and the agent loop, are in
[`docs/architecture.md`](docs/architecture.md).

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

Mutating tools implement the optional `Mutator` interface, which is how the agent knows a
verification is owed. See [`docs/tools.md`](docs/tools.md) for the exact input schema,
validation rules, and failure behaviour of each tool.

---

## Requirements

| Requirement | Notes |
|---|---|
| **Go 1.27+** | See `go.mod` for the pinned toolchain. |
| **OpenRouter API key** | Free-tier models are available; set `OPENROUTER_API_KEY`. |
| **git** | Used at runtime by the `git_diff` tool. |
| **grep** | Used at runtime by the `search_code` tool. |

`git` and `grep` are only needed to *use* the tools; the test suite skips the
corresponding tests automatically when a binary is missing.

---

## Installation

```bash
git clone https://github.com/Aneeshie/zerrcode.git
cd zerrcode

go mod tidy          # install dependencies
go build -o zerrcode .   # build the binary
```

Or run it straight from source without building:

```bash
go run .
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
| `OPENROUTER_API_KEY` | ✅ | Authenticates every request to the OpenRouter API. |

> **Never commit your API key.** `.env` is already listed in `.gitignore`.

The default model is `nvidia/nemotron-3.5-super-vl:free`. It is set in
`internal/client/openrouter-client.go` (`convertToChatRequest`) and can be changed to any
model available on OpenRouter.

---

## Usage

Run the binary from the root of the project you want to work on — the workspace root is
the current working directory, and every tool is confined to it.

```console
$ ./zerrcode
zerrcode — type 'exit' to quit.

zerrcode > list the files in the current directory
```

Then talk to it in plain language:

```console
zerrcode > read main.go and explain what it does

zerrcode > write a new file called hello.go that prints "Hello, World"

zerrcode > rename the variable x to count in main.go and update its callers

zerrcode > delete hello.go

zerrcode > exit
Goodbye!
```

Press `Ctrl+D` to quit as well.

Under the hood, the rename request produces something like this:

```text
[Tool: read_file | Args: "{\"path\":\"main.go\"}"]
[Tool: edit_file | Args: "{\"patch\":\"--- a/main.go\\n+++ b/main.go\\n@@ ...\"}"]
[Tool: git_diff | Args: "{}"]
```

The final `git_diff` call is not optional — the agent loop refuses to let the model
declare success while a change is unverified.

---

## Testing

Testing is a first-class part of this project, not an afterthought. The suite is
**177 test cases across 58 test functions**, all of which pass.

### Coverage

| Package | Statement coverage | Notes |
|---|:---:|---|
| `internal/agent` | **100.0%** | Every branch of the loop, the verification gate, and the error paths. |
| `internal/tools` | **83.4%** | All tools, all path-confinement and argument-validation paths. |
| `internal/client` | 0.0% | Thin SDK adapter; exercised end-to-end via the OpenRouter API. |
| `internal/permissions` | — | Interface contract only (no implementation yet). |
| `main.go` | 0.0% | REPL wiring. |
| **Total** | **67.3%** | Across the whole module. |

### Running the suite

```bash
go test ./...                    # run everything
go test ./... -v                 # verbose, with subtest names
go test ./... -race              # race detector
go test ./... -cover             # per-package coverage
go test ./... -coverprofile=cov.out && go tool cover -html=cov.out   # HTML report
```

### What the tests actually cover

The test suite is written against **real behaviour, not mocks**, wherever the behaviour
under test is a filesystem or git behaviour.

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

**The system prompt is tested as a contract.** `TestSystemPromptStatesKeyRules` asserts
that the prompt still contains each rule that the product depends on (no shell, never
claim unverified work, keep the two lists separate, the `edit_file` retry rule, and the
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

> **This is not a hardened sandbox.** Symlink checks are subject to time-of-check /
> time-of-use races, and concurrent filesystem changes and permissions need careful
> consideration for untrusted workspaces. The complete list of known limitations is in
> [`docs/decisions.md`](docs/decisions.md#known-limitations).

**Recommendation:** run the assistant in a dedicated workspace — ideally a git repository,
so every change can be reviewed and reverted — and review its file-modification behaviour
before pointing it at important data.

---

## Project Structure

```text
zerrcode/
├── main.go                          # CLI entry point: wires the client, registry, and REPL
├── go.mod / go.sum
├── LICENSE                          # MIT
├── README.md
├── docs/
│   ├── architecture.md              # components, agent loop, request lifecycle, security model
│   ├── decisions.md                 # design decisions, trade-offs, limitations, change log
│   └── tools.md                     # per-tool reference: inputs, validation, failures
└── internal/
    ├── agent/
    │   ├── agent.go                 # the bounded agent loop + verification gate
    │   ├── prompt.go                # SystemPrompt (honesty, editing, verification rules)
    │   ├── agent_test.go            # scripted fake client + fake tools
    │   └── prompt_test.go
    ├── client/
    │   ├── client.go                # LLMClient interface, Message / ToolCall types
    │   └── openrouter-client.go     # OpenRouter SDK adapter (streaming + tool calls)
    ├── permissions/
    │   └── persmissions.go          # approval contract (interface only)
    └── tools/
        ├── tool.go                  # Tool and Mutator interfaces
        ├── registry.go              # registry: register / get / execute / IsMutating
        ├── path.go                  # resolveInWorkspace — shared symlink-aware path gate
        ├── read_file.go
        ├── list_files.go
        ├── search_code.go
        ├── git_diff.go
        ├── write_file.go
        ├── edit_file.go             # + validatePath, the stricter model-path gate
        ├── delete_file.go
        └── *_test.go
```

### Design at a glance

| Layer | Responsibility | Knows about |
|---|---|---|
| `main.go` | Chooses a client, registers tools, runs the REPL. | Everything — the only place that wires things together. |
| `internal/agent` | The loop: call the model, run tools, feed results back, enforce limits and verification. | The `LLMClient` interface and the `Registry`. Nothing provider-specific. |
| `internal/client` | Translate between internal types and the OpenRouter SDK. | The SDK, `Message`, `ToolDefinition`. |
| `internal/tools` | The capabilities themselves. | The filesystem, `grep`, `git`, and the workspace root. |

The result is a codebase where adding a tool means implementing one interface and calling
`registry.Register(...)` — the agent, the client, and the prompt all stay untouched.

---

## Documentation

| Document | Contents |
|---|---|
| [`docs/architecture.md`](docs/architecture.md) | Components, the request lifecycle, the agent loop, the system prompt, and the security model. Includes Mermaid diagrams. |
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
- [x] Comprehensive test suite
- [ ] Wire up the `permissions.Manager` approval contract for interactive confirmation of destructive operations
- [ ] Additional LLM providers behind the `LLMClient` interface
- [ ] TUI frontend
- [ ] Token / cost accounting per session

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
