package common

import "time"

type ICacheInfo interface {
	BaseInfo() CacheBase
	UpdateCacheKey(key string)
}

type CacheBase struct {
	Key     string        // 缓存的Key
	ExpTime time.Duration // key的过期时间
}

func NewCacheBase(key string, exp time.Duration) *CacheBase {
	ret := &CacheBase{Key: key, ExpTime: exp}
	CheckCacheBase(*ret)
	return ret
}

type StringCache struct {
	CacheBase
}

func (c *StringCache) BaseInfo() CacheBase {
	return c.CacheBase
}

func (c *StringCache) UpdateCacheKey(key string) {
	c.Key = key
}

func NewStringCache(Key string, ExpTime time.Duration) *StringCache {
	base := CacheBase{
		Key:     Key,
		ExpTime: ExpTime,
	}
	CheckCacheBase(base)
	return &StringCache{
		CacheBase: base,
	}
}

type HashCache struct {
	CacheBase
	SubKey string // 子key，对应hash中的field
}

func (c *HashCache) BaseInfo() CacheBase {
	return c.CacheBase
}

func (c *HashCache) UpdateCacheKey(key string) {
	c.Key = key
}

func NewHashCache(Key, SubKey string, ExpTime time.Duration) *HashCache {
	base := CacheBase{
		Key:     Key,
		ExpTime: ExpTime,
	}
	CheckCacheBase(base)
	CheckHashSubKey(SubKey)
	return &HashCache{
		CacheBase: base,
		SubKey:    SubKey,
	}
}
