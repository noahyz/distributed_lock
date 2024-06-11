package main

import (
	"context"
	"github.com/noahyz/distributed_lock/api/database"
	"github.com/redis/go-redis/v9"
)

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
	pong, err := r.client.Ping(ctx).Result()
	if err != nil {
		return "", err
	}
	return pong, nil
}

func (r *RedisClient) Eval(ctx context.Context, script string, keys []string,
	args ...interface{}) (interface{}, error) {
	return r.client.Eval(ctx, script, keys, args...).Result()
}

func (r *RedisClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.LRange(ctx, key, start, stop).Result()
}

func (r *RedisClient) ZRangeWithScores(ctx context.Context, key string, start, stop int64) (map[string]float64, error) {
	memberScores, err := r.client.ZRangeWithScores(ctx, key, start, stop).Result()
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

func (r *RedisClient) Exists(ctx context.Context, keys ...string) (bool, error) {
	result, err := r.client.Exists(ctx, keys...).Result()
	if err != nil {
		return false, err
	}
	if result > 0 {
		return true, nil
	}
	return false, nil
}

func (r *RedisClient) Subscribe(ctx context.Context, channels ...string) database.WrapPubSub {
	pubSub := r.client.Subscribe(ctx, channels...)
	return NewPubSub(pubSub)
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

// PubSub 包装 redis 的发布订阅
type PubSub struct {
	redisPubSub *redis.PubSub
	msgChannel  chan string
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewPubSub(redisPubSub *redis.PubSub) *PubSub {
	result := &PubSub{
		redisPubSub: redisPubSub,
		msgChannel:  make(chan string, 100),
	}
	result.ctx, result.cancel = context.WithCancel(context.Background())
	return result
}

func (r *PubSub) Unsubscribe(ctx context.Context, channels ...string) error {
	return r.redisPubSub.Unsubscribe(ctx, channels...)
}

func (r *PubSub) Channel() <-chan string {
	rawChannel := r.redisPubSub.Channel()
	go func(ctx context.Context) {
		for {
			select {
			case msg := <-rawChannel:
				r.msgChannel <- msg.String()
			case <-ctx.Done():
				return
			}
		}
	}(r.ctx)
	return r.msgChannel
}

func (r *PubSub) Close() error {
	err := r.redisPubSub.Close()
	r.cancel()
	return err
}
