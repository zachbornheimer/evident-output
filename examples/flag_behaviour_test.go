package examples_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// tmpDirArg in a pin's args expands to a fresh directory, so an example that
// persists state or writes a working tree never touches a shared path.
const tmpDirArg = "{tmp}"

// fastFlag is the flag every timed example declares to shorten its sleeps.
// Every pin runs with it, so the table leaves it out and the runner appends
// it; that run is the --fast pin, and the declared-flag check counts it.
const fastFlag = "--fast"

// flagPin is one invocation of one example and the behaviour it must have.
// The exit code is the contract: 0 Succeeded, Satisfied or Excluded, 1 Refused,
// 2 Failed (usage errors included), 130 Cancelled (CONTRACT section 4).
type flagPin struct {
	example string
	args    []string
	exit    int
	// stdoutEmpty pins that nothing reached stdout: a payload only a
	// successful run may write.
	stdoutEmpty bool
	// contains and absent are checked against stdout and stderr together.
	contains []string
	absent   []string
	// twice runs the invocation again and judges the second run.
	twice bool
}

// flagPins is every documented flag and its exit code. The meta checks in
// TestExamplesPinDocumentedFlagBehaviour keep this table equal to the flags the
// examples declare and the invocations their doc comments show.
var flagPins = []flagPin{
	{example: "print"},
	{example: "verbose", absent: []string{"registry mirror"}},
	{example: "verbose", args: []string{"--verbose"}, contains: []string{"registry mirror"}},

	// repo-status refuses while the repository has blockers: exit 1, not 0.
	{example: "repo-status", exit: 1, contains: []string{"2 branches need attention", "[refused]  bpp-csharp", "git branch -vv"}, absent: []string{"HEAD"}},
	{example: "repo-status", args: []string{"--clean"}},
	{example: "repo-status", args: []string{"--name", "my-repo"}, exit: 1, contains: []string{"[refused]  my-repo"}},
	{example: "repo-status", args: []string{"--verbose"}, exit: 1, contains: []string{"HEAD"}},
	{example: "repo-status", args: []string{"--color", "never"}, exit: 1, absent: []string{"\x1b["}},
	{example: "repo-status", args: []string{"--color", "always"}, exit: 1, contains: []string{"\x1b["}},
	{example: "repo-status", args: []string{"--color", "sometimes"}, exit: 2},

	{example: "install-pipeline"},
	{example: "install-pipeline", args: []string{"--fail-tests"}, exit: 2},

	// migrate plans by default and commits only with --apply. A failed
	// backup fails the run and the schema steps never start.
	{example: "migrate", contains: []string{"[planned]"}, absent: []string{"[changed]"}},
	{example: "migrate", args: []string{"--apply"}, contains: []string{"[changed]"}},
	{example: "migrate", args: []string{"--fail"}, contains: []string{"[planned]"}},
	{
		example: "migrate", args: []string{"--apply", "--fail"}, exit: 2,
		contains: []string{"backup failed", "partial"}, absent: []string{"[changed]", "email column created"},
	},

	// doctor reports findings and exits 0; --strict makes them blockers.
	{example: "doctor", contains: []string{"cannot connect to docker socket", "[succeeded]"}},
	{example: "doctor", args: []string{"--strict"}, exit: 1, contains: []string{"[refused]"}},
	{example: "doctor", args: []string{"--verbose"}, contains: []string{"probe interval"}},
	{example: "doctor", args: []string{"--json"}, contains: []string{`"evo.run"`}},
	{example: "doctor", args: []string{"--json", "--strict"}, exit: 1, contains: []string{`"evo.run"`}},

	// data-command: the payload is the only thing on stdout, and only a
	// successful link writes it.
	{example: "data-command", contains: []string{`"artifact"`}},
	{example: "data-command", args: []string{"--pretty=false"}, contains: []string{`{"artifact":"bin/app"`}},
	{example: "data-command", args: []string{"--fail-link"}, exit: 2, stdoutEmpty: true},

	{example: "named-tasks"},
	{example: "many-tasks"},
	{example: "many-tasks", args: []string{"--tasks", "20", "--hold", "1ms", "--late", "5ms"}},
	{example: "live-progress"},
	{example: "quiet-writer"},
	{example: "quiet-writer", args: []string{"--quiet", "10ms"}},
	{example: "debug-history", contains: []string{"[DEBUG] working tree scanned"}},
	{example: "debug-pane"},
	{example: "debug-pane", args: []string{"--fail"}, exit: 1, contains: []string{"[refused"}},
	{example: "terminal-driver"},
	{example: "terminal-driver", args: []string{"--frames"}, contains: []string{"── frame 1 ──"}},
	{example: "terminal-driver", args: []string{"--step"}, contains: []string{"── frame 1 ──"}},

	// The state-dir examples persist freshness: the second run finds both
	// stages current and spawns neither.
	{
		example: "generate-pipeline", args: []string{"--state-dir", tmpDirArg, "--work-dir", tmpDirArg},
		twice: true, contains: []string{"[satisfied]"}, absent: []string{"[changed]"},
	},
	{
		example: "launch-agent-file", args: []string{"--state-dir", tmpDirArg},
		twice: true, contains: []string{"[satisfied]"}, absent: []string{"[changed]"},
	},
}

// TestExamplesPinDocumentedFlagBehaviour runs every documented flag of every
// example and requires the exit code the contract assigns the scenario it
// demonstrates, so an example cannot teach "exit 0" for a run that refused or
// failed. Two meta checks keep the table honest: every flag an example
// declares has a pin, and every invocation an example's doc comment shows has
// a pin.
func TestExamplesPinDocumentedFlagBehaviour(t *testing.T) {
	root := findRepoRoot(t)
	examplesDir := filepath.Join(root, "examples")
	declared := declaredFlagsByExample(t, examplesDir)

	t.Run("every declared flag is pinned", func(t *testing.T) {
		pinned := pinnedFlagsByExample()
		for example, flags := range declared {
			for _, name := range flags {
				if name != strings.TrimPrefix(fastFlag, "--") && !slices.Contains(pinned[example], name) {
					t.Errorf("%s declares --%s but no pin exercises it", example, name)
				}
			}
		}
		for example, names := range pinned {
			for _, name := range names {
				if !slices.Contains(declared[example], name) {
					t.Errorf("%s pin uses --%s, which the example does not declare", example, name)
				}
			}
		}
	})

	t.Run("every documented invocation is pinned", func(t *testing.T) {
		for example, invocations := range documentedInvocations(t, examplesDir) {
			for _, args := range invocations {
				if !hasPinFor(example, args) {
					t.Errorf("%s doc comment shows `%s` but no pin runs it", example, strings.Join(args, " "))
				}
			}
		}
	})

	t.Run("every Exec that declares Outputs declares Inputs", func(t *testing.T) {
		for _, at := range execSpecsWithoutInputs(t, examplesDir) {
			t.Errorf("ExecSpec at %s declares Outputs but no Inputs, so freshness cannot cascade", at)
		}
	})

	bins := buildAllExamples(t, examplesDir, declared)
	for _, p := range flagPins {
		t.Run(p.label(), func(t *testing.T) {
			t.Parallel()
			runPin(t, p, bins[p.example], slices.Contains(declared[p.example], "fast"))
		})
	}
}

func (p flagPin) label() string {
	if len(p.args) == 0 {
		return p.example + "/default"
	}
	return p.example + "/" + strings.Join(p.args, "_")
}

func runPin(t *testing.T, p flagPin, bin string, hasFast bool) {
	t.Helper()
	args := expandTmp(t, p.args)
	if hasFast && !slices.Contains(args, fastFlag) {
		args = append(args, fastFlag)
	}
	stateHome := t.TempDir()
	var stdout, stderr bytes.Buffer
	for run := 1; ; run++ {
		stdout.Reset()
		stderr.Reset()
		cmd := exec.Command(bin, args...)
		cmd.Env = append(cleanEnv(), "NO_COLOR=1", "XDG_STATE_HOME="+stateHome, "XDG_CACHE_HOME="+stateHome)
		if slices.Contains(p.args, "always") {
			cmd.Env = append(cmd.Env, "FORCE_COLOR=1")
			cmd.Env = slices.DeleteFunc(cmd.Env, func(e string) bool { return e == "NO_COLOR=1" })
		}
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := exitCode(t, cmd.Run())
		combined := stdout.String() + stderr.String()
		if code != p.exit {
			t.Fatalf("run %d: exit %d, want %d\nstdout:\n%s\nstderr:\n%s", run, code, p.exit, stdout.String(), stderr.String())
		}
		if !p.twice || run == 2 {
			checkPinOutput(t, p, stdout.String(), combined)
			return
		}
	}
}

func checkPinOutput(t *testing.T, p flagPin, stdout, combined string) {
	t.Helper()
	if p.stdoutEmpty && stdout != "" {
		t.Errorf("stdout must be empty, got:\n%s", stdout)
	}
	for _, want := range p.contains {
		if !strings.Contains(combined, want) {
			t.Errorf("output lacks %q\n%s", want, combined)
		}
	}
	for _, bad := range p.absent {
		if strings.Contains(combined, bad) {
			t.Errorf("output must not contain %q\n%s", bad, combined)
		}
	}
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run: %v", err)
	}
	return ee.ExitCode()
}

// cleanEnv is the process environment without the variables that select an
// encoding or colour, so each pin judges the example's own defaults.
func cleanEnv() []string {
	drop := []string{"EVO_", "NO_COLOR=", "FORCE_COLOR="}
	return slices.DeleteFunc(os.Environ(), func(e string) bool {
		for _, prefix := range drop {
			if strings.HasPrefix(e, prefix) {
				return true
			}
		}
		return false
	})
}

func expandTmp(t *testing.T, args []string) []string {
	t.Helper()
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = a
		if a == tmpDirArg {
			out[i] = t.TempDir()
		}
	}
	return out
}

// buildAllExamples compiles every example with one go command and returns
// the binary path of each, keyed by example name.
func buildAllExamples(t *testing.T, examplesDir string, declared map[string][]string) map[string]string {
	t.Helper()
	outDir := t.TempDir()
	build := newCommand("go", "build", "-buildvcs=false", "-o", outDir+string(filepath.Separator), "./...")
	build.Dir = examplesDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build examples: %v\n%s", err, out)
	}
	bins := make(map[string]string, len(declared))
	for example := range declared {
		bins[example] = filepath.Join(outDir, example)
	}
	return bins
}

// flagConstructors are the flag package functions whose first argument is
// the flag's name.
var flagConstructors = []string{"Bool", "String", "Int", "Int64", "Uint", "Float64", "Duration", "Func"}

// declaredFlagsByExample reads each example's main.go and returns the names
// of the flags it declares: the example's own flag documentation.
func declaredFlagsByExample(t *testing.T, examplesDir string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	forEachExample(t, examplesDir, func(name string, _ *token.FileSet, file *ast.File) {
		out[name] = nil
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !isIdent(sel.X, "flag") || !slices.Contains(flagConstructors, sel.Sel.Name) {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				out[name] = append(out[name], strings.Trim(lit.Value, `"`))
			}
			return true
		})
	})
	return out
}

// documentedInvocations returns, per example, the arguments of every
// `go run ./examples/<name>/ ARGS` line in the example's package doc.
func documentedInvocations(t *testing.T, examplesDir string) map[string][][]string {
	t.Helper()
	out := map[string][][]string{}
	forEachExample(t, examplesDir, func(name string, _ *token.FileSet, file *ast.File) {
		if file.Doc == nil {
			return
		}
		marker := "go run ./examples/" + name
		for line := range strings.SplitSeq(file.Doc.Text(), "\n") {
			_, rest, found := strings.Cut(line, marker)
			if !found {
				continue
			}
			rest = strings.TrimPrefix(rest, "/")
			for _, stop := range []string{"#", "|", "2>"} {
				rest, _, _ = strings.Cut(rest, stop)
			}
			out[name] = append(out[name], strings.Fields(rest))
		}
	})
	return out
}

func forEachExample(t *testing.T, examplesDir string, visit func(name string, fset *token.FileSet, file *ast.File)) {
	t.Helper()
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("read %s: %v", examplesDir, err)
	}
	for _, entry := range entries {
		src := filepath.Join(examplesDir, entry.Name(), "main.go")
		if !entry.IsDir() || entry.Name() == "internal" {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, src, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", src, err)
		}
		visit(entry.Name(), fset, file)
	}
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// pinnedFlagsByExample lists the flag names each example's pins exercise.
func pinnedFlagsByExample() map[string][]string {
	out := map[string][]string{}
	for _, p := range flagPins {
		out[p.example] = append(out[p.example], flagNames(p.args)...)
	}
	return out
}

func flagNames(args []string) []string {
	var names []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimLeft(a, "-"), "=")
		names = append(names, name)
	}
	return names
}

// hasPinFor reports whether a pin runs the example with the documented
// arguments, ignoring the flags every run adds or isolates: --fast and any
// flag whose value is a per-run temporary directory.
func hasPinFor(example string, documented []string) bool {
	want := comparableArgs(documented)
	for _, p := range flagPins {
		if p.example == example && slices.Equal(comparableArgs(p.args), want) {
			return true
		}
	}
	return false
}

func comparableArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == fastFlag:
		case i+1 < len(args) && args[i+1] == tmpDirArg:
			i++
		default:
			out = append(out, args[i])
		}
	}
	return out
}

// execSpecsWithoutInputs returns the position of each evo.ExecSpec literal
// that sets Outputs without Inputs: with no Inputs, nothing says what the
// Outputs derive from, so the cascade the example claims would not exist.
func execSpecsWithoutInputs(t *testing.T, examplesDir string) []string {
	t.Helper()
	var out []string
	forEachExample(t, examplesDir, func(_ string, fset *token.FileSet, file *ast.File) {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isSelector(lit.Type, "evo", "ExecSpec") {
				return true
			}
			if hasField(lit, "Outputs") && !hasField(lit, "Inputs") {
				out = append(out, fset.Position(lit.Pos()).String())
			}
			return true
		})
	})
	return out
}

func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && isIdent(sel.X, pkg) && sel.Sel.Name == name
}

func hasField(lit *ast.CompositeLit, field string) bool {
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok && isIdent(kv.Key, field) {
			return true
		}
	}
	return false
}
