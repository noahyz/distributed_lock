package utils

import (
	"context"
	"sync"
)

type GoroutinePool struct {
	goroutineNum uint64
	jobChan      chan func()
	ctx          context.Context
	cancel       context.CancelFunc
}

func NewGoroutinePool(goroutineNum uint64) *GoroutinePool {
	r := &GoroutinePool{
		goroutineNum: goroutineNum,
		jobChan:      make(chan func(), 100),
	}

	r.ctx, r.cancel = context.WithCancel(context.Background())
	go func() {
		var wg sync.WaitGroup
		var i uint64 = 0
		for ; i < goroutineNum; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case job := <-r.jobChan:
						job()
					case <-r.ctx.Done():
						return
					}
				}
			}()
		}
		wg.Wait()
	}()
	return r
}

// Submit 提交一个任务
func (r *GoroutinePool) Submit(job func()) {
	r.jobChan <- job
}

// ShutdownNow 尝试停止所有线程
func (r *GoroutinePool) ShutdownNow() {
	r.cancel()
}
