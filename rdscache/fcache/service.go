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

type CF func() (interface{}, error)

type fCacheService struct {
	rds *redis.Client // 使用redis作为缓存
}

// Get 从缓存中获取缓存内容, 缓存如果不存在则将函数结果保存至缓存
// 需要将结果反序列化到data中时，data必须和cacheFunc返回类型一致！
func (s *fCacheService) Get(ctx context.Context, fc *fCache, data interface{}, cacheFunc CF) (string, error) {
	// 前置校验
	if fc.needUnMarshal && data == nil {
		return "", errors.New("need UnMarshal data must not nil")
	}

	// 从缓存中获取
	needReturn, res, err := s.getNeedReturn(ctx, fc, data)
	if needReturn {
		return res, err
	}

	// 需加锁获取，防止缓存击穿
	if fc.lock != nil {
		fc.lock.Lock()
		defer fc.lock.Unlock()
	}

	// 再从缓存中获取下，有则直接返回，没有代表第一个拿到锁的协程，需从函数中获取缓存信息
	needReturn, res, err = s.getNeedReturn(ctx, fc, data)
	if needReturn {
		if err != nil {
			return "", err
		}
		return res, nil
	}

	// 从函数中获取缓存
	funcRes, err := cacheFunc()
	if err != nil {
		return "", err
	}

	// 放入缓存中
	ret, err := s.set(ctx, fc, funcRes)
	if err != nil {
		return "", err
	}

	if !fc.needUnMarshal {
		return ret, nil
	}

	// 首次放入缓存需要反序列化在这里进行
	err = json.UnmarshalFromString(ret, data)
	if err != nil {
		return "", err
	}

	return ret, nil
}

// getNeedReturn 从缓存中获取后，更具第一个值来判断是否需要直接返回结果
func (s *fCacheService) getNeedReturn(ctx context.Context, fc *fCache, data interface{}) (bool, string, error) {
	// 从缓存中获取
	res, err := s.get(ctx, fc)
	// 报错直接返回错误
	if err != nil && err != redis.Nil {
		return true, "", err
	}

	// 缓存结果不为空或者为空(特意缓存空信息，预防缓存穿透), 直接返回
	if res != "" || res == "" && err != redis.Nil {
		if res != "" && fc.needUnMarshal {
			err = json.UnmarshalFromString(res, data)
		}
		return true, res, err
	}

	// 无缓存不可以直接返回
	return false, "", nil
}

// get 从redis中获取缓存的数据
func (s *fCacheService) get(ctx context.Context, fc *fCache) (string, error) {
	switch fc.kt {
	case common.KTOfString:
		return s.getFromString(ctx, fc.rk)
	case common.KTOfHash:
		return s.getFromHash(ctx, fc.rk, fc.sk)
	default:
		return "", errors.New("unknown KT")
	}
}

// getFromString 从string中获取数据
func (s *fCacheService) getFromString(ctx context.Context, rk string) (string, error) {
	return s.rds.Get(rk).Result()
}

// getFromHash 从hash中获取数据
func (s *fCacheService) getFromHash(ctx context.Context, rk string, sk string) (string, error) {
	return s.rds.HGet(rk, sk).Result()
}

// set 在redis中缓存数据
func (s *fCacheService) set(ctx context.Context, fc *fCache, res interface{}) (string, error) {
	var err error
	var resStr string
	resStr, err = json.MarshalToString(res)
	if err != nil {
		return "", err
	}

	if (res == nil || res != nil && reflect.ValueOf(res).IsZero()) && !fc.needCacheZero {
		return "", nil
	}

	switch fc.kt {
	case common.KTOfString:
		err = s.setToString(ctx, fc.rk, resStr, fc.expTime)
	case common.KTOfHash:
		err = s.setToHash(ctx, fc.rk, fc.sk, resStr, fc.expTime)
	default:
		return "", errors.New("unknown KT")
	}
	return resStr, err
}

// setToString 向string中设置缓存数据
func (s *fCacheService) setToString(ctx context.Context, rk string, res string, expTime time.Duration) error {
	_, err := s.rds.Set(rk, res, expTime).Result()
	return err
}

// setToHash 向hash中设置缓存数据
func (s *fCacheService) setToHash(
	ctx context.Context, rk string, sk string, res string, expTime time.Duration) error {
	_, err := s.rds.HSet(rk, sk, res).Result()
	if err != nil {
		return err
	}

	if expTime <= 0 {
		return nil
	}
	_, err = s.rds.Expire(rk, expTime).Result()
	return err
}

// Del 清除缓存
func (s *fCacheService) Del(ctx context.Context, rk string) error {
	_, err := s.rds.Del(rk).Result()
	return err
}

func NewFCacheService(rds *redis.Client) (*fCacheService, error) {
	if rds == nil {
		return nil, errors.New("redis must not nil")
	}
	return &fCacheService{rds: rds}, nil
}
