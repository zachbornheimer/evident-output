package record

import "testing"

func TestMutexSectionHoldsNotificationsUntilTheMutexIsFree(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1"})
	var mu Mutex
	mu.Bind(run, nil)
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

func TestMutexTellsItsOwnListenerInsideTheSection(t *testing.T) {
	run := NewRun()
	task := run.NewTask(TaskInit{ID: "task_1"})
	var mu Mutex
	held := &callbackListener{}
	mu.Bind(run, held)
	outside := &callbackListener{}
	run.SetListener(outside)

	mu.Lock()
	task.SetPhase("in the section")
	run.AppendEvent(Event{Type: "x"}, 0)
	heardBeforeUnlock := len(held.tasks) + len(held.events)
	mu.Unlock()

	if heardBeforeUnlock != 0 {
		t.Errorf("the bound listener heard %d changes mid-section, want them told at Unlock", heardBeforeUnlock)
	}
	if len(held.tasks) != 1 || len(held.events) != 1 {
		t.Errorf("bound listener heard %v and %v, want one of each", held.tasks, held.events)
	}
	if len(outside.tasks)+len(outside.events) != 0 {
		t.Errorf("the run's listener heard %v and %v, which the bound listener already took", outside.tasks, outside.events)
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
