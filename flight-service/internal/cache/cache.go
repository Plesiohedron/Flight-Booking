package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/soa/flight-service/internal/repository"
)

const (
	flightTTL = 10 * time.Minute
	searchTTL = 10 * time.Minute
)

// Cache provides Redis-backed caching with Cache-Aside pattern.
type Cache struct {
	client *redis.Client
}

// New creates a new Cache instance.
func New(client *redis.Client) *Cache {
	return &Cache{client: client}
}

func flightKey(id int64) string {
	return fmt.Sprintf("flight:%d", id)
}

func searchKey(origin, destination, date string) string {
	return fmt.Sprintf("search:%s:%s:%s", origin, destination, date)
}

// GetFlight retrieves a cached flight by ID.
// Returns nil, nil on cache miss.
func (c *Cache) GetFlight(ctx context.Context, id int64) (*repository.Flight, error) {
	key := flightKey(id)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		log.Printf("CACHE MISS  key=%s", key)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get %s: %w", key, err)
	}

	var f repository.Flight
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("unmarshal flight: %w", err)
	}
	log.Printf("CACHE HIT   key=%s", key)
	return &f, nil
}

// SetFlight stores a flight in the cache.
func (c *Cache) SetFlight(ctx context.Context, id int64, f *repository.Flight) error {
	key := flightKey(id)
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal flight: %w", err)
	}
	if err := c.client.Set(ctx, key, data, flightTTL).Err(); err != nil {
		return fmt.Errorf("redis set %s: %w", key, err)
	}
	log.Printf("CACHE SET   key=%s ttl=%s", key, flightTTL)
	return nil
}

// DeleteFlight removes a flight from the cache (used on mutation).
func (c *Cache) DeleteFlight(ctx context.Context, id int64) error {
	key := flightKey(id)
	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis del %s: %w", key, err)
	}
	log.Printf("CACHE INVALIDATE key=%s", key)
	return nil
}

// GetSearch retrieves cached search results.
// Returns nil, nil on cache miss.
func (c *Cache) GetSearch(ctx context.Context, origin, destination, date string) ([]*repository.Flight, error) {
	key := searchKey(origin, destination, date)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		log.Printf("CACHE MISS  key=%s", key)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get %s: %w", key, err)
	}

	var flights []*repository.Flight
	if err := json.Unmarshal(data, &flights); err != nil {
		return nil, fmt.Errorf("unmarshal search results: %w", err)
	}
	log.Printf("CACHE HIT   key=%s count=%d", key, len(flights))
	return flights, nil
}

// SetSearch stores search results in the cache.
func (c *Cache) SetSearch(ctx context.Context, origin, destination, date string, flights []*repository.Flight) error {
	key := searchKey(origin, destination, date)
	data, err := json.Marshal(flights)
	if err != nil {
		return fmt.Errorf("marshal search results: %w", err)
	}
	if err := c.client.Set(ctx, key, data, searchTTL).Err(); err != nil {
		return fmt.Errorf("redis set %s: %w", key, err)
	}
	log.Printf("CACHE SET   key=%s count=%d ttl=%s", key, len(flights), searchTTL)
	return nil
}

// DeleteSearchByPattern removes all search cache entries matching a pattern.
// Used to invalidate search results when a flight changes.
func (c *Cache) DeleteSearchByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	var keys []string
	for {
		batch, next, err := c.client.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return fmt.Errorf("redis scan: %w", err)
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if len(keys) == 0 {
		return nil
	}
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("redis del pattern %s: %w", pattern, err)
	}
	log.Printf("CACHE INVALIDATE pattern=%s deleted=%d keys", pattern, len(keys))
	return nil
}
