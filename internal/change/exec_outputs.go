// This file owns what Exec runs and checks around the child: resolving the
// executable, merging its environment, and verifying its declared Outputs
// after a zero exit.

package change

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	sysfs "github.com/zachbornheimer/evident-output/v2/internal/fs"
	"github.com/zachbornheimer/evident-output/v2/internal/process"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// SpawnCommand is one resolved command Exec asks its Host to spawn.
type SpawnCommand struct {
	// Path is the resolved executable.
	Path string
	// Args are passed to the child literally.
	Args []string
	// Dir is the child's working directory.
	Dir string
	// Env is the child's complete environment, in order.
	Env []string
}

// resolveExecutable resolves ExecSpec.Executable to an absolute path: a name
// containing a path separator resolves against dir like a shell would (spec
// §8.4 does not PATH-search an explicit path); a bare name is resolved on
// PATH through the process facade.
func resolveExecutable(executable, dir string) (string, error) {
	if process.IsExplicitPath(executable) {
		return resolvePathAgainst(dir, executable), nil
	}
	resolved, lookErr := process.LookPath(executable)
	if lookErr != nil {
		return "", fmt.Errorf("%w: %s", ErrExecExecutableNotFound, executable)
	}
	return resolved, nil
}

// mergedExecEnv merges ExecSpec.Env onto the process's own environment,
// later entries overriding a repeated name (spec §8.4) — returned sorted so
// the child's own env order is deterministic across runs.
func mergedExecEnv(overrides map[string]string) []string {
	base := process.Environ()
	if len(overrides) == 0 {
		return base
	}
	merged := make(map[string]string, len(base)+len(overrides))
	for _, kv := range base {
		if before, after, ok := strings.Cut(kv, "="); ok {
			merged[before] = after
		}
	}
	maps.Copy(merged, overrides)
	env := make([]string, 0, len(merged))
	for _, name := range slices.Sorted(maps.Keys(merged)) {
		env = append(env, name+"="+merged[name])
	}
	return env
}

// observeVerifiedExecOutputs wraps verifiedExecOutputs with one
// tracked_resource.observed event per declared output (schema/event.v2.json), mirroring
// File's single tracked_resource.observed for its one managed path — Exec
// has as many tracked resources as it has declared Outputs.
func (r *Run) observeVerifiedExecOutputs(ctx context.Context, id record.TaskID, outputs []string) ([]freshness.OutputRecord, error) {
	for _, out := range outputs {
		_, statErr := sysfs.Stat(out)
		r.host.Observe(Observation{Task: id, Kind: ResourceObserved, Subject: Subject{Kind: SubjectExecOutput, Name: out}, Exists: statErr == nil})
	}
	return verifiedExecOutputs(ctx, outputs)
}

// verifiedExecOutputs re-inspects every declared output after a successful
// (exit 0) run (spec §8.4: "after exit 0 every declared output must exist
// and fingerprint, else verification failure") and returns their tracked
// output records.
func verifiedExecOutputs(ctx context.Context, outputs []string) ([]freshness.OutputRecord, error) {
	records := make([]freshness.OutputRecord, len(outputs))
	for i, out := range outputs {
		if _, statErr := sysfs.Stat(out); statErr != nil {
			return nil, fmt.Errorf("%w: %s", ErrExecOutputMissingAfterSuccess, out)
		}
		digest, digestErr := freshness.PathOutputDigest(ctx, out)
		if digestErr != nil {
			return nil, fmt.Errorf("evo: Exec output %q: %w", out, digestErr)
		}
		records[i] = freshness.OutputRecord{Kind: freshness.OutputKindExec, Path: out, Digest: digest}
	}
	return records, nil
}
