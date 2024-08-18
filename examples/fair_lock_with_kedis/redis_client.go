package main

import (
	"context"
	"git.corp.kuaishou.com/infra/infra-framework-go.git/kedis"
	"github.com/noahyz/distributed_lock/api/database"
	"time"
)

type RedisClient struct {
	client *kedis.Kedis
}

func NewRedisClient(client *kedis.Kedis) *RedisClient {
	return &RedisClient{
		client: client,
	}
}

func (r *RedisClient) Ping(_ context.Context) (string, error) {
	pong, err := r.client.Ping().Result()
	if err != nil {
		return "", err
	}
	return pong, nil
}

func (r *RedisClient) Eval(_ context.Context, script string, keys []string,
	args ...interface{}) (interface{}, error) {
	return r.client.Eval(script, keys, args...).Result()
}

func (r *RedisClient) LRange(_ context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.LRange(key, start, stop).Result()
}

func (r *RedisClient) ZRangeWithScores(_ context.Context, key string, start, stop int64) (map[string]float64, error) {
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

func (r *RedisClient) Exists(_ context.Context, key string) (bool, error) {
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

func (r *RedisClient) Subscribe(_ context.Context, key string) database.WrapPubSub {
	return NewPubSub(r.client, key)
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

// PubSub 包装 redis 的发布订阅
type PubSub struct {
	client *kedis.Kedis
	key    string

	msgChannel chan string
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewPubSub(client *kedis.Kedis, key string) *PubSub {
	result := &PubSub{
		client:     client,
		key:        key,
		msgChannel: make(chan string, 100),
	}
	result.ctx, result.cancel = context.WithCancel(context.Background())
	return result
}

func (r *PubSub) Unsubscribe(_ context.Context, _ string) error {
	r.cancel()
	return nil
}

func (r *PubSub) Channel() <-chan string {
	go func(ctx context.Context) {
		// 如果没有获取到，则睡眠时间以2倍递增；如果获取到了则继续获取
		sleepTime := 50 * time.Millisecond
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
