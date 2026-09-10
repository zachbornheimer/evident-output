package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/modpin"
)

func maybeAutoUpdate(cwd string, argv []string, env Env, files Files, run Runner, execer Execer, id Identity) {
	if env.Get(envNoAutoUpdate) != "" {
		return
	}
	if cwd == "" {
		return
	}
	_, pin, err := pinFromWalk(cwd, files)
	if err != nil {
		return
	}
	if !updateNeeded(id, pin) {
		return
	}
	plan, err := PlanInstall("", cwd, env, files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evident-output-mcp: auto-update plan: %v\n", err)
		return
	}
	if err := ExecuteInstall(plan, run, files); err != nil {
		fmt.Fprintf(os.Stderr, "evident-output-mcp: auto-update install: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "evident-output-mcp: re-exec after update (%s)\n", plan.LinkSrc)
	newEnv := os.Environ()
	if plan.BuiltFrom != "" {
		newEnv = append(newEnv, envBuiltFrom+"="+plan.BuiltFrom)
	}
	if err := execer.Exec(plan.LinkSrc, argv, newEnv); err != nil {
		fmt.Fprintf(os.Stderr, "evident-output-mcp: re-exec: %v\n", err)
	}
}

func pinForReview(res reviewPin) modpin.Pin {
	return modpin.Pin{
		Version:     firstNonEmpty(res.DesiredVersion, res.ModuleVersion),
		ReplacePath: res.ReplacePath,
	}
}

type reviewPin struct {
	DesiredVersion string
	ModuleVersion  string
	ReplacePath    string
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
