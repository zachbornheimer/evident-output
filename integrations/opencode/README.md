# OpenCode integration — Evident Output

**Pin:** `v0.4.6` (never `@latest` for persistent install).

## Paths

| What       | Path                                                              |
| ---------- | ----------------------------------------------------------------- |
| GitHub     | `https://github.com/zachbornheimer/evident-output`                |
| MCP module | `github.com/zachbornheimer/evident-output/cmd/evident-output-mcp` |
| Binary     | `$HOME/.local/bin/evident-output-mcp`                             |

## Install + register

```bash
mkdir -p "$HOME/.local/bin"
go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@v0.4.6
ln -sfn "$(go env GOPATH)/bin/evident-output-mcp" "$HOME/.local/bin/evident-output-mcp"
# After bumping evo: evident-output-mcp update --directory <repo> then restart the host.

"$HOME/.local/bin/evident-output-mcp" config --client opencode
```

Paste the printed snippet. Use an absolute command path.

## Tools

`evident_output_list_sections`, `evident_output_get_documentation`, `evident_output_adopt_plan`, `evident_output_review`, `evident_output_preview`, `evident_output_explain`, `evident_output_update` (underscores).

Skill: [`../../skills/cli-output/SKILL.md`](../../skills/cli-output/SKILL.md)
