# Codex integration — Evident Output

**Pin:** `v0.2.16` (never `@latest` for persistent install).

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
  github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@v0.2.16

"$HOME/.local/bin/evident-output-mcp" config --client codex
```

Paste the printed snippet. Use an absolute command path.

## Tools

`evident_output_list_guides`, `evident_output_get_guidance`, `evident_output_review`, `evident_output_preview`, `evident_output_explain` (underscores).

Skill: [`../../skills/cli-output/SKILL.md`](../../skills/cli-output/SKILL.md)
