package mcache

import (
	"sync"
)

// MCOption model缓存可选项
type MCOption struct {
	lock          sync.Locker // 需要预防缓存击穿时，传入lock
	needCacheZero bool        // 是否需要缓存零值，默认不需要
}

func NewMCOption(opts ...MCOptionWrap) *MCOption {
	ret := &MCOption{}
	for _, op := range opts {
		op(ret)
	}
	return ret
}

type MCOptionWrap func(o *MCOption)

func WithLock(lock sync.Locker) MCOptionWrap {
	return func(o *MCOption) {
		o.lock = lock
	}
}

func WithNeedCacheZero() MCOptionWrap {
	return func(o *MCOption) {
		o.needCacheZero = true
	}
}
