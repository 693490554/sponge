package mcache

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/693490554/sponge/rdscache"
	"github.com/693490554/sponge/rdscache/common"
	"github.com/allegro/bigcache"
	. "github.com/glycerine/goconvey/convey"
	"github.com/go-redis/redis"
	json "github.com/json-iterator/go"
	goCache "github.com/patrickmn/go-cache"
)

var (
	ctx = context.Background()
	rds = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	mcSvc           = NewModelCacheSvc(rds)
	key             = "testMcache"
	shardingKey     = "testMcache_01"
	subKey          = "subKey"
	lock            = &sync.Mutex{}
	expTime         = time.Duration(0)
	testModelAValue = 1
)

func delTestData() {
	rds.Del(key)
	rds.Del(shardingKey)
}

type TestStringModel struct {
	A int `json:"a"`
}

func (m *TestStringModel) CacheInfo() common.ICacheInfo {
	return common.NewStringCache(key, expTime)
}

func (m *TestStringModel) GetOri() (ICacheModel, error) {

	return &TestStringModel{A: testModelAValue}, nil
}

func (m *TestStringModel) Marshal() (string, error) {
	return json.MarshalToString(m)
}

func (m *TestStringModel) UnMarshal(value string) error {
	return json.UnmarshalFromString(value, m)
}

type TestHashModel struct {
	A int `json:"a"`
}

func (m *TestHashModel) CacheInfo() common.ICacheInfo {
	return common.NewHashCache(key, subKey, expTime)
}

func (m *TestHashModel) GetOri() (ICacheModel, error) {
	return nil, rdscache.ErrNoData
}

func (m *TestHashModel) Marshal() (string, error) {
	return json.MarshalToString(m)
}

func (m *TestHashModel) UnMarshal(value string) error {
	return json.UnmarshalFromString(value, m)
}

func TestMain(m *testing.M) {
	code := m.Run()
	delTestData()
	os.Exit(code)
}

func Test_mCacheService_Set(t *testing.T) {
	Convey("string的model缓存", t, func() {
		delTestData()
		cacheStr, _ := (&TestStringModel{}).Marshal()

		Convey("不存在过期时间", func() {
			cacheInfo := (&TestStringModel{}).CacheInfo()
			err := mcSvc.Set(ctx, cacheInfo, cacheStr, nil)
			So(err, ShouldBeNil)
			v, err := rds.Get(key).Result()
			So(v, ShouldEqual, cacheStr)
			So(err, ShouldBeNil)
			ttl, _ := rds.TTL(key).Result()
			So(ttl, ShouldEqual, -1*time.Second)
		})

		Convey("存在过期时间", func() {
			expTime = 10 * time.Second
			cacheInfo := (&TestStringModel{}).CacheInfo()
			err := mcSvc.Set(ctx, cacheInfo, cacheStr, nil)
			So(err, ShouldBeNil)
			v, err := rds.Get(key).Result()
			So(v, ShouldEqual, cacheStr)
			So(err, ShouldBeNil)
			ttl, _ := rds.TTL(key).Result()
			So(ttl, ShouldBeGreaterThan, 9*time.Second)
			expTime = 0 // 还原全局变量
		})

	})

	Convey("hash的model缓存", t, func() {
		delTestData()
		expTime = 0
		cacheStr, _ := (&TestHashModel{}).Marshal()

		Convey("不存在过期时间", func() {
			cacheInfo := (&TestHashModel{}).CacheInfo()
			err := mcSvc.Set(ctx, cacheInfo, cacheStr, nil)
			So(err, ShouldBeNil)
			v, err := rds.HGet(key, subKey).Result()
			So(v, ShouldEqual, cacheStr)
			So(err, ShouldBeNil)
			ttl, _ := rds.TTL(key).Result()
			So(ttl, ShouldEqual, -1*time.Second)
		})

		Convey("存在过期时间", func() {
			expTime = 10 * time.Second
			cacheInfo := (&TestHashModel{}).CacheInfo()
			err := mcSvc.Set(ctx, cacheInfo, cacheStr, nil)
			So(err, ShouldBeNil)
			v, err := rds.HGet(key, subKey).Result()
			So(v, ShouldEqual, cacheStr)
			So(err, ShouldBeNil)
			ttl, _ := rds.TTL(key).Result()
			So(ttl, ShouldBeGreaterThan, 9*time.Second)
		})

	})
}

func Test_mCacheService_GetOrCreate(t *testing.T) {
	Convey("获取string缓存:原始数据数据不为nil", t, func() {
		delTestData()
		Convey("传入nil", func() {
			err := mcSvc.GetOrCreate(ctx, nil)
			So(err, ShouldEqual, rdscache.ErrModuleMustNotNil)
		})

		Convey("首次获取:model=nil指针", func() {
			var data *TestStringModel
			err := mcSvc.GetOrCreate(ctx, data)
			So(err, ShouldBeNil)

			v, err := rds.Get(key).Result()
			So(v, ShouldNotEqual, "")
			So(err, ShouldBeNil)
		})

		Convey("2次获取:带锁:第一次锁住", func() {
			var data *TestStringModel
			lock.Lock()
			newCtx, cancel := context.WithTimeout(context.Background(), time.Second*2) // 两秒后超时
			defer cancel()
			go func() {
				_ = mcSvc.GetOrCreate(newCtx, data, WithLock(lock))
			}()
			startTs := time.Now().Unix()
			for range newCtx.Done() {
				cancel()
				endTs := time.Now().Unix()
				// 被锁住了超时返回后，缓存中无数据
				_, err := rds.Get(key).Result()
				So(err, ShouldEqual, redis.Nil)
				So(endTs-startTs, ShouldBeGreaterThan, 1)
			}
			// 释放锁, 释放锁后函数会继续运行
			lock.Unlock()
			// 等待函数运行完毕后再清数据
			time.Sleep(time.Second)
			// 因为ctx取消后函数内部并没有实现中断, 清理数据假定为中断没继续运行``
			delTestData()
			err := mcSvc.GetOrCreate(ctx, data, WithLock(lock))
			So(err, ShouldBeNil)
			// 缓存中有数据
			v, err := rds.Get(key).Result()
			So(v, ShouldEqual, `{"a":1}`)
			So(err, ShouldBeNil)
		})

		Convey("2次获取:带锁:正常获取", func() {
			var data *TestStringModel
			// 第一次获取，直接获取原始数据
			err := mcSvc.GetOrCreate(ctx, data, WithLock(lock))
			So(err, ShouldBeNil)
			// 缓存中有数据
			v, err := rds.Get(key).Result()
			So(v, ShouldEqual, `{"a":1}`)
			So(err, ShouldBeNil)

			// 第二次获取会从缓存中获取数据
			err = mcSvc.GetOrCreate(ctx, data, WithLock(lock))
			// 因为data为nil指针，无法被反序列化，会报错
			So(err, ShouldNotBeNil)

			var data2 TestStringModel
			// 零值结构体指针可以被反序列化
			err = mcSvc.GetOrCreate(ctx, &data2, WithLock(lock))
			So(err, ShouldBeNil)
			So(data2.A, ShouldEqual, testModelAValue)
		})

		Convey("2次获取:带锁:模拟并发在等待锁的情况", func() {
			var data TestStringModel
			lock.Lock()
			go func() {
				// 1秒后锁释放，两个协程里的方法竞争获取锁
				time.Sleep(time.Second)
				lock.Unlock()
			}()
			// 模拟并发
			go func() {
				_ = mcSvc.GetOrCreate(ctx, &data, WithLock(lock))
			}()
			// 同时获取两次, 1秒后释放锁喉下面两个方法会竞争一个lock
			// 后拿到锁的直接获取到缓存返回
			err2 := mcSvc.GetOrCreate(ctx, &data, WithLock(lock))
			So(err2, ShouldBeNil)
			// 缓存中有数据
			v, err := rds.Get(key).Result()
			So(v, ShouldEqual, `{"a":1}`)
			So(err, ShouldBeNil)
		})
	})
}

func Test_mCacheService_GetOrCreateUseHash(t *testing.T) {

	Convey("获取hash缓存:原始数据数据不为nil", t, func() {

		delTestData()
		Convey("传入nil", func() {
			err := mcSvc.GetOrCreate(ctx, nil)
			So(err, ShouldEqual, rdscache.ErrModuleMustNotNil)
		})

		Convey("3次获取:第1次不需要缓存空数据:第2次需要缓存空数据", func() {
			var data TestHashModel
			err := mcSvc.GetOrCreate(ctx, &data)
			// hash的获取原始数据方法固定返回nil, nil
			So(err, ShouldEqual, rdscache.ErrNoData)
			_, err = rds.HGet(key, subKey).Result()
			So(err, ShouldEqual, redis.Nil)
			// 第二次获取，将空缓存下来
			err = mcSvc.GetOrCreate(ctx, &data, WithNeedCacheNoData())
			So(err, ShouldEqual, rdscache.ErrNoData)
			v, err := rds.HGet(key, subKey).Result()
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "")
			// 第三次获取
			err = mcSvc.GetOrCreate(ctx, &data, WithNeedCacheNoData())
			So(err, ShouldEqual, rdscache.ErrNoData)
			v, err = rds.HGet(key, subKey).Result()
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "")
		})

		Convey("2次获取:带锁:第一次锁住", func() {
			var data TestHashModel
			lock.Lock()
			newCtx, cancel := context.WithTimeout(context.Background(), time.Second*2) // 两秒后超时
			defer cancel()
			go func() {
				_ = mcSvc.GetOrCreate(newCtx, &data, WithLock(lock))
			}()
			startTs := time.Now().Unix()
			for range newCtx.Done() {
				cancel()
				endTs := time.Now().Unix()
				// 被锁住了超时返回后，缓存中无数据
				_, err := rds.HGet(key, subKey).Result()
				So(err, ShouldEqual, redis.Nil)
				So(endTs-startTs, ShouldBeGreaterThan, 1)
			}
			// 释放锁, 释放锁后函数会继续运行
			lock.Unlock()
			// 等待函数运行完毕后再清数据
			time.Sleep(time.Second)
			// 因为ctx取消后函数内部并没有实现中断, 清理数据假定为中断没继续运行``
			delTestData()
			err := mcSvc.GetOrCreate(ctx, &data, WithLock(lock))
			So(err, ShouldEqual, rdscache.ErrNoData)
			// 未设置缓存零值，缓存中无数据
			_, err = rds.HGet(key, subKey).Result()
			So(err, ShouldEqual, redis.Nil)
		})

		Convey("2次获取:带锁:正常获取", func() {
			var data *TestHashModel
			// 第一次获取，直接获取原始数据
			err := mcSvc.GetOrCreate(ctx, data, WithLock(lock))
			So(err, ShouldEqual, rdscache.ErrNoData)
			// 未设置缓存零值，缓存中无数据
			_, err = rds.HGet(key, subKey).Result()
			So(err, ShouldEqual, redis.Nil)

			// 第二次获取会从缓存中获取数据
			err = mcSvc.GetOrCreate(ctx, data, WithLock(lock))
			So(err, ShouldEqual, rdscache.ErrNoData)
		})
	})
}

func Test_mCacheService_HotKeyOption(t *testing.T) {

	Convey("测试处理热key流程", t, func() {
		delTestData()
		Convey("使用分片方式处理热key", func() {
			data := &TestStringModel{}
			// 热key判断函数结果为false
			hotKeyOption, _ := common.NewHotKeyOption(
				common.WithIsHotKey(func() bool { return false }),
				common.WithGetShardingKey(func() string { return shardingKey }))

			err := mcSvc.GetOrCreate(ctx, data, WithHotKeyOption(hotKeyOption))
			So(err, ShouldBeNil)

			_, err = rds.Get(key).Result()
			So(err, ShouldBeNil)
			_, err = rds.Get(shardingKey).Result()
			So(err, ShouldEqual, redis.Nil)

			// 热key判定结果为true
			hotKeyOption, _ = common.NewHotKeyOption(
				common.WithGetShardingKey(func() string { return shardingKey }))
			_ = mcSvc.GetOrCreate(ctx, data, WithHotKeyOption(hotKeyOption))

			_, err = rds.Get(shardingKey).Result()
			So(err, ShouldBeNil)
		})

		Convey("使用本地缓存-bigCache处理热key", func() {
			data := &TestStringModel{}
			cacheBase := common.NewCacheBase(key, time.Second)
			cache, _ := bigcache.NewBigCache(bigcache.Config{
				LifeWindow: time.Second * 2, CleanWindow: time.Second, Shards: 128,
			})
			wrapBigCache := common.NewWrapBigCache(cache)
			hotKeyOption, _ := common.NewHotKeyOption(
				common.WithLocalCache(wrapBigCache, cacheBase),
				common.WithGetShardingKey(func() string {
					return shardingKey
				}))

			// 第一次本地缓存无数据
			err := mcSvc.GetOrCreate(ctx, data, WithHotKeyOption(hotKeyOption))
			So(err, ShouldBeNil)

			_, err = rds.Get(key).Result()
			So(err, ShouldBeNil)
			_, err = rds.Get(shardingKey).Result()
			So(err, ShouldEqual, redis.Nil)

			localCacheStr, err := wrapBigCache.Get(key)
			So(err, ShouldBeNil)
			So(localCacheStr, ShouldNotEqual, "")

			// 再次获取，走本地缓存
			err = mcSvc.GetOrCreate(ctx, data, WithHotKeyOption(hotKeyOption))
			So(err, ShouldBeNil)

			// 测下localCache是否会失效
			time.Sleep(time.Second)
			localCacheStr, err = wrapBigCache.Get(key)
			So(err, ShouldBeNil)
			So(localCacheStr, ShouldNotEqual, "")

			time.Sleep(time.Second * 2)
			localCacheStr, err = wrapBigCache.Get(key)
			So(err, ShouldEqual, rdscache.ErrLocalCacheNoData)
			So(localCacheStr, ShouldEqual, "")

		})

		Convey("使用本地缓存(goCache)-预防缓存穿透", func() {
			data := &TestHashModel{}

			cacheBase := common.NewCacheBase(key, time.Second*2)
			wrapGoCache := common.NewWrapGoCache(goCache.New(time.Second, time.Second))
			hotKeyOption, _ := common.NewHotKeyOption(
				common.WithLocalCache(wrapGoCache, cacheBase))
			err := mcSvc.GetOrCreate(
				ctx, data, WithHotKeyOption(hotKeyOption), WithNeedCacheNoData(), WithGetFromRdsCallBack(func() {
					fmt.Printf("i am a callback")
				}))

			So(err, ShouldEqual, rdscache.ErrNoData)

			// 再获取一次直接走本地缓存
			err = mcSvc.GetOrCreate(
				ctx, data, WithHotKeyOption(hotKeyOption), WithNeedCacheNoData())
			So(err, ShouldEqual, rdscache.ErrNoData)

			localCacheStr, err := wrapGoCache.Get(key)
			So(err, ShouldBeNil)
			So(localCacheStr, ShouldEqual, "")

			// 测试下本地缓存失效
			time.Sleep(time.Second * 3)
			_, err = wrapGoCache.Get(key)
			So(err, ShouldEqual, rdscache.ErrLocalCacheNoData)
		})

	})
}
