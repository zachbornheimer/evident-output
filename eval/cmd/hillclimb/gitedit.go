package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zachbornheimer/evident-output/eval/hillclimb"
)

const (
	promptGlob   = "internal/agent/evaltask/testdata/tasks/*/prompt.md"
	docsGuideDir = "docs/guides"
	docsRefFile  = "docs/reference.md"
	addedPrefix  = "+"
	fileHeader   = "+++"
)

// gitRepo reads a repository through the git command line.
type gitRepo struct{ root string }

func (g gitRepo) output(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", g.root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// editSince is the working tree's change relative to base, as data.
func (g gitRepo) editSince(base string) (hillclimb.Edit, error) {
	names, err := g.output("diff", "--name-only", base)
	if err != nil {
		return hillclimb.Edit{}, fmt.Errorf("list changed files: %w", err)
	}
	diff, err := g.output("diff", "-U0", base)
	if err != nil {
		return hillclimb.Edit{}, fmt.Errorf("read diff: %w", err)
	}
	edit := hillclimb.Edit{Added: addedText(diff), Files: map[string]hillclimb.FilePair{}}
	for name := range strings.FieldsSeq(names) {
		edit.Changed = append(edit.Changed, name)
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		before, _ := g.output("show", base+":"+name) // absent in base: a new file
		after, err := os.ReadFile(filepath.Join(g.root, name))
		if err != nil {
			return hillclimb.Edit{}, fmt.Errorf("read edited %s: %w", name, err)
		}
		edit.Files[name] = hillclimb.FilePair{Before: []byte(before), After: after}
	}
	return edit, nil
}

func addedText(diff string) string {
	var added []string
	for line := range strings.SplitSeq(diff, "\n") {
		if strings.HasPrefix(line, addedPrefix) && !strings.HasPrefix(line, fileHeader) {
			added = append(added, strings.TrimPrefix(line, addedPrefix))
		}
	}
	return strings.Join(added, "\n")
}

// nounGuard builds the noun lint from the task prompts and the docs as they
// were at base.
func (g gitRepo) nounGuard(base string) (hillclimb.Guard, error) {
	paths, err := filepath.Glob(filepath.Join(g.root, filepath.FromSlash(promptGlob)))
	if err != nil {
		return hillclimb.Guard{}, fmt.Errorf("find task prompts: %w", err)
	}
	var prompts []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return hillclimb.Guard{}, fmt.Errorf("read prompt %s: %w", path, err)
		}
		prompts = append(prompts, string(data))
	}
	vocabulary, err := g.docsAt(base)
	if err != nil {
		return hillclimb.Guard{}, err
	}
	return hillclimb.Guard{Nouns: hillclimb.DistinctiveNouns(prompts, vocabulary)}, nil
}

func (g gitRepo) docsAt(base string) (string, error) {
	listing, err := g.output("ls-tree", "-r", "--name-only", base, docsGuideDir, docsRefFile)
	if err != nil {
		return "", fmt.Errorf("list tuned docs at %s: %w", base, err)
	}
	var docs strings.Builder
	for name := range strings.FieldsSeq(listing) {
		text, err := g.output("show", base+":"+name)
		if err != nil {
			return "", fmt.Errorf("read %s at %s: %w", name, base, err)
		}
		docs.WriteString(text)
		docs.WriteByte('\n')
	}
	return docs.String(), nil
}
