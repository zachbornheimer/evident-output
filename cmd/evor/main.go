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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"golang.org/x/term"
)

const defaultPromptTail = 64 << 10

type Config struct {
	Title         string  `json:"title"`
	Mode          string  `json:"mode"`
	Heartbeat     string  `json:"heartbeat"`
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
	FinishedAt time.Time       `json:"finished_at,omitempty"`
	Root       string          `json:"root"`
	Config     string          `json:"config,omitempty"`
	Mode       string          `json:"mode"`
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
	bundle        string
	heartbeat     time.Duration
	stream        bool
	promptTailMax int

	mu       sync.Mutex
	manifest manifest
}

func main() {
	var (
		configPath  = flag.String("config", "evor.json", "project runner config, relative to -root")
		rootPath    = flag.String("root", ".", "target project root")
		smoke       = flag.Bool("smoke", false, "run a self-test instead of reading config")
		evidenceDir = flag.String("evidence-dir", "", "evidence directory to create; must not already exist (default: unique OS temp directory)")
	)
	flag.Parse()

	root, r, title, plain, setupErr := setup(*rootPath, *configPath, *smoke, *evidenceDir)
	evo.Init(evo.Config{
		Title:   title,
		Subject: root,
		Plain:   plain,
	})
	os.Exit(evo.Main(func(ctx context.Context) error {
		if setupErr != nil {
			return setupErr
		}
		seq := r.define(ctx)
		runErr := seq.Wait()
		finErr := r.publishEvidence(runErr)
		if runErr != nil {
			return runErr
		}
		return finErr
	}))
}

func setup(rootPath, configPath string, smoke bool, evidenceDir string) (string, *runner, string, bool, error) {
	title := "evor"
	stdoutIsTerminal := term.IsTerminal(int(os.Stdout.Fd()))
	autoMode, _ := resolvePresentation(modeAuto, stdoutIsTerminal)
	plain := streamMode(autoMode)

	root, err := filepath.Abs(rootPath)
	if err != nil {
		return rootPath, nil, title, plain, fmt.Errorf("project root: %w", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return root, nil, title, plain, fmt.Errorf("project root %q is not a directory", root)
	}

	var cfg Config
	if smoke {
		cfg = smokeConfig()
	} else {
		configFile := configPath
		if !filepath.IsAbs(configFile) {
			configFile = filepath.Join(root, configFile)
		}
		cfg, err = loadConfig(configFile)
		if err != nil {
			return root, nil, title, plain, fmt.Errorf("config: %w", err)
		}
	}
	if cfg.Title != "" {
		title = cfg.Title
	}
	if err := normalizeConfig(&cfg); err != nil {
		return root, nil, title, plain, fmt.Errorf("config: %w", err)
	}
	title = cfg.Title
	cfg.Mode, err = resolvePresentation(cfg.Mode, stdoutIsTerminal)
	if err != nil {
		return root, nil, title, plain, fmt.Errorf("config: %w", err)
	}
	plain = streamMode(cfg.Mode)

	r, err := newRunner(root, configPath, cfg, evidenceDir)
	if err != nil {
		return root, nil, title, plain, fmt.Errorf("runner: %w", err)
	}
	return root, r, title, plain, nil
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
		cfg.Mode = modeAuto
	}
	switch cfg.Mode {
	case modeAuto, modeTTY, modeStream:
	default:
		return fmt.Errorf("mode must be %q, %q, or %q", modeAuto, modeTTY, modeStream)
	}
	if cfg.Heartbeat == "" {
		cfg.Heartbeat = "12s"
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

func newRunner(root, configPath string, cfg Config, requestedEvidence string) (*runner, error) {
	hb, err := time.ParseDuration(cfg.Heartbeat)
	if err != nil {
		return nil, fmt.Errorf("heartbeat %q: %w", cfg.Heartbeat, err)
	}
	if hb < time.Second {
		return nil, errors.New("heartbeat must be at least 1s")
	}
	if cfg.Mode != modeTTY && cfg.Mode != modeStream {
		return nil, fmt.Errorf("presentation mode %q is not resolved to %q or %q", cfg.Mode, modeTTY, modeStream)
	}

	evidence, bundle, err := createEvidenceDir(requestedEvidence)
	if err != nil {
		return nil, err
	}

	r := &runner{
		root:          root,
		cfg:           cfg,
		evidence:      evidence,
		bundle:        bundle,
		heartbeat:     hb,
		stream:        streamMode(cfg.Mode),
		promptTailMax: cfg.PromptTailMax,
		manifest: manifest{
			StartedAt: time.Now(),
			Root:      root,
			Config:    configPath,
			Mode:      cfg.Mode,
			Result:    "running",
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

func (r *runner) define(ctx context.Context) *evo.SequenceHandle {
	seq := evo.Sequence("stages")
	for i := range r.cfg.Stages {
		stage := r.cfg.Stages[i]
		index := i + 1
		task := seq.Task(stage.Name)
		task.Define(func(ctx context.Context) error {
			return r.runStage(ctx, task, index, stage)
		})
	}
	return seq
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
			task.Progress(attempt, stage.Attempts)
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
			verifyFailure = ""
			break
		}

		task.Doing("verify")
		ok, failure, err := r.runChecks(ctx, task, index, stage.Name, fmt.Sprintf("verify-%02d", attempt), stage.Verify)
		if err != nil {
			r.setStageResult(stage.Name, attempt, "failed")
			return err
		}
		if ok {
			verifyFailure = ""
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

func (r *runner) runCommand(parent context.Context, task *evo.TaskHandle, stageIndex int, stageName, label string, spec Command, prompt string) (string, error) {
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
	defer logFile.Close()

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
	writeLog(logFile, "$ %s\n\n", displayCommand(spec.Command))
	if r.stream {
		evo.Println("$ " + displayCommand(spec.Command))
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
	if r.stream {
		go r.heartbeatLoop(ctx, sink, commandName, done)
	}

	started := time.Now()
	err = cmd.Run()
	close(done)
	elapsed := time.Since(started).Round(time.Millisecond)

	if err != nil {
		writeLog(logFile, "\n[runner] exit after %s: %v\n", elapsed, err)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return tail.String(), fmt.Errorf("timeout after %s", spec.Timeout)
		}
		return tail.String(), err
	}
	writeLog(logFile, "\n[runner] exit 0 after %s\n", elapsed)
	return tail.String(), nil
}

func (r *runner) heartbeatLoop(ctx context.Context, sink *activityWriter, name string, done <-chan struct{}) {
	for {
		last := time.Unix(0, sink.lastUnix.Load())
		timer := time.NewTimer(nextHeartbeatWait(time.Now(), last, r.heartbeat))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-done:
			timer.Stop()
			return
		case <-timer.C:
			last = time.Unix(0, sink.lastUnix.Load())
			if time.Since(last) >= r.heartbeat {
				writeLog(sink, "[runner] still running: %s\n", name)
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

func (r *runner) publishEvidence(runErr error) error {
	r.mu.Lock()
	r.manifest.FinishedAt = time.Now()
	if runErr == nil {
		r.manifest.Result = "passed"
	} else {
		r.manifest.Result = "failed"
	}
	manifestErr := r.writeManifestLocked()
	r.mu.Unlock()

	var firstErr error
	if manifestErr != nil {
		firstErr = fmt.Errorf("write manifest: %w", manifestErr)
	}

	bundleErr := zipEvidence(r.evidence, r.bundle)
	if bundleErr != nil && firstErr == nil {
		firstErr = fmt.Errorf("create results bundle: %w", bundleErr)
	}

	evo.Println()
	evo.Println("Evidence: " + r.evidence)
	if bundleErr == nil {
		digest, err := fileSHA256(r.bundle)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("hash results bundle: %w", err)
			}
		} else {
			evo.Println("Bundle:   " + r.bundle)
			evo.Println("SHA256:   " + digest)
		}
	}
	return firstErr
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
		name := filepath.ToSlash(filepath.Join(base, rel))

		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = name
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

	closeZipErr := zw.Close()
	closeFileErr := f.Close()
	if walkErr != nil {
		_ = os.Remove(bundle)
		return walkErr
	}
	if closeZipErr != nil {
		_ = os.Remove(bundle)
		return closeZipErr
	}
	if closeFileErr != nil {
		_ = os.Remove(bundle)
		return closeFileErr
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
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

func writeLog(w io.Writer, format string, args ...any) {
	_, _ = io.WriteString(w, fmt.Sprintf(format, args...))
}

func boolPtr(v bool) *bool { return &v }

func smokeConfig() Config {
	return Config{
		Title:     "evor smoke",
		Mode:      modeAuto,
		Heartbeat: "1s",
		Stages: []Stage{{
			Name:      "synthetic child",
			Attempts:  1,
			Preverify: boolPtr(false),
			Work: Command{
				Name:    "synthetic worker",
				Command: []string{"sh", "-c", "echo starting; sleep 6; echo finished"},
				Timeout: "20s",
			},
			Verify: []Command{{
				Name:    "synthetic verifier",
				Command: []string{"sh", "-c", "echo verifier-ok"},
				Timeout: "5s",
			}},
		}},
	}
}
