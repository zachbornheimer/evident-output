// Package runner runs task x model x k samples through the driver, grades
// each submission with the deterministic grader, and records a transcript.
package runner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/eval/driver"
)

// DefaultSamples is k, the samples per task and model.
const DefaultSamples = 3

// Names of the guard flags and credential variable, shared with the messages
// that demand them.
const (
	FlagMaxUSD       = "--max-usd"
	FlagConfirmSpend = "--confirm-spend"
	FlagPriceMissing = "--price-override"
	// CredentialEnv names the environment variable the credential is read from.
	CredentialEnv = "ANTHROPIC_API_KEY"
)

// ErrRefusedToStart means a guard flag or credential is missing; nothing was
// sent anywhere.
var ErrRefusedToStart = errors.New("refusing to start")

// Config is one run's settings.
type Config struct {
	TaskIDs      []string
	AllReady     bool
	Models       []string
	Samples      int
	MaxUSD       float64
	ConfirmSpend bool
	DryRun       bool
	// Credential is the value read from CredentialEnv.
	Credential string
	Prices     driver.PriceTable
	ResultsDir string
	TasksDir   string
	MCPBinary  string
	RepoRoot   string
}

// Validate refuses a real run that lacks the credential, a positive
// --max-usd, or --confirm-spend. A dry run spends nothing, so only a sane
// plan is needed.
func (c Config) Validate() error {
	var missing []string
	if len(c.TaskIDs) == 0 && !c.AllReady {
		missing = append(missing, "tasks: pass task ids or --all-ready")
	}
	if len(c.Models) == 0 {
		missing = append(missing, "models: pass at least one --model")
	}
	if c.Samples <= 0 {
		missing = append(missing, fmt.Sprintf("samples: must be positive, got %d", c.Samples))
	}
	if !c.DryRun {
		missing = append(missing, c.missingSpendGuards()...)
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrRefusedToStart, strings.Join(missing, "; "))
	}
	return nil
}

func (c Config) missingSpendGuards() []string {
	var missing []string
	if c.Credential == "" {
		missing = append(missing, CredentialEnv+" is not set")
	}
	if c.MaxUSD <= 0 {
		missing = append(missing, FlagMaxUSD+" is required and must be positive")
	}
	if !c.ConfirmSpend {
		missing = append(missing, FlagConfirmSpend+" is required for a run that spends money")
	}
	return missing
}
