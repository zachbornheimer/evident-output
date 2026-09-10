// Package modpin parses a go.mod for the evident-output require pin and
// replace directive so MCP update and the usage-audit share one parser.
package modpin

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ModulePath is the Go module path for evident-output.
const ModulePath = "github.com/zachbornheimer/evident-output"

// ErrNonPathReplace is returned when a replace points at another module
// instead of a filesystem path.
var ErrNonPathReplace = fmt.Errorf("evident-output replace must be a filesystem path, not a module")

// Pin is the evident-output identity declared by one go.mod.
type Pin struct {
	// Module is the `module` line of the go.mod (the consuming module).
	Module string
	// Version is the required evident-output version, if any.
	Version string
	// ReplacePath is the resolved filesystem replace target, if any.
	ReplacePath string
	// SelfModule is true when this go.mod *is* evident-output.
	SelfModule bool
}

// RequireVersion finds modulePath's version in go.mod source, honoring both
// the single-line ("require path v1.2.3") and block ("require (\n\tpath
// v1.2.3\n)") forms. It returns "" when modulePath is not required.
func RequireVersion(goMod, modulePath string) string {
	var version string
	eachDirective(goMod, "require", func(fields []string) {
		if version == "" && len(fields) >= 2 && fields[0] == modulePath {
			version = fields[1]
		}
	})
	return version
}

// Parse extracts the evident-output pin from go.mod text. goModDir is the
// directory containing that go.mod, used to resolve a relative replace.
func Parse(goMod, goModDir string) (Pin, error) {
	pin := Pin{
		Module:     moduleLine(goMod),
		Version:    RequireVersion(goMod, ModulePath),
		SelfModule: moduleLine(goMod) == ModulePath,
	}
	replace, err := replaceTarget(goMod, ModulePath)
	if err != nil {
		return Pin{}, err
	}
	if replace != "" {
		pin.ReplacePath = resolveReplace(replace, goModDir)
	}
	return pin, nil
}

func moduleLine(goMod string) string {
	for _, line := range strings.Split(goMod, "\n") {
		trimmed := stripComment(strings.TrimSpace(line))
		if strings.HasPrefix(trimmed, "module ") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return ""
}

func replaceTarget(goMod, modulePath string) (string, error) {
	var target string
	var nonPath bool
	eachDirective(goMod, "replace", func(fields []string) {
		old, neu, ok := splitReplace(fields)
		if !ok || old != modulePath {
			return
		}
		if !isFilesystemReplace(neu) {
			nonPath = true
			return
		}
		target = neu
	})
	if nonPath {
		return "", fmt.Errorf("%w", ErrNonPathReplace)
	}
	return target, nil
}

func splitReplace(fields []string) (old, neu string, ok bool) {
	arrow := -1
	for i, f := range fields {
		if f == "=>" {
			arrow = i
			break
		}
	}
	if arrow < 1 || arrow+1 >= len(fields) {
		return "", "", false
	}
	return fields[0], fields[arrow+1], true
}

func isFilesystemReplace(p string) bool {
	if p == "" {
		return false
	}
	if filepath.IsAbs(p) {
		return true
	}
	return strings.HasPrefix(p, ".")
}

func resolveReplace(p, goModDir string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(goModDir, p))
}

func eachDirective(goMod, keyword string, fn func(fields []string)) {
	inBlock := false
	blockHeader := keyword + " ("
	linePrefix := keyword + " "
	for _, line := range strings.Split(goMod, "\n") {
		trimmed := stripComment(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(trimmed, blockHeader):
			inBlock = true
			continue
		case inBlock && trimmed == ")":
			inBlock = false
			continue
		}
		var rest string
		switch {
		case inBlock:
			rest = trimmed
		case strings.HasPrefix(trimmed, linePrefix):
			rest = strings.TrimPrefix(trimmed, linePrefix)
		default:
			continue
		}
		if rest == "" {
			continue
		}
		fn(strings.Fields(rest))
	}
}

func stripComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return strings.TrimSpace(line[:i])
	}
	return line
}
