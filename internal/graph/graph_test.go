package graph

import (
	"log"
	"sync"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

func newGraph() *Graph { return New(record.NewRun()) }

func TestStableKeyIsKindParentKeyAndName(t *testing.T) {
	g := newGraph()
	root := g.AddContainer(nil, "deploy", false)
	child := must(g.AddTask(root, "build", record.TaskInit{}))
	nested := g.AddContainer(root, "tests", true)

	if got, want := root.Key(), "group:/deploy"; got != want {
		t.Errorf("root key = %q, want %q", got, want)
	}
	if got, want := child.Key(), "task:group:/deploy/build"; got != want {
		t.Errorf("task key = %q, want %q", got, want)
	}
	if got, want := nested.Key(), "sequence:group:/deploy/tests"; got != want {
		t.Errorf("nested key = %q, want %q", got, want)
	}
	if KeyOf(nil) != "" {
		t.Error("the root has a key")
	}
}

func TestDeclaredNameStripsControlCharactersSoSiblingsCannotDiverge(t *testing.T) {
	if DeclaredName("deploy\x01") != DeclaredName("deploy\x02") {
		t.Error("two names that render alike normalize differently")
	}
}

func TestAddTaskNumbersIDsAndDeclarationsInOrder(t *testing.T) {
	g := newGraph()
	a := must(g.AddTask(nil, "a", record.TaskInit{}))
	b := must(g.AddTask(nil, "b", record.TaskInit{}))
	c := g.AddContainer(nil, "c", false)

	if a.ID != "task_1" || b.ID != "task_2" || c.ID != "tasks_3" {
		t.Errorf("ids = %s %s %s, want task_1 task_2 tasks_3", a.ID, b.ID, c.ID)
	}
	if a.Declaration != 1 || b.Declaration != 2 || c.Declaration != 3 || g.Declarations() != 3 {
		t.Errorf("declarations = %d %d %d (total %d), want 1 2 3 (3)", a.Declaration, b.Declaration, c.Declaration, g.Declarations())
	}
	if g.NextDeclaration() != 4 {
		t.Error("NextDeclaration did not continue the numbering")
	}
	if g.Task(a.ID) != a || g.Container(c.ID) != c || g.Task("missing") != nil || g.Container(a.ID) != nil {
		t.Error("registry lookup returned the wrong node")
	}
}

func TestAddTaskStartsItsRecordUnderItsID(t *testing.T) {
	task := must(newGraph().AddTask(nil, "a", record.TaskInit{State: record.Pending}))
	if task.Rec.ID() != record.TaskID(task.ID) || task.Rec.State() != record.Pending {
		t.Errorf("record = id %q state %q, want id %q pending", task.Rec.ID(), task.Rec.State(), task.ID)
	}
}

func TestTasksAndContainersAreSeparateNamespaces(t *testing.T) {
	g := newGraph()
	g.ClaimName(nil, KindTask, "x")
	g.ClaimName(nil, KindGroup, "y")

	if !g.NameTaken(nil, KindTask, "x") || g.NameTaken(nil, KindGroup, "x") {
		t.Error("a Task name leaked into the container namespace")
	}
	if !g.NameTaken(nil, KindGroup, "y") || !g.NameTaken(nil, KindSequence, "y") || g.NameTaken(nil, KindTask, "y") {
		t.Error("a Group and a Sequence must not share a name, nor a Task share with either")
	}
}

func TestNamesAreClaimedPerParent(t *testing.T) {
	g := newGraph()
	left := g.AddContainer(nil, "left", false)
	right := g.AddContainer(nil, "right", false)
	g.ClaimName(left, KindTask, "same")

	if !g.NameTaken(left, KindTask, "same") {
		t.Error("claimed name not taken under its parent")
	}
	if g.NameTaken(right, KindTask, "same") || g.NameTaken(nil, KindTask, "same") {
		t.Error("claimed name taken under another parent")
	}
}

func TestAddTaskDoesNotClaimItsName(t *testing.T) {
	g := newGraph()
	must(g.AddTask(nil, "a", record.TaskInit{}))
	if g.NameTaken(nil, KindTask, "a") {
		t.Error("AddTask claimed the name; a refused declaration must leave it free")
	}
}

func TestRekeyReplacesReportsUnchangedAndRefusesATakenKey(t *testing.T) {
	g := newGraph()
	a := must(g.AddTask(nil, "a", record.TaskInit{}))
	b := must(g.AddTask(nil, "b", record.TaskInit{}))

	if got := g.Rekey(a, "platform-a"); got != KeyReplaced || a.Key() != "platform-a" {
		t.Errorf("Rekey(a) = %v key %q, want replaced platform-a", got, a.Key())
	}
	if got := g.Rekey(a, "platform-a"); got != KeyUnchanged {
		t.Errorf("repeating the key = %v, want unchanged", got)
	}
	if got := g.Rekey(b, "platform-a"); got != KeyTaken || b.Key() != "task:/b" {
		t.Errorf("Rekey(b) onto a taken key = %v key %q, want taken and untouched", got, b.Key())
	}
	g.Rekey(a, "platform-a2")
	if got := g.Rekey(b, "platform-a"); got != KeyReplaced {
		t.Errorf("a released key = %v, want it free again", got)
	}
}

func TestGraphIsSafeForConcurrentDeclaration(t *testing.T) {
	g := newGraph()
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 50 {
				task := must(g.AddTask(nil, "t", record.TaskInit{}))
				g.Rekey(task, task.ID)
				_ = task.Key()
				g.ClaimName(nil, KindTask, task.ID)
			}
		})
	}
	wg.Wait()
	if g.Declarations() != 16*50 {
		t.Errorf("declarations = %d, want %d", g.Declarations(), 16*50)
	}
}

// must is the Task AddTask returned; a test graph never refuses one.
func must(t *Task, err error) *Task {
	if err != nil {
		log.Fatalf("AddTask refused a Task: %v", err)
	}
	return t
}
