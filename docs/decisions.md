# Architecture decisions and change history

This records the significant design decisions in `zerrcode`, why they were made, and what they cost. Each entry follows the same shape: **Context**, **Decision**, **Why**, **Trade-offs**. A chronological change log is at the end.

For how the system fits together see [`architecture.md`](architecture.md); for how each tool works see [`tools.md`](tools.md).

## Index

1. [Tools describe themselves](#1-tools-describe-themselves)
2. [The client is decoupled from tool definitions](#2-the-client-is-decoupled-from-tool-definitions)
3. [One shared workspace path gate](#3-one-shared-workspace-path-gate)
4. [A stricter `validatePath` layer for model-supplied paths](#4-a-stricter-validatepath-layer-for-model-supplied-paths)
5. [`edit_file` takes only a patch, and the patch names the files](#5-edit_file-takes-only-a-patch-and-the-patch-names-the-files)
6. [`edit_file` failures must not fall back to rewriting the file](#6-edit_file-failures-must-not-fall-back-to-rewriting-the-file)
7. [The agent loop is bounded and cancellable](#7-the-agent-loop-is-bounded-and-cancellable)
8. [Tool errors go back to the model](#8-tool-errors-go-back-to-the-model)
9. [Verification before reporting completion](#9-verification-before-reporting-completion)
10. [A system prompt that separates what was run from what was not](#10-a-system-prompt-that-separates-what-was-run-from-what-was-not)
11. [Strict, uniform argument parsing](#11-strict-uniform-argument-parsing)
12. [`search_code` shells out to `grep`](#12-search_code-shells-out-to-grep)
13. [Test strategy](#13-test-strategy)
14. [Known limitations](#known-limitations)
15. [Change log](#change-log)

---

## 1. Tools describe themselves

**Context.** Tool definitions (name, description, JSON schema) originally lived in the OpenRouter client, away from the code that parses the arguments.

**Decision.** The `Tool` interface carries `Name()`, `Description()`, `InputSchema()` and `Execute()`. Adding a tool means implementing the interface and calling `registry.Register(...)`.

**Why.** The schema sits next to the parsing code, so the two cannot drift apart. The agent and client no longer need editing when a tool is added.

**Trade-offs.** Tool files are slightly longer because each carries its schema.

## 2. The client is decoupled from tool definitions

**Context.** `convertToChatRequest` hardcoded `readFileDefinition()` and friends.

**Decision.** The agent builds `[]client.ToolDefinition` from the registry and passes it to `LLMClient.Stream`. The OpenRouter client converts them to SDK types in one function, `convertToolDefinitions`. The four hardcoded definition functions were deleted.

**Why.** The client becomes a pure adapter to the SDK. Because the agent depends on the `LLMClient` interface, it can be tested with a scripted fake and the SDK can be swapped.

**Trade-offs.** `Send` (the non-streaming path) passes no tools; it is not used by the agent loop.

## 3. One shared workspace path gate

**Context.** `read_file` and `list_files` each had a private lexical `..` check. They did not follow symlinks, so a symlink inside the workspace pointing outside it allowed reading, listing, writing or deleting files outside.

**Decision.** All tools resolve paths through `resolveInWorkspace` (`internal/tools/path.go`). It checks lexically *and* after resolving symlinks on the longest existing prefix of the path, comparing against the real workspace root.

**Why.**
- One fix covers every tool, and one test (`TestToolsRejectSymlinkEscape`) covers all of them.
- Resolving only the existing prefix keeps "create a new file" working, since the target does not exist yet.
- Comparing against the *real* root handles workspaces that themselves live behind symlinks (for example macOS `/var` → `/private/var`).

**Trade-offs.** There is an unavoidable time-of-check/time-of-use window: a symlink could be swapped after the check. This is not a hardened sandbox (see [limitations](#known-limitations)).

## 4. A stricter `validatePath` layer for model-supplied paths

**Context.** `resolveInWorkspace` silently *contains* some bad input: the absolute path `/etc/passwd` is joined under the root and accepted as `<root>/etc/passwd`.

**Decision.** `edit_file` (every path in a patch) and `git_diff` (its `path`) additionally call `validatePath`, which rejects: empty paths, paths over 4096 bytes, control characters (including NUL), absolute paths, any `..` segment (even harmless ones), and any `.git` component. `/dev/null` is allowed as the placeholder for the missing side of a created/deleted file.

**Why.** These paths come from model-generated text. A wrong path is almost always a model mistake, and an explicit error teaches the model; a silent fix-up hides the mistake. Blocking `.git` stops a patch from touching hooks or config.

**Trade-offs.** It is stricter than strictly necessary (`a/../b` is harmless but rejected). `.git` protection currently applies only to these two tools, not to `write_file`/`delete_file`.

## 5. `edit_file` takes only a patch, and the patch names the files

**Context.** The first version took `path` and `patch`. A unified diff can contain several files, so `path` was both redundant and wrong: two sources of truth that could disagree, and no way to express a multi-file change. An early attempt to patch this by checking that the patch matched `path` only papered over the design problem.

**Decision.** Remove `path`. The patch is the only input and identifies every target file. Each file is validated **independently**. Execution is **two-phase**: phase 1 validates everything and computes every result in memory; phase 2 writes only if all of phase 1 succeeded.

Per-file rules: binary and copy patches rejected; only the side that exists is validated; a path may appear once; creates and renames cannot overwrite; a modify with differing old/new paths must be a rename; at most 50 files.

**Why.**
- One bad file must not be excused by good ones, so each is checked on its own.
- Writing as you go leaves a half-applied edit when file 4 of 5 has a bad hunk, which a model cannot easily recover from. Computing everything first makes the patch effectively all-or-nothing.
- A duplicate path would be applied against content that no longer matches what was validated.

**What the implementation learned about `gitdiff` (v0.9.0).** Inside `diff --git` blocks it has already stripped `a/` and `b/`; for plain `---`/`+++` diffs it keeps the prefix and reports the *same* new name on both sides of a modification. `stripDiffPrefix` strips one prefix only for plain diffs. A first version that stripped per side and stripped both `a/` and `b/` mangled real paths like `a/b/x.go`.

**Trade-offs.** Phase 2 is not transactional: a failure part-way (for example permissions) leaves earlier files written and reports them. Format detection is per patch, so mixed plain/`diff --git` patches are treated as git format throughout.

## 6. `edit_file` failures must not fall back to rewriting the file

**Context.** When a hunk does not apply, the file usually differs from what the model remembers. The tempting shortcut is `write_file` with the whole file, which silently discards anything the model did not know about.

**Decision.** The failure error states that no files were modified, forbids falling back to `write_file`/rewriting the whole file, and instructs the model to re-read the file with `read_file` and generate a new minimal patch from its current content. The tool description and system prompt carry the same rule.

**Why.** Re-reading and re-patching keeps edits small, reviewable, and based on the file's real current content.

**Trade-offs.** It is guidance, not enforcement: the model can still call `write_file`. A hard block was not added because `write_file` is legitimate for new files and for explicit "replace this file" requests.

## 7. The agent loop is bounded and cancellable

**Context.** The loop ran until the model stopped asking for tools. A model stuck in a tool-call loop would run forever and keep spending tokens.

**Decision.** `Agent.MaxIterations` (default 25) caps model round-trips per `Run`; exceeding it returns `ErrMaxIterations`. `ctx` is checked before each model call.

**Why.** Bounded cost and a guaranteed exit; cancellation actually stops work.

**Trade-offs.** A legitimately long task can hit the cap; it is configurable (`<= 0` disables it).

## 8. Tool errors go back to the model

**Decision.** A tool error does not abort the run. The agent appends `Tool call failed: <error>` as the tool result and continues. Errors from the LLM client *do* abort the run.

**Why.** The model can correct bad arguments, a missing file, or a failed patch. A client error leaves nothing to recover with.

## 9. Verification before reporting completion

**Context.** Models tend to report "done" without checking what actually changed on disk.

**Decision.**
- A read-only `git_diff` tool reports the changed-file list and the diff against `HEAD`.
- Tools that change the workspace implement the optional `Mutator` interface; `Registry.IsMutating(name)` exposes it.
- The agent tracks an `unverified` flag: set after any **successful** mutating call, cleared by a successful `git_diff`. If the model tries to finish (a response with no tool calls) while `unverified`, the agent appends a reminder user message and loops, up to `MaxVerificationNudges` times (default 2; `<= 0` disables).

**Why.** The prompt can only ask the model to verify; the agent loop makes it *actually call* the tool. Only successful changes count (a failed edit changed nothing), and a later change makes an earlier verification stale.

**Trade-offs.** The agent can force the call but cannot force the model to judge the diff correctly. After the nudge limit the run returns anyway rather than looping forever. The reminder costs an extra model round-trip. `git_diff` requires a git repository.

## 10. A system prompt that separates what was run from what was not

**Context.** The agent has no shell, yet language models readily write "I ran the tests and they pass".

**Decision.** `agent.SystemPrompt` states that the agent can act only through its tools and has no shell; forbids claiming to have run/tested/built/verified anything without a matching tool result; requires giving the user the exact command for anything it cannot run; and requires the final report to keep two lists apart: **Done and observed** and **Not run / not verified**. It also states the `edit_file` retry rule and the verify-before-completion rule.

**Why.** Users act on the report. Mixing "I reasoned that it works" with "I observed it working" is the failure being prevented.

**Trade-offs.** A prompt guides but does not enforce; it is covered by a test that checks the key rules are present, not that the model obeys them.

## 11. Strict, uniform argument parsing

**Decision.** Every tool decodes with `DisallowUnknownFields` and rejects trailing JSON values. The trailing check decodes into `json.RawMessage`.

**Why.** Malformed calls become clear errors the model can fix instead of being silently half-honoured. (An earlier version decoded the trailing value into `struct{}`, which failed with a misleading `unknown field` error before the intended `multiple json values` message could be reached.)

**Trade-offs.** Stale calls fail loudly: for example, sending the removed `path` field to `edit_file` is an error.

## 12. `search_code` shells out to `grep`

**Decision.** The tool runs `grep -rnIF --exclude-dir=.git … -e <query> -- <path>` from the workspace root.

**Why.** It is fast, simple, and `-F`/`-e`/`--` make the query literal and flag-safe. Exit code 1 (no match) is reported as `No matches found`, not an error. Output is capped at 32 KB.

**What was wrong before.** The original passed `"grep"` twice (so `grep` became the pattern), treated "no matches" as an error, silently accepted empty queries, and put `--include` after the path.

**Trade-offs.** It depends on `grep` being installed (tests skip otherwise) and is literal-only. A pure-Go implementation would remove the dependency at the cost of more code.

## 13. Test strategy

- Every tool has tests for **path traversal, missing files, empty/blank input, bad JSON and unknown fields**: the failure modes that matter when code runs on model output.
- Rejections are asserted to leave the workspace **untouched**, including files outside it.
- Tools are tested against **real temporary directories** (and real `git` repositories for `git_diff`) rather than mocks, because symlinks and permissions are exactly what is being checked.
- The agent is tested with a **scripted fake client and fake tools**: normal loops, multiple calls per turn, error forwarding, unknown tools, the iteration cap, cancellation, and the verification nudge (including that it does not fire for read-only runs or failed changes, fires again after a later change, and gives up after the limit).
- Tests that need `grep` or `git` skip themselves when the binary is missing.

## Known limitations

- **Not a hardened sandbox.** Symlink checks are time-of-check/time-of-use; concurrent filesystem changes and permissions need care for untrusted workspaces.
- **`edit_file` phase 2 is not transactional**; a mid-write failure leaves earlier files written.
- **`edit_file` format detection is per patch**; mixed plain/`diff --git` patches are read as git format; in plain diffs with no prefixes a real top-level `a`/`b` directory is mis-stripped; binary patches are unsupported.
- **`write_file` is not atomic** and `read_file` has no size limit.
- **`.git` protection** applies to `edit_file` and `git_diff` only.
- **`git_diff`** does not show the contents of untracked files (they are listed only).
- **`search_code`** needs `grep` and is literal-only.
- **Prompt rules are guidance**; only the iteration cap and the verification nudge are enforced in code.

---

## Change log

Grouped by pull request / branch, oldest first.

### Initial work (`main`)

- `list_files` implemented: sorted directory listing, `/` suffix for directories, empty path means workspace root.
- `write_file` added: create or overwrite, creates parent directories, preserves permissions.
- `delete_file` added: single file only, refuses directories.
- `resolveInWorkspace` added as the shared path gate.
- Tool definitions moved out of the OpenRouter client into the tools (`InputSchema`); the client converts the definitions it is given.
- `search_code` fixed (duplicated `grep` argument, "no matches" treated as error, empty queries, flag ordering, stderr mixing, output cap).

### PR #2 — `test/tools-and-agent-coverage` (merged)

1. `test(tools): cover workspace path traversal checks`
2. `test(tools): cover read_file and list_files edge cases`
3. `test(tools): cover write_file and delete_file`
4. `test(tools): cover search_code`
5. `fix(agent): bound tool-call loop and honor context cancellation` — adds `MaxIterations`, `ErrMaxIterations`, context check.
6. `test(agent): cover tool-call loop behavior`
7. `fix(tools): block symlink escapes from the workspace` — symlink-aware `resolveInWorkspace`; `read_file` and `list_files` switched to the shared helper.
8. `fix(tools): report multiple JSON values accurately`

### `feat/edit-file-tool`

1. `feat(tools): add multi-file edit_file tool` — patch-only input, per-file validation, two-phase apply; `go-gitdiff` added as a direct dependency.
2. `test(tools): cover edit_file and validatePath`
3. `feat(tools): add read-only git_diff tool`
4. `feat(tools): make edit_file failures steer towards a fresh targeted patch`
5. `feat(agent): system prompt that separates commands run from commands not run` — `agent.SystemPrompt`; `main.go` uses it and registers `edit_file` and `git_diff`.
6. `feat(agent): require git diff verification before reporting completion` — `Mutator`, `Registry.IsMutating`, `MaxVerificationNudges`.
7. Documentation: this file, `tools.md`, and updates to `architecture.md` and the README.
