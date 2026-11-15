package cache

func (c *CacheWithConstTTL[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictExpiredLocked()

	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	return el.Value.(*entry[K, V]).value, true
}
