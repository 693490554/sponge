package fcache

import (
	"context"
	"errors"
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
	goCache "github.com/patrickmn/go-cache"
)

var (
	ctx = context.Background()
	rds = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	fcSvc, _ = NewFCacheService(rds)
	rk       = "test"
	sk       = "subKey"
	lock     = &sync.Mutex{}
	shardKey = rk + "_01"
)

func delTestData() {
	rds.Del(rk)
	rds.Del(shardKey)
}

func TestMain(m *testing.M) {
	code := m.Run()
	delTestData()
	os.Exit(code)
}

//nolint:typecheck
func Test_fCacheService_GetFromString(t *testing.T) {

	Convey("从string中获取缓存", t, func() {
		// todo fc为整个测试用例所需要的参数，每执行完下面的一个convey，都会回到这里再重新执行初始化，然后执行下一个convey!
		// todo 执行流程并不是从上到下一次性运行完！
		cacheInfo := common.NewStringCache(rk, 0)
		delTestData()

		Convey("需要反序列化到data:函数返回结果为nil", func() {
			var data *struct{}
			So(data, ShouldBeNil)
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return nil, rdscache.ErrNoData
			}, WithUnMarshalData(data))
			So(ret, ShouldEqual, "")
			So(err, ShouldEqual, rdscache.ErrNoData)
			So(data, ShouldBeNil)
		})

		Convey("首次获取:函数无error并且返回nil:无需缓存零值的情况", func() {
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return nil, nil
			})
			So(ret, ShouldEqual, "null")
			So(err, ShouldBeNil)
			// 从缓存中获取下，缓存将不存在
			tmp, err := rds.Get(rk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldEqual, "null")
		})

		Convey("首次获取:函数无error并且返回nil:需缓存零值的情况", func() {
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return nil, nil
			}, WithNeedCacheNoData())
			So(ret, ShouldEqual, "null")
			So(err, ShouldBeNil)
			// 从缓存中获取下，缓存将不存在
			tmp, err := rds.Get(rk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldEqual, "null")
		})

		Convey("首次获取:函数返回error的情况", func() {
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return nil, errors.New("")
			})
			So(ret, ShouldEqual, "")
			So(err, ShouldNotBeNil)
		})

		Convey("首次获取:函数正常返回struct:不需要序列化到data的情况", func() {
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return struct {
					A int
					B string
				}{1, "test"}, nil
			})
			So(ret, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			tmp, err := rds.Get(rk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldNotEqual, "")
		})

		Convey("首次获取:函数正常返回struct:存在lock:需要序列化到data", func() {
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			data := &testS{}
			funcRet := &testS{1, "test"}
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return funcRet, nil
			}, WithUnMarshalData(data), WithLock(lock))
			So(err, ShouldBeNil)
			So(ret, ShouldNotEqual, "")
			_, err = rds.Get(rk).Result()
			So(err, ShouldBeNil)
			// check下过期时间
			expTime, _ := rds.TTL(rk).Result()
			// 无过期时间
			So(expTime, ShouldEqual, -1*time.Second)
		})

		Convey("获取2次:函数正常返回struct:存在lock:需要反序列化", func() {
			tmp, err := rds.Get(rk).Result()
			So(err, ShouldEqual, redis.Nil)
			So(tmp, ShouldEqual, "")

			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			data := &testS{}
			funcRet := &testS{1, "test"}
			for i := 1; i <= 2; i++ {
				ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
					return funcRet, nil
				}, WithUnMarshalData(data), WithLock(lock))
				So(ret, ShouldNotEqual, "")
				So(err, ShouldBeNil)
				So(data.A, ShouldEqual, 1)
				So(data.B, ShouldEqual, "test")
			}
			tmp, err = rds.Get(rk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldNotEqual, "")
		})

		Convey("获取2次:存在lock:lock先上锁:模拟并发获取", func() {

			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			funcRet := &testS{1, "test"}
			lock.Lock()
			go func() {
				time.Sleep(time.Second)
				// 1秒后释放锁，会有2个协程竞争获取锁
				lock.Unlock()
			}()
			go func() {
				_, _ = fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
					return funcRet, nil
				}, WithLock(lock))
			}()
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return funcRet, nil
			}, WithLock(lock))
			So(ret, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			tmp, err := rds.Get(rk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldNotEqual, "")
		})

		Convey("首次获取:函数正常返回struct:存在lock:设过期时间", func() {
			cacheInfo.ExpTime = time.Second * 5
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			funcRet := &testS{1, "test"}
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return funcRet, nil
			}, WithLock(lock))
			So(ret, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			_, err = rds.Get(rk).Result()
			So(err, ShouldBeNil)
			// check下过期时间
			expTime, _ := rds.TTL(rk).Result()
			So(expTime, ShouldBeGreaterThan, time.Second*4)
			So(expTime, ShouldBeLessThanOrEqualTo, time.Second*5)
		})

		Convey("函数正常返回struct:存在lock:lock被锁", func() {
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			funcRet := &testS{1, "test"}
			lock.Lock()
			nowTs := time.Now().Unix()
			newCtx, cFunc := context.WithTimeout(context.Background(), time.Second*2)
			defer cFunc()
			go func() {
				_, _ = fcSvc.GetOrCreate(newCtx, cacheInfo, func() (interface{}, error) {
					return funcRet, nil
				}, WithLock(lock))
			}()
			for range newCtx.Done() {
				useSec := time.Now().Unix() - nowTs
				// 因为被锁住了，所以耗时一定大于1.5秒
				So(useSec, ShouldBeGreaterThan, 1.5)
				break
			}
			// 释放锁后可正常获取
			lock.Unlock()
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return funcRet, nil
			})
			So(ret, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			ret, err = rds.Get(rk).Result()
			So(err, ShouldBeNil)
			So(ret, ShouldNotEqual, "")
		})

	})

}

func Test_fCacheService_GetFromHash(t *testing.T) {

	Convey("从hash中获取缓存", t, func() {
		delTestData()
		cacheInfo := common.NewHashCache(rk, sk, 0)

		Convey("fCacheService未传入rds", func() {
			tmp, err := NewFCacheService(nil)
			So(err, ShouldNotBeNil)
			So(tmp, ShouldBeNil)
		})

		Convey("首次获取:函数无error并且返回nil:无需缓存零值的情况", func() {
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return nil, nil
			})
			So(ret, ShouldEqual, "null")
			So(err, ShouldBeNil)
			// 从缓存中获取下，缓存将不存在
			v, err := rds.HGet(rk, sk).Result()
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "null")
		})

		Convey("获取2次:函数无error并且返回nil和no data:需缓存零值的情况", func() {
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return nil, rdscache.ErrNoData
			}, WithNeedCacheNoData())
			So(ret, ShouldEqual, "")
			So(err, ShouldEqual, rdscache.ErrNoData)
			// 从缓存中获取下，缓存将不存在
			tmp, err := rds.HGet(rk, sk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldEqual, "")

			// 再获取一次
			ret, err = fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return nil, rdscache.ErrNoData
			}, WithNeedCacheNoData())
			So(ret, ShouldEqual, "")
			So(err, ShouldEqual, rdscache.ErrNoData)
		})

		Convey("首次获取:函数返回error的情况", func() {
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return nil, errors.New("")
			})
			So(ret, ShouldEqual, "")
			So(err, ShouldNotBeNil)
		})

		Convey("首次获取:函数正常返回struct:不需要序列化到data的情况", func() {
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return struct {
					A int
					B string
				}{1, "test"}, nil
			})
			So(ret, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			tmp, err := rds.HGet(rk, sk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldNotEqual, "")
		})

		Convey("首次获取:函数正常返回struct:存在lock:需要序列化到data", func() {
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			data := &testS{}
			funcRet := &testS{1, "test"}
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return funcRet, nil
			}, WithUnMarshalData(data), WithLock(lock))
			So(ret, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			So(data.A, ShouldEqual, 1)
			So(data.B, ShouldEqual, "test")
			tmp, err := rds.HGet(rk, sk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldNotEqual, "")
			// check下过期时间
			expTime, _ := rds.TTL(rk).Result()
			// 无过期时间
			So(expTime, ShouldEqual, -1*time.Second)
		})

		Convey("获取2次:函数正常返回struct:存在lock:需要反序列化", func() {
			tmp, err := rds.HGet(rk, sk).Result()
			So(err, ShouldEqual, redis.Nil)
			So(tmp, ShouldEqual, "")

			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			data := &testS{}
			funcRet := &testS{1, "test"}
			for i := 1; i <= 2; i++ {
				ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
					return funcRet, nil
				}, WithUnMarshalData(data), WithLock(lock))
				So(ret, ShouldNotEqual, "")
				So(err, ShouldBeNil)
				So(data.A, ShouldEqual, 1)
				So(data.B, ShouldEqual, "test")
			}
			tmp, err = rds.HGet(rk, sk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldNotEqual, "")
		})

		Convey("首次获取:函数正常返回struct:存在lock:设过期时间", func() {
			cacheInfo.ExpTime = time.Second * 5
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			funcRet := &testS{1, "test"}
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return funcRet, nil
			}, WithLock(lock))
			So(ret, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			tmp, err := rds.HGet(rk, sk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldNotEqual, "")
			// check下过期时间
			expTime, _ := rds.TTL(rk).Result()
			So(expTime, ShouldBeGreaterThan, time.Second*4)
			So(expTime, ShouldBeLessThanOrEqualTo, time.Second*5)
		})

		Convey("函数正常返回struct:存在lock:lock被锁", func() {
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			funcRet := &testS{1, "test"}
			lock.Lock()
			nowTs := time.Now().Unix()
			newCtx, cFunc := context.WithTimeout(context.Background(), time.Second*2)
			defer cFunc()
			go func() {
				_, _ = fcSvc.GetOrCreate(newCtx, cacheInfo, func() (interface{}, error) {
					return funcRet, rdscache.ErrNoData
				}, WithLock(lock))
			}()
			for range newCtx.Done() {
				useSec := time.Now().Unix() - nowTs
				// 因为被锁住了，所以耗时一定大于1.5秒
				So(useSec, ShouldBeGreaterThan, 1.5)
				break
			}
			// 释放锁后可正常获取
			lock.Unlock()
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return funcRet, nil
			})
			So(ret, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			ret, err = rds.HGet(rk, sk).Result()
			So(err, ShouldBeNil)
			So(ret, ShouldNotEqual, "")
		})
	})
}

// Test_fCacheService_HandleHotKey 测试处理热key
func Test_fCacheService_HandleHotKey(t *testing.T) {
	Convey("测试处理热key流程", t, func() {
		delTestData()
		cacheInfo := common.NewStringCache(rk, 0)
		Convey("使用分片方式处理热key", func() {
			// 热key判断函数结果为false
			hotKeyOption, _ := common.NewHotKeyOption(
				common.WithIsHotKey(func() bool { return false }),
				common.WithGetShardingKey(func() string { return shardKey }))

			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return "test", nil
			}, WithHotKeyOption(hotKeyOption))
			So(err, ShouldBeNil)
			So(ret, ShouldNotEqual, "test")

			_, err = rds.Get(rk).Result()
			So(err, ShouldBeNil)
			_, err = rds.Get(shardKey).Result()
			So(err, ShouldEqual, redis.Nil)

			// 热key判定结果为true
			hotKeyOption, _ = common.NewHotKeyOption(
				common.WithGetShardingKey(func() string { return shardKey }))
			_, err = fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return "test", nil
			}, WithHotKeyOption(hotKeyOption))
			So(err, ShouldBeNil)

			_, err = rds.Get(shardKey).Result()
			So(err, ShouldBeNil)
		})

		Convey("使用本地缓存-bigCache处理热key", func() {

			cacheBase := common.NewCacheBase(rk, time.Second)
			cache, _ := bigcache.NewBigCache(bigcache.Config{
				LifeWindow: time.Second * 2, CleanWindow: time.Second, Shards: 128,
			})
			wrapBigCache := common.NewWrapBigCache(cache)
			hotKeyOption, _ := common.NewHotKeyOption(
				common.WithLocalCache(wrapBigCache, cacheBase),
				common.WithGetShardingKey(func() string {
					return shardKey
				}))

			// 第一次获取，会将回源数据放入到redis和localCache中
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return "test", nil
			}, WithHotKeyOption(hotKeyOption))
			So(err, ShouldBeNil)
			So(ret, ShouldEqual, `"test"`)

			_, err = rds.Get(rk).Result()
			So(err, ShouldBeNil)
			_, err = rds.Get(shardKey).Result()
			So(err, ShouldEqual, redis.Nil)

			localCacheStr, err := wrapBigCache.Get(rk)
			So(err, ShouldBeNil)
			So(localCacheStr, ShouldEqual, `"test"`)

			// 再次获取，走本地缓存, 测试下将数据反序列化到data中
			var data string
			ret, err = fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return "test", nil
			}, WithHotKeyOption(hotKeyOption), WithUnMarshalData(&data))
			So(err, ShouldBeNil)
			So(ret, ShouldEqual, `"test"`)
			So(data, ShouldEqual, "test")

			// 测下localCache是否会失效
			time.Sleep(time.Second)
			localCacheStr, err = wrapBigCache.Get(rk)
			So(err, ShouldBeNil)
			So(localCacheStr, ShouldEqual, `"test"`)

			time.Sleep(time.Second * 2)
			localCacheStr, err = wrapBigCache.Get(rk)
			So(err, ShouldEqual, rdscache.ErrLocalCacheNoData)
			So(localCacheStr, ShouldEqual, "")

			// 本地缓存已失效，但是redis缓存未失效
			// 第一次获取会将redis缓存放入本地缓存
			ret, err = fcSvc.GetOrCreate(
				ctx, cacheInfo, func() (interface{}, error) {
					return "test", nil
				}, WithHotKeyOption(hotKeyOption))
			So(err, ShouldBeNil)
			So(ret, ShouldEqual, `"test"`)

			// 再获取一次直接走本地缓存，仍可正常获取
			ret, err = fcSvc.GetOrCreate(
				ctx, cacheInfo, func() (interface{}, error) {
					return "test", nil
				}, WithHotKeyOption(hotKeyOption))
			So(err, ShouldBeNil)
			So(ret, ShouldEqual, `"test"`)
		})

		Convey("使用本地缓存(goCache)-预防缓存穿透", func() {

			cacheBase := common.NewCacheBase(rk, time.Second*2)
			wrapGoCache := common.NewWrapGoCache(goCache.New(time.Second, time.Second))
			hotKeyOption, _ := common.NewHotKeyOption(
				common.WithLocalCache(wrapGoCache, cacheBase))
			ret, err := fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return "", rdscache.ErrNoData
			}, WithHotKeyOption(hotKeyOption), WithNeedCacheNoData(), WithGetFromRdsCallBack(func() {
				fmt.Printf("i am a callback")
			}))
			So(err, ShouldEqual, rdscache.ErrNoData)
			So(ret, ShouldEqual, "")

			// 再获取一次直接走本地缓存
			ret, err = fcSvc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
				return "", rdscache.ErrNoData
			}, WithHotKeyOption(hotKeyOption), WithNeedCacheNoData())
			So(err, ShouldEqual, rdscache.ErrNoData)
			So(ret, ShouldEqual, "")

			localCacheStr, err := wrapGoCache.Get(rk)
			So(err, ShouldBeNil)
			So(localCacheStr, ShouldEqual, "")

			// 测试下本地缓存失效
			time.Sleep(time.Second * 3)
			_, err = wrapGoCache.Get(rk)
			So(err, ShouldEqual, rdscache.ErrLocalCacheNoData)
		})

	})
}
