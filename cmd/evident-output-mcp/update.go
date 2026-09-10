package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var updateErr io.Writer = os.Stderr
var updateOut io.Writer = os.Stdout

func runUpdate(args []string) int {
	if len(args) == 0 || args[0] != "update" {
		return -1
	}
	version, directory, err := parseUpdateArgs(args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(updateErr, err)
		return 2
	}
	plan, err := performUpdate(version, directory, osEnv{}, osFiles{}, execRunner{})
	if err != nil {
		_, _ = fmt.Fprintln(updateErr, err)
		return 1
	}
	_, _ = fmt.Fprintf(updateOut, "installed %s", plan.LinkSrc)
	if !plan.SkipLink {
		_, _ = fmt.Fprintf(updateOut, " -> %s", plan.LinkDest)
	}
	_, _ = fmt.Fprint(updateOut, "\nrestart the MCP host\n")
	return 0
}

func parseUpdateArgs(args []string) (version, directory string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			return "", "", fmt.Errorf("usage: evident-output-mcp update --version <tag> | --directory <repo>")
		case a == "--version" && i+1 < len(args):
			version = args[i+1]
			i++
		case strings.HasPrefix(a, "--version="):
			version = strings.TrimPrefix(a, "--version=")
		case a == "--directory" && i+1 < len(args):
			directory = args[i+1]
			i++
		case strings.HasPrefix(a, "--directory="):
			directory = strings.TrimPrefix(a, "--directory=")
		default:
			return "", "", fmt.Errorf("update: unknown argument %q (want --version or --directory)", a)
		}
	}
	if (strings.TrimSpace(version) == "") == (strings.TrimSpace(directory) == "") {
		return "", "", fmt.Errorf("update requires --version XOR --directory")
	}
	return version, directory, nil
}

func performUpdate(version, directory string, env Env, files Files, run Runner) (InstallPlan, error) {
	plan, err := PlanInstall(version, directory, env, files)
	if err != nil {
		return InstallPlan{}, err
	}
	if err := ExecuteInstall(plan, run, files); err != nil {
		return InstallPlan{}, err
	}
	return plan, nil
}

func handleUpdateTool(id any, args map[string]any) {
	version, _ := args["version"].(string)
	directory, _ := args["directory"].(string)
	if _, err := performUpdate(version, directory, osEnv{}, osFiles{}, execRunner{}); err != nil {
		writeRPC(id, toolError(err.Error()))
		return
	}
	writeRPC(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": "installed; restart the MCP host"}},
		"structuredContent": map[string]any{
			"schema":      "evident_output_update.v1",
			"next_action": "restart the MCP host",
		},
	})
}
