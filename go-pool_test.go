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
	// simulate job
	time.Sleep(taskDuration)
	if t.doF != nil {
		t.doF()
	}
	done <- struct{}{}
}

func TestNew(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	pool, err := New(ctx, 0, 0, 0, 0)
	if err != ErrInvalidSize {
		t.Fatalf("expected error: %v, got: %v", ErrInvalidSize, err)
	}

	pool, err = New(ctx, 1, 1, 2, 0)
	if err != ErrInvalidSize {
		t.Fatalf("expected error: %v, got: %v", ErrInvalidSize, err)
	}

	pool, err = New(ctx, 1, 1, 0, 0)
	if err != ErrInvalidOptions {
		t.Fatalf("expected error: %v, got: %v", ErrInvalidSize, err)
	}

	pool, err = New(ctx, 3, 2, 1, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pool.Shutdown()

	if 2 != cap(pool.queue) {
		t.Fatalf("expected queue capacity: %v, got: %v", 2, cap(pool.queue))
	}
	if time.Second != pool.workerIdleTimeout {
		t.Fatalf("expected workerIdleTimeout: %v, got: %v", time.Second, pool.workerIdleTimeout)
	}
}

func TestGoPool_Schedule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testDone := make(chan struct{})
	pool, err := New(ctx, 1, 1, 1, time.Millisecond)
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

// start with single permanent worker
func TestGoPool_Schedule_ColdStart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testDone := make(chan struct{})
	pool, err := New(ctx, 3, 1, 1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Shutdown()

	for i := 0; i < 3; i++ {
		_ = pool.Schedule(fakeTask{doF: func() { testDone <- struct{}{} }})
	}

	for i := 0; i < 3; i++ {
		select {
		case <-testDone:
		case <-time.After(2 * time.Second):
			t.Fatal("expected tasks to be done")
		}
	}
}

// tasks scheduled with timeout might be expired, reach the deadline
func TestGoPool_ScheduleWithTimeout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testDone := make(chan struct{})
	// ok
	pool, err := New(ctx, 1, 1, 1, 0)
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
	pool, err = New(ctx, 1, 0, 0, 0)
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
	pool, err := New(ctx, 2, 2, 1, taskDuration*2)
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

// on cancel of root ctx all workers should be released
func TestGoPool_CancelContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	pool, err := New(ctx, 2, 2, 1, taskDuration*2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 2; i++ {
		_ = pool.Schedule(fakeTask{})
	}

	cancel()
	pool.wg.Wait()

	if len(pool.semaphore) != 0 {
		t.Fatalf("expected to be zero, got: %d", len(pool.semaphore))
	}
}

// temporary workers should be released by idle timeout
func TestGoPool_TemporaryRelease(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testDone := make(chan struct{})
	pool, err := New(ctx, 3, 0, 0, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 3; i++ {
		_ = pool.Schedule(fakeTask{doF: func() { testDone <- struct{}{} }})
	}

	for i := 0; i < 3; i++ {
		select {
		case <-testDone:
		case <-time.After(2 * time.Second):
			t.Fatal("expected tasks to be done")
		}
	}

	pool.wg.Wait()

	if len(pool.semaphore) != 0 {
		t.Fatalf("expected to be zero, got: %d", len(pool.semaphore))
	}
}

// deadlock in case pool size is limited and lock occurs while processing
func TestGoPool_Schedule_Deadlock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testDone := make(chan struct{}) // bottleneck
	pool, err := New(ctx, 1, 0, 0, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Shutdown()

	var isLock bool
	tasksNum := 3

	for i := 0; i < tasksNum; i++ {
		_ = pool.Schedule(fakeTask{doF: func() {
			select {
			case testDone <- struct{}{}:
			case <-time.After(taskDuration * 2):
				isLock = true
				// prevent deadlock
			}
		}})
	}

	var processed int
	for i := 0; i < tasksNum; i++ {
		select {
		case <-testDone:
			processed++
		case <-time.After(taskDuration * 3):
			// prevent deadlock
		}
	}

	if processed == tasksNum {
		t.Fatal("expected some tasks to be skipped")
	}

	if isLock == false {
		t.Fatal("expected isLock to be true")
	}
}
