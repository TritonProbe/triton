package runid

import (
	"sync"
	"testing"
)

func TestNewReturnsUniqueIDsUnderConcurrency(t *testing.T) {
	const workers = 32
	const perWorker = 64

	seen := sync.Map{}
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				id := New("pr")
				if _, loaded := seen.LoadOrStore(id, struct{}{}); loaded {
					t.Errorf("duplicate id generated: %s", id)
				}
			}
		}()
	}

	wg.Wait()
}
