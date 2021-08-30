package fcache

import (
	"context"
	"errors"
	"reflect"
	"sponge/rdscache/common"
	"time"

	"github.com/go-redis/redis"
	json "github.com/json-iterator/go"
)

var DataNotExistsErr = errors.New("data not exists")

type CF func() (interface{}, error)

type IFuncCacheSvc interface {
	GetOrCreate(ctx context.Context, cacheInfo common.ICacheInfo, cacheFunc CF, opts ...FCOptionWrap) (string, error)
}

type fCacheService struct {
	rds *redis.Client // 使用redis作为缓存
}

// GetOrCreate 从缓存中获取缓存原始内容, 如果缓存不存在则将函数结果放入缓存
func (s *fCacheService) GetOrCreate(ctx context.Context, cacheInfo common.ICacheInfo, cacheFunc CF, opts ...FCOptionWrap) (string, error) {
	// 前置校验
	err := common.CheckCacheInfo(cacheInfo)
	if err != nil {
		return "", err
	}
	options := NewFCacheOption(opts...)

	// 从缓存中获取
	needReturn, res, err := s.getNeedReturn(ctx, cacheInfo, options.data)
	if needReturn {
		return res, err
	}

	// 需加锁获取，防止缓存击穿
	if options.lock != nil {
		options.lock.Lock()
		defer options.lock.Unlock()

		// 再从缓存中获取下，有则直接返回，没有代表第一个拿到锁的协程，需从函数中获取缓存信息
		needReturn, res, err = s.getNeedReturn(ctx, cacheInfo, options.data)
		if err != nil {
			return "", err
		}
		if needReturn {
			return res, nil
		}
	}

	// 从函数中获取缓存
	funcRes, err := cacheFunc()
	if err != nil {
		return "", err
	}

	// 放入缓存中
	ret, err := s.set(ctx, cacheInfo, funcRes, options)
	if err != nil {
		return "", err
	}

	// 不需要反序列化到data直接返回
	if options.data == nil {
		return ret, nil
	}

	// 函数返回结果为nil或零值直接返回无数据异常
	if funcRes == nil || reflect.ValueOf(funcRes).IsZero() {
		return "", DataNotExistsErr
	}

	// 首次放入缓存需要反序列化在这里进行
	err = json.UnmarshalFromString(ret, options.data)
	if err != nil {
		return "", err
	}

	return ret, nil
}

// getNeedReturn 从缓存中获取后，根据第一个值来判断是否需要直接返回结果
func (s *fCacheService) getNeedReturn(ctx context.Context, cacheInfo common.ICacheInfo, data interface{}) (bool, string, error) {
	// 从缓存中获取
	res, err := s.get(ctx, cacheInfo)
	// 报错直接返回错误
	if err != nil && err != redis.Nil {
		return true, "", err
	}

	// 缓存了空返回无数据异常
	if res == "" && err != redis.Nil {
		return true, "", DataNotExistsErr
	}

	// 缓存结果不为空或者为空(特意缓存空信息，预防缓存穿透), 直接返回
	if res != "" {
		if data == nil {
			return true, res, nil
		}

		err = json.UnmarshalFromString(res, data)
		if err != nil {
			return true, "", err
		}
		return true, res, nil
	}

	// 无缓存不可以直接返回
	return false, "", nil
}

// get 从redis中获取缓存的数据
func (s *fCacheService) get(ctx context.Context, cacheInfo common.ICacheInfo) (string, error) {
	switch cacheInfo := cacheInfo.(type) {
	case *common.StringCache:
		return s.getFromString(ctx, cacheInfo.Key)
	case *common.HashCache:
		return s.getFromHash(ctx, cacheInfo.Key, cacheInfo.SubKey)
	default:
		return "", errors.New("unknown cache type")
	}
}

// getFromString 从string中获取数据
func (s *fCacheService) getFromString(ctx context.Context, key string) (string, error) {
	return s.rds.Get(key).Result()
}

// getFromHash 从hash中获取数据
func (s *fCacheService) getFromHash(ctx context.Context, key string, sk string) (string, error) {
	return s.rds.HGet(key, sk).Result()
}

// set 在redis中缓存数据
func (s *fCacheService) set(
	ctx context.Context, cacheInfo interface{}, res interface{}, options *fCacheOption) (string, error) {

	resIsZero := res == nil || reflect.ValueOf(res).IsZero()

	var err error
	var resStr string
	// res非nil并且非0值才要序列化为str
	if !resIsZero {
		resStr, err = json.MarshalToString(res)
		if err != nil {
			return "", err
		}
	}

	// 无需缓存nil或零值，直接返回
	if resIsZero && !options.needCacheZero {
		return "", nil
	}

	switch cacheInfo := cacheInfo.(type) {
	case *common.StringCache:
		err = s.setToString(ctx, cacheInfo.Key, resStr, cacheInfo.ExpTime)
	case *common.HashCache:
		err = s.setToHash(ctx, cacheInfo.Key, cacheInfo.SubKey, resStr, cacheInfo.ExpTime)
	default:
		return "", errors.New("unknown KT")
	}
	return resStr, err
}

// setToString 向string中设置缓存数据
func (s *fCacheService) setToString(ctx context.Context, key string, res string, expTime time.Duration) error {
	_, err := s.rds.Set(key, res, expTime).Result()
	return err
}

// setToHash 向hash中设置缓存数据
func (s *fCacheService) setToHash(
	ctx context.Context, key string, sk string, res string, expTime time.Duration) error {
	_, err := s.rds.HSet(key, sk, res).Result()
	if err != nil {
		return err
	}

	if expTime <= 0 {
		return nil
	}
	_, err = s.rds.Expire(key, expTime).Result()
	return err
}

func NewFCacheService(rds *redis.Client) (IFuncCacheSvc, error) {
	if rds == nil {
		return nil, errors.New("redis must not nil")
	}
	return &fCacheService{rds: rds}, nil
}
