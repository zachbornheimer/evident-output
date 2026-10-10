//go:build unix

package process_test

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/process"
)

// deliveryBudget bounds how long a test waits for a signal it just sent.
const deliveryBudget = 5 * time.Second

func TestRunCancellationSignalsAreInterruptAndTerminate(t *testing.T) {
	if process.Interrupt != syscall.SIGINT || process.Terminate != syscall.SIGTERM {
		t.Fatalf("Interrupt, Terminate = %v, %v; want SIGINT, SIGTERM", process.Interrupt, process.Terminate)
	}
}

func TestNotifyDeliversTheListedSignalToTheChannel(t *testing.T) {
	delivered := make(chan process.Signal, 1)
	process.Notify(delivered, syscall.SIGUSR1)
	defer process.StopNotify(delivered)

	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatalf("send SIGUSR1: %v", err)
	}
	select {
	case got := <-delivered:
		if got != syscall.SIGUSR1 {
			t.Fatalf("delivered %v, want SIGUSR1", got)
		}
	case <-time.After(deliveryBudget):
		t.Fatal("SIGUSR1 was not delivered to the channel")
	}
}

// A channel stopped with StopNotify receives nothing, while another channel
// still registered for the same signal keeps the process alive and receives it.
func TestStopNotifyEndsDeliveryToThatChannelOnly(t *testing.T) {
	stopped := make(chan process.Signal, 1)
	kept := make(chan process.Signal, 1)
	process.Notify(stopped, syscall.SIGUSR2)
	process.Notify(kept, syscall.SIGUSR2)
	defer process.StopNotify(kept)
	process.StopNotify(stopped)

	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatalf("send SIGUSR2: %v", err)
	}
	select {
	case <-kept:
	case <-time.After(deliveryBudget):
		t.Fatal("SIGUSR2 was not delivered to the channel still registered")
	}
	select {
	case got := <-stopped:
		t.Fatalf("stopped channel received %v", got)
	default:
	}
}
