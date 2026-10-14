package graph

import "testing"

func TestPanicErrorNamesARefusedReadWithoutCallingItAPanic(t *testing.T) {
	for _, refused := range []error{ErrComputedUnordered, ErrComputedUnsettled} {
		if got := panicError(refused); got != refused {
			t.Errorf("panicError(%v) = %q, want the refusal itself", refused, got)
		}
	}
	if got := panicError("kaput").Error(); got != "panic: kaput" {
		t.Errorf("panicError(a real panic) = %q, want %q", got, "panic: kaput")
	}
}
