package utils

import (
	"testing"
	"time"
)

func TestGoroutinePool(t *testing.T) {
	// 创建协程池
	pool := NewGoroutinePool(3)

	for i := 0; i < 10; i++ {
		pool.Submit(func() {
			t.Logf("goroutine_id: %v say: hello world", GetGoroutineId())
		})
	}
	time.Sleep(3 * time.Second)
	pool.ShutdownNow()
	t.Logf("exec exit")
}
