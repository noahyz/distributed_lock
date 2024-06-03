package fair_lock

import "fmt"

func getGoroutineQueueName(name string) string {
	return fmt.Sprintf("REDIS_DISTRIBUTED_LOCK_GOROUTINE_QUEUE_NAME_%s", name)
}

func getTimeoutSetName(name string) string {
	return fmt.Sprintf("REDIS_DISTRIBUTED_LOCK_TIEMOUT_SET_NAME_%s", name)
}

func getKeyHashName(name string) string {
	return fmt.Sprintf("REDIS_DISTRIBUTED_LOCK_KEY_HASH_BANE_%s", name)
}

func getEntryName(uuid, key string) string {
	return fmt.Sprintf("REDIS_DISTRIBUTED_LOCK_ENTRY_NAME_%s_%s", uuid, key)
}

func getChannelPrefixName(fairLockName string) string {
	return fmt.Sprintf("REDIS_DISTRIBUTED_LOCK_CHANNEL_NAME_%s", fairLockName)
}

func getLockName(uuid string, goroutineId int64) string {
	return fmt.Sprintf("REDIS_DISTRIBUTED_LOCK_GOROUTINE_LOCK_NAME_%s_%d", uuid, goroutineId)
}

func getUnlockLatchName(requestId string) string {
	return fmt.Sprintf("REDIS_DISTRIBUTED_LOCK_UNLOCK_LATCH_NMAE_%s", requestId)
}

func getPublishCommand() string {
	return "PUBLISH"
}
