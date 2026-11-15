package cache

func (c *CacheWithConstTTL[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpiredLocked()
	return c.order.Len()
}
