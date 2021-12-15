package rdscache

import "errors"

var (
	ErrNoData               = errors.New("no data error")
	ErrModuleMustNotNil     = errors.New("model must not nil")
	ErrLocalCacheNoData     = errors.New("local cache no data")
	ErrHotKeyOptionInitFail = errors.New("init error, please check your parameter") // 预防热key的方式均为nil
)
