package mcache

import (
	"context"
	"errors"
	"sponge/rdscache"
	"sponge/rdscache/common"
	"time"

	"github.com/go-redis/redis"
)

type IModelCacheSvc interface {
	// 从缓存中获取model, 如果不存在则获取原始数据并放入缓存中
	// 从缓存获取到的数据，或者第一次从数据源中获取到的数据，通过model的UnMarshal方法反序列化到model中
	// todo 如果缓存了""，或者数据源返回不存在数据，则返回ErrNoData
	GetOrCreate(ctx context.Context, model ICacheModel, opts ...MCOptionWrap) error
	// 将model保存至缓存中
	Set(ctx context.Context, cacheInfo common.ICacheInfo, cacheStr string) error
}

type mCacheService struct {
	rds *redis.Client
}

func (s *mCacheService) GetOrCreate(ctx context.Context, model ICacheModel, opts ...MCOptionWrap) error {
	if model == nil {
		return rdscache.ErrModuleMustNotNil
	}

	option := NewMCOption(opts...)
	cacheInfo := model.CacheInfo()

	// 从缓存中获取
	needReturn, err := s.getNeedReturn(ctx, cacheInfo, model)
	if needReturn {
		return err
	}

	// 需要预防缓存击穿
	if option.lock != nil {
		option.lock.Lock()
		defer option.lock.Unlock()

		// 拿到锁后再从缓存中获取下
		needReturn, err = s.getNeedReturn(ctx, cacheInfo, model)
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
	err = s.Set(ctx, cacheInfo, cacheStr)
	if err != nil {
		return err
	}

	return noDataErr
}

// getNeedReturn 从缓存中获取后，根据第一个值来判断是否需要直接返回结果
func (s *mCacheService) getNeedReturn(
	ctx context.Context, cacheInfo common.ICacheInfo, model ICacheModel) (bool, error) {

	// 从缓存中获取
	res, err := s.get(ctx, cacheInfo)
	// 报错直接返回错误
	if err != nil && err != redis.Nil {
		return true, err
	}

	// 缓存结果不为空
	if res != "" {
		err = model.UnMarshal(res)
		if err != nil {
			return true, err
		}
		return true, nil
	}

	// 空缓存, 返回数据不存在错误
	if res == "" && err == nil {
		return true, rdscache.ErrNoData
	}

	// 无缓存不可以直接返回
	return false, nil
}

func (s *mCacheService) get(ctx context.Context, cacheInfo common.ICacheInfo) (string, error) {
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

// Set 缓存model，支持缓存零值model, 因为model可能为nil，所以cacheInfo需传入
func (s *mCacheService) Set(ctx context.Context, cacheInfo common.ICacheInfo, cacheStr string) error {
	return s.set(ctx, cacheInfo, cacheStr)
}

// set 在redis中缓存数据
func (s *mCacheService) set(ctx context.Context, cacheInfo common.ICacheInfo, res string) error {
	var err error

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

func NewModelCacheSvc(rds *redis.Client) IModelCacheSvc {
	return &mCacheService{rds: rds}
}
