package common

import "github.com/693490554/sponge/rdscache"

// HotKeyOption 热key处理选项
type HotKeyOption struct {
	// isHotKey 通过注册的函数，用于判断是否是hotKey，结合热key统计可以实现动态热key处理, 如果函数为nil则默认是热key
	isHotKey func() bool
	// getShardingKey 获取key经过分片处理后的key, 支持以分片的方式解决热key问题
	getShardingKey func() string
	// localCache 支持以localCache解决热key, 如果既有分片解决热方案又有localCache解决热key方案，优先以localCache为主
	localCache ILocalCache
	cacheInfo  *CacheBase
}

func (o *HotKeyOption) IsHotKey() bool {
	// 如果判断函数没有传的话, 默认是热key
	if o.isHotKey == nil {
		return true
	}
	return o.isHotKey()
}

// UseLocalCache 当需要解决热key问题时,优先考虑使用本地缓存
func (o *HotKeyOption) UseLocalCache() bool {
	return o.localCache != nil && o.cacheInfo != nil
}

func (o *HotKeyOption) GetFromLocalCache() (string, error) {
	return o.localCache.Get(o.cacheInfo.Key)
}

func (o *HotKeyOption) SetToLocalCache(v string) error {
	return o.localCache.Set(o.cacheInfo, v)
}

func (o *HotKeyOption) GetShardingKey() string {
	return o.getShardingKey()
}

func NewHotKeyOption(opts ...HotKeyOptionWrap) (*HotKeyOption, error) {
	o := &HotKeyOption{}
	for _, opt := range opts {
		opt(o)
	}

	// 策略均没有指定或指定了本地缓存策略,但是缓存信息没有指定
	if o.localCache == nil && o.getShardingKey == nil || o.localCache != nil && o.cacheInfo == nil {
		return nil, rdscache.ErrHotKeyOptionInitFail
	}

	return o, nil
}

type HotKeyOptionWrap func(o *HotKeyOption)

func WithIsHotKey(f func() bool) HotKeyOptionWrap {
	return func(o *HotKeyOption) {
		o.isHotKey = f
	}
}

func WithGetShardingKey(f func() string) HotKeyOptionWrap {
	return func(o *HotKeyOption) {
		o.getShardingKey = f
	}
}

func WithLocalCache(localCache ILocalCache, cacheInfo *CacheBase) HotKeyOptionWrap {
	return func(o *HotKeyOption) {
		o.localCache = localCache
		o.cacheInfo = cacheInfo
	}
}
