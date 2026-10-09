package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	rdb *redis.Client
}

func Connect(url string) (*Cache, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return &Cache{rdb: rdb}, nil
}

func (c *Cache) GetJSON(ctx context.Context, key string, dest any) bool {
	if c == nil {
		return false
	}
	b, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(b, dest) == nil
}

func (c *Cache) SetJSON(ctx context.Context, key string, val any, ttl time.Duration) {
	if c == nil {
		return
	}
	b, err := json.Marshal(val)
	if err != nil {
		return
	}
	_ = c.rdb.Set(ctx, key, b, ttl).Err()
}

func (c *Cache) DelPrefix(ctx context.Context, prefix string) {
	if c == nil {
		return
	}
	iter := c.rdb.Scan(ctx, 0, prefix+"*", 200).Iterator()
	for iter.Next(ctx) {
		_ = c.rdb.Del(ctx, iter.Val()).Err()
	}
}

func (c *Cache) Lock(ctx context.Context, key string, ttl time.Duration) bool {
	if c == nil {
		return true
	}
	ok, err := c.rdb.SetNX(ctx, key, "1", ttl).Result()
	return err == nil && ok
}

func (c *Cache) Ping(ctx context.Context) error {
	if c == nil {
		return nil
	}
	return c.rdb.Ping(ctx).Err()
}
