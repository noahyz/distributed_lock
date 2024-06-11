package option

type FairLockParam struct {
	GoroutineWaitTimeMs int64
}

type FairLockParamOption func(fairLock *FairLockParam)

func WithGoroutineWaitTimeMs(goroutineWaitTimeMs int64) FairLockParamOption {
	return func(r *FairLockParam) {
		r.GoroutineWaitTimeMs = goroutineWaitTimeMs
	}
}
