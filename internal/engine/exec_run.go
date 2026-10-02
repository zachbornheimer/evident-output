package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ExecOutput is one declared Exec output, flattened from evo.File and
// evo.Tree by the public layer.
type ExecOutput struct {
	Path string
	// Tree is true for a directory output, false for a regular file.
	Tree bool
	// WantBytes is the declared literal content of a File output; HasBytes
	// is false when no literal content was declared.
	WantBytes []byte
	HasBytes  bool
}

// ExecRequest is evo.Exec after the public layer's flattening.
type ExecRequest struct {
	Path    string
	Args    []string
	Dir     string
	Env     []string
	Outputs []ExecOutput
}

// RunExec spawns one child, captures its output into the Task's evidence,
// and verifies declared Outputs after a zero exit. ctx must come from a
// Task's Define callback.
func RunExec(ctx context.Context, req ExecRequest) (ExecResult, error) {
	task, scopeErr := beginOperation(ctx, fmt.Sprintf("Exec %q", req.Path))
	if scopeErr != nil {
		return ExecResult{}, scopeErr
	}
	if req.Path == "" {
		return ExecResult{}, ErrExecPathMissing
	}
	o := task.out
	dir := o.resolveWorkspacePath(req.Dir)
	if o.DryRun() {
		o.recordExecEffect(task.id, req.Path)
		return ExecResult{}, nil
	}
	resolved, err := resolveChildExecutable(req.Path, dir, req.Env)
	if err != nil {
		return ExecResult{}, fmt.Errorf("evo: Exec: %w", err)
	}
	result, err := o.spawnChild(ctx, task.id, ProcessCommand{
		Path: resolved,
		Args: append([]string(nil), req.Args...),
		Dir:  dir,
		Env:  childEnv(req.Env),
	})
	if err != nil {
		return ExecResult{}, fmt.Errorf("evo: Exec %q: %w", req.Path, err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, fmt.Errorf("evo: Exec %q: %w", req.Path, ctxErr)
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("%w (exit %d): %s", ErrExecNonzeroExit, result.ExitCode, req.Path)
	}
	if err := o.verifyExecOutputs(dir, req.Outputs); err != nil {
		return result, fmt.Errorf("evo: Exec %q: %w", req.Path, err)
	}
	if err := o.recordExecOutputs(ctx, task.id, dir, req.Outputs); err != nil {
		return result, fmt.Errorf("evo: Exec %q: %w", req.Path, err)
	}
	o.recordExecEffect(task.id, req.Path)
	return result, nil
}

// childEnv is the environment the child runs with: nil inherits the
// parent's, any non-nil slice (even empty) is the whole environment.
func childEnv(env []string) []string {
	if env == nil {
		return processEnviron()
	}
	return append([]string{}, env...)
}

// resolveChildExecutable maps Exec.Path to a file: a path with a separator
// resolves against dir; a bare name is searched on the PATH the child will
// run with (Env's last PATH entry, else the parent's). Relative PATH
// entries never match, so a binary in the working directory is not run by
// accident.
func resolveChildExecutable(path, dir string, env []string) (string, error) {
	if strings.ContainsRune(path, os.PathSeparator) {
		full := resolvePathAgainst(dir, path)
		if _, err := os.Stat(full); errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrExecExecutableNotFound, path)
		}
		return full, nil
	}
	for _, entry := range filepath.SplitList(childPATH(env)) {
		if !filepath.IsAbs(entry) {
			continue
		}
		candidate := filepath.Join(entry, path)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrExecExecutableNotFound, path)
}

func childPATH(env []string) string {
	source := env
	if source == nil {
		source = processEnviron()
	}
	value := ""
	for _, kv := range source {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			value = v
		}
	}
	return value
}

// spawnChild runs cmd with stdout and stderr feeding the Task's Capture,
// the same evidence path every Exec uses.
func (o *Output) spawnChild(ctx context.Context, taskID string, cmd ProcessCommand) (ExecResult, error) {
	task := &TaskHandle{out: o, id: taskID}
	ev := task.Capture(activityFeed(func(line string) { task.Doing(line) }))
	cmd.Stdout, cmd.Stderr = ev.Stdout(), ev.Stderr()
	outcome, runErr := o.cfg.processRunner.Run(ctx, cmd)
	if closeErr := ev.Close(); closeErr != nil && runErr == nil {
		runErr = fmt.Errorf("flush capture: %w", closeErr)
	}
	if runErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ExecResult{}, ctxErr
		}
		return ExecResult{}, fmt.Errorf("spawn %q: %w", cmd.Path, runErr)
	}
	return ExecResult{
		Ran:       true,
		ExitCode:  outcome.ExitCode,
		Stdout:    ev.streamText(CaptureStreamStdout),
		Stderr:    ev.streamText(CaptureStreamStderr),
		Truncated: ev.wasTruncated(),
	}, nil
}

// verifyExecOutputs checks every declared output exists with the declared
// kind and, for a File with literal content, that content.
func (o *Output) verifyExecOutputs(dir string, outputs []ExecOutput) error {
	for _, out := range outputs {
		if out.Path == "" {
			return ErrPathMissing
		}
		if err := o.verifyExecOutput(resolvePathAgainst(dir, out.Path), out); err != nil {
			return err
		}
	}
	return nil
}

func (o *Output) verifyExecOutput(path string, out ExecOutput) error {
	fsys := o.fileFSOrDefault()
	info, err := fsys.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrExecOutputMissingAfterSuccess, path)
		}
		return fmt.Errorf("Exec output %q: %w", path, err)
	}
	if out.Tree {
		if info.Mode()&fs.ModeSymlink != 0 {
			info, err = os.Stat(path)
			if err != nil {
				return fmt.Errorf("%w: %s: %w", ErrExecOutputMissingAfterSuccess, path, err)
			}
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: %s", ErrTreePathTypeMismatch, path)
		}
		return nil
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", ErrFilePathIsSymlink, path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrFilePathTypeMismatch, path)
	}
	if !out.HasBytes {
		return nil
	}
	got, err := fsys.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Exec output %q: %w", path, err)
	}
	if !bytes.Equal(got, out.WantBytes) {
		return fmt.Errorf("%w: %s", ErrVerifyMismatch, path)
	}
	return nil
}
