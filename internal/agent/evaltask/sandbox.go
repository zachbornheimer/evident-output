package evaltask

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

const (
	// SandboxModule is the module path candidates compile under; a
	// candidate imports SandboxModule+"/fixture".
	SandboxModule = "evalsandbox"
	libraryModule = "github.com/zachbornheimer/evident-output"
	answerDirName = "answer"
	fileMode      = 0o644
	dirMode       = 0o755
)

// sandboxGoMod is the throwaway module: the library is replaced by the
// checkout under test, so a candidate builds against the worktree offline.
const sandboxGoMod = `module %s

go %s

require (
	%s v0.0.0
	golang.org/x/term %s // indirect
)

replace %s => %s
`

var (
	goDirective = regexp.MustCompile(`(?m)^go (\S+)$`)
	termVersion = regexp.MustCompile(`golang\.org/x/term (v\S+)`)
)

// Sandbox is a throwaway module holding one candidate and its fixture.
type Sandbox struct {
	Root string
}

// AnswerDir is the directory review and the build target point at.
func (s Sandbox) AnswerDir() string { return filepath.Join(s.Root, answerDirName) }

// NewSandbox lays out root as a module replacing the library with
// repoRoot, with fixture under fixture/ and candidate under answer/.
func NewSandbox(root, repoRoot string, fixture, candidate fs.FS) (Sandbox, error) {
	sb := Sandbox{Root: root}
	if err := writeGoMod(root, repoRoot); err != nil {
		return Sandbox{}, err
	}
	if err := copyFile(filepath.Join(repoRoot, "go.sum"), filepath.Join(root, "go.sum")); err != nil {
		return Sandbox{}, err
	}
	if err := copyTree(fixture, filepath.Join(root, "fixture")); err != nil {
		return Sandbox{}, err
	}
	if err := copyTree(candidate, sb.AnswerDir()); err != nil {
		return Sandbox{}, err
	}
	return sb, nil
}

func writeGoMod(root, repoRoot string) error {
	repoMod, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return fmt.Errorf("read library go.mod in %s: %w", repoRoot, err)
	}
	goVersion := goDirective.FindSubmatch(repoMod)
	term := termVersion.FindSubmatch(repoMod)
	if goVersion == nil || term == nil {
		return fmt.Errorf("library go.mod in %s lacks a go directive or golang.org/x/term requirement", repoRoot)
	}
	body := fmt.Sprintf(sandboxGoMod, SandboxModule, goVersion[1], libraryModule, term[1], libraryModule, repoRoot)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(body), fileMode); err != nil {
		return fmt.Errorf("write sandbox go.mod in %s: %w", root, err)
	}
	return nil
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("read %s: %w", from, err)
	}
	if err := os.WriteFile(to, data, fileMode); err != nil {
		return fmt.Errorf("write %s: %w", to, err)
	}
	return nil
}

// copyTree copies the regular files of src into dir, preserving layout.
func copyTree(src fs.FS, dir string) error {
	return fs.WalkDir(src, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", name, err)
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if entry.IsDir() {
			if err := os.MkdirAll(target, dirMode); err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			return nil
		}
		data, err := fs.ReadFile(src, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := os.WriteFile(target, data, fileMode); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}
