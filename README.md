# go-pool [![Build Status](https://travis-ci.org/Yalantis/go-pool.svg?branch=master)](https://travis-ci.org/Yalantis/go-pool)

go-pool is a library which allows to create auto-scalable pools of workers.  
Temporary workers spawned in case tasks queue overflow and `poolSize` is not reached.  
Temporary workers which in IDLE state more then `workerIdleTimeout` are terminated.  

```go
func New(ctx context.Context, poolSize, queueSize, spawn int, workerIdleTimeout time.Duration) (*GoPool, error)
```
`ctx` - root context, used to notify every worker to exit. might be used as forced shutdown  
`poolSize` - size of pool, maximum allowed workers  
`queueSize` - size of queue chan, tasks queue  
`spawn` - number of workers spawned on pool creation and marked as permanent. It means that they are not terminated to IDLE timeout  
`workerIdleTimeout` - IDLE timeout. If temporary worker doesn't receive any task more then IDLE timeout - it's terminated 

### Example
```go
rootCtx := context.Background()

poolSize := 30
queueSize := 100
spawn := 1
workerIdleTimeout := time.Minute

pool, err := gopool.New(rootCtx, poolSize, queueSize, spawn, workerIdleTimeout)
if err != nil {
	log.Fatalln(err)
}

if err := pool.Schedule(Task{"Task's name"}); err != nil {
	log.Fatalln(err)
}

pool.Shutdown()
```
