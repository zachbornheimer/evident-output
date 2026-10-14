package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

type scopeOwner struct{ name string }

type otherScopeOwner struct{}

// TestScopeOfBareContextReturnsErrNoTaskContext proves §7.1: a context that
// never came from a Define callback reports ErrNoTaskContext.
func TestScopeOfBareContextReturnsErrNoTaskContext(t *testing.T) {
	var noContext context.Context
	for name, ctx := range map[string]context.Context{"background": context.Background(), "nil": noContext} {
		if _, err := ScopeOf[*scopeOwner](ctx); !errors.Is(err, ErrNoTaskContext) {
			t.Errorf("ScopeOf(%s) = %v, want ErrNoTaskContext", name, err)
		}
	}
}

// TestScopeOfInsideCallbackReturnsTheTaskAndItsOwner proves §7.1: inside a
// Define callback the scope names the Task actually running and hands back
// the owner the operations act through.
func TestScopeOfInsideCallbackReturnsTheTaskAndItsOwner(t *testing.T) {
	owner := &scopeOwner{name: "run"}
	ctx := WithScope(context.Background(), NewScope(New(record.NewRun()), "t1", owner))

	scope, err := ScopeOf[*scopeOwner](ctx)
	if err != nil {
		t.Fatalf("ScopeOf inside the callback = %v, want nil", err)
	}
	if scope.Task() != "t1" || scope.Owner() != owner {
		t.Errorf("scope = (%q, %p), want (t1, %p)", scope.Task(), scope.Owner(), owner)
	}
}

// TestScopeOfAfterCloseReturnsErrTaskClosed proves §7.1: a context captured
// during a Define callback and reused after the callback returned reports
// ErrTaskClosed. The callback's execution window is what "open" means, not
// the Task's lifetime.
func TestScopeOfAfterCloseReturnsErrTaskClosed(t *testing.T) {
	scope := NewScope(New(record.NewRun()), "t1", &scopeOwner{})
	ctx := WithScope(context.Background(), scope)

	scope.Close()

	if _, err := ScopeOf[*scopeOwner](ctx); !errors.Is(err, ErrTaskClosed) {
		t.Fatalf("ScopeOf after Close = %v, want ErrTaskClosed", err)
	}
}

// TestScopeOfIgnoresAScopeOfAnotherOwnerType proves a context scoped for one
// kind of owner is not mistaken for another's.
func TestScopeOfIgnoresAScopeOfAnotherOwnerType(t *testing.T) {
	ctx := WithScope(context.Background(), NewScope(New(record.NewRun()), "t1", otherScopeOwner{}))

	if _, err := ScopeOf[*scopeOwner](ctx); !errors.Is(err, ErrNoTaskContext) {
		t.Fatalf("ScopeOf for a different owner type = %v, want ErrNoTaskContext", err)
	}
}

// TestBeginOperationRefusesADoneContextAndTellsTheSink proves a cancelled Run
// never reads as silent success whichever operation the caller used: the
// refusal wraps the context's error and reaches the MisuseSink.
func TestBeginOperationRefusesADoneContextAndTellsTheSink(t *testing.T) {
	sink := &misuseLog{}
	g := New(record.NewRun(), WithMisuseSink(sink))
	scoped := WithScope(context.Background(), NewScope(g, "t1", &scopeOwner{}))
	ctx, cancel := context.WithCancel(scoped)
	cancel()

	scope, err := BeginOperation[*scopeOwner](ctx, `File "a.txt"`)

	if scope != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("BeginOperation on a done context = (%v, %v), want (nil, context.Canceled)", scope, err)
	}
	if len(sink.errs) != 1 || !errors.Is(sink.errs[0], context.Canceled) {
		t.Errorf("sink heard %v, want one error wrapping context.Canceled", sink.errs)
	}
}

// TestBeginOperationAdmitsALiveContext proves the happy path returns the
// scope and tells nobody.
func TestBeginOperationAdmitsALiveContext(t *testing.T) {
	sink := &misuseLog{}
	g := New(record.NewRun(), WithMisuseSink(sink))
	ctx := WithScope(context.Background(), NewScope(g, "t1", &scopeOwner{}))

	scope, err := BeginOperation[*scopeOwner](ctx, "Patch")

	if err != nil || scope == nil || scope.Task() != "t1" {
		t.Fatalf("BeginOperation = (%v, %v), want the t1 scope", scope, err)
	}
	if len(sink.errs) != 0 {
		t.Errorf("sink heard %v, want nothing", sink.errs)
	}
}
