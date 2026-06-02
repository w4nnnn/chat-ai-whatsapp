package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	msgPrefix       = "whatsapp:sessions:"
	maxMessages     = 20
	ttlSeconds      = 86400 // 24h
	rateLimitPrefix = "whatsapp:ratelimit:"
	maxPerMinute    = 10
)

type Message struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type Store struct {
	rdb *redis.Client
}

func New(redisURL string) (*Store, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opts)

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Store{rdb: rdb}, nil
}

func (s *Store) Close() error {
	return s.rdb.Close()
}

func messagesKey(phone string) string {
	return fmt.Sprintf("%s%s:messages", msgPrefix, phone)
}

func (s *Store) GetMessages(ctx context.Context, phone string) ([]Message, error) {
	raw, err := s.rdb.LRange(ctx, messagesKey(phone), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("lrange: %w", err)
	}

	messages := make([]Message, 0, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		var msg Message
		if err := json.Unmarshal([]byte(raw[i]), &msg); err != nil {
			return nil, fmt.Errorf("unmarshal message: %w", err)
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *Store) AddMessage(ctx context.Context, phone string, msg Message) error {
	key := messagesKey(phone)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	pipe := s.rdb.Pipeline()
	pipe.LPush(ctx, key, data)
	pipe.LTrim(ctx, key, 0, maxMessages-1)
	pipe.Expire(ctx, key, ttlSeconds*time.Second)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Store) CheckRateLimit(ctx context.Context, phone string) (bool, error) {
	key := fmt.Sprintf("%s%s", rateLimitPrefix, phone)
	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("incr: %w", err)
	}

	if count == 1 {
		s.rdb.Expire(ctx, key, 60*time.Second)
	} else if count > 1 {
		ttl, err := s.rdb.TTL(ctx, key).Result()
		if err == nil && ttl < 0 {
			s.rdb.Expire(ctx, key, 60*time.Second)
		}
	}

	if count > int64(maxPerMinute) {
		return false, nil
	}
	return true, nil
}

func (s *Store) ClearSession(ctx context.Context, phone string) error {
	return s.rdb.Del(ctx, messagesKey(phone)).Err()
}
