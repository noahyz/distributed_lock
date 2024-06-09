package fair_lock

import (
	"context"
	"distributed-lock/pkg/utils"
	_ "embed"
	"errors"
	"fmt"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/redis/go-redis/v9"
	"time"
)

var (
	//go:embed script/acquire.lua
	acquireLockLuaScript string
	//go:embed script/try_acquire.lua
	tryAcquireLockLuaScript string
	//go:embed script/release.lua
	releaseLockLuaScript string
	//go:embed script/force_release.lua
	forceReleaseLockLuaScript string
	//go:embed script/renewal.lua
	renewalLockLuaScript string

	acquireLua      = redis.NewScript(acquireLockLuaScript)
	tryAcquireLua   = redis.NewScript(tryAcquireLockLuaScript)
	releaseLua      = redis.NewScript(releaseLockLuaScript)
	forceReleaseLua = redis.NewScript(forceReleaseLockLuaScript)
	renewalLua      = redis.NewScript(renewalLockLuaScript)

	ErrorNotObtained = errors.New("redis lock: not obtained")
)

const (
	defaultWatchDogTimeoutMs int64 = 30 * 1000
	// 默认的协程等待时间
	defaultGoroutineWaitTimeMs int64 = 300 * 1000

	defaultUnlockChannelMessage = "unlock message"
)

type RedisFairLock struct {
	// redis 客户端
	redisClient *redis.Client
	// 锁的名字
	fairLockName string
	// 锁续期时间（过期时间）
	lockLeaseTimeMs int64
	// 协程等待时间
	goroutineWaitTimeMs int64
	// 协程等待队列
	goroutineQueueName string
	// 协程超时集合
	timeoutSetName string
	// 此 uuid 用于增大碰撞概率
	uuid string
	// 线程安全的 map，用于存储协程
	renewMap cmap.ConcurrentMap
	// redis 哈希的名字，此哈希的字段是协程，值为加锁次数
	keyHashName string
}

func NewFairLock(redisClient *redis.Client, fairLockName string, opts ...FairLockOption) *RedisFairLock {
	fairLock := &RedisFairLock{
		redisClient:         redisClient,
		fairLockName:        fairLockName,
		lockLeaseTimeMs:     defaultWatchDogTimeoutMs,
		goroutineWaitTimeMs: defaultGoroutineWaitTimeMs,
		goroutineQueueName:  getGoroutineQueueName(fairLockName),
		timeoutSetName:      getTimeoutSetName(fairLockName),
		uuid:                utils.GetUUID(),
		renewMap:            cmap.New(),
		keyHashName:         getKeyHashName(fairLockName),
	}
	for _, opt := range opts {
		opt(fairLock)
	}
	return fairLock
}

func (r *RedisFairLock) Lock(opts ...LockParamOption) error {
	param := &LockParam{
		LeaseTimeMs: -1,
	}
	for _, opt := range opts {
		opt(param)
	}
	return r.lockInner(param.LeaseTimeMs)
}

func (r *RedisFairLock) TryLock(opts ...TryLockParamOption) error {
	param := &TryLockParam{
		LeaseTimeMs: -1,
		WaitTimeMs:  0,
	}
	for _, opt := range opts {
		opt(param)
	}
	return r.tryLockInner(param.WaitTimeMs, param.LeaseTimeMs)
}

func (r *RedisFairLock) Unlock() error {
	goroutineId := utils.GetGoroutineId()
	defer func() {
		r.cancelExpirationRenewal(goroutineId)
	}()
	requestId := utils.GetUUID()
	// TODO 超时时间需要优化
	var unlockLatchExpireTimeMs int64 = 3000
	err := r.tryRelease(requestId, goroutineId, unlockLatchExpireTimeMs)
	if err != nil {
		return err
	}
	return nil
}

func (r *RedisFairLock) ForceUnlock() {
	r.cancelExpirationRenewal(0)
	r.lockLeaseTimeMs = defaultWatchDogTimeoutMs
	_ = r.execForceReleaseLuaScript()
}

func (r *RedisFairLock) IsLocked() bool {
	return r.isExistsFairLock()
}

// GetGoroutineQueueData 获取当前协程等待队列中的数据，用于调试
func (r *RedisFairLock) GetGoroutineQueueData() ([]string, error) {
	key := getGoroutineQueueName(r.fairLockName)
	return r.redisClient.LRange(context.Background(), key, 0, -1).Result()
}

func (r *RedisFairLock) GetTimeoutSetData() ([]redis.Z, error) {
	key := getTimeoutSetName(r.fairLockName)
	return r.redisClient.ZRangeWithScores(context.Background(), key, 0, -1).Result()
}

func (r *RedisFairLock) IsExistHashKey() (bool, error) {
	res, err := r.redisClient.Exists(context.Background(), r.keyHashName).Result()
	if err != nil {
		return false, err
	}
	if res > 0 {
		return true, nil
	}
	return false, nil
}

func (r *RedisFairLock) Close() error {
	return r.redisClient.Close()
}

func (r *RedisFairLock) lockInner(leaseTimeMs int64) error {
	goroutineId := utils.GetGoroutineId()

	// 先尝试加锁一次
	isSuccess, ttlMs, err := r.tryAcquire(-1, leaseTimeMs, goroutineId)
	if err != nil {
		return err
	}
	if isSuccess {
		return nil
	}

	// 订阅 redis 的频道，等待解锁消息
	channelPrefixName := getChannelPrefixName(r.fairLockName)
	lockGoroutineName := getLockName(r.uuid, goroutineId)
	subscribe := r.redisClient.Subscribe(context.Background(), channelPrefixName+"_"+lockGoroutineName)
	defer func() {
		_ = subscribe.Unsubscribe(context.Background(), channelPrefixName+"_"+lockGoroutineName)
		_ = subscribe.Close()
	}()
	// log.Printf("goroutine: %v, locking, channel name: %v", goroutineId, channelPrefixName+"_"+lockGoroutineName)

	// 这里上面已经尝试加了一次锁，为什么不去等待解锁消息，反而继续尝试加锁呢？
	// 原因：协程有可能错过 channel 中的解锁消息，导致等待时间过长
	// 这种场景发生在：还未监听这个 channel 时，消息已经发到这个 channel 了。导致没有接收到这个消息
	// 一直去尝试加锁，直到加锁成功
	for {
		isSuccess, ttlMs, err = r.tryAcquire(-1, leaseTimeMs, goroutineId)
		if err != nil {
			return err
		}
		if isSuccess {
			return nil
		}

		// log.Printf("goroutine: %v, tryAcquire ttlMs: %vms", utils.GetGoroutineId(), ttlMs)
		msgChannel := subscribe.Channel()
		if ttlMs > 0 {
			tCtx, tCancel := context.WithTimeout(context.Background(), time.Duration(ttlMs)*time.Millisecond)
			select {
			case <-msgChannel:
				// log.Printf("goroutine: %v, receive channel msg:%v", utils.GetGoroutineId(), msg)
				tCancel()
			case <-tCtx.Done():
				tCancel()
			}
		} else { // 一般情况下 ttlMs 都是大于 0 的，这里为了程序的健壮性
			select {
			case <-msgChannel:
			}
		}
	}
}

func (r *RedisFairLock) tryLockInner(waitTimeMs int64, leaseTimeMs int64) error {
	// 用于计算加锁剩余时间，如果超时了，直接返回加锁失败
	lockWaitTimeMs := waitTimeMs
	currTimeMs := utils.GetCurrentTimeMs()
	goroutineId := utils.GetGoroutineId()
	// 先尝试加锁一次
	isSuccess, ttlMs, err := r.tryAcquire(waitTimeMs, leaseTimeMs, goroutineId)
	if err != nil {
		return err
	}
	// 加锁成功直接返回
	if isSuccess {
		return nil
	}
	// 协程加锁时间耗尽，无法获取锁
	lockWaitTimeMs -= utils.GetCurrentTimeMs() - currTimeMs
	if lockWaitTimeMs <= 0 {
		return ErrorNotObtained
	}

	// 订阅 redis 的频道，等待解锁消息
	channelPrefixName := getChannelPrefixName(r.fairLockName)
	lockGoroutineName := getLockName(r.uuid, goroutineId)
	subscribe := r.redisClient.Subscribe(context.Background(), channelPrefixName+"_"+lockGoroutineName)
	defer func() {
		_ = subscribe.Unsubscribe(context.Background(), channelPrefixName+"_"+lockGoroutineName)
		_ = subscribe.Close()
	}()

	// 一直尝试加锁，直到协程加锁时间耗尽
	for {
		currTimeMs = utils.GetCurrentTimeMs()
		// ttlMs 表示当前分布式锁的剩余时间
		isSuccess, ttlMs, err = r.tryAcquire(waitTimeMs, leaseTimeMs, goroutineId)
		if err != nil {
			return err
		}
		if isSuccess {
			return nil
		}

		// 协程加锁时间耗尽，无法获取锁
		lockWaitTimeMs -= utils.GetCurrentTimeMs() - currTimeMs
		if lockWaitTimeMs <= 0 {
			return ErrorNotObtained
		}

		// 等待 redis 的频道的解锁消息，等待时间取最小值
		timeoutMs := lockWaitTimeMs
		if ttlMs >= 0 && ttlMs < lockWaitTimeMs {
			timeoutMs = ttlMs
		}
		tCtx, tCancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
		msgChannel := subscribe.Channel()
		select {
		case <-msgChannel:
			tCancel()
		case <-tCtx.Done():
			tCancel()
		}

		// 协程加锁时间耗尽，无法获取锁
		lockWaitTimeMs -= utils.GetCurrentTimeMs() - currTimeMs
		if lockWaitTimeMs <= 0 {
			return ErrorNotObtained
		}
	}
}

func (r *RedisFairLock) tryAcquire(waitTimeMs int64, leaseTimeMs int64, goroutineId int64) (bool, int64, error) {
	expireTimeMs := r.lockLeaseTimeMs
	if leaseTimeMs > 0 {
		expireTimeMs = leaseTimeMs
	}
	isSuccess, ttlMs, err := r.tryAcquireInner(waitTimeMs, expireTimeMs, goroutineId)
	if err != nil {
		return false, -1, err
	}
	// 加锁成功
	if isSuccess {
		if leaseTimeMs > 0 {
			r.lockLeaseTimeMs = leaseTimeMs
		} else {
			r.scheduleExpirationRenewal(goroutineId)
		}
		return true, -1, nil
	}
	return false, ttlMs, nil
}

// 返回值说明: [是否加锁成功，加锁失败情况下当前协程或这把锁的过期时间，错误]
func (r *RedisFairLock) tryAcquireInner(waitTimeMs int64, leaseTimeMs int64, goroutineId int64) (bool, int64, error) {
	goroutineWaitTimeMs := defaultGoroutineWaitTimeMs
	if waitTimeMs > 0 {
		goroutineWaitTimeMs = waitTimeMs
	}
	lockGoroutineName := getLockName(r.uuid, goroutineId)
	currTimeMs := utils.GetCurrentTimeMs()

	// 当前协程去获取锁
	isSuccess, ttlMs, err := r.runAcquireLockLuaScript(lockGoroutineName, leaseTimeMs, goroutineWaitTimeMs, currTimeMs)
	return isSuccess, ttlMs, err
}

// 返回值说明: [是否加锁成功，加锁失败情况下当前协程或这把锁的过期时间，错误]
func (r *RedisFairLock) runAcquireLockLuaScript(
	lockGoroutineName string, leaseTimeMs, goroutineWaitTimeMs, currTimeMs int64) (bool, int64, error) {
	result, err := acquireLua.Run(context.Background(), r.redisClient,
		[]string{r.keyHashName, r.goroutineQueueName, r.timeoutSetName},
		lockGoroutineName, leaseTimeMs, goroutineWaitTimeMs, currTimeMs).Result()
	if err != nil {
		// 加锁成功
		if errors.Is(err, redis.Nil) {
			return true, -1, nil
		}
		return false, -1, err
	}
	if ttlMs, ok := result.(int64); ok {
		return false, ttlMs, nil
	}
	return false, -1, fmt.Errorf("lua script acquire lock of return value is not a number")
}

// 返回值说明: [加锁是否成功，加锁失败时的错误信息]
func (r *RedisFairLock) runOnceAcquireLockLuaScript(
	lockGoroutineName string, leaseTimeMs, goroutineWaitTimeMs, currTimeMs int64) (bool, error) {
	if leaseTimeMs <= 0 {
		leaseTimeMs = r.lockLeaseTimeMs
	}
	_, err := tryAcquireLua.Run(context.Background(), r.redisClient,
		[]string{r.keyHashName, r.goroutineQueueName, r.timeoutSetName},
		lockGoroutineName, leaseTimeMs, goroutineWaitTimeMs, currTimeMs).Result()
	if err != nil {
		// 加锁成功
		if errors.Is(err, redis.Nil) {
			return true, nil
		}
		return false, err
	}
	return false, fmt.Errorf("internal error")
}

func (r *RedisFairLock) scheduleExpirationRenewal(goroutineId int64) {
	entryName := getEntryName(r.uuid, r.fairLockName)
	if oldEntry, ok := r.renewMap.Get(entryName); ok {
		oldEntry.(*RenewEntry).addGoroutineId(goroutineId)
	} else {
		newEntry := NewRenewEntry()
		newEntry.addGoroutineId(goroutineId)
		ctx, cancelFunc := context.WithCancel(context.Background())

		go r.renewExpirationSchedulerGoroutine(ctx, goroutineId)

		newEntry.cancelFunc = cancelFunc
		r.renewMap.Set(entryName, newEntry)
	}
}

func (r *RedisFairLock) renewExpirationSchedulerGoroutine(cancel context.Context, goroutineId int64) {
	entryName := getEntryName(r.uuid, r.fairLockName)

	ticker := time.NewTicker(time.Duration(r.lockLeaseTimeMs/3) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			renew, err := r.renewExpiration(goroutineId)
			if err != nil {
				r.renewMap.Remove(entryName)
				return
			}
			// 表示这个 goroutineId 不在分布式锁(哈希表)中，因此看门狗协程退出
			if renew == 0 {
				r.cancelExpirationRenewal(0)
				return
			}
		case <-cancel.Done():
			return
		}
	}
}

func (r *RedisFairLock) renewExpiration(goroutineId int64) (int64, error) {
	// 返回 1 表示续期成功；返回 0 表示查找到 key
	res, err := renewalLua.Run(context.Background(), r.redisClient, []string{r.keyHashName}, goroutineId, r.lockLeaseTimeMs).Result()
	if err != nil {
		return -1, err
	}
	if isSucc, ok := res.(int64); ok {
		return isSucc, nil
	} else {
		return -1, fmt.Errorf("renewal script return value is not number")
	}
}

func (r *RedisFairLock) cancelExpirationRenewal(goroutineId int64) {
	entryName := getEntryName(r.uuid, r.fairLockName)
	entry, ok := r.renewMap.Get(entryName)
	if !ok {
		return
	}
	task := entry.(*RenewEntry)
	if goroutineId != 0 {
		task.removeGoroutineId(goroutineId)
	}
	if goroutineId == 0 || task.isHasGoroutine() {
		if task.cancelFunc != nil {
			task.cancelFunc()
			task.cancelFunc = nil
		}
		r.renewMap.Remove(entryName)
	}
}

func (r *RedisFairLock) tryRelease(requestId string, goroutineId int64, unlockLatchExpireTimeMs int64) error {
	keys := []string{
		r.keyHashName,
		r.goroutineQueueName,
		r.timeoutSetName,
		getChannelPrefixName(r.fairLockName),
		getUnlockLatchName(requestId),
	}
	args := []interface{}{
		getLockName(r.uuid, goroutineId),
		unlockLatchExpireTimeMs,
		r.lockLeaseTimeMs,
		utils.GetCurrentTimeMs(),
		getPublishCommand(),
		defaultUnlockChannelMessage,
	}
	// log.Printf("goroutine: %v, unlock start, channel name: %v", goroutineId, channelName)
	// 返回 nil，表示当前协程并没有加锁，不用解锁
	// 返回 0，表示当前协程解锁了，但是当前协程因为可重入，还没有删除锁
	// 返回 1，表示当前协程解锁了，并且可重入次数用完，锁释放了
	// 返回 0/1，都表示解锁了，所以不用区分，表示成功
	_, err := releaseLua.Run(context.Background(), r.redisClient, keys, args...).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("attempt was made to unlock, but the goroutine did not hold the lock")
		}
		return err
	}
	return nil
}

func (r *RedisFairLock) execForceReleaseLuaScript() error {
	keys := []string{
		r.keyHashName,
		r.goroutineQueueName,
		r.timeoutSetName,
		getChannelPrefixName(r.fairLockName),
	}
	args := []interface{}{
		getPublishCommand(),
		defaultUnlockChannelMessage,
		utils.GetCurrentTimeMs(),
	}
	// 返回 0，表示当前协程未加锁，没有找到对应的 key
	// 返回 1，表示当前协程加锁了，并且删除了对应的 key，完成强制解锁
	// 返回 0/1，都完成了我们强制解锁的目标，所以不用区分
	_, err := forceReleaseLua.Run(context.Background(), r.redisClient, keys, args...).Result()
	if err != nil {
		return err
	}
	return nil
}

func (r *RedisFairLock) isExistsFairLock() bool {
	exists, err := r.redisClient.Exists(context.Background(), r.keyHashName).Result()
	if err != nil {
		// TODO 这里的错误需要返回
		return false
	}
	if exists > 0 {
		return true
	}
	return false
}
