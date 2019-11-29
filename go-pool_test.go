package gopool

import (
	"context"
	"testing"
	"time"
)

var taskDuration = time.Millisecond * 100

type fakeTask struct {
	doF func()
}

func (t fakeTask) Do(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	select {
	case <-ctx.Done():
	case <-time.After(taskDuration):
		// simulate job
	}

	if t.doF != nil {
		t.doF()
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	pool, err := New(ctx, 0, 0, 0, 0, 0)
	if err != ErrInvalidSize {
		t.Fatalf("expected error: %v, got: %v", ErrInvalidSize, err)
	}

	pool, err = New(ctx, 1, 1, 2, 0, 0)
	if err != ErrInvalidSize {
		t.Fatalf("expected error: %v, got: %v", ErrInvalidSize, err)
	}

	pool, err = New(ctx, 1, 1, 0, 0, 0)
	if err != ErrInvalidOptions {
		t.Fatalf("expected error: %v, got: %v", ErrInvalidSize, err)
	}

	pool, err = New(ctx, 3, 2, 1, time.Second, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pool.Shutdown()

	if 2 != cap(pool.queue) {
		t.Fatalf("expected queue capacity: %v, got: %v", 2, cap(pool.queue))
	}
	if time.Second != pool.taskTimeout {
		t.Fatalf("expected taskTimeout: %v, got: %v", time.Second, pool.taskTimeout)
	}
}

func TestGoPool_Schedule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testDone := make(chan struct{})
	pool, err := New(ctx, 1, 1, 1, time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Shutdown()

	_ = pool.Schedule(fakeTask{doF: func() { testDone <- struct{}{} }})

	select {
	case <-testDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected task to be done")
	}
}

func TestGoPool_ScheduleWithTimeout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testDone := make(chan struct{})
	// ok
	pool, err := New(ctx, 1, 1, 1, time.Millisecond, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Shutdown()

	err = pool.ScheduleWithTimeout(fakeTask{doF: func() { testDone <- struct{}{} }}, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-testDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected task to be done")
	}

	// error
	pool, err = New(ctx, 1, 0, 0, time.Millisecond, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Shutdown()

	pool.semaphore <- struct{}{} // block spawning

	err = pool.ScheduleWithTimeout(fakeTask{}, 0)
	if err != ErrScheduleDeadline {
		t.Fatalf("expected error: %v, got: %v", ErrScheduleDeadline, err)
	}
}

func TestGoPool_Shutdown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, err := New(ctx, 2, 2, 1, taskDuration*2, taskDuration*2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 2; i++ {
		_ = pool.Schedule(fakeTask{})
	}

	pool.Shutdown()

	if len(pool.semaphore) != 0 {
		t.Fatalf("expected to be zero, got: %d", len(pool.semaphore))
	}
}

func TestGoPool_CancelContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	pool, err := New(ctx, 2, 2, 1, taskDuration*2, taskDuration*2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Shutdown()

	for i := 0; i < 2; i++ {
		_ = pool.Schedule(fakeTask{})
	}

	cancel()
	pool.wg.Wait()

	if len(pool.semaphore) != 0 {
		t.Fatalf("expected to be zero, got: %d", len(pool.semaphore))
	}
}

func TestGoPool_TemporaryRelease(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testDone := make(chan struct{})
	pool, err := New(ctx, 3, 0, 0, time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 3; i++ {
		_ = pool.Schedule(nil)
		_ = pool.Schedule(fakeTask{doF: func() { testDone <- struct{}{} }})
	}

	for i := 0; i < 3; i++ {
		select {
		case <-testDone:
		case <-time.After(2 * time.Second):
			t.Fatal("expected task to be done")
		}
	}

	pool.wg.Wait()

	if len(pool.semaphore) != 0 {
		t.Fatalf("expected to be zero, got: %d", len(pool.semaphore))
	}
}
