package agent

import (
	"sync"
	"time"
)

// A write answers in under a second, as everything in the filter's path does: the project's checker
// gets this long, and past it the write returns and the end-of-turn checker has the last word.
const diagnosticsBudget = 800 * time.Millisecond

var diagnosticsBusy sync.Map // one checker per project at a time: a burst of writes never piles them up

// diagnosticsWithin returns the checker's result if it finishes within the budget. done is false when
// it did not, or when another write's checker is still running.
func diagnosticsWithin(root, abs string, budget time.Duration) (string, bool) {
	if _, running := diagnosticsBusy.LoadOrStore(root, true); running {
		return "", false
	}
	out := make(chan string, 1)
	go func() {
		defer diagnosticsBusy.Delete(root)
		out <- fileDiagnostics(root, abs)
	}()
	select {
	case d := <-out:
		return d, true
	case <-time.After(budget):
		return "", false
	}
}
