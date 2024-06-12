package main

import (
	"github.com/go-redis/redis/v7"
	"github.com/noahyz/distributed_lock/api"
	"github.com/noahyz/distributed_lock/pkg/utils"
	"log"
	"sync"
	"time"
)

func main() {
	exampleUsage()
	exampleExecuteOrderly()
}

func exampleUsage() {
	// 创建一把分布式锁
	lockName := "example_usage"
	client := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "1234",
		DB:       0,
	})
	redisClient := NewRedisClient(client)
	fairLock, err := api.NewFairLockByRedis(lockName, redisClient)
	if err != nil {
		log.Printf("create FairLock by redis err: %v\n", err)
		return
	}
	if err = fairLock.Lock(); err != nil {
		log.Printf("Lock err: %v\n", err)
		return
	}
	log.Printf("goroutine: %v, Lock success", utils.GetGoroutineId())
	time.Sleep(10 * time.Millisecond)
	if err = fairLock.Unlock(); err != nil {
		log.Printf("Unlock err: %v\n", err)
		return
	}
	log.Printf("goroutine: %v, Unlock success", utils.GetGoroutineId())
}

func exampleExecuteOrderly() {
	// 创建一把分布式锁
	lockName := "example_execute_orderly_with_pubsub"
	client := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "1234",
		DB:       0,
	})
	redisClient := NewRedisClient(client)
	fairLock, err := api.NewFairLockByRedis(lockName, redisClient)
	if err != nil {
		log.Printf("create FairLock by redis err: %v\n", err)
		return
	}
	// 一共有 10 个协程，每个协程每秒都进行加锁，在当前时间段(1s)只能看到一个协程加锁成功
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if err = fairLock.Lock(); err != nil {
					log.Printf("goroutine: %v, lock err: %v\n", utils.GetGoroutineId(), err)
					return
				}
				log.Printf("goroutine: %v, lock success, execute business logic\n", utils.GetGoroutineId())
				time.Sleep(1 * time.Second)
				if err = fairLock.Unlock(); err != nil {
					log.Printf("goroutine: %v, unlock err: %v\n", utils.GetGoroutineId(), err)
				}
				//log.Printf("goroutine: %v, unlock sucess\n", utils.GetGoroutineId())
			}
		}()
	}
	wg.Wait()
}
