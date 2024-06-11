package api

import (
	"github.com/noahyz/distributed_lock/api/database"
	"github.com/noahyz/distributed_lock/api/option"
	"github.com/noahyz/distributed_lock/internal/plugins/redis_lock/fair_lock"
)

type DistributedLock interface {
	Lock(param ...option.LockParamOption) error
	TryLock(param ...option.TryLockParamOption) error
	Unlock() error
	ForceUnlock()
	IsLocked() bool
}

func NewFairLockByRedis(lockName string, redisClient database.WrapRedisClient) (DistributedLock, error) {
	return fair_lock.NewFairLock(redisClient, lockName)
}
