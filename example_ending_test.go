package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// Fail ends a Task Failed with a structured Problem: it ran and broke.
func ExampleFail() {
	var out bytes.Buffer
	result := evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		r.Task("lint").Define(func(context.Context) error {
			return evo.Fail("2 files need formatting", evo.Detail("main.go\nutil.go"))
		})
		return nil
	})
	fmt.Println(result.Conclusion.Outcome, result.ExitCode())
	// Output:
	// failed 2
}

// Refuse ends a Task Refused: a precondition stopped it before any mutation,
// and Next names what the user can do about it.
func ExampleRefuse() {
	var out bytes.Buffer
	result := evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		return evo.Refuse("no remote configured", evo.Next(evo.Action{Label: "add a remote"}))
	})
	fmt.Println(result.Conclusion.Outcome, result.ExitCode())
	// Output:
	// refused 1
}

// Exclude records that policy chose not to run a Task. It folds into the
// run's tally and the run still exits 0.
func ExampleExclude() {
	var out bytes.Buffer
	result := evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		r.Task("prune origin/main").Define(func(context.Context) error {
			return evo.Exclude("protected")
		})
		return nil
	})
	fmt.Println(result.Conclusion.Outcome, result.ExitCode())
	// Output:
	// excluded 0
}

// A Verb names the kind of change an EffectSpec reports. The set is closed:
// Create, Update and Delete.
func ExampleVerb() {
	var out bytes.Buffer
	cfg := quietRun(&out)
	cfg.DryRun = true
	evo.Execute(context.Background(), cfg, func(ctx context.Context, r *evo.Run) error {
		steps := r.Sequence("changes")
		for _, verb := range []evo.Verb{evo.Create, evo.Update, evo.Delete} {
			steps.Task(string(verb) + " branches").Define(func(ctx context.Context) error {
				return evo.Effect(ctx, evo.EffectSpec{Verb: verb, Object: "branch", Quantity: 2}, func(context.Context) error { return nil })
			})
		}
		return nil
	})
	fmt.Print(out.String())
	// Output:
	// [dry-run] no changes will be made
	//
	// ✓ changes
	//    ✓ create branches
	//    ✓ update branches
	//    ✓ delete branches
	//
	// [planned] create branches  create 2 branches
	// [planned] update branches  update 2 branches
	// [planned] delete branches  delete 2 branches
	//
	// [planned]  demo
}

// ask puts one Confirm to a user at a terminal who types reply. It returns
// the decision and the text the terminal kept of the exchange.
func ask(reply, question string, opts ...evo.ConfirmOption) (bool, string) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	cfg := evo.Config{Facades: evo.Facades{Stdin: strings.NewReader(reply), Stdout: io.Discard, Stderr: io.Discard, Terminal: screen}}
	var yes bool
	evo.Execute(context.Background(), cfg, func(ctx context.Context, r *evo.Run) error {
		yes = r.Confirm(question, opts...)
		return nil
	})
	return yes, screen.PersistedText()
}

// A ConfirmOption shapes the question Confirm asks. Detail adds the context
// the user needs to decide.
func ExampleConfirmOption() {
	detail := evo.Detail("40 branches, 3 unmerged")
	for _, reply := range []string{"y\n", "n\n"} {
		yes, _ := ask(reply, "delete 40 branches?", detail)
		fmt.Printf("%q -> %v\n", reply, yes)
	}
	// Output:
	// "y\n" -> true
	// "n\n" -> false
}

// Destructive marks the question as one that cannot be undone, and an empty
// reply still declines.
func ExampleDestructive() {
	yes, shown := ask("\n", "drop the production table?", evo.Destructive())
	prompt, _, _ := strings.Cut(shown, "\n")
	fmt.Println(prompt)
	fmt.Println("confirmed:", yes)
	// Output:
	// ?  drop the production table?  (destructive)  [y/N]
	// confirmed: false
}

// NonInteractive names the flag that answers yes unattended; with no one to
// ask, a declined Confirm is Refused and points the user at it.
func ExampleNonInteractive() {
	var out bytes.Buffer
	result := evo.Execute(context.Background(), quietRun(&out), func(ctx context.Context, r *evo.Run) error {
		r.Confirm("delete 40 branches?", evo.NonInteractive("--yes"))
		return nil
	})
	fmt.Println(result.Conclusion.Outcome)
	fmt.Print(out.String())
	// Output:
	// refused
	// ⊘ delete 40 branches?  blocked by policy
	//
	// [refused]  demo
	// →  demo --yes
}
