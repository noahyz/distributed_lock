package fair_lock

import (
	"distributed-lock/pkg/utils"
	"errors"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func getRedisFairLockClient(t *testing.T, lockName string) *RedisFairLock {
	t.Helper()
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "123",
		DB:       0,
	})
	redisFairLock := NewFairLock(redisClient, lockName)
	require.NotNilf(t, redisFairLock, "NewFairLock failed")
	return redisFairLock
}

// TestLock 测试 Lock/UnLock 功能是否正常
func TestLock(t *testing.T) {
	lockName := "redis_fair_lock_test_name"
	redisFairLock := getRedisFairLockClient(t, lockName)
	defer redisFairLock.Close()

	// 加锁 & 解锁
	leaseTimeMsOption := WithLockLeaseTimeMs(5000)
	if err := redisFairLock.Lock(leaseTimeMsOption); err != nil {
		t.Errorf("lock err: %v\n", err)
		return
	}
	t.Logf("lock success of lockName: %v\n", lockName)

	time.Sleep(200 * time.Millisecond)

	if err := redisFairLock.Unlock(); err != nil {
		t.Errorf("unlock err: %v\n", err)
		return
	}
	t.Logf("unlock success of lockName: %v\n", lockName)
}

// TestTryLock 测试 TryLock/UnLock 功能是否正常
func TestTryLock(t *testing.T) {
	lockName := "redis_fair_lock_test_name"
	redisFairLock := getRedisFairLockClient(t, lockName)
	defer redisFairLock.Close()

	// 尝试加锁 & 解锁
	leaseTimeMsOption := WithTryLockLeaseTimeMs(5000)
	waitTimeMsOption := WithTryLockWaitTimeMs(5000)
	if err := redisFairLock.TryLock(leaseTimeMsOption, waitTimeMsOption); err != nil {
		t.Errorf("tryLock err: %v\n", err)
		return
	}
	t.Logf("tryLock success of lockName: %v\n", lockName)

	time.Sleep(200 * time.Millisecond)

	if err := redisFairLock.Unlock(); err != nil {
		t.Errorf("unlock err: %v\n", err)
		return
	}
	t.Logf("unlock success of lockName: %v\n", lockName)
}

// TestMultipleLocks 测试多个协程加锁更新一个数
func TestMultipleLocks(t *testing.T) {
	lockName := "redis_fair_lock_test_multiple_lock_name"
	redisFairLock := getRedisFairLockClient(t, lockName)
	defer redisFairLock.Close()

	var wg sync.WaitGroup
	number := 0
	startTime := time.Now()
	// 多个协程不加锁，同时去更新这个 number，导致结果有误
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			number++
			wg.Done()
		}()
	}
	wg.Wait()
	t.Logf("When unlocked, expect number: %d, the result is: %d, cost time: %vns",
		500, number, time.Since(startTime).Nanoseconds())

	var wg2 sync.WaitGroup
	number = 0
	startTime = time.Now()
	// 多个协程加锁，同时去更新这个 number，结果正确
	for i := 0; i < 500; i++ {
		wg2.Add(1)
		go func() {
			leaseTimeMsOption := WithLockLeaseTimeMs(5000)
			if err := redisFairLock.Lock(leaseTimeMsOption); err != nil {
				t.Errorf("lock err: %v", err)
				t.FailNow()
			}
			//t.Logf("goroutine: %v, lock success", utils.GetGoroutineId())
			number++
			if err := redisFairLock.Unlock(); err != nil {
				t.Errorf("unlock err: %v", err)
				t.FailNow()
			}
			//t.Logf("goroutine: %v, unlock sucess", utils.GetGoroutineId())
			wg2.Done()
		}()
	}
	wg2.Wait()
	if number != 500 {
		t.Errorf("When locked, expect number: %d, the result is: %d, cost time: %dns",
			500, number, time.Since(startTime).Nanoseconds())
	} else {
		t.Logf("When locked, expect number: %d, the result is: %d, cost time: %dns",
			500, number, time.Since(startTime).Nanoseconds())
	}
}

// TestTryLockNonDelayed 测试 tryLock 在无延迟/无等待时间的场景下，加锁的情况
// 场景一: 一个协程加锁后，未释放锁之前。另一个协程加锁会失败，并且这个协程会残留在 "协程等待队列" 和 "超时等待队列" 中，直到过期被删除
// 场景二: 当场景一结束后，我们再来一个协程加锁，此时还会失败。因为 "协程等待队列" 中尚有之前的协程
func TestTryLockNonDelayed(t *testing.T) {
	lockName := "redis_fair_lock_test_tryLock_nonDelayed"
	redisFairLock := getRedisFairLockClient(t, lockName)
	defer redisFairLock.Close()

	printFairLockInternalData := func() {
		isExist, err := redisFairLock.IsExistHashKey()
		if err != nil {
			t.Logf("query hash key exist err: %v", err)
		} else {
			t.Logf("hash key exist: %v", isExist)
		}
		goroutineQueueData, err := redisFairLock.GetGoroutineQueueData()
		if err != nil {
			t.Logf("get goroutine queue data err: %v", err)
		} else {
			t.Logf("get goroutine queue data: %v", goroutineQueueData)
		}
		timeoutSetData, err := redisFairLock.GetTimeoutSetData()
		if err != nil {
			t.Logf("get timeout set data: %v", err)
		} else {
			t.Logf("get timeout set data: %v", timeoutSetData)
		}
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.Logf("goroutine: %v, start tryLock(wait: 0ms)", utils.GetGoroutineId())
		waitTimeMsOption := WithTryLockWaitTimeMs(0)
		if err := redisFairLock.TryLock(waitTimeMsOption); err != nil {
			t.Errorf("tryLock err: %v", err)
			return
		}
		t.Logf("goroutine: %v, tryLock(wait: 0ms) success", utils.GetGoroutineId())
		time.Sleep(1 * time.Second)
		if err := redisFairLock.Unlock(); err != nil {
			t.Errorf("unlock err: %v", err)
			return
		}
		t.Logf("goroutine: %v, unlock success", utils.GetGoroutineId())
	}()

	// 规范协程运行顺序，让上面的协程先运行
	time.Sleep(100 * time.Millisecond)
	wg.Add(1)
	go func() {
		defer wg.Done()
		t.Logf("goroutine: %v, start tryLock(wait: 200ms)", utils.GetGoroutineId())
		waitTimeMsOption := WithTryLockWaitTimeMs(200)
		err := redisFairLock.TryLock(waitTimeMsOption)
		// 这里一定会加锁失败，并且返回错误应该是: ErrorNotObtained
		if err != nil {
			if errors.Is(err, ErrorNotObtained) {
				t.Logf("goroutine: %v, tryLock failed, meet expectation", utils.GetGoroutineId())
				printFairLockInternalData()
				return
			}
			t.Errorf("goroutine: %v, tryLock err: %v", utils.GetGoroutineId(), err)
			return
		} else {
			t.Errorf("goroutine: %v, should not lock successfully", utils.GetGoroutineId())
		}
		t.Logf("goroutine: %v, tryLock(wait: 200ms) success", utils.GetGoroutineId())
		if err = redisFairLock.Unlock(); err != nil {
			t.Errorf("unlock err: %v", err)
			return
		}
		t.Logf("goroutine: %v, unlock success", utils.GetGoroutineId())
	}()
	wg.Wait()

	t.Logf("goroutine: %v, start tryLock(wait: 0ms)", utils.GetGoroutineId())
	waitTimeMsOption := WithTryLockWaitTimeMs(0)
	err := redisFairLock.TryLock(waitTimeMsOption)
	// 这里应该加锁失败，并且返回操作应该是: ErrorNotObtained
	if err != nil {
		if errors.Is(err, ErrorNotObtained) {
			t.Logf("goroutine: %v, tryLock failed, meet expectation", utils.GetGoroutineId())
			printFairLockInternalData()
			return
		}
		t.Errorf("goroutine: %v, tryLock err: %v", utils.GetGoroutineId(), err)
		return
	} else {
		t.Errorf("goroutine: %v, should not lock successfully", utils.GetGoroutineId())
	}
	t.Logf("goroutine: %v, tryLock(wait: 0ms) success", utils.GetGoroutineId())
	if err = redisFairLock.Unlock(); err != nil {
		t.Errorf("unlock err: %v", err)
	}
	t.Logf("goroutine: %v, unlock success", utils.GetGoroutineId())
}

// TestWaitTimeoutDrift 测试加锁过程中，锁的超时时间是否在预期范围内
func TestWaitTimeoutDrift(t *testing.T) {
	lockName := "redis_fair_lock_test_wait_timeout_lock_name"
	redisFairLock := getRedisFairLockClient(t, lockName)
	defer redisFairLock.Close()

	var leaseTimeMs int64 = 30000
	// 创建一个只有 3 个协程的协程池，让这三个协程总共加锁 50 次，加锁时设置非常短的超时时间，以及较长的过期时间
	// 这样队列中协程的超时分数将是一个比较大的值
	executor := utils.NewGoroutinePool(3)
	for i := 0; i < 50; i++ {
		executor.Submit(func() {
			waitMs := WithTryLockWaitTimeMs(500)
			leaseMs := WithTryLockLeaseTimeMs(leaseTimeMs)
			if err := redisFairLock.TryLock(waitMs, leaseMs); err == nil {
				t.Logf("goroutine: %v, lock success", utils.GetGoroutineId())
				time.Sleep(10 * time.Second)
				_ = redisFairLock.Unlock()
				t.Logf("goroutine: %v, unlock success", utils.GetGoroutineId())
			}
		})
		// 让协程池中的协程可以均匀的执行
		if i < 3 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	timeoutSet, err := redisFairLock.GetTimeoutSetData()
	require.NoErrorf(t, err, "GetTimeoutSetData failed")
	nowTimeMs := time.Now().UnixMilli()
	for _, item := range timeoutSet {
		t.Logf("item: %v expire: %vms", item.Member, int64(item.Score)-nowTimeMs)
	}

	// 现在再启动一个任务，并在他超时失败之前 kill 他，
	var lastGoroutineTryingToLockStatus = false
	executor.Submit(func() {
		t.Logf("Final goroutine trying to take the lock with goroutine id: %v", utils.GetGoroutineId())
		lastGoroutineTryingToLockStatus = true
		waitMs := WithTryLockWaitTimeMs(30000)
		leaseMs := WithTryLockLeaseTimeMs(30000)
		err = redisFairLock.TryLock(waitMs, leaseMs)
		if err == nil {
			t.Logf("Lock taken by final goroutine: %v", utils.GetGoroutineId())
			time.Sleep(1000 * time.Millisecond)
			_ = redisFairLock.Unlock()
			t.Logf("Lock released by final goroutine: %v", utils.GetGoroutineId())
		}
	})
	// 到这里，等待所有其他协程停止加锁，只有最后一个协程在运行
	for !lastGoroutineTryingToLockStatus {
		time.Sleep(100 * time.Millisecond)
	}
	// 尝试去 kill 最后一个协程，不要让他自己清理
	executor.ShutdownNow()
	// 强制解锁
	redisFairLock.ForceUnlock()
	isLocked := redisFairLock.IsLocked()
	require.Falsef(t, isLocked, "redisFairLock should have been unlocked by now")
	// 检查队列中协程的超时分数，应该都在合理的范围内
	queue, err := redisFairLock.GetGoroutineQueueData()
	require.NoErrorf(t, err, "GetGoroutineQueueData failed")
	t.Logf("queue data: %+v", queue)
	timeoutSet, err = redisFairLock.GetTimeoutSetData()
	require.NoErrorf(t, err, "GetTimeoutSetData failed")
	nowTimeMs = time.Now().UnixMilli()
	for _, item := range timeoutSet {
		expireMs := int64(item.Score) - nowTimeMs
		t.Logf("item: %v, expire : %vms", item.Member, expireMs)
		// 实现中 300000 毫秒是默认的协程等待时间
		if expireMs > leaseTimeMs+300000 {
			t.Errorf("It would take more than %vms to get the lock", leaseTimeMs)
		}
	}
}

// TestLockAcquiredTimeoutDrift 测试锁的超时时间设置是否合理
func TestLockAcquiredTimeoutDrift(t *testing.T) {
	lockName := "redis_fair_lock_test_lock_acquired_timeout_lock_name"
	redisFairLock := getRedisFairLockClient(t, lockName)
	defer redisFairLock.Close()

	// 构建场景：有一个 3 个协程的协程库。使用比较短的等待时间、和较长的过期时间。这样在经过加锁之后
	// 队列中的等待协程的分数，将是一个较大的值
	var leaseTimeMs int64 = 30000
	executor := utils.NewGoroutinePool(3)
	for i := 0; i < 3; i++ {
		currIndex := i
		executor.Submit(func() {
			t.Logf("running %v in goroutineId: %v", currIndex, utils.GetGoroutineId())
			waitMs := WithTryLockWaitTimeMs(3000)
			leaseMs := WithTryLockLeaseTimeMs(leaseTimeMs)
			err := redisFairLock.TryLock(waitMs, leaseMs)
			if err == nil {
				t.Logf("Lock taken by goroutine: %v", utils.GetGoroutineId())
				time.Sleep(100 * time.Millisecond)
				_ = redisFairLock.Unlock()
				t.Logf("Lock released by goroutine: %v", utils.GetGoroutineId())
			}
		})
		// 让协程池中的协程尽量均匀的加锁
		time.Sleep(50 * time.Millisecond)
	}
	lastGoroutineTryingToLock := false
	executor.Submit(func() {
		t.Logf("Final goroutine trying to take the lock with goroutineId: %v", utils.GetGoroutineId())
		lastGoroutineTryingToLock = true
		waitMs := WithTryLockWaitTimeMs(30000)
		leaseMs := WithTryLockLeaseTimeMs(30000)
		err := redisFairLock.TryLock(waitMs, leaseMs)
		if err == nil {
			time.Sleep(1 * time.Second)
			_ = redisFairLock.Unlock()
			t.Logf("Lock released by final goroutine: %v", utils.GetGoroutineId())
		}
	})
	for !lastGoroutineTryingToLock {
		time.Sleep(100 * time.Millisecond)
	}
	executor.ShutdownNow()
	redisFairLock.ForceUnlock()
	isLocked := redisFairLock.IsLocked()
	require.Falsef(t, isLocked, "redisFairLock should have been unlocked by now")
	// 检查队列中协程的超时分数，应该都在合理的范围内
	queue, err := redisFairLock.GetGoroutineQueueData()
	require.NoErrorf(t, err, "GetGoroutineQueueData failed")
	t.Logf("queue data: %+v", queue)
	timeoutSet, err := redisFairLock.GetTimeoutSetData()
	require.NoErrorf(t, err, "GetTimeoutSetData failed")
	nowTimeMs := time.Now().UnixMilli()
	for _, item := range timeoutSet {
		expireMs := int64(item.Score) - nowTimeMs
		t.Logf("item: %v, expire : %vms", item.Member, expireMs)
		// 实现中 300000 毫秒是默认的协程等待时间
		if expireMs > leaseTimeMs+300000 {
			t.Errorf("It would take more than %vms to get the lock", leaseTimeMs)
		}
	}
}

// TestForceUnlock 测试强制解锁
func TestForceUnlock(t *testing.T) {
	lockName := "redis_fair_lock_test_force_unlock_lock_name"
	redisFairLock := getRedisFairLockClient(t, lockName)
	defer redisFairLock.Close()

	err := redisFairLock.Lock()
	require.NoErrorf(t, err, "Lock failed")
	redisFairLock.ForceUnlock()
	require.Falsef(t, redisFairLock.IsLocked(), "Lock status should be Unlock")
}

// TestExpire 测试过期时间
func TestExpire(t *testing.T) {
	lockName := "redis_fair_lock_test_expire_lock_name"
	redisFairLock := getRedisFairLockClient(t, lockName)
	defer redisFairLock.Close()

	// 设置锁过期时间为 2 秒
	waitMs := WithLockLeaseTimeMs(2000)
	err := redisFairLock.Lock(waitMs)
	require.NoErrorf(t, err, "Lock failed")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		startTime := time.Now()
		err = redisFairLock.Lock()
		require.NoErrorf(t, err, "Lock failed")

		spendTimeMs := time.Now().Sub(startTime).Milliseconds()
		t.Logf("spend time: %vms", spendTimeMs)
		require.Truef(t, spendTimeMs < 2020, "lock cost time should less than 2s")

		err = redisFairLock.Unlock()
		require.NoErrorf(t, err, "Unlock failed")
	}()
	wg.Wait()

	err = redisFairLock.Unlock()
	require.NoErrorf(t, err, "Unlock failed")
}
