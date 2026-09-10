# Agents — Evident Output MCP

This is the Svelte-parity contract. Treat `evident-output-mcp` the way Svelte
treats `@sveltejs/mcp`: official, mandatory, always-current, loop-until-clean.

zq is the canary consumer. If MCP cannot take zq from dirty dialect to
`recheck_required=false` without a human inventing API, MCP is not on par.

## When you MUST use it

Whenever you write or change CLI output / presentation in a repo that uses
`github.com/zachbornheimer/evident-output` (or is adopting it). Do not guess
spellings from memory. Do not treat a compile as “the output is done.”

Call the tools. A passing `go test` is not a review.

## Svelte analog (must match this loop)

| Svelte                                                                                             | Evident Output                                                                                          |
| -------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| “This is the official Svelte MCP server. It MUST be used whenever svelte development is involved.” | Same sentence, swap in CLI output / this module.                                                        |
| `list-sections` → `get-documentation`                                                              | `evident_output_list_sections` → `evident_output_get_documentation`                                     |
| `svelte-autofixer` on the component, then again                                                    | `evident_output_review` on the Go / directory, then again                                               |
| Stop when autofixer reports no issues                                                              | Stop when `findings` is empty **and** `recheck_required=false`                                          |
| `npx -y @sveltejs/mcp` (always latest at spawn)                                                    | Spawn-time auto-update from the cwd `go.mod` pin (or path replace). Skip with `EVO_MCP_NO_AUTO_UPDATE`. |

Svelte does not let the model “remember Svelte 4.” We do not let the model
remember `DisplayGroup` / `Task.Each` / quantity-first `Delete`.

## Tools (underscores)

On Grok, names are `evident-output__evident_output_*`.

| Tool                                                                | Role                                                                                                                   |
| ------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `evident_output_list_sections` / `evident_output_get_documentation` | Docs corpus. Call these instead of opening local markdown.                                                             |
| `evident_output_adopt_plan`                                         | Inventory non-evo `fmt.Print*` / `log.*` / spinner libs. Page with `cursor` until `next_action=clean`.                 |
| `evident_output_review`                                             | Autofixer. Apply **every** suggestion. Call again on the same source. Repeat until clean.                              |
| `evident_output_update`                                             | Reinstall the server to match a go.mod pin (`directory`) XOR a release tag (`version`). Then **restart the MCP host**. |
| `evident_output_explain`                                            | One rule. Argument is `rule_id`, not `id`.                                                                             |
| `evident_output_preview`                                            | Plain-profile preview.                                                                                                 |

Review kinds: `go` (default, `source` or `file`), `directory` (`directory` =
absolute local path), `package` (`files` map), `transcript`, `json`.

`explain` takes `{ "rule_id": "API-032" }`. `id` is wrong.

`desired_version`: omit for current rec. A go.mod **path replace** to this
module lints as current rec even if the require says `v0.4.3`. An explicit old
pin (`desired_version=v0.2.9`) must not fire rec-only rules.

## MUST-loop (non-negotiable)

```
review → apply every suggestion → review the same source
```

Stop only when `recheck_required=false` and `findings` is empty (or
`next_action` is `clean`). One green compile is not the loop.

When review reports `update_needed`, call `evident_output_update` then restart
the host before reviewing again. Do not keep applying an old autofixer.

## Stale host (this session’s failure mode)

The TUI attaches MCP **once**, at session start. `go install` + `ln -sfn` does
not refresh an already-attached server.

Stale if any of these are true:

- `evident_output_review` schema has no `directory`
- `evident_output_update` is missing from `tools/list`
- `list_sections` returns ~11 sections (current rec is more)
- review fires **API-032 on `Create(object, fn)` / `Delete(object, fn)`**

That last one is inverted rec. Applying those suggestions **reverts** the
dialect. Do not apply. Reinstall, then start a **fresh** Grok process.

```bash
# never GOBIN=$HOME/.local/bin — that self-symlinks and deletes the binary
cd /path/to/evident-output          # this checkout, or the consumer's replace
GOBIN="$(go env GOPATH)/bin" go install ./cmd/evident-output-mcp
ln -sfn "$(go env GOPATH)/bin/evident-output-mcp" "$HOME/.local/bin/evident-output-mcp"
"$HOME/.local/bin/evident-output-mcp" --version
```

Then **restart the MCP host**. A long-lived Grok TUI will keep the old schema
until it restarts. For a canary without killing the TUI, spawn a fresh process
in tmux (below).

`grok mcp doctor evident-output --json` → healthy, 7 tools, protocol
`2025-06-18`. `tool_count:0` means dotted names; use underscores.

## Live TTY — tmux (required for canaries)

Grok’s in-session shell is not a TTY. Live evo (spinners, `Each` aggregates,
in-place progress) will dump a line per tick there. That is **not** how a user
sees zq.

Run live CLI and fresh-MCP canaries in tmux:

```bash
tmux new-session -d -s zq-evo-canary -n mcp \
  -c "$HOME/Developer/Personal/evident-output"
tmux new-window -t zq-evo-canary -n extract \
  -c "$HOME/Developer/Personal/evident-output"
tmux new-window -t zq-evo-canary -n flight \
  -c "$HOME/Developer/Zysys/flight"
# attach: tmux attach -t zq-evo-canary
```

| Window    | What                                                                                                                      |
| --------- | ------------------------------------------------------------------------------------------------------------------------- |
| `mcp`     | Reinstall binary, `grok mcp doctor`, then a **fresh** `grok -p` that calls `evident_output_review` with `kind=directory`. |
| `extract` | `mise run evo-usage-audit -- <repo> --output FILE`                                                                        |
| `flight`  | `time zq clean-repo --dry-run` on a 100+ worktree repo. Real TTY.                                                         |

Do not start a second `zq clean-repo` on the same repo while one is still
classifying — it serializes on the same git dirs.

## Inventory another repo’s call sites

```bash
mise run evo-usage-audit -- <repo-path> --output FILE
# example:
mise run evo-usage-audit -- ~/Developer/Personal/zq --output ~/Desktop/zq-evo.md
```

Flags **after** the task name. `mise run --output FILE evo-usage-audit` is
swallowed by mise’s own `-o`.

## Dialect the MCP enforces (current rec)

```go
evo.Init(evo.Config{Title: "tool", DryRun: dry})
evo.Main(run)

evo.Task("check config").Define(checkConfig)

for path, task := range evo.Group("worktrees").Each(paths) {
    task.Delete("worktree", func() error { return remove(path) })
}

evo.Task("fetch").After(worktrees, branches).Define(fetchPrune)
```

- **Task** is atomic. No `Task.Each`, no `Task.Run` (use `task.Writer()` on
  `cmd.Stdout`/`Stderr`), no `DisplayGroup` (it is `Group`).
- **Group** = independent children (scheduler may overlap). **Sequence** =
  declaration order, one Running child.
- **Define** / mutation verbs (`Delete(object, fn)`, optional `Affected(n)`)
  submit work. They do not mean “run this callback synchronously now.”
- **Done** is only for already-resolved work with no callback.
- Dry-run skips **mutation** callbacks only. `Define` still runs.
- Callers do not `errgroup` / `go func` to make evo rows parallel. Predeclare
  with `Group.Each` or `Group.Task` + `Define` and let evo’s scheduler run them.
  A domain graph engine (zq mise/gate) may still own _eligibility_; wrap the
  executor body in `Define` and wait only when you need the result on this
  stack (`defineAndWait` is that adapter — not a second scheduler).

## zq canary (MCP must survive this)

1. Consumer `go.mod` has `replace github.com/zachbornheimer/evident-output => ../evident-output`.
2. Fresh Grok (tmux `mcp` window) calls `evident_output_review` with
   `kind=directory`, `directory=<abs zq>`. **Not** this long-lived TUI if its
   schema is stale.
3. Apply every remaining finding. Re-review until `recheck_required=false`.
4. Extract `~/Desktop/zq-evo.md` via the mise task. A human (or a reviewer
   subagent) reads that file for clarity — MCP findings are not a clarity review.
5. Live: `tmux` window `flight`, `time zq clean-repo --dry-run` in a real TTY.

CLI fallback when the attached host is stale (do **not** skip MCP on a fresh
process):

```bash
go run ./cmd/evident-output review /Users/zbornheimer/Developer/Personal/zq
```

`desired_version` in that result may still print the require (`v0.4.3`); detectors
use empty (current rec) because of the path replace.

## What review does not catch yet (do not treat silence as correct)

These are real dialect defects MCP currently misses. Fix them in the consumer
anyway; add detectors when they recur:

- `errgroup` / `go func` driving **predeclared** evo Tasks (rec is `Group.Each` + `Define`)
- `for _, task := range x.Each(` (discards the item name)
- empty mutation callback / work-then-`Create` theater
- `Doing` + I/O + `Done` instead of `Define` / a mutation verb
- `Doing().Done()` on the same line for work that just happened off-row

`defineAndWait` in zq is the documented adapter, not a defect.

## Install / pin

- Library pin in consumer `go.mod`; MCP binary matches that pin (or the replace).
- `ln -sfn "$(go env GOPATH)/bin/evident-output-mcp" ~/.local/bin/evident-output-mcp`
- Host config: **absolute** command (`${HOME}/.local/bin/evident-output-mcp`).
- Integrations: `integrations/grok/README.md` (and claude-code, codex, gemini, opencode).

Authoritative prose is the MCP docs corpus, not this file. This file is the
operating contract for agents.
