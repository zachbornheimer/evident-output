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
		return `# Grok Build — user (~/.grok/config.toml) or project (.grok/config.toml)
# Only [mcp_servers.*] is read from project configs.
# Prefer PATH lookup so machines without a fixed install path still work.
# Install: go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest
# Or:      grok mcp add evident-output -- evident-output-mcp

[mcp_servers.evident-output]
command = "evident-output-mcp"
args = []
enabled = true
startup_timeout_sec = 30
`, nil
	case "claude-code", "claude", "claude_code":
		return `{
  "mcpServers": {
    "evident-output": {
      "command": "evident-output-mcp",
      "args": []
    }
  }
}
`, nil
	case "codex":
		return `# Codex MCP (stdio) — place under the host's MCP servers map.
# Install: go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest

[mcp_servers.evident-output]
command = "evident-output-mcp"
args = []
`, nil
	case "gemini":
		return `{
  "mcpServers": {
    "evident-output": {
      "command": "evident-output-mcp",
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
      "command": ["evident-output-mcp"],
      "enabled": true
    }
  }
}
`, nil
	default:
		return "", fmt.Errorf("unknown client %q (want claude-code|codex|gemini|grok|opencode)", client)
	}
}
