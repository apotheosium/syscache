package cache_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"syscache/cache"
)

// cacheAPI фиксирует публичный контракт кэша на этапе компиляции.
type cacheAPI[K comparable, V any] interface {
	Set(key K, value V) time.Time
	Get(key K) (V, bool)
	Delete(key K)
	Len() int
}

var _ cacheAPI[string, int] = (*cache.CacheWithConstTTL[string, int])(nil)

// fakeClock — управляемые часы для детерминированных тестов TTL.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{t: t}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

func TestNewDefaults(t *testing.T) {
	c := cache.New[int, int]()

	if got := c.Len(); got != 0 {
		t.Fatalf("Len() нового кэша = %d, ожидалось 0", got)
	}

	before := time.Now()
	exp := c.Set(1, 100)
	after := time.Now()

	if v, ok := c.Get(1); !ok || v != 100 {
		t.Fatalf("Get(1) = (%d, %v), ожидалось (100, true)", v, ok)
	}

	// По умолчанию TTL равен одному часу.
	if exp.Before(before.Add(time.Hour)) || exp.After(after.Add(time.Hour)) {
		t.Fatalf("exp = %v, ожидалось около now + 1h", exp)
	}
}

func TestSetGet(t *testing.T) {
	c := cache.New[string, string]()

	c.Set("hello", "world")

	v, ok := c.Get("hello")
	if !ok {
		t.Fatal("Get(hello) вернул ok = false, ожидалось true")
	}
	if v != "world" {
		t.Fatalf("Get(hello) = %q, ожидалось %q", v, "world")
	}
}

func TestGetMissing(t *testing.T) {
	c := cache.New[string, int]()

	v, ok := c.Get("absent")
	if ok {
		t.Fatal("Get(absent) вернул ok = true, ожидалось false")
	}
	if v != 0 {
		t.Fatalf("Get(absent) = %d, ожидался zero value 0", v)
	}
}

func TestGetMissingCustomValue(t *testing.T) {
	type payload struct {
		Name string
	}

	c := cache.New[string, payload]()

	v, ok := c.Get("absent")
	if ok {
		t.Fatal("Get(absent) вернул ok = true, ожидалось false")
	}
	if v != (payload{}) {
		t.Fatalf("Get(absent) = %+v, ожидалось zero value", v)
	}
}

func TestSetUpdatesExisting(t *testing.T) {
	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	c := cache.New[string, int](
		cache.WithClock[string, int](clock.Now),
		cache.WithTTL[string, int](10*time.Minute),
	)

	first := c.Set("key", 1)
	clock.Advance(5 * time.Minute)
	second := c.Set("key", 2)

	if v, ok := c.Get("key"); !ok || v != 2 {
		t.Fatalf("Get(key) = (%d, %v), ожидалось (2, true)", v, ok)
	}
	if got := c.Len(); got != 1 {
		t.Fatalf("Len() = %d, ожидалось 1 (обновление не должно добавлять запись)", got)
	}
	if !second.Equal(first.Add(5 * time.Minute)) {
		t.Fatalf("exp обновления = %v, ожидалось %v", second, first.Add(5*time.Minute))
	}
	if got, want := second.Sub(clock.Now()), 10*time.Minute; got != want {
		t.Fatalf("остаток TTL после обновления = %v, ожидалось %v", got, want)
	}
}

func TestSetReturnsExpiration(t *testing.T) {
	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	ttl := 30 * time.Second
	c := cache.New[int, int](
		cache.WithClock[int, int](clock.Now),
		cache.WithTTL[int, int](ttl),
	)

	exp := c.Set(1, 1)
	if want := clock.Now().Add(ttl); !exp.Equal(want) {
		t.Fatalf("Set вернул exp = %v, ожидалось %v", exp, want)
	}

	clock.Advance(time.Minute)
	exp = c.Set(2, 2)
	if want := clock.Now().Add(ttl); !exp.Equal(want) {
		t.Fatalf("Set вернул exp = %v, ожидалось %v", exp, want)
	}
}

func TestExpiration(t *testing.T) {
	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	ttl := time.Minute
	c := cache.New[string, int](
		cache.WithClock[string, int](clock.Now),
		cache.WithTTL[string, int](ttl),
	)

	c.Set("key", 1)
	clock.Advance(ttl - time.Nanosecond)

	if v, ok := c.Get("key"); !ok || v != 1 {
		t.Fatalf("до истечения TTL Get(key) = (%d, %v), ожидалось (1, true)", v, ok)
	}

	clock.Advance(time.Nanosecond) // ровно момент истечения

	if v, ok := c.Get("key"); ok || v != 0 {
		t.Fatalf("в момент истечения Get(key) = (%d, %v), ожидалось (0, false)", v, ok)
	}
	if got := c.Len(); got != 0 {
		t.Fatalf("Len() после истечения = %d, ожидалось 0", got)
	}
}

func TestLenEvictsExpired(t *testing.T) {
	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	c := cache.New[string, int](
		cache.WithClock[string, int](clock.Now),
		cache.WithTTL[string, int](time.Minute),
	)

	c.Set("a", 1)
	c.Set("b", 2)
	clock.Advance(2 * time.Minute)

	if got := c.Len(); got != 0 {
		t.Fatalf("Len() = %d, ожидалось 0 после истечения всех записей", got)
	}
}

func TestPartialExpiration(t *testing.T) {
	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	ttl := 10 * time.Minute
	c := cache.New[string, int](
		cache.WithClock[string, int](clock.Now),
		cache.WithTTL[string, int](ttl),
	)

	c.Set("old", 1)
	clock.Advance(5 * time.Minute)
	c.Set("new", 2)

	clock.Advance(5 * time.Minute) // "old" истёк, "new" ещё жив

	if _, ok := c.Get("old"); ok {
		t.Fatal("Get(old) вернул ok = true, ожидалось false")
	}
	if v, ok := c.Get("new"); !ok || v != 2 {
		t.Fatalf("Get(new) = (%d, %v), ожидалось (2, true)", v, ok)
	}
	if got := c.Len(); got != 1 {
		t.Fatalf("Len() = %d, ожидалось 1", got)
	}
}

func TestSetAfterExpiration(t *testing.T) {
	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	c := cache.New[string, int](
		cache.WithClock[string, int](clock.Now),
		cache.WithTTL[string, int](time.Minute),
	)

	c.Set("key", 1)
	clock.Advance(2 * time.Minute)

	if _, ok := c.Get("key"); ok {
		t.Fatal("запись должна была истечь")
	}

	c.Set("key", 2)
	if v, ok := c.Get("key"); !ok || v != 2 {
		t.Fatalf("после повторной записи Get(key) = (%d, %v), ожидалось (2, true)", v, ok)
	}
	if got := c.Len(); got != 1 {
		t.Fatalf("Len() = %d, ожидалось 1", got)
	}
}

func TestZeroTTL(t *testing.T) {
	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	c := cache.New[string, int](
		cache.WithClock[string, int](clock.Now),
		cache.WithTTL[string, int](0),
	)

	c.Set("key", 1)

	if _, ok := c.Get("key"); ok {
		t.Fatal("при TTL = 0 запись должна истекать сразу")
	}
	if got := c.Len(); got != 0 {
		t.Fatalf("Len() = %d, ожидалось 0", got)
	}
}

func TestNegativeTTL(t *testing.T) {
	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	c := cache.New[string, int](
		cache.WithClock[string, int](clock.Now),
		cache.WithTTL[string, int](-time.Minute),
	)

	c.Set("key", 1)

	if _, ok := c.Get("key"); ok {
		t.Fatal("при отрицательном TTL запись должна истекать сразу")
	}
}

func TestCapacityEviction(t *testing.T) {
	c := cache.New[string, int](cache.WithCapacity[string, int](2))

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	if got := c.Len(); got != 2 {
		t.Fatalf("Len() = %d, ожидалось 2", got)
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("самая старая запись a должна быть вытеснена")
	}
	if v, ok := c.Get("b"); !ok || v != 2 {
		t.Fatalf("Get(b) = (%d, %v), ожидалось (2, true)", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != 3 {
		t.Fatalf("Get(c) = (%d, %v), ожидалось (3, true)", v, ok)
	}
}

func TestCapacityOne(t *testing.T) {
	c := cache.New[int, int](cache.WithCapacity[int, int](1))

	c.Set(1, 1)
	c.Set(2, 2)

	if got := c.Len(); got != 1 {
		t.Fatalf("Len() = %d, ожидалось 1", got)
	}
	if _, ok := c.Get(1); ok {
		t.Fatal("запись 1 должна быть вытеснена")
	}
	if _, ok := c.Get(2); !ok {
		t.Fatal("запись 2 должна остаться")
	}
}

func TestCapacityUnlimited(t *testing.T) {
	for _, capacity := range []int{0, -1, -100} {
		t.Run(fmt.Sprintf("capacity=%d", capacity), func(t *testing.T) {
			c := cache.New[int, int](cache.WithCapacity[int, int](capacity))

			const n = 100
			for i := 0; i < n; i++ {
				c.Set(i, i)
			}

			if got := c.Len(); got != n {
				t.Fatalf("Len() = %d, ожидалось %d", got, n)
			}
			for i := 0; i < n; i++ {
				if v, ok := c.Get(i); !ok || v != i {
					t.Fatalf("Get(%d) = (%d, %v), ожидалось (%d, true)", i, v, ok, i)
				}
			}
		})
	}
}

func TestUpdateMovesToBack(t *testing.T) {
	c := cache.New[string, int](cache.WithCapacity[string, int](2))

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("a", 10) // обновление делает a самой свежей
	c.Set("c", 3)  // должна вытесниться b

	if _, ok := c.Get("a"); !ok {
		t.Fatal("обновлённая запись a не должна быть вытеснена")
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("запись b должна быть вытеснена после обновления a")
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("новая запись c должна присутствовать")
	}
}

func TestGetDoesNotAffectEvictionOrder(t *testing.T) {
	c := cache.New[string, int](cache.WithCapacity[string, int](2))

	c.Set("a", 1)
	c.Set("b", 2)

	if _, ok := c.Get("a"); !ok {
		t.Fatal("запись a должна присутствовать до вытеснения")
	}

	c.Set("c", 3) // порядок вытеснения не зависит от Get: вытесняется a

	if _, ok := c.Get("a"); ok {
		t.Fatal("Get не должен продлевать жизнь записи: a должна быть вытеснена")
	}
	if _, ok := c.Get("b"); !ok {
		t.Fatal("запись b должна остаться")
	}
}

func TestUpdateDoesNotEvictAtCapacity(t *testing.T) {
	c := cache.New[string, int](cache.WithCapacity[string, int](2))

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("a", 10)

	if got := c.Len(); got != 2 {
		t.Fatalf("Len() = %d, ожидалось 2", got)
	}
	if v, ok := c.Get("a"); !ok || v != 10 {
		t.Fatalf("Get(a) = (%d, %v), ожидалось (10, true)", v, ok)
	}
	if _, ok := c.Get("b"); !ok {
		t.Fatal("запись b не должна быть вытеснена при обновлении a")
	}
}

func TestDelete(t *testing.T) {
	c := cache.New[string, int]()

	c.Set("a", 1)
	c.Set("b", 2)
	c.Delete("a")

	if _, ok := c.Get("a"); ok {
		t.Fatal("Get(a) после Delete вернул ok = true")
	}
	if v, ok := c.Get("b"); !ok || v != 2 {
		t.Fatalf("Get(b) = (%d, %v), ожидалось (2, true)", v, ok)
	}
	if got := c.Len(); got != 1 {
		t.Fatalf("Len() = %d, ожидалось 1", got)
	}
}

func TestDeleteMissing(t *testing.T) {
	c := cache.New[string, int]()

	// Не должно паниковать и не должно менять состояние.
	c.Delete("absent")
	c.Set("a", 1)
	c.Delete("absent")

	if got := c.Len(); got != 1 {
		t.Fatalf("Len() = %d, ожидалось 1", got)
	}
}

func TestDeleteThenReinsert(t *testing.T) {
	c := cache.New[string, int](cache.WithCapacity[string, int](1))

	c.Set("a", 1)
	c.Delete("a")
	c.Set("a", 2)

	if v, ok := c.Get("a"); !ok || v != 2 {
		t.Fatalf("Get(a) = (%d, %v), ожидалось (2, true)", v, ok)
	}
	if got := c.Len(); got != 1 {
		t.Fatalf("Len() = %d, ожидалось 1", got)
	}
}

func TestLen(t *testing.T) {
	c := cache.New[int, int]()

	if got := c.Len(); got != 0 {
		t.Fatalf("Len() пустого кэша = %d, ожидалось 0", got)
	}

	for i := 0; i < 5; i++ {
		c.Set(i, i)
	}
	if got := c.Len(); got != 5 {
		t.Fatalf("Len() = %d, ожидалось 5", got)
	}

	c.Set(0, 100) // обновление не меняет размер
	if got := c.Len(); got != 5 {
		t.Fatalf("Len() после обновления = %d, ожидалось 5", got)
	}
}

func TestWithClockNilKeepsDefault(t *testing.T) {
	c := cache.New[int, int](cache.WithClock[int, int](nil))

	before := time.Now()
	exp := c.Set(1, 1)
	after := time.Now()

	if exp.Before(before.Add(time.Hour)) || exp.After(after.Add(time.Hour)) {
		t.Fatalf("exp = %v, ожидалось около now + 1h (WithClock(nil) не должен ломать часы)", exp)
	}
	if v, ok := c.Get(1); !ok || v != 1 {
		t.Fatalf("Get(1) = (%d, %v), ожидалось (1, true)", v, ok)
	}
}

func TestCustomTypes(t *testing.T) {
	type user struct {
		ID   int
		Name string
	}

	c := cache.New[int, user](cache.WithTTL[int, user](time.Hour))
	want := user{ID: 7, Name: "Ada"}

	c.Set(42, want)

	got, ok := c.Get(42)
	if !ok {
		t.Fatal("Get(42) вернул ok = false")
	}
	if got != want {
		t.Fatalf("Get(42) = %+v, ожидалось %+v", got, want)
	}
}

func TestOptionsAppliedInOrder(t *testing.T) {
	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	c := cache.New[int, int](
		cache.WithTTL[int, int](time.Hour),
		cache.WithTTL[int, int](time.Minute), // последняя опция побеждает
		cache.WithClock[int, int](clock.Now),
		cache.WithCapacity[int, int](1),
	)

	exp := c.Set(1, 1)
	if want := clock.Now().Add(time.Minute); !exp.Equal(want) {
		t.Fatalf("exp = %v, ожидалось %v", exp, want)
	}

	c.Set(2, 2)
	if _, ok := c.Get(1); ok {
		t.Fatal("при capacity = 1 запись 1 должна быть вытеснена")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := cache.New[int, int](
		cache.WithTTL[int, int](time.Hour),
		cache.WithCapacity[int, int](0),
	)

	const (
		workers   = 8
		perWorker = 200
	)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				key := w*perWorker + i
				c.Set(key, key)

				if v, ok := c.Get(key); !ok || v != key {
					t.Errorf("Get(%d) = (%d, %v), ожидалось (%d, true)", key, v, ok, key)
				}
				c.Len()

				if i%3 == 0 {
					c.Delete(key)
				}
			}
		}(w)
	}
	wg.Wait()

	// Каждый воркер удаляет ceil(perWorker/3) ключей.
	want := workers * (perWorker - (perWorker+2)/3)
	if got := c.Len(); got != want {
		t.Fatalf("Len() = %d, ожидалось %d", got, want)
	}
}

func TestConcurrentSameKey(t *testing.T) {
	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	c := cache.New[string, int](
		cache.WithClock[string, int](clock.Now),
		cache.WithTTL[string, int](time.Hour),
	)

	const (
		workers = 4
		iters   = 500
	)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				c.Set("shared", w)
				c.Get("shared")
				if i%10 == 0 {
					c.Len()
				}
			}
		}(w)
	}
	wg.Wait()

	if got := c.Len(); got != 1 {
		t.Fatalf("Len() = %d, ожидалось 1", got)
	}
	if _, ok := c.Get("shared"); !ok {
		t.Fatal("общий ключ должен присутствовать после конкурентных операций")
	}
}

func ExampleCacheWithConstTTL() {
	c := cache.New[string, int](
		cache.WithTTL[string, int](time.Minute),
		cache.WithCapacity[string, int](2),
	)

	c.Set("a", 1)
	c.Set("b", 2)

	v, ok := c.Get("a")
	fmt.Println(v, ok)
	fmt.Println(c.Len())

	c.Delete("a")
	_, ok = c.Get("a")
	fmt.Println(ok)

	// Output:
	// 1 true
	// 2
	// false
}
