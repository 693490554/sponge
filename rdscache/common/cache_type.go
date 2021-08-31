package common

import "time"

type ICacheInfo interface {
	BaseInfo() cacheBase
}

type cacheBase struct {
	Key     string        // redis Key
	ExpTime time.Duration // key的过期时间
}

type StringCache struct {
	cacheBase
}

func (c *StringCache) BaseInfo() cacheBase {
	return c.cacheBase
}

func NewStringCache(Key string, ExpTime time.Duration) *StringCache {
	base := cacheBase{
		Key:     Key,
		ExpTime: ExpTime,
	}
	CheckCacheBase(base)
	return &StringCache{
		cacheBase: base,
	}
}

type HashCache struct {
	cacheBase
	SubKey string // 子key，对应hash中的field
}

func (c *HashCache) BaseInfo() cacheBase {
	return c.cacheBase
}

func NewHashCache(Key, SubKey string, ExpTime time.Duration) *HashCache {
	base := cacheBase{
		Key:     Key,
		ExpTime: ExpTime,
	}
	CheckCacheBase(base)
	CheckHashSubKey(SubKey)
	return &HashCache{
		cacheBase: base,
		SubKey:    SubKey,
	}
}
