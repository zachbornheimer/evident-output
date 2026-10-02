package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const defaultPromptTail = 64 << 10

type Config struct {
	Title         string  `json:"title"`
	Mode          string  `json:"mode"`
	Heartbeat     string  `json:"heartbeat"`
	EvidenceDir   string  `json:"evidence_dir"`
	PromptTailMax int     `json:"prompt_tail_max"`
	Stages        []Stage `json:"stages"`
}

type Stage struct {
	Name       string    `json:"name"`
	Attempts   int       `json:"attempts"`
	Preverify  *bool     `json:"preverify,omitempty"`
	PromptFile string    `json:"prompt_file,omitempty"`
	Work       Command   `json:"work"`
	Verify     []Command `json:"verify,omitempty"`
	After      []Command `json:"after,omitempty"`
}

type Command struct {
	Name    string            `json:"name,omitempty"`
	Command []string          `json:"command"`
	Dir     string            `json:"dir,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Timeout string            `json:"timeout,omitempty"`
	Stdin   bool              `json:"stdin,omitempty"`
}

type manifest struct {
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at"`
	Root       string          `json:"root"`
	Config     string          `json:"config,omitempty"`
	Result     string          `json:"result"`
	Stages     []stageManifest `json:"stages"`
}

type stageManifest struct {
	Name     string `json:"name"`
	Attempts int    `json:"attempts"`
	Result   string `json:"result"`
}

type runner struct {
	root          string
	cfg           Config
	evidence      string
	heartbeat     time.Duration
	stream        bool
	promptTailMax int

	mu       sync.Mutex
	manifest manifest
}

func main() {
	var (
		configPath = flag.String("config", "evor.json", "project runner config, relative to -root")
		rootPath   = flag.String("root", ".", "target project root")
		smoke      = flag.Bool("smoke", false, "run a streaming self-test instead of reading config")
	)
	flag.Parse()

	root, err := filepath.Abs(*rootPath)
	if err != nil {
		fatalf("project root: %v", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		fatalf("project root %q is not a directory", root)
	}

	var cfg Config
	if *smoke {
		cfg = smokeConfig()
	} else {
		configFile := *configPath
		if !filepath.IsAbs(configFile) {
			configFile = filepath.Join(root, configFile)
		}
		cfg, err = loadConfig(configFile)
		if err != nil {
			fatalf("config: %v", err)
		}
	}
	if err := normalizeConfig(&cfg); err != nil {
		fatalf("config: %v", err)
	}

	evo.Init(evo.Config{
		Title:   cfg.Title,
		Subject: root,
		Plain:   cfg.Mode == "stream",
	})

	r, err := newRunner(root, *configPath, cfg)
	if err != nil {
		fatalf("runner: %v", err)
	}

	code := evo.Main(func(ctx context.Context) error {
		return r.define(ctx)
	})
	r.finish(code)
	os.Exit(code)
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, err
	}
	if dec.More() {
		return Config{}, errors.New("multiple JSON values in config")
	}
	return cfg, nil
}

func normalizeConfig(cfg *Config) error {
	if cfg.Title == "" {
		cfg.Title = "evor"
	}
	if cfg.Mode == "" {
		cfg.Mode = "stream"
	}
	if cfg.Mode != "stream" && cfg.Mode != "tty" {
		return fmt.Errorf("mode must be %q or %q", "stream", "tty")
	}
	if cfg.Heartbeat == "" {
		cfg.Heartbeat = "12s"
	}
	if cfg.EvidenceDir == "" {
		cfg.EvidenceDir = ".evor/evidence"
	}
	if cfg.PromptTailMax <= 0 {
		cfg.PromptTailMax = defaultPromptTail
	}
	if len(cfg.Stages) == 0 {
		return errors.New("no stages configured")
	}

	for i := range cfg.Stages {
		s := &cfg.Stages[i]
		if s.Name == "" {
			return fmt.Errorf("stage %d has no name", i+1)
		}
		if s.Attempts <= 0 {
			s.Attempts = 1
		}
		if len(s.Work.Command) == 0 && len(s.Verify) == 0 {
			return fmt.Errorf("stage %q has neither work nor verify commands", s.Name)
		}
		for _, c := range append(append([]Command{s.Work}, s.Verify...), s.After...) {
			if len(c.Command) == 0 {
				continue
			}
			if strings.TrimSpace(c.Command[0]) == "" {
				return fmt.Errorf("stage %q has an empty executable", s.Name)
			}
			if c.Timeout != "" {
				if _, err := time.ParseDuration(c.Timeout); err != nil {
					return fmt.Errorf("stage %q timeout %q: %w", s.Name, c.Timeout, err)
				}
			}
		}
	}
	return nil
}

func newRunner(root, configPath string, cfg Config) (*runner, error) {
	hb, err := time.ParseDuration(cfg.Heartbeat)
	if err != nil {
		return nil, fmt.Errorf("heartbeat %q: %w", cfg.Heartbeat, err)
	}
	if hb < time.Second {
		return nil, errors.New("heartbeat must be at least 1s")
	}

	stamp := time.Now().Format("20060102-150405.000000000")
	evidence := filepath.Join(root, cfg.EvidenceDir, stamp)
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		return nil, fmt.Errorf("create evidence dir: %w", err)
	}

	r := &runner{
		root:          root,
		cfg:           cfg,
		evidence:      evidence,
		heartbeat:     hb,
		stream:        cfg.Mode == "stream",
		promptTailMax: cfg.PromptTailMax,
		manifest: manifest{
			StartedAt: time.Now(),
			Root:      root,
			Config:    configPath,
			Result:    "running",
		},
	}
	if err := r.writeManifest(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *runner) define(ctx context.Context) error {
	seq := evo.Sequence("stages")
	for i := range r.cfg.Stages {
		stage := r.cfg.Stages[i]
		index := i + 1
		task := seq.Task(stage.Name)
		task.Define(func(ctx context.Context) error {
			return r.runStage(ctx, task, index, stage)
		})
	}
	return nil
}

func (r *runner) runStage(ctx context.Context, task *evo.TaskHandle, index int, stage Stage) error {
	r.appendStage(stageManifest{Name: stage.Name, Result: "running"})

	basePrompt, err := r.readPrompt(stage.PromptFile)
	if err != nil {
		r.setStageResult(stage.Name, 0, "failed")
		return err
	}

	preverify := true
	if stage.Preverify != nil {
		preverify = *stage.Preverify
	}
	if preverify && len(stage.Verify) > 0 {
		task.Doing("pre-verify")
		ok, _, err := r.runChecks(ctx, task, index, stage.Name, "pre", stage.Verify)
		if err != nil {
			r.setStageResult(stage.Name, 0, "failed")
			return err
		}
		if ok {
			r.setStageResult(stage.Name, 0, "already satisfied")
			return nil
		}
	}

	var verifyFailure string
	usedAttempts := 0
	for attempt := 1; attempt <= stage.Attempts; attempt++ {
		usedAttempts = attempt
		r.setStageAttempts(stage.Name, attempt)

		if len(stage.Work.Command) > 0 {
			task.Doing(fmt.Sprintf("work attempt %d/%d", attempt, stage.Attempts))
			prompt := basePrompt
			if verifyFailure != "" {
				prompt += "\n\nIndependent verifier failed. Fix the implementation, not the verifier.\n\n--- VERIFIER ---\n"
				prompt += trimTail(verifyFailure, r.promptTailMax)
				prompt += "\n--- END VERIFIER ---\n"
			}

			_, err := r.runCommand(ctx, task, index, stage.Name, fmt.Sprintf("work-%02d", attempt), stage.Work, prompt)
			if err != nil {
				r.setStageResult(stage.Name, attempt, "failed")
				return fmt.Errorf("%s work: %w", stage.Name, err)
			}
		}

		if len(stage.Verify) == 0 {
			break
		}

		task.Doing("verify")
		ok, failure, err := r.runChecks(ctx, task, index, stage.Name, fmt.Sprintf("verify-%02d", attempt), stage.Verify)
		if err != nil {
			r.setStageResult(stage.Name, attempt, "failed")
			return err
		}
		if ok {
			break
		}

		verifyFailure = failure
		if attempt == stage.Attempts {
			r.setStageResult(stage.Name, attempt, "verification failed")
			return fmt.Errorf("%s verification failed after %d attempt(s)", stage.Name, attempt)
		}
	}

	for i, command := range stage.After {
		task.Doing("after")
		if _, err := r.runCommand(ctx, task, index, stage.Name, fmt.Sprintf("after-%02d", i+1), command, basePrompt); err != nil {
			r.setStageResult(stage.Name, usedAttempts, "after failed")
			return fmt.Errorf("%s after command: %w", stage.Name, err)
		}
	}

	r.setStageResult(stage.Name, usedAttempts, "passed")
	return nil
}

func (r *runner) runChecks(ctx context.Context, task *evo.TaskHandle, stageIndex int, stageName, label string, checks []Command) (bool, string, error) {
	var failures strings.Builder
	for i, check := range checks {
		name := fmt.Sprintf("%s-%02d", label, i+1)
		tail, err := r.runCommand(ctx, task, stageIndex, stageName, name, check, "")
		if err != nil {
			fmt.Fprintf(&failures, "$ %s\n%s\n%v\n\n", displayCommand(check.Command), tail, err)
			return false, failures.String(), nil
		}
	}
	return true, "", nil
}

func (r *runner) runCommand(parent context.Context, task *evo.TaskHandle, stageIndex int, stageName, label string, spec Command, prompt string) (_ string, err error) {
	if len(spec.Command) == 0 {
		return "", nil
	}

	argv := make([]string, len(spec.Command))
	for i, arg := range spec.Command {
		argv[i] = expand(arg, r.root, prompt)
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
		d, err := time.ParseDuration(spec.Timeout)
		if err != nil {
			return "", err
		}
		ctx, cancel = context.WithTimeout(parent, d)
	}
	defer cancel()

	logPath := filepath.Join(r.evidence, fmt.Sprintf("%02d-%s-%s.log", stageIndex, slug(stageName), slug(label)))
	logFile, err := os.Create(logPath)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", logPath, err)
	}
	defer func() {
		if closeErr := logFile.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", logPath, closeErr)
		}
	}()

	tail := newTailBuffer(r.promptTailMax)
	sink := &activityWriter{
		raw:  r.stream,
		log:  logFile,
		task: task.Writer(),
		tail: tail,
	}
	sink.touch()

	commandName := spec.Name
	if commandName == "" {
		commandName = displayCommand(spec.Command)
	}
	_, headerErr := fmt.Fprintf(logFile, "$ %s\n\n", displayCommand(spec.Command))
	if r.stream {
		fmt.Printf("$ %s\n", displayCommand(spec.Command))
	}
	task.Doing(commandName)

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = mergedEnv(spec.Env)
	if spec.Stdin {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = sink
	cmd.Stderr = sink

	done := make(chan struct{})
	go r.heartbeatLoop(ctx, sink, commandName, done)

	started := time.Now()
	runErr := cmd.Run()
	close(done)
	elapsed := time.Since(started).Round(time.Millisecond)

	footer := fmt.Sprintf("\n[runner] exit 0 after %s\n", elapsed)
	if runErr != nil {
		footer = fmt.Sprintf("\n[runner] exit after %s: %v\n", elapsed, runErr)
	}
	_, footerErr := io.WriteString(logFile, footer)

	// A command failure outranks a failure to record it in the evidence log.
	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return tail.String(), fmt.Errorf("timeout after %s", spec.Timeout)
		}
		return tail.String(), runErr
	}
	if logErr := errors.Join(headerErr, footerErr); logErr != nil {
		return tail.String(), fmt.Errorf("write %s: %w", logPath, logErr)
	}
	return tail.String(), nil
}

func (r *runner) heartbeatLoop(ctx context.Context, sink *activityWriter, name string, done <-chan struct{}) {
	ticker := time.NewTicker(r.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			last := time.Unix(0, sink.lastUnix.Load())
			if time.Since(last) >= r.heartbeat {
				_, _ = fmt.Fprintf(sink, "[runner] still running: %s\n", name)
			}
		}
	}
}

func (r *runner) readPrompt(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.root, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt %s: %w", path, err)
	}
	return string(data), nil
}

func (r *runner) finish(code int) {
	r.mu.Lock()
	r.manifest.FinishedAt = time.Now()
	if code == 0 {
		r.manifest.Result = "passed"
	} else {
		r.manifest.Result = fmt.Sprintf("exit %d", code)
	}
	_ = r.writeManifestLocked()
	r.mu.Unlock()
	fmt.Printf("\nEvidence: %s\n", r.evidence)
}

func (r *runner) appendStage(s stageManifest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manifest.Stages = append(r.manifest.Stages, s)
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
			if attempts > 0 {
				r.manifest.Stages[i].Attempts = attempts
			}
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
	data, err := json.MarshalIndent(r.manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.evidence, "manifest.json"), append(data, '\n'), 0o644)
}

type activityWriter struct {
	mu       sync.Mutex
	raw      bool
	log      io.Writer
	task     io.Writer
	tail     io.Writer
	lastUnix atomic.Int64
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
}

type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func newTailBuffer(max int) *tailBuffer { return &tailBuffer{max: max} }

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

func expand(s, root, prompt string) string {
	return strings.NewReplacer(
		"{{root}}", root,
		"{{prompt}}", prompt,
	).Replace(s)
}

func mergedEnv(overrides map[string]string) []string {
	if len(overrides) == 0 {
		return os.Environ()
	}

	values := make(map[string]string, len(os.Environ())+len(overrides))
	order := make([]string, 0, len(os.Environ())+len(overrides))
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

func trimTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "[runner] verifier output truncated to tail\n" + s[len(s)-max:]
}

func boolPtr(v bool) *bool { return &v }

func smokeConfig() Config {
	return Config{
		Title:       "evor smoke",
		Mode:        "stream",
		Heartbeat:   "1s",
		EvidenceDir: ".evor/evidence",
		Stages: []Stage{{
			Name:      "streaming child output",
			Attempts:  1,
			Preverify: boolPtr(false),
			Work: Command{
				Name:    "synthetic worker",
				Command: []string{"sh", "-c", "echo starting; sleep 2; echo finished"},
				Timeout: "10s",
			},
			Verify: []Command{{
				Name:    "synthetic verifier",
				Command: []string{"sh", "-c", "echo verifier-ok"},
				Timeout: "5s",
			}},
		}},
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "evor: "+format+"\n", args...)
	os.Exit(2)
}
