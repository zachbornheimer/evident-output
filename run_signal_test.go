//go:build unix

package evo_test

import (
	"bytes"
	"os"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func TestMain_SIGINTCancelsActiveTaskAndExits130(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))
	task := evo.Task("work")
	started := make(chan struct{})

	go func() {
		<-started
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	code := evo.Main(func() error {
		close(started)
		deadline := time.Now().Add(2 * time.Second)
		for task.Snapshot().State != evo.Cancelled && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		return nil
	})

	if code != evo.ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, evo.ExitCancelled, buf.String())
	}
	if got := task.Snapshot().State; got != evo.Cancelled {
		t.Fatalf("active task state = %v, want Cancelled", got)
	}
}

func TestMain_SecondSIGINTExits130WithoutWaitingForRun(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))
	evo.Task("work")
	started := make(chan struct{})

	go func() {
		<-started
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		time.Sleep(30 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	done := make(chan int, 1)
	go func() {
		done <- evo.Main(func() error {
			close(started)
			select {} // a run that never unwinds on its own; only the 2nd signal ends the call
		})
	}()

	select {
	case code := <-done:
		if code != evo.ExitCancelled {
			t.Fatalf("exit %d, want %d (ExitCancelled)", code, evo.ExitCancelled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Main did not return after a second SIGINT")
	}
}
