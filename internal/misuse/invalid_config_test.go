package misuse

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestEveryRefusalSentinelWrapsErrInvalidConfig(t *testing.T) {
	for name, err := range map[string]error{
		"duplicate sibling":  ErrDuplicateSiblingName,
		"declared in a call": ErrDeclaredInCallback,
		"compute and verify": ErrComputeWithVerify,
	} {
		if !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: %v does not wrap ErrInvalidConfig", name, err)
		}
	}
}

func TestInvalidConfigHint(t *testing.T) {
	spec := fmt.Errorf("%w: EffectSpec sets both Path and Resource", ErrInvalidConfig)
	if got := hintFor(spec, "", ""); got != "EffectSpec sets both Path and Resource" {
		t.Errorf("a refused spec's hint = %q, want its own detail", got)
	}
	if got := hintFor(ErrInvalidConfig, "", ""); !strings.Contains(got, "Define each task once") {
		t.Errorf("the bare sentinel's hint = %q, want the After/Verify/Define advice", got)
	}
	if got := hintFor(ErrDeclaredInCallback, "", ""); !strings.Contains(got, "Group.Build") {
		t.Errorf("a declaration from a callback hints %q, want the builder advice", got)
	}
}
