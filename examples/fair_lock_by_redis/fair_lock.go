package main

import (
	"github.com/noahyz/distributed_lock/api"
	"github.com/noahyz/distributed_lock/pkg/utils"
	"github.com/redis/go-redis/v9"
	"log"
	"sync"
	"time"
)

func main() {
	exampleUpdateInteger()
	exampleExecuteOrderly()
}

// exampleUpdateInteger 多个协程同时更新一个数字
func exampleUpdateInteger() {
	// 创建一把分布式锁
	lockName := "example_update_integer"
	client := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "123",
		DB:       0,
	})
	redisClient := NewRedisClient(client)
	fairLock, err := api.NewFairLockByRedis(lockName, redisClient)
	if err != nil {
		log.Printf("create FairLock by redis err: %v\n", err)
		return
	}
	// 多个协程加锁，同时去更新这个 number，结果正确
	var wg sync.WaitGroup
	number := 0
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

// exampleExecuteOrderly 多个协程有序执行。每一秒只有一个协程加锁成功并且执行
func exampleExecuteOrderly() {
	// 创建一把分布式锁
	lockName := "example_execute_orderly"
	client := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "123",
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
	for i := 0; i < 10; i++ {
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
				log.Printf("goroutine: %v, unlock sucess\n", utils.GetGoroutineId())
			}
		}()
	}
	wg.Wait()
}
