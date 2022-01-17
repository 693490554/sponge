package mcache

import (
	"context"
	"errors"
	"fmt"
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

// MGetOrCreate 批量从缓存中获取数据, 数据不存在需要回源, 回源后的数据会放入缓存中
// todo-未来功能: 获取对象个数暂不限制(1次性从redis中全部获取)，后续将支持并发分批获取，防止单次获取过多阻塞redis
// todo-未来功能: 后续将支持使用本地缓存解决热key(一批数据中，可能部分数据是热数据，需要从本地缓存中获取)。
// TODO-使用注意事项: mGetFromOriFunc-批量回源查询时，建议如果某条数据不存在时返回nil，即查X条返回X条(其中不存在的为nil)
func (s *mCacheService) MGetOrCreate(
	ctx context.Context, models []ICanMGetModel,
	mGetFromOriFunc func(ctx context.Context, noCacheModels []ICanMGetModel) ([]ICanMGetModel, error),
	optionWraps ...MGetOptionWrap) error {

	if len(models) == 0 {
		return nil
	}

	option := NewMGetOption(optionWraps...)

	// 获取所有的key信息，批量从缓存中获取
	var cacheInfos []common.ICacheInfo
	for _, m := range models {
		cacheInfos = append(cacheInfos, m.CacheInfo())
	}
	cacheValues, err := s.mGet(ctx, cacheInfos)
	if err != nil {
		return err
	}

	// 获取从缓存中没有找到的数据
	var unMarshalErr error
	var noCacheModels []ICanMGetModel
	var noCacheModelsIdxs []int
	for idx, v := range cacheValues {
		// 缓存了空，无需反序列化, 无需回源(防止缓存穿透)
		if v == common.CacheEmptyValue {
			continue
		} else if v != nil && v != common.CacheEmptyValue {
			err = models[idx].UnMarshal(v.(string))
			// 可能是脏数据或者其它原因导致反序列化失败，这种情况打个错误日志，并返回特殊错误
			if err != nil {
				unMarshalErr = rdscache.ErrMGetHaveSomeUnMarshalFail
				fmt.Println("MGet UnMarshal fail ", models[idx])
			}
		} else { // 缓存中无数据
			noCacheModels = append(noCacheModels, models[idx].Clone())
			noCacheModelsIdxs = append(noCacheModelsIdxs, idx)
		}
	}

	// 数据在缓存中全部存在，不用回源，直接返回
	if len(noCacheModels) == 0 {
		return unMarshalErr
	}

	// 批量回源查询数据
	originModels, err := mGetFromOriFunc(ctx, noCacheModels)
	// TODO：回源方法必须返回全部数据, 例如获取三个缓存中不存在的数据，必须返回三个回源数据, 不存在的数据需返回nil
	if len(noCacheModels) != len(originModels) {
		return rdscache.ErrMGetFromOriRetCntNotCorrect
	}
	if err != nil {
		return err
	}

	// 将回源后的数据更新到models中
	for idx, m := range originModels {
		models[noCacheModelsIdxs[idx]].UpdateSelf(m)
	}

	// 将回源后的数据放入缓存
	err = s.mSet(ctx, originModels, noCacheModels, option)
	if err != nil {
		return err
	}

	return unMarshalErr

}

// mGet 批量获取
func (s *mCacheService) mGet(
	ctx context.Context, cacheInfos []common.ICacheInfo) ([]interface{}, error) {
	return s.mGetFromRds(ctx, cacheInfos)
}

// mGetFromRds 从redis中批量获取
func (s *mCacheService) mGetFromRds(
	ctx context.Context, cacheInfos []common.ICacheInfo) ([]interface{}, error) {

	switch cacheInfo := cacheInfos[0].(type) {
	case *common.StringCache:
		var keys []string
		for _, info := range cacheInfos {
			keys = append(keys, info.(*common.StringCache).Key)
		}
		return s.mGetFromString(ctx, keys)
	case *common.HashCache:
		var subKeys []string
		for _, info := range cacheInfos {
			subKeys = append(subKeys, info.(*common.HashCache).SubKey)
		}
		return s.mGetFromHash(ctx, cacheInfo.Key, subKeys)
	default:
		return nil, errors.New("unknown KT")
	}
}

func (s *mCacheService) mGetFromString(ctx context.Context, keys []string) ([]interface{}, error) {
	return s.rds.MGet(keys...).Result()
}

func (s *mCacheService) mGetFromHash(ctx context.Context, key string, subKeys []string) ([]interface{}, error) {
	return s.rds.HMGet(key, subKeys...).Result()
}

// mSet 批量设置缓存
func (s *mCacheService) mSet(
	ctx context.Context, oriModels, noCacheModels []ICanMGetModel,
	option *MGetOption) error {
	return s.mSetToRds(ctx, oriModels, noCacheModels, option)
}

// mGetFromRds 从redis中批量获取
func (s *mCacheService) mSetToRds(
	ctx context.Context, oriModels, noCacheModels []ICanMGetModel,
	option *MGetOption) error {

	switch noCacheModels[0].CacheInfo().(type) {
	case *common.StringCache:
		var mSetModels []*common.MSetModel
		for idx, model := range oriModels {
			var v string
			var err error

			// 回源数据不存在
			if model == nil {
				if !option.needCacheNoData {
					continue
				}
			} else {
				v, err = model.Marshal()
				if err != nil {
					return err
				}
			}
			// 回源数据如果不存在，会返回nil并且会设置到对应的oriModels中，这个时候一些原始信息可能已经改变了
			// 此时通过oriModels可能拿不到正确的CacheInfo(), 所以需要从对应的没有缓存的noCacheModels(Clone自oriModels)中获取缓存信息
			mSetModels = append(
				mSetModels, common.NewMSetModel(
					noCacheModels[idx].CacheInfo().BaseInfo().Key, v,
					noCacheModels[idx].CacheInfo().BaseInfo().ExpTime))
		}

		if len(mSetModels) == 0 {
			return nil
		}
		return s.mSetToString(ctx, mSetModels)
	case *common.HashCache:
		fields := map[string]interface{}{}
		for idx, model := range oriModels {
			var v string
			var err error

			if model == nil {
				if !option.needCacheNoData {
					continue
				}
			} else {
				v, err = model.Marshal()
				if err != nil {
					return err
				}
			}
			fields[noCacheModels[idx].CacheInfo().(*common.HashCache).SubKey] = v
		}

		if len(fields) == 0 {
			return nil
		}
		return s.mSetToHash(
			ctx, noCacheModels[0].CacheInfo().BaseInfo().Key, fields, noCacheModels[0].CacheInfo().BaseInfo().ExpTime)
	default:
		return errors.New("unknown KT")
	}
}

func (s *mCacheService) mSetToString(ctx context.Context, models []*common.MSetModel) error {
	p := s.rds.Pipeline()
	defer func() { _ = p.Close() }()

	var pairs []interface{}
	for _, model := range models {
		pairs = append(pairs, model.Key, model.Value)
	}
	p.MSet(pairs...)

	for _, model := range models {
		p.Expire(model.Key, model.ExpTime)
	}
	_, err := p.Exec()
	return err
}

func (s *mCacheService) mSetToHash(
	ctx context.Context, key string, fields map[string]interface{}, expTime time.Duration) error {
	p := s.rds.Pipeline()
	defer func() { _ = p.Close() }()
	p.HMSet(key, fields)
	p.Expire(key, expTime)
	_, err := p.Exec()
	return err
}

// get 从缓存中获取后，根据第一个值来判断是否需要直接返回结果
func (s *mCacheService) get(
	ctx context.Context, cacheInfo common.ICacheInfo, model ICacheModel, option *MCOption) (directReturn bool, err error) {

	directReturn = true
	var res string
	// 首先判断是否需要进行hot key处理
	hotKeyOption := option.hotKeyOption
	needSetToLocalCache := false
	if hotKeyOption != nil && hotKeyOption.IsHotKey() {
		// 优先考虑使用本地缓存解决
		if hotKeyOption.UseLocalCache() {
			res, err = hotKeyOption.GetFromLocalCache()
			// 存在数据
			if err == nil {
				// 缓存了空直接返回
				if res == common.CacheEmptyValue {
					err = rdscache.ErrNoData
				} else {
					err = model.UnMarshal(res)
				}
			} else {
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

	// 从缓存中获取
	res, err = s.getFromRds(ctx, cacheInfo)
	// 访问redis回调
	if option.getFromRdsCallBack != nil {
		go option.getFromRdsCallBack()
	}

	directReturn = true
	// 报错直接返回错误
	if err != nil {
		if err == redis.Nil {
			directReturn, err = false, nil
		}
	} else {
		// 缓存结果不为空
		if res != common.CacheEmptyValue {
			err = model.UnMarshal(res)
		} else {
			err = rdscache.ErrNoData
		}
	}

	// 本地缓存失效，但是redis缓存存在时，需将数据同步至本地缓存
	if directReturn && needSetToLocalCache {
		err = hotKeyOption.SetToLocalCache(res)
	}
	return
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
	if option != nil {
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
