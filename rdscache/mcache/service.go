package mcache

import (
	"context"
	"errors"
	"time"

	"github.com/693490554/sponge/rdscache"
	"github.com/693490554/sponge/rdscache/common"
	"github.com/go-redis/redis"
)

type mCacheService struct {
	rds *redis.Client
}

// GetOrCreate 从缓存中获取model, 如果不存在则获取原始数据并放入缓存中
// 从缓存获取到的数据，或者第一次从数据源中获取到的数据，通过model的UnMarshal方法反序列化到model中
func (s *mCacheService) GetOrCreate(ctx context.Context, model ICacheModel, opts ...MCOptionWrap) error {
	if model == nil {
		return rdscache.ErrModuleMustNotNil
	}

	option := NewMCOption(opts...)
	cacheInfo := model.CacheInfo()

	// 从缓存中获取
	needReturn, err := s.get(ctx, cacheInfo, model, option)
	if needReturn {
		return err
	}

	// 需要预防缓存击穿
	if option.lock != nil {
		option.lock.Lock()
		defer option.lock.Unlock()

		// 拿到锁后再从缓存中获取下
		needReturn, err = s.get(ctx, cacheInfo, model, option)
		if needReturn {
			return err
		}
	}

	// 不存在则获取数据源
	var noDataErr error
	oriData, err := model.GetOri()
	if err != nil && err != rdscache.ErrNoData {
		return err
	}
	if err == rdscache.ErrNoData {
		noDataErr = rdscache.ErrNoData
	}

	// 不需要缓存零值直接返回
	if noDataErr != nil && !option.needCacheNoData {
		// 是nil或者零值返回不存在数据异常
		return noDataErr
	}

	// 获取需缓存的数据并且缓存下来, noData缓存空字符串
	var cacheStr string
	if noDataErr == nil {
		cacheStr, err = oriData.Marshal()
		if err != nil {
			return err
		}
	}
	err = s.Set(ctx, cacheInfo, cacheStr, option)
	if err != nil {
		return err
	}

	return noDataErr
}

// Set 缓存model，支持缓存零值model, 因为model可能为nil，所以cacheInfo需传入
func (s *mCacheService) Set(
	ctx context.Context, cacheInfo common.ICacheInfo, cacheStr string, option *MCOption) error {
	return s.set(ctx, cacheInfo, cacheStr, option)
}

// get 从缓存中获取后，根据第一个值来判断是否需要直接返回结果
func (s *mCacheService) get(
	ctx context.Context, cacheInfo common.ICacheInfo, model ICacheModel, option *MCOption) (directReturn bool, err error) {

	var res string
	// 首先判断是否需要进行hot key处理
	hotKeyOption := option.hotKeyOption
	if hotKeyOption != nil && hotKeyOption.IsHotKey() {
		// 优先考虑使用本地缓存解决
		if hotKeyOption.UseLocalCache() {
			res, err = hotKeyOption.GetFromLocalCache()
			// 存在数据
			if err == nil {
				// 缓存了空直接返回
				if res == "" {
					return true, rdscache.ErrNoData
				}

				err = model.UnMarshal(res)
				if err != nil {
					return true, err
				}
				return true, nil
			}
		} else {
			// 利用分片方案解决热key，将原始的key patch掉
			cacheInfo.UpdateCacheKey(hotKeyOption.GetShardingKey())
		}
	}
	// 从缓存中获取
	res, err = s.getFromRds(ctx, cacheInfo)
	// 访问redis回调
	if option.getFromRdsCallBack != nil {
		go option.getFromRdsCallBack()
	}
	// 报错直接返回错误
	if err != nil {
		if err != redis.Nil {
			return true, err
		}
		return false, nil
	}

	// 缓存结果不为空
	if res != "" {
		err = model.UnMarshal(res)
		if err != nil {
			return true, err
		}
		return true, nil
	} else {
		return true, rdscache.ErrNoData
	}

}

func (s *mCacheService) getFromRds(ctx context.Context, cacheInfo common.ICacheInfo) (string, error) {
	switch cacheInfo := cacheInfo.(type) {
	case *common.StringCache:
		return s.getFromString(ctx, cacheInfo.Key)
	case *common.HashCache:
		return s.getFromHash(ctx, cacheInfo.Key, cacheInfo.SubKey)
	default:
		return "", errors.New("unknown KT")
	}
}

func (s *mCacheService) getFromString(ctx context.Context, key string) (string, error) {
	return s.rds.Get(key).Result()
}

func (s *mCacheService) getFromHash(ctx context.Context, key string, subKey string) (string, error) {
	return s.rds.HGet(key, subKey).Result()
}

// set 在redis中缓存数据
func (s *mCacheService) set(ctx context.Context, cacheInfo common.ICacheInfo, res string, option *MCOption) error {
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
		_ = hotKeyOption.SetToLocalCache(res)
	}

	switch cacheInfo := cacheInfo.(type) {
	case *common.StringCache:
		err = s.setToString(ctx, cacheInfo.Key, res, cacheInfo.ExpTime)
	case *common.HashCache:
		err = s.setToHash(ctx, cacheInfo.Key, cacheInfo.SubKey, res, cacheInfo.ExpTime)
	default:
		err = errors.New("unknown KT")
	}
	return err
}

// setToString 向string中设置缓存数据
func (s *mCacheService) setToString(ctx context.Context, key string, res string, expTime time.Duration) error {
	_, err := s.rds.Set(key, res, expTime).Result()
	return err
}

// setToHash 向hash中设置缓存数据
func (s *mCacheService) setToHash(
	ctx context.Context, key string, subKey string, res string, expTime time.Duration) error {
	_, err := s.rds.HSet(key, subKey, res).Result()
	if err != nil {
		return err
	}

	if expTime <= 0 {
		return nil
	}
	_, err = s.rds.Expire(key, expTime).Result()
	return err
}

func NewModelCacheSvc(rds *redis.Client) *mCacheService {
	return &mCacheService{rds: rds}
}
