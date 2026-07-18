package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	*redis.Client
}

func NewClient(addr, password string, db int) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &Client{Client: rdb}
}

func (c *Client) SetUserOnline(ctx context.Context, userID string) error {
	return c.Set(ctx, "online:"+userID, "1", 30*time.Minute).Err()
}

func (c *Client) SetUserOffline(ctx context.Context, userID string) error {
	return c.Del(ctx, "online:"+userID).Err()
}

func (c *Client) IsUserOnline(ctx context.Context, userID string) (bool, error) {
	val, err := c.Get(ctx, "online:"+userID).Result()
	if err == redis.Nil {
		return false, nil
	}
	return val == "1", err
}
