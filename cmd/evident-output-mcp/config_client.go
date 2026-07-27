package main

import (
	"fmt"
	"os"
	"strings"
)

// config --client prints host-specific MCP wiring (print-only; §28.11).
// It never rewrites user files.

func runConfig(args []string) int {
	if len(args) == 0 || args[0] != "config" {
		return -1 // not handled
	}
	client := ""
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--client" && i+1 < len(args) {
			client = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(a, "--client=") {
			client = strings.TrimPrefix(a, "--client=")
			continue
		}
		if a == "-h" || a == "--help" {
			printConfigHelp()
			return 0
		}
	}
	if client == "" {
		fmt.Fprintln(os.Stderr, "usage: evident-output-mcp config --client <claude-code|codex|gemini|grok|opencode>")
		return 2
	}
	body, err := clientConfig(client)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	// Configuration snippets are user-facing paste targets on stdout.
	fmt.Print(body)
	return 0
}

func printConfigHelp() {
	fmt.Fprintf(os.Stderr, `evident-output-mcp config --client <host>

Print deterministic MCP configuration for a coding-agent host (does not write files).

Hosts:
  claude-code   Claude Code .mcp.json / settings snippet
  codex         Codex config.toml snippet
  gemini        Gemini CLI settings snippet
  grok          Grok ~/.grok/config.toml or project .grok/config.toml
  opencode      OpenCode MCP config snippet

Preferred install (developer path):
  go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest

Then ensure the binary is on PATH as "evident-output-mcp".
`)
}

func clientConfig(client string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(client)) {
	case "grok":
		return `# Grok Build — prefer user scope ($HOME/.grok/config.toml).
# Absolute path required: Grok's process PATH often omits $HOME/.local/bin.
# Grok expands ${HOME} in command/args/env/headers.
#
# Install (clone at $HOME/Developer/Personal/evident-output):
#   go build -o "$HOME/.local/bin/evident-output-mcp" \
#     "$HOME/Developer/Personal/evident-output/cmd/evident-output-mcp"
# Or: GOBIN="$HOME/.local/bin" go install \
#       github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest
# Register: grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"
#
# Verify (no TUI restart):
#   grok mcp doctor evident-output --json
#   grok -p 'Call use_tool on evident-output__evident_output_list_guides with {}. Reply CONNECTED or FAILED.' \
#     --output-format plain --max-turns 5 --always-approve \
#     --cwd "$HOME/Developer/Personal/evident-output"
#
# Tools (underscores only): evident_output_list_guides, _get_guidance, _review, _preview, _explain
# Grok ids: evident-output__evident_output_*

[mcp_servers.evident-output]
command = "${HOME}/.local/bin/evident-output-mcp"
enabled = true
startup_timeout_sec = 30
`, nil
	case "claude-code", "claude", "claude_code":
		return `{
  "mcpServers": {
    "evident-output": {
      "command": "${HOME}/.local/bin/evident-output-mcp",
      "args": []
    }
  }
}
`, nil
	case "codex":
		return `# Codex MCP (stdio)
# Install: GOBIN="$HOME/.local/bin" go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest
# Or: go build -o "$HOME/.local/bin/evident-output-mcp" \
#       "$HOME/Developer/Personal/evident-output/cmd/evident-output-mcp"

[mcp_servers.evident-output]
command = "${HOME}/.local/bin/evident-output-mcp"
`, nil
	case "gemini":
		return `{
  "mcpServers": {
    "evident-output": {
      "command": "${HOME}/.local/bin/evident-output-mcp",
      "args": []
    }
  }
}
`, nil
	case "opencode":
		return `{
  "mcp": {
    "evident-output": {
      "type": "local",
      "command": ["${HOME}/.local/bin/evident-output-mcp"],
      "enabled": true
    }
  }
}
`, nil
	default:
		return "", fmt.Errorf("unknown client %q (want claude-code|codex|gemini|grok|opencode)", client)
	}
}
