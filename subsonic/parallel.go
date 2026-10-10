package subsonic

import "sync"

// maxConcurrency bounds the number of simultaneous requests sent to Subsonic.
const maxConcurrency = 4

// mapConcurrent applies fn to every item with at most limit calls in flight.
// The result order matches the input order. The first error encountered is
// returned and no new calls are started after it.
func mapConcurrent[T, R any](items []T, limit int, fn func(T) (R, error)) ([]R, error) {
	res := make([]R, len(items))

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	sem := make(chan struct{}, limit)

	failed := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return firstErr != nil
	}

	for i, item := range items {
		sem <- struct{}{}
		if failed() {
			<-sem
			break
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			r, err := fn(item)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			res[i] = r
		}()
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return res, nil
}
