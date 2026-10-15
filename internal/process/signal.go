package process

import (
	"os"
	"os/signal"
	"syscall"
)

// Signal is an operating-system signal delivered to this process.
type Signal = os.Signal

// Interrupt and Terminate are the signals a run treats as cancellation.
var (
	Interrupt Signal = syscall.SIGINT
	Terminate Signal = syscall.SIGTERM
)

// Notify delivers the listed signals to c until StopNotify(c).
func Notify(c chan<- Signal, sig ...Signal) { signal.Notify(c, sig...) }

// StopNotify stops delivering signals to c.
func StopNotify(c chan<- Signal) { signal.Stop(c) }
