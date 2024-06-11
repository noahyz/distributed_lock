package option

type LockParam struct {
	LeaseTimeMs int64
}

type LockParamOption func(param *LockParam)

func WithLockLeaseTimeMs(leaseTimeMs int64) LockParamOption {
	return func(r *LockParam) {
		r.LeaseTimeMs = leaseTimeMs
	}
}
