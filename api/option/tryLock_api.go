package option

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
