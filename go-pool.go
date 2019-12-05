package gopool

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrInvalidOptions   = errors.New("invalid options")
	ErrInvalidSize      = errors.New("invalid size option")
	ErrScheduleDeadline = errors.New("schedule deadline exceeded")
)

var timersPool = sync.Pool{
	New: func() interface{} {
		t := time.NewTimer(time.Hour)
		t.Stop()
		return t
	},
}

// Task interface
type Task interface {
	// done chan must be closed inside Do after a job is done
	Do(ctx context.Context, done chan<- struct{})
}

// GoPool handler goroutine pool
type GoPool struct {
	wg                *sync.WaitGroup
	semaphore         chan struct{}
	queue             chan Task
	workerIdleTimeout time.Duration
	ctx               context.Context
	shutdown          chan struct{}
}

// New must be called at the beginning of the main
func New(ctx context.Context, poolSize, queueSize, spawn int, workerIdleTimeout time.Duration) (*GoPool, error) {
	if poolSize <= 0 || poolSize < spawn {
		return nil, ErrInvalidSize
	}
	if spawn <= 0 && 0 < queueSize {
		return nil, ErrInvalidOptions
	}

	pool := &GoPool{
		wg:                &sync.WaitGroup{},
		semaphore:         make(chan struct{}, poolSize),
		queue:             make(chan Task, queueSize),
		workerIdleTimeout: workerIdleTimeout,
		ctx:               ctx,
		shutdown:          make(chan struct{}),
	}

	// spawn permanent workers
	for i := 0; i < spawn; i++ {
		pool.semaphore <- struct{}{}
		worker(pool, nil, 0)
	}

	return pool, nil
}

// Schedule add task to queue
func (g *GoPool) Schedule(task Task) error {
	return g.schedule(task, nil)
}

// ScheduleWithTimeout tries add task to queue with deadline/expiration
func (g *GoPool) ScheduleWithTimeout(task Task, timeout time.Duration) error {
	return g.schedule(task, time.After(timeout))
}

// add task to queue with deadline
// spawn new worker if tasks queue is full and workers limit is not reached
// in case deadline exceeded skip task and return an error
func (g *GoPool) schedule(task Task, deadline <-chan time.Time) error {
	select {
	case <-deadline:
		return ErrScheduleDeadline
	case g.queue <- task:
	case g.semaphore <- struct{}{}:
		worker(g, task, g.workerIdleTimeout)
	}

	return nil
}

// worker process task from tasks queue
// self terminates in case idle more then workerIdleTimeout
func worker(g *GoPool, task Task, workerIdleTimeout time.Duration) {
	g.wg.Add(1)

	go func() { // call wg.Add outside of goroutine to avoid race
		if task != nil {
			do(g.ctx, task, g.shutdown)
		}

		var idleTimer *time.Timer
		var chTick <-chan time.Time
		isTemporary := workerIdleTimeout > 0
		taskDone := make(chan struct{}, 1)

		if isTemporary {
			idleTimer = timersPool.Get().(*time.Timer)
			idleTimer.Reset(workerIdleTimeout)
			chTick = idleTimer.C
		}

		releaseTimer := func() {
			if isTemporary {
				if !idleTimer.Stop() {
					<-chTick // drain
				}
				timersPool.Put(idleTimer)
			}
		}

		resetTimer := func() {
			if isTemporary {
				// reset timer before next iteration
				if !idleTimer.Stop() {
					<-chTick // drain
				}
				idleTimer.Reset(workerIdleTimeout)
			}
		}

	loop:
		for {
			select {
			case <-g.ctx.Done():
				releaseTimer()
				break loop
			case <-g.shutdown:
				releaseTimer()
				break loop
			case <-chTick:
				idleTimer.Stop()
				timersPool.Put(idleTimer)
				break loop
			case task := <-g.queue:
				task.Do(g.ctx, taskDone)

				select {
				case <-g.ctx.Done():
				case _, ok := <-taskDone:
					// kept for backward compatibility with `close(taskDone)`
					// might be deprecated
					if !ok {
						taskDone = make(chan struct{}, 1)
					}
				}

				resetTimer()
			}
		}

		<-g.semaphore
		g.wg.Done()
	}()
}

func do(poolCtx context.Context, task Task, poolDone <-chan struct{}) {
	taskDone := make(chan struct{}, 1)

	task.Do(poolCtx, taskDone)

	select {
	case <-poolCtx.Done():
	case <-poolDone:
	case <-taskDone:
	}
}

// Shutdown notifies workers to stop.
// Waits until all workers are stopped.
// Doesn't break workers which in processing
func (g *GoPool) Shutdown() {
	close(g.shutdown)
	g.wg.Wait()
}
