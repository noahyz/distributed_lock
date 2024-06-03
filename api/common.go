package api

import (
	"context"
	"github.com/redis/go-redis/v9"
)

func checkRedisStatus(redisClient *redis.Client) error {
	// 检查 redis 是否可用
	_, err := redisClient.Ping(context.Background()).Result()
	if err != nil {
		return err
	}
	return nil
}
