package api

import (
	"distributed-lock/internal/plugins/redis_lock/fair_lock"
	"fmt"
	"github.com/redis/go-redis/v9"
)

func NewFairLockByRedis(lockName string, redisClient *redis.Client) (*fair_lock.RedisFairLock, error) {
	if err := checkRedisStatus(redisClient); err != nil {
		return nil, fmt.Errorf("redis not available, err: %v", err)
	}
	fairLock := fair_lock.NewFairLock(redisClient, lockName)
	return fairLock, nil
}
