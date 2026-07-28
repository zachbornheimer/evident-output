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
	fmt.Print(body)
	return 0
}

func printConfigHelp() {
	fmt.Fprint(os.Stderr, "evident-output-mcp config --client <host>\n\n")
	fmt.Fprint(os.Stderr, "Print deterministic MCP configuration for a coding-agent host (does not write files).\n\n")
	fmt.Fprint(os.Stderr, "Hosts:\n")
	fmt.Fprint(os.Stderr, "  claude-code   Claude Code .mcp.json / settings snippet\n")
	fmt.Fprint(os.Stderr, "  codex         Codex config.toml snippet\n")
	fmt.Fprint(os.Stderr, "  gemini        Gemini CLI settings snippet\n")
	fmt.Fprint(os.Stderr, "  grok          Grok ~/.grok/config.toml or project .grok/config.toml\n")
	fmt.Fprint(os.Stderr, "  opencode      OpenCode MCP config snippet\n\n")
	fmt.Fprint(os.Stderr, "Preferred install (pin a release tag — not @latest):\n")
	fmt.Fprintf(os.Stderr, "  GOBIN=\"$HOME/.local/bin\" go install \\\n    github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@%s\n\n", Version)
	fmt.Fprint(os.Stderr, "Then ensure the binary is on PATH as \"evident-output-mcp\".\n")
}

func clientConfig(client string) (string, error) {
	pin := Version
	if pin == "" || pin == "dev" {
		pin = "v0.2.6"
	}
	switch strings.ToLower(strings.TrimSpace(client)) {
	case "grok":
		return fmt.Sprintf(`# Grok Build — prefer user scope ($HOME/.grok/config.toml).
# Absolute path required: Grok's process PATH often omits $HOME/.local/bin.
#
# Install (pin release — not @latest):
#   GOBIN="$HOME/.local/bin" go install \
#     github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@%s
# Register: grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"
#
# Verify: grok mcp doctor evident-output --json
# Tools: evident_output_* (underscores); Grok ids: evident-output__evident_output_*

[mcp_servers.evident-output]
command = "${HOME}/.local/bin/evident-output-mcp"
enabled = true
startup_timeout_sec = 30
`, pin), nil
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
		return fmt.Sprintf(`# Codex MCP (stdio)
# Install: GOBIN="$HOME/.local/bin" go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@%s

[mcp_servers.evident-output]
command = "${HOME}/.local/bin/evident-output-mcp"
`, pin), nil
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
