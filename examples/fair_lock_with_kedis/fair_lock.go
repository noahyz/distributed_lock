package main

import (
	"git.corp.kuaishou.com/infra/infra-framework-go.git/kedis"
	"github.com/noahyz/distributed_lock/api"
	"github.com/noahyz/distributed_lock/pkg/utils"
	"log"
	"sync"
	"time"
)

func main() {
	exampleUsage()
}

// exampleUsage 锁的使用方法
func exampleUsage() {
	// 创建一把分布式锁
	lockName := "test_distributed_lock"
	kRedis, err := kedis.NewKedis("cdnSelfDispatchTable")
	if err != nil {
		log.Printf("NewKedis err: %v\n", err)
		return
	}
	redisClient := NewRedisClient(kRedis)
	fairLock, err := api.NewFairLockByRedis(lockName, redisClient)
	if err != nil {
		log.Printf("NewFairLockByRedis err: %v\n", err)
		return
	}

	if err = fairLock.Lock(); err != nil {
		log.Printf("lock err: %v\n", err)
		return
	}
	log.Printf("goroutine: %v, lock success\n", utils.GetGoroutineId())
	if err = fairLock.Unlock(); err != nil {
		log.Printf("unlock err: %v\n", err)
		return
	}
	log.Printf("goroutine: %v, unlock sucess\n", utils.GetGoroutineId())
}

// exampleUpdateInteger 加锁更新一个数字
func exampleUpdateInteger() {
	// 创建一把分布式锁
	lockName := "test_distributed_lock"
	kRedis, err := kedis.NewKedis("cdnSelfDispatchTable")
	if err != nil {
		log.Printf("NewKedis err: %v\n", err)
		return
	}
	redisClient := NewRedisClient(kRedis)
	fairLock, err := api.NewFairLockByRedis(lockName, redisClient)
	if err != nil {
		log.Printf("NewFairLockByRedis err: %v\n", err)
		return
	}

	var number = 0
	var wg sync.WaitGroup
	startTime := time.Now()
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err = fairLock.Lock(); err != nil {
				log.Printf("goroutine: %v, lock err: %v\n", utils.GetGoroutineId(), err)
				return
			}
			log.Printf("goroutine: %v, lock success\n", utils.GetGoroutineId())
			number++
			if err = fairLock.Unlock(); err != nil {
				log.Printf("goroutine: %v, unlock err: %v\n", utils.GetGoroutineId(), err)
			}
			log.Printf("goroutine: %v, unlock sucess\n", utils.GetGoroutineId())
		}()
	}
	wg.Wait()
	log.Printf("When locked, expect number: %d, the result is: %d, cost time: %dns",
		500, number, time.Since(startTime).Nanoseconds())
}
