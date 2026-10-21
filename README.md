# zerrcode

`zerrcode` is an interactive, terminal-based AI coding assistant written in Go. It uses an LLM (via [OpenRouter](https://openrouter.ai)) to help you read, search, edit, write, and delete files in your workspace through natural conversation — with a full-screen TUI that lets you review and approve every change before it touches disk.

## Features

### Core Agent
- **Conversational coding agent** — chat with an LLM that can reason about your codebase
- **Tool use** — the agent calls file-system tools to inspect and modify files on your behalf
- **Streaming responses** — LLM output is streamed to the terminal in real time
- **Conversation history** — the agent maintains context across your session
- **Bounded agent loop** — capped at 25 model round-trips per turn (configurable), with context cancellation support
- **Verified completion** — after changing files the agent must inspect `git diff` before reporting it is done
- **Honest reporting** — the system prompt separates commands the agent actually ran from ones it could not run

### Interactive TUI (`-tui` mode)
- **Full-screen Bubble Tea interface** — header bar, scrollable chat viewport, input composer, and contextual footer shortcuts
- **Interactive change approval** — every file-modifying tool call shows a real before/after diff for you to approve (y) or reject (n) before anything is written
- **Multi-file diff review** — cycle through affected files with Tab, toggle diff view with d
- **Real-time streaming** — assistant responses stream token-by-token into the chat
- **Turn cancellation** — Ctrl+C or Esc cancels the current agent turn mid-flight
- **Slash commands** — `/clear`, `/exit`, `/quit`, `/help`
- **Auto-scroll** — follows new content; manual scroll (↑/↓/PgUp/PgDn) pauses auto-scroll

### Security & Safety
- **Path sandboxing** — all file operations are restricted to the workspace directory, including through symlinks
- **Optimistic concurrency guards** — if a file changes while you're reviewing an approval, the write is refused rather than silently overwriting your edits
- **Approval-gated writes** — in TUI mode, no file is created, modified, or deleted without explicit user approval
- **Strict input validation** — unknown JSON fields, trailing values, control characters, absolute paths, `..` segments, and `.git` components are all rejected

### Tools
- **Multi-file edits** — `edit_file` applies a unified diff that can change several files, all-or-nothing
- **Structured diffs** — real before/after diffs computed from actual file content, not from model arguments

## Built-in Tools

| Tool | Changes workspace | Description |
|------|:-:|-------------|
| `read_file` | | Read the contents of a file within the workspace |
| `list_files` | | List files and directories in a workspace directory |
| `search_code` | | Literal-text search across files (uses `grep`) |
| `git_diff` | | Read-only: changed files and the diff against `HEAD` |
| `write_file` | ✓ | Create or overwrite a file with given content |
| `edit_file` | ✓ | Apply a multi-file unified diff, all-or-nothing |
| `delete_file` | ✓ | Delete a file within the workspace |

See [`docs/tools.md`](docs/tools.md) for how each tool works.

## Requirements

- Go 1.27+
- An [OpenRouter](https://openrouter.ai) API key
- `git` and `grep` on your `PATH` (used by `git_diff` and `search_code`)
- A terminal with true-color support (for the TUI)

## Installation

```bash
git clone https://github.com/Aneeshie/zerrcode.git
cd zerrcode
go mod tidy
go build -o zerrcode .
```

## Usage

1. Create a `.env` file in the project root:

```env
OPENROUTER_API_KEY=your_openrouter_api_key_here
```

2. Run in **TUI mode** (recommended):

```bash
./zerrcode -tui
```

Or in **classic REPL mode** (auto-approves all changes):

```bash
./zerrcode
```

3. Interact:

```
# In REPL mode:
zerrcode > list the files in the current directory
zerrcode > read main.go and explain what it does
zerrcode > rename the variable x to count in main.go
zerrcode > exit

# In TUI mode:
# Just type and press Enter. Use /help for commands.
# When the agent proposes a change, review the diff and press y/n.
```

## Project Structure

```
zerrcode/
├── main.go                     # CLI entry point: REPL and TUI wiring
├── go.mod / go.sum
├── .env                        # Local environment variables
├── .gitignore
├── LICENSE
├── README.md
├── docs/
│   ├── architecture.md         # Components, loop, security model
│   ├── decisions.md            # Design decisions, trade-offs, change log
│   └── tools.md                # How each tool works
└── internal/
    ├── agent/
    │   ├── agent.go            # Agent loop, events, verification
    │   ├── prompt.go           # System prompt
    │   └── *_test.go
    ├── client/
    │   ├── client.go           # LLMClient interface, Message types
    │   └── openrouter-client.go
    ├── diff/
    │   ├── diff.go             # Structured diff computation
    │   └── diff_test.go
    ├── permissions/
    │   └── persmissions.go     # Approval contract (Manager, FileChange)
    ├── tools/
    │   ├── tool.go             # Tool and Mutator interfaces
    │   ├── plan.go             # Plan/Planner pattern
    │   ├── registry.go         # Tool registry with approval gate
    │   ├── path.go             # Workspace path confinement
    │   ├── read_file.go
    │   ├── list_files.go
    │   ├── search_code.go
    │   ├── git_diff.go
    │   ├── write_file.go       # Planner: approval-gated writes
    │   ├── edit_file.go        # Planner: multi-file patches
    │   ├── delete_file.go      # Planner: approval-gated deletes
    │   └── *_test.go
    └── tui/
        ├── model.go            # Bubble Tea Model (chat UI)
        ├── theme.go            # ANSI theme, diff rendering
        ├── approver.go         # TUI ↔ permissions bridge
        └── model_test.go
```

## Documentation

- [`docs/architecture.md`](docs/architecture.md) — components, request lifecycle, agent loop, security model
- [`docs/tools.md`](docs/tools.md) — how each tool works: inputs, validation, output, failure behaviour
- [`docs/decisions.md`](docs/decisions.md) — design decisions with reasoning, trade-offs, and change log

## Configuration

The default model is `minimax/minimax-m3.1-flash:free`. You can change it in `internal/client/openrouter-client.go`.

## License

MIT License. See [LICENSE](LICENSE) for details.
