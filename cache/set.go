package cache

import "time"

func (c *CacheWithConstTTL[K, V]) Set(key K, value V) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictExpiredLocked()

	exp := c.now().Add(c.ttl)

	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry[K, V])
		e.value = value
		e.expireAt = exp
		if el != c.order.Back() {
			c.order.MoveToBack(el)
		}
		return exp
	}

	el := c.order.PushBack(&entry[K, V]{key: key, value: value, expireAt: exp})
	c.items[key] = el

	if c.capacity > 0 && c.order.Len() > c.capacity {
		c.removeFrontLocked()
	}
	return exp
}
