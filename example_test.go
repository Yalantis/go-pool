package gopool_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	gopool "github.com/Yalantis/go-pool"
)

type Task struct {
	Name string
}

func (t Task) Do(ctx context.Context, done chan<- struct{}) {
	_, _ = fmt.Fprint(ioutil.Discard, t.Name)
	done <- struct{}{}
}

func ExampleNew() {
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poolSize := 30
	queueSize := 100
	spawn := 1
	workerIdleTimeout := time.Minute

	pool, err := gopool.New(rootCtx, poolSize, queueSize, spawn, workerIdleTimeout)
	if err != nil {
		log.Fatalln(err)
	}

	for i := 0; i < 100; i++ {
		if err := pool.Schedule(Task{"Task's name"}); err != nil {
			log.Fatalln(err)
		}
	}

	for i := 0; i < 100; i++ {
		if err := pool.ScheduleWithTimeout(Task{"Task's name"}, time.Second); err != nil {
			log.Fatalln(err)
		}
	}

	pool.Shutdown()
}
