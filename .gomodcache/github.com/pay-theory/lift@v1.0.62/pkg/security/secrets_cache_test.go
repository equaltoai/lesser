package security

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSecretCacheConcurrentExpiredGet(t *testing.T) {
	cache := NewSecretCache(10 * time.Millisecond)
	cache.Set("token", "value")

	time.Sleep(20 * time.Millisecond)

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errs <- fmt.Errorf("panic: %v", r)
				}
			}()

			if val := cache.Get("token"); val != "" {
				errs <- fmt.Errorf("expected empty value, got %q", val)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestSecretCacheReturnsFreshValueAfterExpiration(t *testing.T) {
	cache := NewSecretCache(5 * time.Millisecond)
	cache.Set("token", "stale")
	time.Sleep(10 * time.Millisecond)

	if val := cache.Get("token"); val != "" {
		t.Fatalf("expected empty value for expired secret, got %q", val)
	}

	cache.Set("token", "fresh")
	if val := cache.Get("token"); val != "fresh" {
		t.Fatalf("expected fresh value, got %q", val)
	}
}
