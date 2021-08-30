package common

import (
	"errors"
)

func CheckCacheBase(base cacheBase) {
	if base.Key == "" {
		panic("key must not empty")
	}
	if base.ExpTime < 0 {
		panic("expTime must > 0")
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
