# opencode integration — Evident Output

## Paths

| What | Path |
|------|------|
| GitHub | `https://github.com/zachbornheimer/evident-output` |
| MCP module | `github.com/zachbornheimer/evident-output/cmd/evident-output-mcp` |
| Binary | `$HOME/.local/bin/evident-output-mcp` |
| Local clone | `$HOME/Developer/Personal/evident-output` |

## Install + register

```bash
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/evident-output-mcp" \
  "$HOME/Developer/Personal/evident-output/cmd/evident-output-mcp"
# or: GOBIN="$HOME/.local/bin" go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest

"$HOME/.local/bin/evident-output-mcp" config --client opencode
```

Paste the printed snippet. Prefer `/Users/zbornheimer/.local/bin/evident-output-mcp` (or an absolute path).

## Tools

`evident_output_list_guides`, `evident_output_get_guidance`, `evident_output_review`, `evident_output_preview`, `evident_output_explain` (underscores).

Skill: [`../../skills/cli-output/SKILL.md`](../../skills/cli-output/SKILL.md)
