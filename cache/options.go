package cache

import "time"

type Option[K comparable, V any] func(*CacheWithConstTTL[K, V])

// WithClock подменяет источник времени.
func WithClock[K comparable, V any](now func() time.Time) Option[K, V] {
	return func(c *CacheWithConstTTL[K, V]) {
		if now != nil {
			c.now = now
		}
	}
} // WithCapacity задаёт лимит записей; при переполнении вытесняется голова

// (самый ранний expireAt). 0 или отрицательное — без лимита.
func WithCapacity[K comparable, V any](capacity int) Option[K, V] {
	return func(c *CacheWithConstTTL[K, V]) {
		c.capacity = capacity
	}
}

func WithTTL[K comparable, V any](ttl time.Duration) Option[K, V] {
	return func(c *CacheWithConstTTL[K, V]) {
		c.ttl = ttl
	}
}
