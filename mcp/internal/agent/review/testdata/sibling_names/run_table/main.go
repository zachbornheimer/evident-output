package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type scenario struct {
	name string
	cfg  evo.Config
	body evo.RunFunc
}

func ok(context.Context) error { return nil }

func main() {
	scenarios := []scenario{
		{"duplicate sibling", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			r.Task("a").Define(ok)
			r.Task("a").Define(ok)
			return nil
		}},
		{"effect quantity zero", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			r.Task("e").Define(func(ctx context.Context) error {
				return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "branch"}, ok)
			})
			return nil
		}},
		{"declare inside define", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			r.Task("outer").Define(func(ctx context.Context) error {
				r.Task("inner").Define(ok)
				return nil
			})
			return nil
		}},
		{"compute with verify", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			evo.Compute(r.Task("c").Verify(func(context.Context) (bool, error) { return false, nil }), func(context.Context) (int, error) { return 1, nil })
			return nil
		}},
		{"unordered get", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			c := evo.Compute(r.Task("c"), func(context.Context) (int, error) { return 1, nil })
			r.Task("reader").Define(func(context.Context) error { _ = c.Get(); return nil })
			return nil
		}},
		{"slog info default", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			slog.New(r.SlogHandler()).Info("info record", "k", "v")
			slog.New(r.SlogHandler()).Debug("debug record")
			r.Task("t").Define(ok)
			return nil
		}},
		{"after group with excluded", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			g := r.Group("g")
			g.Task("x").Define(func(context.Context) error { return evo.Exclude("policy") })
			g.Task("y").Define(ok)
			r.Task("dep").After(g).Define(ok)
			x := r.Task("direct-excluded").Define(func(context.Context) error { return evo.Exclude("no") })
			r.Task("dep2").After(x).Define(ok)
			return nil
		}},
		{"exec no outputs twice", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			r.Task("echo").Define(func(ctx context.Context) error {
				if _, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"true"}}); err != nil {
					return fmt.Errorf("true: %w", err)
				}
				return nil
			})
			return nil
		}},
		{"body error wrapped refuse", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			r.Task("t").Define(ok)
			return fmt.Errorf("wrap: %w", evo.Refuse("no remote"))
		}},
		{"get after verify-less fail", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			c := evo.Compute(r.Task("c"), func(context.Context) (int, error) { return 0, errors.New("boom") })
			r.Task("reader").After(c).Define(func(context.Context) error { _ = c.Get(); return nil })
			return nil
		}},
		{"dry run effect", evo.Config{DryRun: true}, func(ctx context.Context, r *evo.Run) error {
			r.Task("e").Define(func(ctx context.Context) error {
				return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Create, Object: "tag", Quantity: 2}, func(context.Context) error { panic("ran in dry run") })
			})
			return nil
		}},
		{"summary not called name only", evo.Config{Name: "tool x"}, func(ctx context.Context, r *evo.Run) error {
			r.Task("t").Define(ok)
			return nil
		}},
		{"refuse plus misuse", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			r.Task("t").Define(func(context.Context) error { return evo.Refuse("stop") })
			r.Task("t").Define(ok)
			return nil
		}},
		{"verb string", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			r.Task("v").Define(func(ctx context.Context) error {
				return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Verb("install"), Object: "pkg", Quantity: 1}, ok)
			})
			return nil
		}},
		{"path and resource", evo.Config{}, func(ctx context.Context, r *evo.Run) error {
			r.Task("v").Define(func(ctx context.Context) error {
				return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Update, Object: "x", Quantity: 1, Path: "/tmp/a", Resource: "db"}, func(context.Context) error { panic("ran") })
			})
			return nil
		}},
	}
	for _, s := range scenarios {
		var out, errb bytes.Buffer
		s.cfg.Output = evo.Plain
		s.cfg.State = evo.State{Dir: "state"}
		s.cfg.Facades = evo.Facades{Stdout: &out, Stderr: &errb}
		res := evo.Execute(context.Background(), s.cfg, s.body)
		fmt.Printf("##### %s\nexit=%d outcome=%v explanation=%q err=%v invalidConfig=%v unordered=%v\n%s", s.name, res.ExitCode(), res.Conclusion.Outcome, res.Conclusion.Explanation, res.Err, errors.Is(res.Err, evo.ErrInvalidConfig), errors.Is(res.Err, evo.ErrComputedUnordered), errb.String())
	}
}
