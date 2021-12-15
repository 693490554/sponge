package mcache

import (
	"sync"

	"github.com/693490554/sponge/rdscache/common"
)

// MCOption model缓存可选项
type MCOption struct {
	lock               sync.Locker          // 需要预防缓存击穿时，传入lock
	needCacheNoData    bool                 // 是否需要缓存无数据的情况
	getFromRdsCallBack func()               // 访问redis时的回调函数，可用于做监控，及热key统计等等
	hotKeyOption       *common.HotKeyOption // 热key处理选项
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

func WithNeedCacheNoData() MCOptionWrap {
	return func(o *MCOption) {
		o.needCacheNoData = true
	}
}

// WithGetFromRdsCallBack 注册从redis获取数据时的回调
func WithGetFromRdsCallBack(cb func()) MCOptionWrap {
	return func(o *MCOption) {
		o.getFromRdsCallBack = cb
	}
}

// WithHotKeyOption 注册从redis获取数据时的回调
func WithHotKeyOption(option *common.HotKeyOption) MCOptionWrap {
	return func(o *MCOption) {
		o.hotKeyOption = option
	}
}
