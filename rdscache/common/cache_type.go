package common

import "time"

var CacheEmptyValue = "" // 空缓存值

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

func NewStringCache(key string, expTime time.Duration) *StringCache {
	base := CacheBase{
		Key:     key,
		ExpTime: expTime,
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

func NewHashCache(key, subKey string, expTime time.Duration) *HashCache {
	base := CacheBase{
		Key:     key,
		ExpTime: expTime,
	}
	CheckCacheBase(base)
	CheckHashSubKey(subKey)
	return &HashCache{
		CacheBase: base,
		SubKey:    subKey,
	}
}

// MSetModel 批量设置string缓存时的对象信息
type MSetModel struct {
	*StringCache
	Value string
}

func NewMSetModel(key, value string, expTime time.Duration) *MSetModel {
	return &MSetModel{
		StringCache: NewStringCache(key, expTime),
		Value:       value,
	}
}
