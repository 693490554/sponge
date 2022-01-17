package fcache

import (
	"context"
	"errors"
	"time"

	"github.com/693490554/sponge/rdscache"
	"github.com/693490554/sponge/rdscache/common"
	"github.com/go-redis/redis"
	json "github.com/json-iterator/go"
)

// CF 需要缓存的函数闭包
// @return interface{}: 函数返回值
// @return error: 函数错误信息, 如果数据不存在需要返回ErrNoData
// ErrNoData搭配可选项: WithNeedCacheNoData一起使用，预防缓存穿透
type CF func() (interface{}, error)

type fCacheService struct {
	rds *redis.Client // 使用redis作为缓存
}

// GetOrCreate 从缓存中获取缓存原始内容, 如果缓存不存在则将函数结果放入缓存
func (s *fCacheService) GetOrCreate(
	ctx context.Context, cacheInfo common.ICacheInfo, cacheFunc CF, opts ...FCOptionWrap) (string, error) {
	// 前置校验
	err := common.CheckCacheInfo(cacheInfo)
	if err != nil {
		return "", err
	}
	options := NewFCacheOption(opts...)

	// 从缓存中获取
	directReturn, res, err := s.get(ctx, cacheInfo, options)
	if directReturn {
		return res, err
	}

	// 需加锁获取，防止缓存击穿
	if options.lock != nil {
		options.lock.Lock()
		defer options.lock.Unlock()

		// 再从缓存中获取下，有则直接返回，没有代表第一个拿到锁的协程，需从函数中获取缓存信息
		directReturn, res, err = s.get(ctx, cacheInfo, options)
		if err != nil {
			return "", err
		}
		if directReturn {
			return res, nil
		}
	}

	var noDataErr error
	// 从函数中获取缓存
	funcRes, err := cacheFunc()
	if err != nil && err != rdscache.ErrNoData {
		return "", err
	}
	if err == rdscache.ErrNoData {
		noDataErr = rdscache.ErrNoData
	}

	// 不需要缓存无数据
	if noDataErr != nil && !options.needCacheNoData {
		return common.CacheEmptyValue, noDataErr
	}
	var cacheStr string
	// 数据存在,将函数返回结果进行序列化; 数据如果不存在,则缓存空字符串
	if noDataErr == nil {
		cacheStr, err = json.MarshalToString(funcRes)
		if err != nil {
			return "", err
		}
	}

	// 放入缓存中
	err = s.set(ctx, cacheInfo, cacheStr, options)
	if err != nil {
		return "", err
	}

	// 不需要反序列化到data或无数据直接返回
	if options.data == nil || noDataErr != nil {
		return cacheStr, noDataErr
	}

	// 首次放入缓存需要反序列化在这里进行
	err = json.UnmarshalFromString(cacheStr, options.data)
	if err != nil {
		return "", err
	}

	return cacheStr, nil
}

// get 从缓存中获取后，根据第一个值来判断是否需要直接返回结果
func (s *fCacheService) get(ctx context.Context, cacheInfo common.ICacheInfo, option *fCacheOption) (
	directReturn bool, res string, err error) {

	directReturn = true
	// 首先判断是否需要进行hot key处理
	hotKeyOption := option.hotKeyOption
	needSetToLocalCache := false
	if hotKeyOption != nil && hotKeyOption.IsHotKey() {
		// 优先考虑使用本地缓存解决
		if hotKeyOption.UseLocalCache() {
			res, err = hotKeyOption.GetFromLocalCache()
			// 存在数据(不存在数据时会报错，如果没有错误缓存中肯定是存在数据的)
			if err == nil {
				// 缓存了空直接返回
				if res == common.CacheEmptyValue {
					err = rdscache.ErrNoData
				} else {
					if option.data != nil {
						err = json.UnmarshalFromString(res, option.data)
					}
				}
			} else {
				// 从本地缓存中没拿到，不可以直接返回，并且后续如果从redis中拿到了数据需要放入本地缓存中
				directReturn, needSetToLocalCache = false, true
			}

			if directReturn {
				return
			}
		} else {
			// 利用分片方案解决热key，将原始的key patch掉
			cacheInfo.UpdateCacheKey(hotKeyOption.GetShardingKey())
		}
	}

	// 从redis缓存中获取
	directReturn = true // 默认直接返回, 只有少数情况不可以直接返回
	res, err = s.getFromRds(ctx, cacheInfo)
	// rds访问回调函数, 异步执行
	if option.getFromRdsCallBack != nil {
		go option.getFromRdsCallBack()
	}

	if err != nil {
		if err == redis.Nil {
			directReturn, err = false, nil
		}
	} else {
		// 缓存了空返回无数据异常
		if res == common.CacheEmptyValue {
			err = rdscache.ErrNoData
		} else {
			if option.data != nil {
				err = json.UnmarshalFromString(res, option.data)
			}
		}
	}

	// 本地缓存失效，但是redis缓存存在时，需将数据同步至本地缓存
	if directReturn && needSetToLocalCache {
		err = hotKeyOption.SetToLocalCache(res)
	}
	return
}

// getFromRds 从redis中获取缓存的数据
func (s *fCacheService) getFromRds(ctx context.Context, cacheInfo common.ICacheInfo) (string, error) {
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

// set 将数据放入缓存
func (s *fCacheService) set(
	ctx context.Context, cacheInfo common.ICacheInfo, cacheStr string, option *fCacheOption) error {
	var err error

	// 首先判断是否需要进行hot key处理
	needSetToLocalCache := false
	hotKeyOption := option.hotKeyOption
	if hotKeyOption != nil && hotKeyOption.IsHotKey() {
		// 优先考虑使用本地缓存解决
		if hotKeyOption.UseLocalCache() {
			needSetToLocalCache = true
		} else {
			// 利用分片方案解决热key，将原始的key patch掉
			cacheInfo.UpdateCacheKey(hotKeyOption.GetShardingKey())
		}
	}

	if needSetToLocalCache {
		_ = hotKeyOption.SetToLocalCache(cacheStr)
	}

	switch cacheInfo := cacheInfo.(type) {
	case *common.StringCache:
		err = s.setToString(ctx, cacheInfo.Key, cacheStr, cacheInfo.ExpTime)
	case *common.HashCache:
		err = s.setToHash(ctx, cacheInfo.Key, cacheInfo.SubKey, cacheStr, cacheInfo.ExpTime)
	default:
		err = errors.New("unknown KT")
	}
	return err
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

func NewFCacheService(rds *redis.Client) (*fCacheService, error) {
	if rds == nil {
		return nil, errors.New("redis must not nil")
	}
	return &fCacheService{rds: rds}, nil
}
