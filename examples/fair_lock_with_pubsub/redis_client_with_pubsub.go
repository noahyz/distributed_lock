package main

import (
	"context"
	"github.com/go-redis/redis/v7"
	"github.com/noahyz/distributed_lock/api/database"
	"time"
)

/**
有些 redis 客户端无法提供 pub/sub 功能，因此我们使用其他手段进行模拟 redis 的发布订阅功能
发布功能: `PUBLISH channel message`
订阅功能: `SUBSCRIBE channel`

使用列表 List 来模拟发布订阅功能。
使用 RPUSH 来将值插入到列表的尾部。`RPUSH KEY_NAME value`
使用 LPOP 移出并获取列表的第一个元素，如果列表 key 不存在时，则返回 nil。`LPOP KEY_NAME`
(也可以使用 BLPOP，根据当前 redis 支持的命令来决定)
*/

// RedisClient 包装 redis 的 API，这样就可以让用户控制使用那个 redis 客户端了
type RedisClient struct {
	client *redis.Client
}

func NewRedisClient(client *redis.Client) *RedisClient {
	return &RedisClient{
		client: client,
	}
}

func (r *RedisClient) Ping(ctx context.Context) (string, error) {
	pong, err := r.client.Ping().Result()
	if err != nil {
		return "", err
	}
	return pong, nil
}

func (r *RedisClient) Eval(ctx context.Context, script string, keys []string,
	args ...interface{}) (interface{}, error) {
	return r.client.Eval(script, keys, args...).Result()
}

func (r *RedisClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.LRange(key, start, stop).Result()
}

func (r *RedisClient) ZRangeWithScores(ctx context.Context, key string, start, stop int64) (map[string]float64, error) {
	memberScores, err := r.client.ZRangeWithScores(key, start, stop).Result()
	if err != nil {
		return make(map[string]float64), err
	}
	result := make(map[string]float64, len(memberScores))
	for _, ms := range memberScores {
		if member, ok := ms.Member.(string); ok {
			result[member] = ms.Score
		}
	}
	return result, nil
}

func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.client.Exists(key).Result()
	if err != nil {
		return false, err
	}
	if result > 0 {
		return true, nil
	}
	return false, nil
}

func (r *RedisClient) GetPublishCommand() string {
	return "RPUSH"
}

func (r *RedisClient) Subscribe(ctx context.Context, key string) database.WrapPubSub {
	return NewPubSub(r.client, key)
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

// PubSub 包装 redis 的发布订阅
type PubSub struct {
	client *redis.Client
	key    string

	msgChannel chan string
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewPubSub(client *redis.Client, key string) *PubSub {
	result := &PubSub{
		client:     client,
		key:        key,
		msgChannel: make(chan string, 100),
	}
	result.ctx, result.cancel = context.WithCancel(context.Background())
	return result
}

func (r *PubSub) Unsubscribe(ctx context.Context, key string) error {
	r.cancel()
	return nil
}

func (r *PubSub) Channel() <-chan string {
	go func(ctx context.Context) {
		// 如果没有获取到，则睡眠时间以2倍递增；如果获取到了则继续获取
		sleepTime := 1 * time.Millisecond
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			result, err := r.client.LPop(r.key).Result()
			if err != nil {
				time.Sleep(sleepTime)
				sleepTime = 2 * sleepTime
				if sleepTime > 500*time.Millisecond {
					sleepTime = 500 * time.Millisecond
				}
			} else {
				r.msgChannel <- result
				sleepTime = 1 * time.Millisecond
			}
		}
	}(r.ctx)
	return r.msgChannel
}

func (r *PubSub) Close() error {
	r.cancel()
	return nil
}
