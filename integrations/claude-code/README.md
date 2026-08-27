# Claude Code integration — Evident Output

**Pin:** `v0.2.14` (never `@latest` for persistent install).

## Paths

| What       | Path                                                              |
| ---------- | ----------------------------------------------------------------- |
| GitHub     | `https://github.com/zachbornheimer/evident-output`                |
| MCP module | `github.com/zachbornheimer/evident-output/cmd/evident-output-mcp` |
| Binary     | `$HOME/.local/bin/evident-output-mcp`                             |

## Install + register

```bash
mkdir -p "$HOME/.local/bin"
GOBIN="$HOME/.local/bin" go install \
  github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@v0.2.14

"$HOME/.local/bin/evident-output-mcp" config --client claude-code
```

Paste the printed snippet. Prefer an **absolute** path such as
`$HOME/.local/bin/evident-output-mcp` (expand for hosts that do not expand `$HOME`).

## Tools

`evident_output_list_guides`, `evident_output_get_guidance`, `evident_output_review`, `evident_output_preview`, `evident_output_explain` (underscores).

Skill: [`../../skills/cli-output/SKILL.md`](../../skills/cli-output/SKILL.md)
