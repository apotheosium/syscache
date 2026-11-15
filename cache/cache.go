package cache

import (
	"container/list"
	"sync"
	"time"
)

type Cacher interface {
	Add()
	Get()
	Delete()
}
type entry[K comparable, V any] struct {
	key      K
	value    V
	expireAt time.Time
}

type CacheWithConstTTL[K comparable, V any] struct {
	mu       sync.Mutex
	items    map[K]*list.Element
	order    *list.List
	capacity int              // 0 = no limit
	now      func() time.Time // инъекция времени
	ttl      time.Duration
}

func New[K comparable, V any](opts ...Option[K, V]) *CacheWithConstTTL[K, V] {
	c := &CacheWithConstTTL[K, V]{
		items:    make(map[K]*list.Element),
		order:    list.New(),
		capacity: 0,
		now:      time.Now,
		ttl:      time.Hour,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *CacheWithConstTTL[K, V]) evictExpiredLocked() {
	now := c.now()
	for {
		front := c.order.Front()
		if front == nil {
			return
		}
		e := front.Value.(*entry[K, V])
		if e.expireAt.After(now) {
			return
		}
		c.removeFrontLocked()
	}
}

func (c *CacheWithConstTTL[K, V]) removeFrontLocked() {
	front := c.order.Front()
	if front == nil {
		return
	}
	e := front.Value.(*entry[K, V])
	c.order.Remove(front)
	delete(c.items, e.key)
}
