// Command run is the paid pit-of-success eval. A real run needs
// ANTHROPIC_API_KEY, --max-usd, and --confirm-spend; --dry-run prints the plan
// and worst-case cost without any of them.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/eval/driver"
	"github.com/zachbornheimer/evident-output/eval/runner"
	"github.com/zachbornheimer/evident-output/internal/agent/evaltask"
)

const (
	resultsDirMode  = 0o755
	testdataSubpath = "internal/agent/evaltask/testdata"
	cacheSubdir     = ".cache"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
}

func parseConfig(args []string) (runner.Config, error) {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	var models, overrides multiFlag
	cfg := runner.Config{Credential: os.Getenv(runner.CredentialEnv)}
	flags.BoolVar(&cfg.AllReady, "all-ready", false, "run every task the current API can express")
	flags.Var(&models, "model", "model id (repeatable); default "+driver.ModelInnerLoop)
	flags.IntVar(&cfg.Samples, "k", runner.DefaultSamples, "samples per task and model")
	flags.Float64Var(&cfg.MaxUSD, "max-usd", 0, "hard spend cap in USD (required for a real run)")
	flags.BoolVar(&cfg.ConfirmSpend, "confirm-spend", false, "acknowledge that this run spends money")
	flags.BoolVar(&cfg.DryRun, "dry-run", false, "print the plan and worst-case cost, spend nothing")
	flags.Var(&overrides, "price-override", "model=in,out,cache-read,cache-write USD per million tokens (repeatable)")
	flags.StringVar(&cfg.RepoRoot, "repo-root", "..", "evident-output checkout")
	flags.StringVar(&cfg.TasksDir, "tasks-dir", "", "task directory; default the checked-in testdata")
	flags.StringVar(&cfg.ResultsDir, "results-dir", "results", "where transcripts and the docs cache go")
	flags.StringVar(&cfg.MCPBinary, "mcp-binary", "evident-output-mcp", "MCP server binary")
	if err := flags.Parse(args); err != nil {
		return cfg, fmt.Errorf("parse flags: %w", err)
	}
	cfg.TaskIDs = flags.Args()
	cfg.Models = models
	if len(cfg.Models) == 0 {
		cfg.Models = []string{driver.ModelInnerLoop}
	}
	prices, err := priceTable(overrides)
	if err != nil {
		return cfg, fmt.Errorf("build price table: %w", err)
	}
	cfg.Prices = prices
	return cfg, nil
}

func priceTable(overrides []string) (driver.PriceTable, error) {
	parsed := map[string]driver.Price{}
	for _, spec := range overrides {
		model, price, err := driver.ParsePriceOverride(spec)
		if err != nil {
			return nil, fmt.Errorf("read --price-override: %w", err)
		}
		parsed[model] = price
	}
	return driver.PriceTable(driver.PlaceholderPrices).WithOverrides(parsed), nil
}

func run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate flags: %w", err)
	}
	if cfg.RepoRoot, err = filepath.Abs(cfg.RepoRoot); err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	tasks, err := loadTasks(cfg)
	if err != nil {
		return fmt.Errorf("choose tasks: %w", err)
	}
	plan, err := runner.BuildPlan(cfg, tasks)
	if err != nil {
		return fmt.Errorf("plan run: %w", err)
	}
	plan.Print(os.Stdout, cfg.MaxUSD)
	if cfg.DryRun {
		return nil
	}
	if err := execute(cfg, tasks); err != nil {
		return fmt.Errorf("run eval: %w", err)
	}
	return nil
}

func loadTasks(cfg runner.Config) ([]evaltask.Task, error) {
	dir := cfg.TasksDir
	if dir == "" {
		dir = filepath.Join(cfg.RepoRoot, filepath.FromSlash(testdataSubpath))
	}
	all, err := evaltask.LoadTasks(os.DirFS(dir))
	if err != nil {
		return nil, fmt.Errorf("load tasks from %s: %w", dir, err)
	}
	return runner.SelectTasks(all, cfg.TaskIDs, cfg.AllReady)
}

func execute(cfg runner.Config, tasks []evaltask.Task) error {
	ctx := context.Background()
	sha, err := evoSHA(cfg.RepoRoot)
	if err != nil {
		return fmt.Errorf("identify evo commit: %w", err)
	}
	server, err := driver.SpawnMCP(ctx, cfg.MCPBinary)
	if err != nil {
		return fmt.Errorf("start MCP server: %w", err)
	}
	defer server.Close()
	if err := os.MkdirAll(cfg.ResultsDir, resultsDirMode); err != nil {
		return fmt.Errorf("create results directory %s: %w", cfg.ResultsDir, err)
	}
	files := &transcriptFiles{dir: cfg.ResultsDir, sha: sha, now: time.Now()}
	defer files.closeAll()
	live := runner.Runner{
		Grader:   evaltask.Grader{RepoRoot: cfg.RepoRoot, Runner: evaltask.ExecRunner{}},
		Server:   server,
		Cache:    driver.DirBlobStore{Dir: filepath.Join(cfg.ResultsDir, cacheSubdir, sha)},
		NewModel: func(string) driver.Model { return driver.NewAnthropicModel(cfg.Credential) },
		WorkDir:  driver.TempWorkDir,
		SinkFor:  files.sinkFor,
	}
	summary, err := live.Run(ctx, cfg, tasks)
	fmt.Printf("samples=%d passed=%d spent=$%.4f aborted=%v\n", summary.Samples, summary.Passed, summary.SpentUSD, summary.Aborted)
	if err != nil {
		return fmt.Errorf("run samples: %w", err)
	}
	return nil
}

func evoSHA(repoRoot string) (string, error) {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("read evo commit in %s: %w", repoRoot, err)
	}
	return strings.TrimSpace(string(out)), nil
}
