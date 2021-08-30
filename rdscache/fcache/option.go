package fcache

import (
	"sync"
)

// fCacheOption 函数缓存可选项
type fCacheOption struct {
	lock          sync.Locker // 预防缓存击穿时，需要传入lock
	needCacheZero bool        // 是否需要缓存函数返回结果的零值，默认不需要
	// todo: data需传入非nil指针, 如果为nil反序列化将失败
	data interface{} // data != nil 代表需要将结果UnMarshal到data中
}

func NewFCacheOption(opts ...FCOptionWrap) *fCacheOption {
	option := &fCacheOption{}
	for _, o := range opts {
		o(option)
	}
	return option
}

type FCOptionWrap func(o *fCacheOption)

// WithLock 使用锁，防止缓存击穿
func WithLock(lock sync.Locker) FCOptionWrap {
	return func(option *fCacheOption) {
		option.lock = lock
	}
}

// WithNeedCacheZero 需要缓存函数返回的零值
func WithNeedCacheZero() FCOptionWrap {
	return func(option *fCacheOption) {
		option.needCacheZero = true
	}
}

// WithUnMarshalData 需要将缓存结果反序列化
func WithUnMarshalData(data interface{}) FCOptionWrap {
	return func(option *fCacheOption) {
		option.data = data
	}
}
