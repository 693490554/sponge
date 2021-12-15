package common

import (
	"errors"
)

func CheckCacheBase(base CacheBase) {
	if base.Key == "" {
		panic("key must not empty")
	}
}

func CheckHashSubKey(subKey string) {
	if subKey == "" {
		panic("subKey must not empty")
	}
}

func CheckCacheInfo(cacheInfo interface{}) error {
	switch cacheInfo.(type) {
	case *StringCache:
		return nil
	case *HashCache:
		return nil
	default:
		return errors.New("cache info type error")
	}
}
