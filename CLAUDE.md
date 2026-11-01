# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`zerrcode` (module `github.com/Aneeshie/zerrcode`, Go 1.27+) is a human-in-the-loop AI coding agent for the terminal. It drives an LLM via OpenRouter and lets it act only through seven schema-described workspace tools — there is deliberately no shell tool.

## Commands

```bash
go build -o zerrcode .            # build
go run . -tui                     # run TUI mode (interactive approval)
go run .                          # run REPL mode (auto-approves via permissions.AllowAll)
go test ./...                     # all tests
go test ./... -race -cover        # race detector + coverage
go test ./internal/tools -run TestEditFile -v   # single test / package
go vet ./...
```

Running requires `OPENROUTER_API_KEY` (in `.env` or the environment; the process env wins). The workspace root is the current working directory. `git` and `grep` must be on PATH (`git_diff`, `search_code`). The model is a compile-time constant in `convertToChatRequest` in `internal/client/openrouter_client.go`.

## Architecture

- **`internal/agent`** — bounded agent loop (default 25 round-trips, `ErrMaxIterations`), context cancellation throughout. It emits events (assistant text, tool start/result, verification nudge) instead of printing; UIs subscribe. **Verification gate:** after any successful mutating tool call the run is marked unverified and the loop won't let the model finish until it calls `git_diff` (up to `MaxVerificationNudges`, default 2). A user-rejected change is reported to the model with a distinct status and does not count as a workspace change. `prompt.go` holds `SystemPrompt`.
- **`internal/client`** — `LLMClient` interface + `Message`/`ToolCall` types (`client.go`); `openrouter_client.go` is a thin OpenRouter SDK adapter (streaming, tool calling). No unit tests; agent tests use a scripted fake client.
- **`internal/tools`** — `Tool` interface (`Name`, `Description`, `InputSchema`, `Execute`); schemas live beside the arg-parsing code. Optional `Mutator` marks workspace-changing tools. Mutating tools (`write_file`, `edit_file`, `delete_file`) are **planners**: `Plan` validates and computes the full result + diff in memory, then `Registry.Execute` asks the approver, and `Apply` writes only after re-checking file snapshots (optimistic concurrency — refuses if the file changed during review). With no approver set, the registry **fails closed**.
- **Path confinement** — every path from every tool must go through `resolveInWorkspace` in `tools/path.go` (rejects `..`, absolute paths, and symlink escapes). Argument parsing is strict: unknown JSON fields, trailing values, control chars rejected.
- **`internal/permissions`** — `Manager` interface (`RequestApproval`), `Request`/`FileChange`, plus `AllowAll` / `DenyAll`. (File is named `persmissions.go`.)
- **`internal/diff`** — computes approval diffs from actual on-disk content vs. planned content (never from the model's description).
- **`internal/tui`** — Bubble Tea v2 / Lipgloss v2 UI. `TUIApprover` implements `permissions.Manager`: it sends an approval prompt message to the `tea.Program` and blocks the tool goroutine until the user answers `y`/`n` or the turn is cancelled. Tests drive the model as a state machine.
- **`main.go`** — `-tui` flag; wires client, registry, approver, and agent for either mode.

## Testing conventions

Tool tests use real temp directories and real git repos (not mocks) since symlink/permission behavior is what's being verified; they assert rejected calls leave the workspace (and files outside it) untouched. Agent tests use fake clients and fake tools.

## Docs

`docs/architecture.md` (components, loop, approval flow), `docs/tools.md` (per-tool schemas, validation, failure behavior), `docs/decisions.md` (design decisions, known limitations, change log) — update these when changing tool behavior or the loop.

Known gaps worth remembering: `.git` protection only applies to `edit_file` and `git_diff`; `edit_file`'s write phase is not transactional; `write_file` is not atomic.
