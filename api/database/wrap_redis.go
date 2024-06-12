package database

import "context"

/**
  	将内部使用的 redis 方法抽象出来，这样就可以和 redis 驱动客户端解耦
	这部分还需要改造，写的不够优雅
*/

type WrapRedisClient interface {
	Ping(ctx context.Context) (string, error)
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	ZRangeWithScores(ctx context.Context, key string, start, stop int64) (map[string]float64, error)
	Exists(ctx context.Context, key string) (bool, error)
	GetPublishCommand() string
	Subscribe(ctx context.Context, channel string) WrapPubSub
	Close() error
}
type WrapPubSub interface {
	Unsubscribe(ctx context.Context, channel string) error
	Channel() <-chan string
	Close() error
}
