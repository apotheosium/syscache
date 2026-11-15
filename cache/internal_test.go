package cache

import "testing"

// Тесты внутренних помощников: проверяют защитные ветки на пустом кэше.

func TestRemoveFrontLockedEmpty(t *testing.T) {
	c := New[int, int]()

	c.removeFrontLocked() // не должно паниковать

	if got := c.Len(); got != 0 {
		t.Fatalf("Len() = %d, ожидалось 0", got)
	}
}

func TestEvictExpiredLockedEmpty(t *testing.T) {
	c := New[int, int]()

	c.evictExpiredLocked() // не должно паниковать

	if got := c.Len(); got != 0 {
		t.Fatalf("Len() = %d, ожидалось 0", got)
	}
}
