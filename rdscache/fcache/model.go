package fcache

import (
	"errors"
	"sponge/rdscache/common"
	"sync"
	"time"
)

// fCache 函数缓存model
type fCache struct {
	rk            string        // redis key
	kt            common.KT     // key type, hash or string
	sk            string        // sub key, 当kt为hash时sk必传
	lock          sync.Locker   // 需要缓存击穿时，需要传入lock
	expTime       time.Duration // 过期时间, 默认0为不设过期时间
	needCacheZero bool          // 是否需要缓存函数返回结果的零值，默认不需要
	needUnMarshal bool          // 缓存结果需要反序列化到data中, 默认为false
}

func NewFCache(rk string, kt common.KT, options ...FCOptionWrap) (*fCache, error) {
	if rk == "" {
		return nil, errors.New("redis key not allow empty")
	}
	if !common.KTIsOk(kt) {
		return nil, errors.New("key type illegal")
	}

	fc := &fCache{
		rk: rk,
		kt: kt,
	}
	fc.ApplyOption(options...)

	if fc.kt == common.KTOfHash && fc.sk == "" {
		return nil, errors.New("sk not allow empty")
	}
	return fc, nil
}

type FCOptionWrap func(fCache *fCache)

// ApplyOption 应用选项
func (o *fCache) ApplyOption(options ...FCOptionWrap) {
	for _, option := range options {
		option(o)
	}
}

// WithSK 当结构为hash的时候, 需传入sub key
func WithSK(sk string) FCOptionWrap {
	return func(option *fCache) {
		option.sk = sk
	}
}

// WithLock 使用锁，防止缓存击穿
func WithLock(lock sync.Locker) FCOptionWrap {
	return func(option *fCache) {
		option.lock = lock
	}
}

// WithExpTime 设置过期时间
func WithExpTime(expTime time.Duration) FCOptionWrap {
	return func(option *fCache) {
		option.expTime = expTime
	}
}

// WithNeedCacheZero 需要缓存函数返回的零值
func WithNeedCacheZero() FCOptionWrap {
	return func(option *fCache) {
		option.needCacheZero = true
	}
}

// WithNeedUnMarshal 需要将缓存结果反序列化
func WithNeedUnMarshal() FCOptionWrap {
	return func(option *fCache) {
		option.needUnMarshal = true
	}
}
