package evo_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	evident "github.com/zachbornheimer/evident-output/v2"
)

func TestExecuteContextCauseNamesInterruption(t *testing.T) {
	errStop := errors.New("operator pulled the plug")
	deadline := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 50*time.Millisecond)
	}
	cancelled := func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)
		return ctx, func() {}
	}
	custom := func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancelCause(context.Background())
		time.AfterFunc(50*time.Millisecond, func() { cancel(errStop) })
		return ctx, func() {}
	}
	for name, tc := range map[string]struct {
		ctx  func() (context.Context, context.CancelFunc)
		want string
	}{
		"deadline": {deadline, "deadline exceeded"},
		"cancel":   {cancelled, "context canceled"},
		"cause":    {custom, errStop.Error()},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, stop := tc.ctx()
			defer stop()
			res := evident.Execute(ctx, evident.Config{}, func(ctx context.Context, _ *evident.Run) error {
				<-ctx.Done()
				return nil
			})
			got := res.Conclusion.Explanation
			if !strings.Contains(got, tc.want) || strings.Contains(got, "by user") {
				t.Fatalf("Explanation = %q, want it to name %q and not %q", got, tc.want, "by user")
			}
		})
	}
}
