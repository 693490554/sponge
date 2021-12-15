package common

import (
	"errors"

	"github.com/693490554/sponge/rdscache"
	"github.com/allegro/bigcache"
	goCache "github.com/patrickmn/go-cache"
)

// ILocalCache 本地缓存抽象，主要用于解决hot key
type ILocalCache interface {
	// Get 如果数据不存在，则会返回ErrLocalCacheNoData错误
	Get(key string) (string, error)
	Set(cacheInfo *CacheBase, value string) error
	Del(key string) error
}

type wrapBigCache struct {
	*bigcache.BigCache
}

func (obj *wrapBigCache) Get(key string) (string, error) {
	ret, err := obj.BigCache.Get(key)
	if err != nil {
		if errors.Is(err, bigcache.ErrEntryNotFound) {
			return "", rdscache.ErrLocalCacheNoData
		}
		return "", err
	}

	return string(ret), nil
}

func (obj *wrapBigCache) Set(cacheInfo *CacheBase, value string) error {
	return obj.BigCache.Set(cacheInfo.Key, []byte(value))
}

func (obj *wrapBigCache) Del(key string) error {
	return obj.BigCache.Delete(key)
}

func NewWrapBigCache(bigCache *bigcache.BigCache) ILocalCache {
	return &wrapBigCache{bigCache}
}

type wrapGoCache struct {
	*goCache.Cache
}

func (obj *wrapGoCache) Get(key string) (string, error) {
	ret, find := obj.Cache.Get(key)
	if !find {
		return "", rdscache.ErrLocalCacheNoData
	}

	return ret.(string), nil
}

func (obj *wrapGoCache) Set(cacheInfo *CacheBase, value string) error {
	obj.Cache.Set(cacheInfo.Key, value, cacheInfo.ExpTime)
	return nil
}

func (obj *wrapGoCache) Del(key string) error {
	obj.Cache.Delete(key)
	return nil
}

func NewWrapGoCache(cache *goCache.Cache) ILocalCache {
	return &wrapGoCache{cache}
}
