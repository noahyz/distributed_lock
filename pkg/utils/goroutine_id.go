package utils

import "github.com/v2pro/plz/gls"

func GetGoroutineId() int64 {
	// TODO: 使用 goroutine id 是一种不优雅的做法，这里需要优化
	return gls.GoID()
}
