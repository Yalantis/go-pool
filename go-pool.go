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

var permanentTimer = &time.Timer{C: nil}

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
	taskTimeout       time.Duration
	workerIdleTimeout time.Duration
	ctx               context.Context
	shutdown          chan struct{}
}

// New must be called at the beginning of the main
func New(ctx context.Context, size, queue, spawn int, taskTimeout, workerIdleTimeout time.Duration) (*GoPool, error) {
	if size <= 0 || size < spawn {
		return nil, ErrInvalidSize
	}
	if spawn <= 0 && 0 < queue {
		return nil, ErrInvalidOptions
	}

	pool := &GoPool{
		wg:                &sync.WaitGroup{},
		semaphore:         make(chan struct{}, size),
		queue:             make(chan Task, queue),
		taskTimeout:       taskTimeout,
		workerIdleTimeout: workerIdleTimeout,
		ctx:               ctx,
		shutdown:          make(chan struct{}),
	}

	// spawn permanent workers
	for i := 0; i < spawn; i++ {
		pool.semaphore <- struct{}{}
		pool.worker(nil, taskTimeout, 0)
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
		g.worker(task, g.taskTimeout, g.workerIdleTimeout)
	}

	return nil
}

// worker process task from tasks queue
// self terminates in case idle more then workerIdleTimeout
func (g *GoPool) worker(task Task, taskTimeout, workerIdleTimeout time.Duration) {
	g.wg.Add(1)

	go func() { // call wg.Add outside of goroutine to avoid race
		if task != nil {
			do(g.ctx, task, taskTimeout, g.shutdown)
		}

		idleTimer := permanentTimer
		isTemporary := workerIdleTimeout > 0

		if isTemporary {
			idleTimer = time.NewTimer(workerIdleTimeout)
		}

	loop:
		for {
			select {
			case <-g.ctx.Done():
				break loop
			case <-g.shutdown:
				break loop
			case <-idleTimer.C:
				break loop
			case task := <-g.queue:
				if task == nil {
					continue
				}

				ctx, cancel := context.WithTimeout(g.ctx, taskTimeout)
				done := make(chan struct{})

				task.Do(ctx, done)

				select {
				case <-done:
				case <-ctx.Done():
				}

				cancel()

				if isTemporary {
					// reset timer before next iteration
					if !idleTimer.Stop() {
						<-idleTimer.C
					}
					idleTimer.Reset(workerIdleTimeout)
				}
			}
		}

		<-g.semaphore
		g.wg.Done()
	}()
}

func do(poolCtx context.Context, task Task, timeout time.Duration, poolDone <-chan struct{}) {
	ctx, cancel := context.WithTimeout(poolCtx, timeout)
	done := make(chan struct{})

	task.Do(ctx, done)

	select {
	case <-poolDone:
	case <-done:
	case <-ctx.Done():
	}

	cancel()
}

// Shutdown notifies workers to stop.
// Waits until all workers are stopped
func (g *GoPool) Shutdown() {
	close(g.shutdown)
	g.wg.Wait()
}
