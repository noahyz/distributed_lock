package fair_lock

type FairLockOption func(fairLock *RedisFairLock)

func WithGoroutineWaitTimeMs(goroutineWaitTimeMs int64) FairLockOption {
	return func(r *RedisFairLock) {
		r.goroutineWaitTimeMs = goroutineWaitTimeMs
	}
}

type LockParam struct {
	LeaseTimeMs int64
}

type LockParamOption func(param *LockParam)

func WithLockLeaseTimeMs(leaseTimeMs int64) LockParamOption {
	return func(r *LockParam) {
		r.LeaseTimeMs = leaseTimeMs
	}
}

type TryLockParam struct {
	LeaseTimeMs int64
	WaitTimeMs  int64
}

type TryLockParamOption func(param *TryLockParam)

func WithTryLockLeaseTimeMs(leaseTimeMs int64) TryLockParamOption {
	return func(r *TryLockParam) {
		r.LeaseTimeMs = leaseTimeMs
	}
}

func WithTryLockWaitTimeMs(waitTimeMs int64) TryLockParamOption {
	return func(r *TryLockParam) {
		r.WaitTimeMs = waitTimeMs
	}
}
