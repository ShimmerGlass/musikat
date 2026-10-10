package subsonic

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestMapConcurrentOrder(t *testing.T) {
	in := make([]int, 50)
	for i := range in {
		in[i] = i
	}
	out, err := mapConcurrent(in, 4, func(i int) (int, error) {
		time.Sleep(time.Duration(50-i) * 100 * time.Microsecond)
		return i * 2, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range out {
		if v != i*2 {
			t.Fatalf("out[%d]=%d", i, v)
		}
	}
}

func TestMapConcurrentError(t *testing.T) {
	boom := errors.New("boom")
	_, err := mapConcurrent([]int{1, 2, 3, 4, 5, 6}, 2, func(i int) (int, error) {
		if i == 3 {
			return 0, boom
		}
		return i, nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestMapConcurrentLimit(t *testing.T) {
	var cur, peak atomic.Int32
	_, err := mapConcurrent(make([]int, 40), 4, func(int) (int, error) {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		cur.Add(-1)
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if p := peak.Load(); p > 4 || p < 2 {
		t.Fatalf("peak concurrency %d", p)
	}
}

func TestMapConcurrentEmpty(t *testing.T) {
	out, err := mapConcurrent([]int{}, 4, func(i int) (int, error) { return i, nil })
	if err != nil || len(out) != 0 {
		t.Fatal(out, err)
	}
}
