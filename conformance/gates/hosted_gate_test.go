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

// workflowGateTasks lists every `mise run X` target the workflow's gate job
// invokes, in order.
func workflowGateTasks(workflow string) []string {
	var tasks []string
	for _, m := range regexp.MustCompile(`(?m)^\s+run: mise run (\S+)\s*$`).FindAllStringSubmatch(workflow, -1) {
		tasks = append(tasks, m[1])
	}
	return tasks
}

// TestHostedGateIsTheLocalGateWithoutPerf proves the gate GitHub runs proves
// what the local gate does, less only the wall-time bounds recorded on one
// machine: `ci` is `ci-hosted` plus `perf`, and every task the workflow's
// gate job runs is `ci-hosted` or one of its dependencies, so a step GitHub
// fails cannot pass locally.
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

	gate, _, _ := strings.Cut(string(workflow), "\n  min-go:")
	invoked := workflowGateTasks(gate)
	if !slices.Contains(invoked, "ci-hosted") {
		t.Error("ci.yml gate job does not run `mise run ci-hosted`")
	}
	for _, task := range invoked {
		if task != "ci-hosted" && !slices.Contains(hosted, task) {
			t.Errorf("ci.yml gate job runs `mise run %s`, which ci-hosted (so ci) does not: %v", task, hosted)
		}
		if task == "ci" || task == "perf" {
			t.Errorf("ci.yml gate job runs `mise run %s`, whose perf baseline a hosted runner cannot meet", task)
		}
	}
}
