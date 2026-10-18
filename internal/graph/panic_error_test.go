package graph

import "testing"

func TestPanicErrorNamesARefusedReadWithoutCallingItAPanic(t *testing.T) {
	for _, refused := range []error{ErrComputedUnordered, ErrComputedUnsettled} {
		if got := PanicError(refused); got != refused {
			t.Errorf("PanicError(%v) = %q, want the refusal itself", refused, got)
		}
	}
	if got := PanicError("kaput").Error(); got != "panic: kaput" {
		t.Errorf("PanicError(a real panic) = %q, want %q", got, "panic: kaput")
	}
}
