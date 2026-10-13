//go:build unix

package evo

import (
	"bytes"
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestMain_SIGINTCancelsActiveTaskAndExits130(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))
	task := defaultRun().Task("work")
	started := make(chan struct{})

	go func() {
		<-started
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	code := execute_1(context.Background(), func(ctx context.Context) error {
		close(started)
		deadline := time.Now().Add(2 * time.Second)
		for task.snapshot().State != Cancelled && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		return nil
	}).ExitCode()

	if code != exitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, exitCancelled, buf.String())
	}
	if got := task.snapshot().State; got != Cancelled {
		t.Fatalf("active task state = %v, want Cancelled", got)
	}
}

func TestMain_SecondSIGINTExits130WithoutWaitingForRun(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))
	defaultRun().Task("work")
	started := make(chan struct{})

	go func() {
		<-started
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		time.Sleep(30 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	done := make(chan int, 1)
	go func() {
		done <- execute_1(context.Background(), func(ctx context.Context) error {
			close(started)
			select {} // a run that never unwinds on its own; only the 2nd signal ends the call
		}).ExitCode()
	}()

	select {
	case code := <-done:
		if code != exitCancelled {
			t.Fatalf("exit %d, want %d (ExitCancelled)", code, exitCancelled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Main did not return after a second SIGINT")
	}
}
