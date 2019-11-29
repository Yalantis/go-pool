package gopool

import (
	"context"
	"sync"
	"testing"
	"time"
)

type benchTask struct {
	doF func()
}

func (t benchTask) Do(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	time.Sleep(time.Millisecond * time.Duration(100))
	t.doF()
}

func BenchmarkSchedule(b *testing.B) {
	ctx := context.Background()
	pool, err := New(ctx, 500, 500, 500, time.Second, time.Second)
	if err != nil {
		b.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	j := benchTask{doF: func() {
		wg.Done()
	}}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		_ = pool.Schedule(j)
	}

	wg.Wait()
	close(pool.shutdown)
}

// go test -bench=. -count=10

//BenchmarkSchedule-8                4695            220418 ns/op             407 B/op          6 allocs/op
//BenchmarkScheduleGoroutine-8       5071            222976 ns/op             406 B/op          6 allocs/op
