package facade

// run calls the wrapped-output facade's methods from real call sites, the
// way go-task's command handlers call into internal/logger — exactly the
// call sites the facade finding must enumerate.
func run() {
	l := &Logger{}
	l.Outf("starting task %s", "build")
	l.Outf("task %s done", "build")
	l.Errf("task %s failed", "test")
	l.Warnf("retrying %s", "test")
}
