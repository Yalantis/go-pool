package gopool

import (
	"context"
	"sync"
	"testing"
	"time"
)

func BenchmarkGoPool_Schedule(b *testing.B) {
	ctx := context.Background()
	pool, err := New(ctx, 500, 500, 500, time.Second)
	if err != nil {
		b.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	j := fakeTask{doF: func() {
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

func BenchmarkGoPool_Schedule_ColdStart(b *testing.B) {
	ctx := context.Background()
	pool, err := New(ctx, 500, 500, 1, time.Second)
	if err != nil {
		b.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	j := fakeTask{doF: func() {
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

// go 1.11.13
//BenchmarkGoPool_Schedule-8                 10000            206643 ns/op               7 B/op          0 allocs/op
//BenchmarkGoPool_Schedule_ColdStart-8       10000            207100 ns/op              15 B/op          0 allocs/op
// go 1.13.4
//BenchmarkGoPool_Schedule-8                 4696             220266 ns/op              15 B/op          0 allocs/op
//BenchmarkGoPool_Schedule_ColdStart-8       4892             211635 ns/op              33 B/op          0 allocs/op