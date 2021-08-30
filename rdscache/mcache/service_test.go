package mcache

import (
	"context"
	"os"
	"sponge/rdscache/common"
	"sync"
	"testing"
	"time"

	. "github.com/glycerine/goconvey/convey"
	"github.com/go-redis/redis"
	json "github.com/json-iterator/go"
)

var (
	ctx = context.Background()
	rds = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	mcSvc           = NewModelCacheSvc(rds)
	key             = "test"
	subKey          = "subKey"
	lock            = &sync.Mutex{}
	expTime         = time.Duration(0)
	testModelAValue = 1
)

func delTestData() {
	rds.Del(key)
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
	return nil, nil
}

func (m *TestHashModel) Marshal() (string, error) {
	return json.MarshalToString(m)
}

func (m *TestHashModel) UnMarshal(value string) error {
	return json.UnmarshalFromString(value, m)
}

func TestMain(m *testing.M) {
	m.Run()
	delTestData()
	os.Exit(0)
}

func Test_mCacheService_Set(t *testing.T) {
	Convey("string的model缓存", t, func() {
		delTestData()
		cacheInfo := (&TestStringModel{}).CacheInfo()

		Convey("传入nil", func() {
			err := mcSvc.Set(ctx, nil, cacheInfo)
			So(err, ShouldBeNil)
			v, err := rds.Get(key).Result()
			So(v, ShouldEqual, "")
			So(err, ShouldBeNil)
		})

		Convey("nil指针", func() {
			var data *TestStringModel
			err := mcSvc.Set(ctx, data, cacheInfo)
			So(err, ShouldBeNil)

			v, err := rds.Get(key).Result()
			So(v, ShouldEqual, "")
			So(err, ShouldBeNil)
		})

		Convey("空指针", func() {
			data := &TestStringModel{}
			err := mcSvc.Set(ctx, data, cacheInfo)
			So(err, ShouldBeNil)

			v, err := rds.Get(key).Result()
			So(v, ShouldEqual, `{"a":0}`) // 空指针不是零值
			So(err, ShouldBeNil)
		})

		Convey("非空指针", func() {
			data := &TestStringModel{A: testModelAValue}
			err := mcSvc.Set(ctx, data, cacheInfo)
			So(err, ShouldBeNil)

			v, err := rds.Get(key).Result()
			So(v, ShouldEqual, `{"a":1}`)
			So(err, ShouldBeNil)
		})
	})

	Convey("hash的model缓存", t, func() {
		delTestData()
		cacheInfo := (&TestHashModel{}).CacheInfo()

		Convey("传入nil", func() {
			err := mcSvc.Set(ctx, nil, cacheInfo)
			So(err, ShouldBeNil)
			v, err := rds.HGet(key, subKey).Result()
			So(v, ShouldEqual, "")
			So(err, ShouldBeNil)
		})

		Convey("nil指针", func() {
			var data *TestHashModel
			err := mcSvc.Set(ctx, data, cacheInfo)
			So(err, ShouldBeNil)

			v, err := rds.HGet(key, subKey).Result()
			So(v, ShouldEqual, "")
			So(err, ShouldBeNil)
		})

		Convey("空指针", func() {
			data := &TestHashModel{}
			err := mcSvc.Set(ctx, data, cacheInfo)
			So(err, ShouldBeNil)

			v, err := rds.HGet(key, subKey).Result()
			So(v, ShouldEqual, `{"a":0}`) // 空指针不是零值
			So(err, ShouldBeNil)
		})

		Convey("非空指针", func() {
			data := &TestHashModel{A: testModelAValue}
			err := mcSvc.Set(ctx, data, cacheInfo)
			So(err, ShouldBeNil)

			v, err := rds.HGet(key, subKey).Result()
			So(v, ShouldEqual, `{"a":1}`)
			So(err, ShouldBeNil)
		})

	})
}

func Test_mCacheService_GetOrCreate(t *testing.T) {
	Convey("获取string缓存:原始数据数据不为nil", t, func() {
		delTestData()
		Convey("传入nil", func() {
			err := mcSvc.GetOrCreate(ctx, nil)
			So(err, ShouldNotBeNil)
		})

		Convey("首次获取:model=nil指针", func() {
			var data *TestStringModel
			err := mcSvc.GetOrCreate(ctx, data)
			So(err, ShouldBeNil)

			v, err := rds.Get(key).Result()
			So(v, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			dur, err := rds.TTL(key).Result()
			So(err, ShouldBeNil)
			So(dur, ShouldEqual, time.Duration(-1)*time.Second)
		})

		Convey("2次获取:带锁:第一次锁住:缓存存在过期时间", func() {
			expTime = time.Second * 10
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
			// check下缓存过期时间
			dur, err := rds.TTL(key).Result()
			So(err, ShouldBeNil)
			So(dur, ShouldBeGreaterThan, time.Duration(0))
			expTime = time.Duration(0) // 还原
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
			So(err, ShouldNotBeNil)
		})

		Convey("3次获取:第1次不需要缓存空数据:第2次需要缓存空数据+过期时间", func() {
			var data TestHashModel
			err := mcSvc.GetOrCreate(ctx, &data)
			// hash的获取原始数据方法固定返回nil, nil
			So(err, ShouldEqual, DataNotExistsErr)
			_, err = rds.HGet(key, subKey).Result()
			So(err, ShouldEqual, redis.Nil)
			// 第二次获取，将空缓存下来
			expTime = time.Second * 5
			err = mcSvc.GetOrCreate(ctx, &data, WithNeedCacheZero())
			So(err, ShouldEqual, DataNotExistsErr)
			v, err := rds.HGet(key, subKey).Result()
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "")
			dur, err := rds.TTL(key).Result()
			So(err, ShouldBeNil)
			So(dur, ShouldBeGreaterThan, 4*time.Second)
			expTime = 0 //还原
			// 第三次获取
			err = mcSvc.GetOrCreate(ctx, &data, WithNeedCacheZero())
			So(err, ShouldEqual, DataNotExistsErr)
			v, err = rds.HGet(key, subKey).Result()
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "")
		})

		Convey("2次获取:带锁:第一次锁住:缓存存在过期时间", func() {
			expTime = time.Second * 10
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
			So(err, ShouldEqual, DataNotExistsErr)
			// 未设置缓存零值，缓存中无数据
			_, err = rds.HGet(key, subKey).Result()
			So(err, ShouldEqual, redis.Nil)
		})

		Convey("2次获取:带锁:正常获取", func() {
			var data *TestHashModel
			// 第一次获取，直接获取原始数据
			err := mcSvc.GetOrCreate(ctx, data, WithLock(lock))
			So(err, ShouldEqual, DataNotExistsErr)
			// 未设置缓存零值，缓存中无数据
			_, err = rds.HGet(key, subKey).Result()
			So(err, ShouldEqual, redis.Nil)

			// 第二次获取会从缓存中获取数据
			err = mcSvc.GetOrCreate(ctx, data, WithLock(lock))
			So(err, ShouldEqual, DataNotExistsErr)
		})
	})
}
