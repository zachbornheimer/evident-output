package gates_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// miseDepends lists the quoted names in a task's depends array, in order.
func miseDepends(t *testing.T, mise, task string) []string {
	t.Helper()
	body, ok := miseTask(mise, task)
	if !ok {
		t.Fatalf("mise.toml defines no %s task", task)
	}
	_, list, found := strings.Cut(body, "depends = [")
	if !found {
		t.Fatalf("task %s has no depends array", task)
	}
	list, _, _ = strings.Cut(list, "]")
	var names []string
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(list, -1) {
		names = append(names, m[1])
	}
	return names
}

// TestHostedGateIsTheLocalGateWithoutPerf proves the gate GitHub runs proves
// what the local gate does, less only the wall-time bounds recorded on one
// machine: `ci` is `ci-hosted` plus `perf`, and the workflow runs `ci-hosted`.
func TestHostedGateIsTheLocalGateWithoutPerf(t *testing.T) {
	root := repoRoot(t)
	mise, err := os.ReadFile(filepath.Join(root, "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}

	local := miseDepends(t, string(mise), "ci")
	hosted := miseDepends(t, string(mise), "ci-hosted")

	if want := append(slices.Clone(hosted), "perf"); !slices.Equal(local, want) {
		t.Errorf("ci depends on %v, want ci-hosted's %v plus perf", local, hosted)
	}
	if slices.Contains(hosted, "perf") {
		t.Error("ci-hosted depends on perf, whose baseline a hosted runner cannot meet")
	}
	if !regexp.MustCompile(`(?m)^\s+run: mise run ci-hosted$`).Match(workflow) {
		t.Error("ci.yml does not run `mise run ci-hosted`")
	}
	if regexp.MustCompile(`(?m)^\s+run: mise run ci$`).Match(workflow) {
		t.Error("ci.yml runs `mise run ci`, which includes perf")
	}
}
