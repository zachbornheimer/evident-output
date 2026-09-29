package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	defaultAgent        = "grok"
	defaultAttempts     = 3
	defaultHeartbeat    = 12 * time.Second
	defaultAgentTimeout = 90 * time.Minute
	defaultPromptTail   = 64 << 10
)

type ProjectConfig struct {
	Title         string    `json:"title,omitempty"`
	Agent         string    `json:"agent,omitempty"`
	Mode          string    `json:"mode,omitempty"`
	Attempts      int       `json:"attempts,omitempty"`
	Heartbeat     string    `json:"heartbeat,omitempty"`
	AgentTimeout  string    `json:"agent_timeout,omitempty"`
	PromptTailMax int       `json:"prompt_tail_max,omitempty"`
	GrokArgs      []string  `json:"grok_args,omitempty"`
	ClaudeArgs    []string  `json:"claude_args,omitempty"`
	Verify        []Command `json:"verify"`
	After         []Command `json:"after,omitempty"`
}

type Command struct {
	Name    string            `json:"name,omitempty"`
	Command []string          `json:"command"`
	Dir     string            `json:"dir,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Timeout string            `json:"timeout,omitempty"`
	Stdin   bool              `json:"stdin,omitempty"`
}

type stageManifest struct {
	Name     string `json:"name"`
	Attempts int    `json:"attempts"`
	Result   string `json:"result"`
}

type manifest struct {
	SchemaVersion int             `json:"schema_version"`
	StartedAt     time.Time       `json:"started_at"`
	FinishedAt    time.Time       `json:"finished_at"`
	Root          string          `json:"root"`
	Config        string          `json:"config,omitempty"`
	Prompt        string          `json:"prompt,omitempty"`
	Agent         string          `json:"agent,omitempty"`
	Mode          string          `json:"mode"`
	Result        string          `json:"result"`
	Stages        []stageManifest `json:"stages"`
}

type resultFile struct {
	SchemaVersion   int       `json:"schema_version"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	Root            string    `json:"root"`
	Config          string    `json:"config,omitempty"`
	Prompt          string    `json:"prompt,omitempty"`
	Agent           string    `json:"agent,omitempty"`
	Mode            string    `json:"mode"`
	Result          string    `json:"result"`
	ExitCode        int       `json:"exit_code"`
	Attempts        int       `json:"attempts"`
	EvidenceDir     string    `json:"evidence_dir"`
	Bundle          string    `json:"bundle"`
	Failure         string    `json:"failure,omitempty"`
	LastWorkerError string    `json:"last_worker_error,omitempty"`
}

type runner struct {
	root          string
	configPath    string
	promptPath    string
	agent         string
	mode          string
	cfg           ProjectConfig
	evidence      string
	bundle        string
	heartbeat     time.Duration
	agentTimeout  time.Duration
	promptTailMax int

	mu              sync.Mutex
	manifest        manifest
	failure         string
	lastWorkerError string
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "run":
		os.Exit(runCommandMain(os.Args[2:]))
	case "smoke":
		os.Exit(smokeMain(os.Args[2:]))
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "evor: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `evor — bounded implementation worker + deterministic verifier

Usage:
  evor run --prompt FILE [--agent grok|claude] [--root DIR]
  evor smoke [--root DIR]

Project config:
  .evor/evor.json

Agent precedence:
  --agent > EVOR_AGENT > config.agent > grok

Evidence:
  --evidence-dir PATH  PATH must not already exist
  omitted              unique OS temp dir via os.MkdirTemp("", "evor-")`)
}

func runCommandMain(args []string) int {
	fs := flag.NewFlagSet("evor run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	rootFlag := fs.String("root", ".", "project root")
	configFlag := fs.String("config", "", "config path (default: .evor/evor.json under root)")
	promptFlag := fs.String("prompt", "", "implementation prompt file")
	agentFlag := fs.String("agent", "", "implementation agent: grok or claude")
	modeFlag := fs.String("mode", "", "presentation: auto, tty, or stream")
	attemptsFlag := fs.Int("attempts", 0, "override configured retry attempts")
	evidenceFlag := fs.String("evidence-dir", "", "fresh evidence directory; must not exist")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "evor: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	if strings.TrimSpace(*promptFlag) == "" {
		fmt.Fprintln(os.Stderr, "evor: run requires --prompt FILE")
		return 2
	}

	root, err := absoluteDir(*rootFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evor: %v\n", err)
		return 2
	}

	configPath := *configFlag
	if configPath == "" {
		configPath = filepath.Join(root, ".evor", "evor.json")
	} else if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(root, configPath)
	}

	cfg, err := loadProjectConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evor: config: %v\n", err)
		return 2
	}

	promptPath := *promptFlag
	if !filepath.IsAbs(promptPath) {
		promptPath = filepath.Join(root, promptPath)
	}
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evor: prompt: %v\n", err)
		return 2
	}
	if strings.TrimSpace(string(promptBytes)) == "" {
		fmt.Fprintln(os.Stderr, "evor: prompt is empty")
		return 2
	}

	if *modeFlag != "" {
		cfg.Mode = *modeFlag
	}
	if *attemptsFlag > 0 {
		cfg.Attempts = *attemptsFlag
	}
	if err := normalizeConfig(&cfg, root); err != nil {
		fmt.Fprintf(os.Stderr, "evor: config: %v\n", err)
		return 2
	}

	agent, err := resolveAgent(*agentFlag, os.Getenv("EVOR_AGENT"), cfg.Agent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evor: %v\n", err)
		return 2
	}
	if _, err := exec.LookPath(agent); err != nil {
		fmt.Fprintf(os.Stderr, "evor: %s CLI is not on PATH\n", agent)
		return 2
	}

	resolvedMode, err := resolveMode(cfg.Mode, stdoutIsTerminal())
	if err != nil {
		fmt.Fprintf(os.Stderr, "evor: %v\n", err)
		return 2
	}

	evo.Init(evo.Config{
		Title:   cfg.Title,
		Subject: root,
		Plain:   resolvedMode == "stream",
	})

	r, err := newRunner(root, configPath, promptPath, agent, resolvedMode, cfg, *evidenceFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evor: runner: %v\n", err)
		return 2
	}

	if err := os.WriteFile(filepath.Join(r.evidence, "request.md"), promptBytes, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "evor: write request evidence: %v\n", err)
		return r.finish(2)
	}
	if err := writeJSON(filepath.Join(r.evidence, "config.json"), cfg); err != nil {
		fmt.Fprintf(os.Stderr, "evor: write config evidence: %v\n", err)
		return r.finish(2)
	}

	captureGitSnapshot(root, r.evidence, "before")

	basePrompt := boundedWorkerPreamble + "\n\n" + string(promptBytes)
	code := evo.Main(func(ctx context.Context) error {
		return r.defineImplementation(ctx, basePrompt)
	})
	return r.finish(code)
}

func smokeMain(args []string) int {
	fs := flag.NewFlagSet("evor smoke", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	rootFlag := fs.String("root", ".", "project root")
	modeFlag := fs.String("mode", "auto", "presentation: auto, tty, or stream")
	evidenceFlag := fs.String("evidence-dir", "", "fresh evidence directory; must not exist")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "evor: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}

	root, err := absoluteDir(*rootFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evor: %v\n", err)
		return 2
	}
	mode, err := resolveMode(*modeFlag, stdoutIsTerminal())
	if err != nil {
		fmt.Fprintf(os.Stderr, "evor: %v\n", err)
		return 2
	}

	cfg := ProjectConfig{
		Title:         "evor smoke",
		Mode:          mode,
		Attempts:      1,
		Heartbeat:     "1s",
		AgentTimeout:  "10s",
		PromptTailMax: defaultPromptTail,
		Verify: []Command{
			{
				Name:    "synthetic verifier",
				Command: []string{"sh", "-c", "echo verifier-ok"},
				Timeout: "5s",
			},
		},
	}
	evo.Init(evo.Config{
		Title:   cfg.Title,
		Subject: root,
		Plain:   mode == "stream",
	})

	r, err := newRunner(root, "", "", "", mode, cfg, *evidenceFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evor: runner: %v\n", err)
		return 2
	}

	code := evo.Main(func(ctx context.Context) error {
		task := evo.Sequence("stages").Task("synthetic child")
		task.Define(func(ctx context.Context) error {
			r.appendStage(stageManifest{Name: "synthetic child", Result: "running"})
			r.setStageAttempts("synthetic child", 1)
			work := Command{
				Name:    "synthetic worker",
				Command: []string{"sh", "-c", "echo starting; sleep 6; echo finished"},
				Timeout: "10s",
			}
			if _, err := r.runCommand(ctx, task, 1, "synthetic child", "work-01", work, ""); err != nil {
				r.setFailure(err)
				r.setStageResult("synthetic child", 1, "failed")
				return err
			}
			ok, failure, err := r.runChecks(ctx, task, 1, "synthetic child", "verify-01", cfg.Verify)
			if err != nil {
				r.setFailure(err)
				r.setStageResult("synthetic child", 1, "failed")
				return err
			}
			if !ok {
				err := errors.New(failure)
				r.setFailure(err)
				r.setStageResult("synthetic child", 1, "verification failed")
				return err
			}
			r.setStageResult("synthetic child", 1, "passed")
			return nil
		})
		return nil
	})
	return r.finish(code)
}

const boundedWorkerPreamble = `You are a bounded implementation worker controlled by a planning agent.

Follow the requested scope and architecture. Inspect repository instructions and current state before editing.

Rules:
- Continue from the current working tree; preserve existing uncommitted work.
- Do not broaden into adjacent features or redesign the task.
- Do not commit, push, merge, reset, clean, stash, or switch branches unless the prompt explicitly authorizes it.
- Do not weaken or rewrite independent verification merely to make it pass.
- Run focused checks while working when useful; EVOR runs the authoritative verifier after you return.
- If the request cannot be completed safely, explain the concrete blocker and stop.`

func loadProjectConfig(path string) (ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectConfig{}, err
	}
	var cfg ProjectConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return ProjectConfig{}, err
	}
	return cfg, nil
}

func normalizeConfig(cfg *ProjectConfig, root string) error {
	if cfg.Title == "" {
		cfg.Title = filepath.Base(root)
	}
	if cfg.Mode == "" {
		cfg.Mode = "auto"
	}
	if cfg.Attempts <= 0 {
		cfg.Attempts = defaultAttempts
	}
	if cfg.Heartbeat == "" {
		cfg.Heartbeat = defaultHeartbeat.String()
	}
	if cfg.AgentTimeout == "" {
		cfg.AgentTimeout = defaultAgentTimeout.String()
	}
	if cfg.PromptTailMax <= 0 {
		cfg.PromptTailMax = defaultPromptTail
	}
	if _, err := time.ParseDuration(cfg.Heartbeat); err != nil {
		return fmt.Errorf("heartbeat %q: %w", cfg.Heartbeat, err)
	}
	if _, err := time.ParseDuration(cfg.AgentTimeout); err != nil {
		return fmt.Errorf("agent_timeout %q: %w", cfg.AgentTimeout, err)
	}
	if cfg.Attempts < 1 {
		return errors.New("attempts must be positive")
	}
	if len(cfg.Verify) == 0 {
		return errors.New("verify must contain at least one deterministic command")
	}
	for _, command := range append(append([]Command(nil), cfg.Verify...), cfg.After...) {
		if err := validateCommand(command); err != nil {
			return err
		}
	}
	return nil
}

func validateCommand(command Command) error {
	if len(command.Command) == 0 || strings.TrimSpace(command.Command[0]) == "" {
		return errors.New("command must contain an executable")
	}
	if command.Timeout != "" {
		if _, err := time.ParseDuration(command.Timeout); err != nil {
			return fmt.Errorf("command %q timeout %q: %w", command.Name, command.Timeout, err)
		}
	}
	return nil
}

func resolveAgent(flagValue, envValue, configValue string) (string, error) {
	for _, candidate := range []string{flagValue, envValue, configValue, defaultAgent} {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		switch candidate {
		case "grok", "claude":
			return candidate, nil
		default:
			return "", fmt.Errorf("unknown agent %q; supported: grok, claude", candidate)
		}
	}
	panic("unreachable")
}

func resolveMode(requested string, tty bool) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" {
		requested = "auto"
	}
	switch requested {
	case "auto":
		if tty {
			return "tty", nil
		}
		return "stream", nil
	case "tty", "stream":
		return requested, nil
	default:
		return "", fmt.Errorf("mode must be auto, tty, or stream; got %q", requested)
	}
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func absoluteDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return abs, nil
}

func newRunner(root, configPath, promptPath, agent, mode string, cfg ProjectConfig, requestedEvidence string) (*runner, error) {
	heartbeat, err := time.ParseDuration(cfg.Heartbeat)
	if err != nil {
		return nil, err
	}
	agentTimeout, err := time.ParseDuration(cfg.AgentTimeout)
	if err != nil {
		return nil, err
	}
	evidence, bundle, err := createEvidenceDir(requestedEvidence)
	if err != nil {
		return nil, err
	}

	r := &runner{
		root:          root,
		configPath:    configPath,
		promptPath:    promptPath,
		agent:         agent,
		mode:          mode,
		cfg:           cfg,
		evidence:      evidence,
		bundle:        bundle,
		heartbeat:     heartbeat,
		agentTimeout:  agentTimeout,
		promptTailMax: cfg.PromptTailMax,
		manifest: manifest{
			SchemaVersion: 1,
			StartedAt:     time.Now(),
			Root:          root,
			Config:        configPath,
			Prompt:        promptPath,
			Agent:         agent,
			Mode:          mode,
			Result:        "running",
		},
	}
	if err := r.writeManifest(); err != nil {
		return nil, err
	}
	return r, nil
}

func createEvidenceDir(requested string) (string, string, error) {
	if requested == "" {
		dir, err := os.MkdirTemp("", "evor-")
		if err != nil {
			return "", "", fmt.Errorf("create temporary evidence dir: %w", err)
		}
		return dir, dir + ".zip", nil
	}

	dir, err := filepath.Abs(requested)
	if err != nil {
		return "", "", fmt.Errorf("evidence dir: %w", err)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", "", fmt.Errorf("evidence dir already exists: %s", dir)
		}
		return "", "", fmt.Errorf("create evidence dir %s: %w", dir, err)
	}

	bundle := dir + ".zip"
	if _, err := os.Stat(bundle); err == nil {
		_ = os.Remove(dir)
		return "", "", fmt.Errorf("results bundle already exists: %s", bundle)
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(dir)
		return "", "", fmt.Errorf("check results bundle %s: %w", bundle, err)
	}
	return dir, bundle, nil
}

func (r *runner) defineImplementation(ctx context.Context, basePrompt string) error {
	task := evo.Sequence("stages").Task("implementation")
	task.Define(func(ctx context.Context) error {
		return r.runImplementation(ctx, task, basePrompt)
	})
	return nil
}

func (r *runner) runImplementation(ctx context.Context, task *evo.TaskHandle, basePrompt string) error {
	r.appendStage(stageManifest{Name: "implementation", Result: "running"})

	var retryContext string
	for attempt := 1; attempt <= r.cfg.Attempts; attempt++ {
		r.setStageAttempts("implementation", attempt)

		prompt := basePrompt
		if retryContext != "" {
			prompt += "\n\nThe previous attempt did not satisfy the independent verifier.\n"
			prompt += "Continue from the CURRENT working tree. Fix the implementation, not the verifier.\n\n"
			prompt += "--- PRIOR ATTEMPT EVIDENCE ---\n"
			prompt += trimTail(retryContext, r.promptTailMax)
			prompt += "\n--- END PRIOR ATTEMPT EVIDENCE ---\n"
		}

		task.Doing(fmt.Sprintf("%s attempt %d/%d", r.agent, attempt, r.cfg.Attempts))
		worker := r.agentCommand(prompt)
		workerTail, workerErr := r.runCommand(
			ctx,
			task,
			1,
			"implementation",
			fmt.Sprintf("worker-%02d", attempt),
			worker,
			fmt.Sprintf("%s [prompt in request.md]", r.agent),
		)
		if workerErr != nil {
			r.mu.Lock()
			r.lastWorkerError = workerErr.Error()
			r.mu.Unlock()
		}

		task.Doing("verify")
		ok, verifyFailure, verifyErr := r.runChecks(
			ctx,
			task,
			1,
			"implementation",
			fmt.Sprintf("verify-%02d", attempt),
			r.cfg.Verify,
		)
		if verifyErr != nil {
			r.setFailure(verifyErr)
			r.setStageResult("implementation", attempt, "infrastructure failed")
			return verifyErr
		}
		if ok {
			for i, command := range r.cfg.After {
				task.Doing("after")
				if _, err := r.runCommand(
					ctx,
					task,
					1,
					"implementation",
					fmt.Sprintf("after-%02d", i+1),
					command,
					"",
				); err != nil {
					err = fmt.Errorf("after command %q: %w", command.Name, err)
					r.setFailure(err)
					r.setStageResult("implementation", attempt, "after failed")
					return err
				}
			}
			r.setStageResult("implementation", attempt, "passed")
			return nil
		}

		var b strings.Builder
		if workerErr != nil {
			fmt.Fprintf(&b, "Worker process error:\n%v\n\nWorker output tail:\n%s\n\n", workerErr, workerTail)
		}
		fmt.Fprintf(&b, "Independent verifier failure:\n%s", verifyFailure)
		retryContext = b.String()

		if attempt == r.cfg.Attempts {
			err := fmt.Errorf("verification failed after %d attempt(s)", attempt)
			r.setFailure(err)
			r.setStageResult("implementation", attempt, "verification failed")
			return err
		}
	}
	panic("unreachable")
}

func (r *runner) agentCommand(prompt string) Command {
	timeout := r.agentTimeout.String()
	switch r.agent {
	case "grok":
		argv := []string{
			"grok",
			"--cwd", r.root,
			"--always-approve",
			"--effort", "high",
		}
		argv = append(argv, r.cfg.GrokArgs...)
		argv = append(argv, "-p", prompt)
		return Command{
			Name:    "grok",
			Command: argv,
			Timeout: timeout,
		}
	case "claude":
		argv := []string{
			"claude",
			"--print",
			"--dangerously-skip-permissions",
			"--max-turns", "64",
		}
		argv = append(argv, r.cfg.ClaudeArgs...)
		argv = append(argv, "-p", prompt)
		return Command{
			Name:    "claude",
			Command: argv,
			Timeout: timeout,
		}
	default:
		panic("unreachable")
	}
}

func (r *runner) runChecks(
	ctx context.Context,
	task *evo.TaskHandle,
	stageIndex int,
	stageName, label string,
	checks []Command,
) (bool, string, error) {
	var failures strings.Builder
	for i, check := range checks {
		name := fmt.Sprintf("%s-%02d", label, i+1)
		tail, err := r.runCommand(ctx, task, stageIndex, stageName, name, check, "")
		if err == nil {
			continue
		}

		var startErr *exec.Error
		if errors.As(err, &startErr) {
			return false, "", fmt.Errorf("verifier infrastructure: %w", err)
		}
		fmt.Fprintf(
			&failures,
			"$ %s\n%s\n%v\n\n",
			displayCommand(check.Command),
			tail,
			err,
		)
		return false, failures.String(), nil
	}
	return true, "", nil
}

func (r *runner) runCommand(
	parent context.Context,
	task *evo.TaskHandle,
	stageIndex int,
	stageName, label string,
	spec Command,
	displayOverride string,
) (string, error) {
	if err := validateCommand(spec); err != nil {
		return "", err
	}

	dir := r.root
	if spec.Dir != "" {
		dir = spec.Dir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(r.root, dir)
		}
	}

	ctx := parent
	cancel := func() {}
	if spec.Timeout != "" {
		duration, err := time.ParseDuration(spec.Timeout)
		if err != nil {
			return "", err
		}
		ctx, cancel = context.WithTimeout(parent, duration)
	}
	defer cancel()

	logPath := filepath.Join(
		r.evidence,
		fmt.Sprintf("%02d-%s-%s.log", stageIndex, slug(stageName), slug(label)),
	)
	logFile, err := os.Create(logPath)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", logPath, err)
	}
	defer logFile.Close()

	tail := newTailBuffer(r.promptTailMax)
	sink := newActivityWriter(
		r.mode == "stream",
		logFile,
		task.Writer(),
		tail,
	)

	display := displayOverride
	if display == "" {
		display = displayCommand(spec.Command)
	}
	fmt.Fprintf(logFile, "$ %s\n\n", display)
	if r.mode == "stream" {
		fmt.Printf("$ %s\n", display)
	}
	if spec.Name != "" {
		task.Doing(spec.Name)
	} else {
		task.Doing(filepath.Base(spec.Command[0]))
	}

	cmd := exec.CommandContext(ctx, spec.Command[0], spec.Command[1:]...)
	cmd.Dir = dir
	cmd.Env = mergedEnv(spec.Env)
	if spec.Stdin {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = sink
	cmd.Stderr = sink

	done := make(chan struct{})
	if r.mode == "stream" {
		go r.heartbeatLoop(ctx, sink, display, done)
	}

	started := time.Now()
	err = cmd.Run()
	close(done)
	elapsed := time.Since(started).Round(time.Millisecond)

	if err != nil {
		fmt.Fprintf(logFile, "\n[runner] exit after %s: %v\n", elapsed, err)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return tail.String(), fmt.Errorf("timeout after %s", spec.Timeout)
		}
		return tail.String(), err
	}

	fmt.Fprintf(logFile, "\n[runner] exit 0 after %s\n", elapsed)
	return tail.String(), nil
}

func (r *runner) heartbeatLoop(ctx context.Context, sink *activityWriter, name string, done <-chan struct{}) {
	timer := time.NewTimer(r.heartbeat)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-sink.activity:
			resetTimer(timer, r.heartbeat)
		case <-timer.C:
			_, _ = fmt.Fprintf(sink, "[runner] still running: %s\n", name)
			resetTimer(timer, r.heartbeat)
		}
	}
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

func (r *runner) finish(code int) int {
	captureGitSnapshot(r.root, r.evidence, "after")

	r.mu.Lock()
	r.manifest.FinishedAt = time.Now()
	if code == 0 {
		r.manifest.Result = "passed"
	} else {
		r.manifest.Result = "failed"
	}
	_ = r.writeManifestLocked()

	attempts := 0
	if len(r.manifest.Stages) > 0 {
		attempts = r.manifest.Stages[len(r.manifest.Stages)-1].Attempts
	}
	result := resultFile{
		SchemaVersion:   1,
		StartedAt:       r.manifest.StartedAt,
		FinishedAt:      r.manifest.FinishedAt,
		Root:            r.root,
		Config:          r.configPath,
		Prompt:          r.promptPath,
		Agent:           r.agent,
		Mode:            r.mode,
		Result:          r.manifest.Result,
		ExitCode:        code,
		Attempts:        attempts,
		EvidenceDir:     r.evidence,
		Bundle:          r.bundle,
		Failure:         r.failure,
		LastWorkerError: r.lastWorkerError,
	}
	r.mu.Unlock()

	resultPath := filepath.Join(r.evidence, "result.json")
	if err := writeJSON(resultPath, result); err != nil {
		fmt.Fprintf(os.Stderr, "evor: write result: %v\n", err)
		if code == 0 {
			code = 2
		}
	}

	if err := zipEvidence(r.evidence, r.bundle); err != nil {
		fmt.Fprintf(os.Stderr, "evor: create results bundle: %v\n", err)
		if code == 0 {
			code = 2
		}
	}

	fmt.Printf("\nEvidence: %s\n", r.evidence)
	fmt.Printf("Result:   %s\n", resultPath)
	fmt.Printf("Bundle:   %s\n", r.bundle)
	if digest, err := fileSHA256(r.bundle); err == nil {
		fmt.Printf("SHA256:   %s\n", digest)
	}
	fmt.Printf("EVOR_RESULT=%s\n", resultPath)
	fmt.Printf("EVOR_BUNDLE=%s\n", r.bundle)
	return code
}

func (r *runner) setFailure(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failure = err.Error()
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func captureGitSnapshot(root, evidence, phase string) {
	if _, err := exec.LookPath("git"); err != nil {
		return
	}
	check := exec.Command("git", "-C", root, "rev-parse", "--is-inside-work-tree")
	if err := check.Run(); err != nil {
		return
	}

	commands := []struct {
		name string
		args []string
	}{
		{"head", []string{"rev-parse", "HEAD"}},
		{"status", []string{"status", "--short", "--branch"}},
		{"diff", []string{"diff", "--no-ext-diff"}},
		{"cached-diff", []string{"diff", "--cached", "--no-ext-diff"}},
		{"names", []string{"diff", "--name-status", "HEAD"}},
	}
	for _, item := range commands {
		cmd := exec.Command("git", append([]string{"-C", root}, item.args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			out = append(out, []byte(fmt.Sprintf("\n[evor] git command error: %v\n", err))...)
		}
		_ = os.WriteFile(
			filepath.Join(evidence, fmt.Sprintf("git-%s-%s.txt", phase, item.name)),
			out,
			0o644,
		)
	}
}

type activityWriter struct {
	mu       sync.Mutex
	raw      bool
	log      io.Writer
	task     io.Writer
	tail     io.Writer
	activity chan struct{}
	lastUnix atomic.Int64
}

func newActivityWriter(raw bool, log, task, tail io.Writer) *activityWriter {
	w := &activityWriter{
		raw:      raw,
		log:      log,
		task:     task,
		tail:     tail,
		activity: make(chan struct{}, 1),
	}
	w.touch()
	return w
}

func (w *activityWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.touch()
	writers := []io.Writer{w.log, w.task, w.tail}
	if w.raw {
		writers = append([]io.Writer{os.Stdout}, writers...)
	}
	for _, dst := range writers {
		if _, err := dst.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (w *activityWriter) touch() {
	w.lastUnix.Store(time.Now().UnixNano())
	select {
	case w.activity <- struct{}{}:
	default:
	}
}

type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func newTailBuffer(max int) *tailBuffer {
	return &tailBuffer{max: max}
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.buf...))
}

func trimTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "[evor] output truncated to tail\n" + s[len(s)-max:]
}

func mergedEnv(overrides map[string]string) []string {
	if len(overrides) == 0 {
		return os.Environ()
	}
	values := make(map[string]string)
	order := make([]string, 0)
	seen := make(map[string]bool)

	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		if !seen[key] {
			order = append(order, key)
			seen[key] = true
		}
		values[key] = value
	}
	for key, value := range overrides {
		if !seen[key] {
			order = append(order, key)
			seen[key] = true
		}
		values[key] = value
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, key+"="+values[key])
	}
	return out
}

func displayCommand(argv []string) string {
	var b strings.Builder
	for i, arg := range argv {
		if i > 0 {
			b.WriteByte(' ')
		}
		if strings.ContainsAny(arg, " \t\n\"'") {
			fmt.Fprintf(&b, "%q", arg)
		} else {
			b.WriteString(arg)
		}
	}
	return b.String()
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func (r *runner) appendStage(stage stageManifest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manifest.Stages = append(r.manifest.Stages, stage)
	_ = r.writeManifestLocked()
}

func (r *runner) setStageAttempts(name string, attempts int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.manifest.Stages {
		if r.manifest.Stages[i].Name == name {
			r.manifest.Stages[i].Attempts = attempts
		}
	}
	_ = r.writeManifestLocked()
}

func (r *runner) setStageResult(name string, attempts int, result string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.manifest.Stages {
		if r.manifest.Stages[i].Name == name {
			r.manifest.Stages[i].Attempts = attempts
			r.manifest.Stages[i].Result = result
		}
	}
	_ = r.writeManifestLocked()
}

func (r *runner) writeManifest() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.writeManifestLocked()
}

func (r *runner) writeManifestLocked() error {
	return writeJSON(filepath.Join(r.evidence, "manifest.json"), r.manifest)
}

func zipEvidence(dir, bundle string) error {
	f, err := os.OpenFile(bundle, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	base := filepath.Base(dir)

	walkErr := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(base, rel))
		header.Method = zip.Deflate

		dst, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})

	zipErr := zw.Close()
	fileErr := f.Close()
	if walkErr != nil {
		_ = os.Remove(bundle)
		return walkErr
	}
	if zipErr != nil {
		_ = os.Remove(bundle)
		return zipErr
	}
	if fileErr != nil {
		_ = os.Remove(bundle)
		return fileErr
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// runtime is intentionally referenced so evidence from support requests can
// easily add GOOS/GOARCH without changing imports.
var _ = runtime.GOOS
