package record

import "testing"

func TestMutexSectionHoldsNotificationsUntilTheMutexIsFree(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1"})
	var mu Mutex
	mu.Bind(run)
	heard := &callbackListener{}
	tookTheMutex := 0
	heard.onTask = func() {
		// The listener takes the same mutex: it only gets here once the
		// writer's section ended.
		mu.Lock()
		tookTheMutex++
		mu.Unlock()
	}
	run.SetListener(heard)

	mu.Lock()
	task.SetPhase("in the section")
	inside := len(heard.tasks)
	mu.Unlock()

	if inside != 0 || len(heard.tasks) != 1 || tookTheMutex != 1 {
		t.Errorf("heard %d inside the section and %d after it (mutex taken %d times), want 0, 1 and 1",
			inside, len(heard.tasks), tookTheMutex)
	}
}

func TestUnboundMutexIsAnOrdinaryMutex(t *testing.T) {
	var mu Mutex
	sections := 0
	mu.Lock()
	sections++
	mu.Unlock()
	mu.Lock()
	sections++
	mu.Unlock()
	if sections != 2 {
		t.Errorf("ran %d sections, want 2", sections)
	}
}
