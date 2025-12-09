package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisClient wraps Redis operations for the RPKI application
type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisClient creates a new Redis client
func NewRedisClient(url string) (*RedisClient, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	ctx := context.Background()

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{
		client: client,
		ctx:    ctx,
	}, nil
}

// CacheVRPs caches VRP data with expiration
func (r *RedisClient) CacheVRPs(asn int, vrps interface{}, expiration time.Duration) error {
	key := fmt.Sprintf("vrps:asn:%d", asn)

	data, err := json.Marshal(vrps)
	if err != nil {
		return fmt.Errorf("failed to marshal VRPs: %w", err)
	}

	return r.client.Set(r.ctx, key, data, expiration).Err()
}

// GetCachedVRPs retrieves cached VRP data
func (r *RedisClient) GetCachedVRPs(asn int) (interface{}, error) {
	key := fmt.Sprintf("vrps:asn:%d", asn)

	data, err := r.client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, err
	}

	var vrps interface{}
	if err := json.Unmarshal([]byte(data), &vrps); err != nil {
		return nil, fmt.Errorf("failed to unmarshal VRPs: %w", err)
	}

	return vrps, nil
}

// CacheValidationResult caches prefix validation results
func (r *RedisClient) CacheValidationResult(asn int, prefix string, result interface{}, expiration time.Duration) error {
	key := fmt.Sprintf("validation:%d:%s", asn, prefix)

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal validation result: %w", err)
	}

	return r.client.Set(r.ctx, key, data, expiration).Err()
}

// GetCachedValidationResult retrieves cached validation result
func (r *RedisClient) GetCachedValidationResult(asn int, prefix string) (interface{}, error) {
	key := fmt.Sprintf("validation:%d:%s", asn, prefix)

	data, err := r.client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal validation result: %w", err)
	}

	return result, nil
}

// SetJobStatus sets the status of a background job
func (r *RedisClient) SetJobStatus(jobID string, status string, data interface{}) error {
	key := fmt.Sprintf("job:%s", jobID)

	jobData := map[string]interface{}{
		"status":     status,
		"data":       data,
		"updated_at": time.Now().Unix(),
	}

	jsonData, err := json.Marshal(jobData)
	if err != nil {
		return fmt.Errorf("failed to marshal job data: %w", err)
	}

	return r.client.Set(r.ctx, key, jsonData, 24*time.Hour).Err()
}

// GetJobStatus retrieves the status of a background job
func (r *RedisClient) GetJobStatus(jobID string) (map[string]interface{}, error) {
	key := fmt.Sprintf("job:%s", jobID)

	data, err := r.client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Job not found
	}
	if err != nil {
		return nil, err
	}

	var jobData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &jobData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job data: %w", err)
	}

	return jobData, nil
}

// CacheGlobalSummary caches global summary statistics
func (r *RedisClient) CacheGlobalSummary(summary interface{}, expiration time.Duration) error {
	key := "global:summary"

	data, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal global summary: %w", err)
	}

	return r.client.Set(r.ctx, key, data, expiration).Err()
}

// GetCachedGlobalSummary retrieves cached global summary
func (r *RedisClient) GetCachedGlobalSummary() (interface{}, error) {
	key := "global:summary"

	data, err := r.client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, err
	}

	var summary interface{}
	if err := json.Unmarshal([]byte(data), &summary); err != nil {
		return nil, fmt.Errorf("failed to unmarshal global summary: %w", err)
	}

	return summary, nil
}

// InvalidateCache invalidates cache entries matching a pattern
func (r *RedisClient) InvalidateCache(pattern string) error {
	keys, err := r.client.Keys(r.ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return r.client.Del(r.ctx, keys...).Err()
	}

	return nil
}

// HealthCheck checks if the Redis connection is healthy
func (r *RedisClient) HealthCheck(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	return r.client.Close()
}
