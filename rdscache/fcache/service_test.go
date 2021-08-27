package fcache

import (
	"context"
	"errors"
	"os"
	"sponge/rdscache/common"
	"sync"
	"testing"
	"time"

	. "github.com/glycerine/goconvey/convey"
	"github.com/go-redis/redis"
)

var (
	ctx = context.Background()
	rds = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	fcSvc, _ = NewFCacheService(rds)
	rk       = "test"
	sk       = "subKey"
	lock     = &sync.RWMutex{}
)

func delTestData() {
	rds.Del(rk)
}

func TestMain(m *testing.M) {
	m.Run()
	delTestData()
	os.Exit(0)
}

//nolint:typecheck
func Test_fCacheService_GetFromString(t *testing.T) {

	Convey("从string中获取缓存", t, func() {
		// todo fc为整个测试用例所需要的参数，每执行完下面的一个convey，都会回到这里再重新执行初始化，然后执行下一个convey!
		// todo 执行流程并不是从上到下一次性运行完！
		fc, err := NewFCache(rk, common.KTOfString)
		So(err, ShouldBeNil)
		So(fc, ShouldNotBeNil)
		delTestData()

		Convey("非法KT", func() {
			_, err := NewFCache(rk, 9999)
			So(err, ShouldNotBeNil)
		})

		Convey("需要反序列化到data，但是data为nil的情况", func() {
			fc.ApplyOption(WithNeedUnMarshal())
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
				return nil, nil
			})
			So(ret, ShouldEqual, "")
			So(err, ShouldNotBeNil)
		})

		Convey("首次获取,函数无error并且返回nil,无需缓存零值的情况", func() {
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
				return nil, nil
			})
			So(ret, ShouldEqual, "")
			So(err, ShouldBeNil)
			// 从缓存中获取下，缓存将不存在
			tmp, err := rds.Get(rk).Result()
			So(err, ShouldEqual, redis.Nil)
			So(tmp, ShouldEqual, "")
		})

		Convey("首次获取,函数无error并且返回nil,需缓存零值的情况", func() {
			fc.ApplyOption(WithNeedCacheZero())
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
				return nil, nil
			})
			So(ret, ShouldEqual, "null")
			So(err, ShouldBeNil)
			// 从缓存中获取下，缓存将不存在
			tmp, err := rds.Get(rk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldEqual, "null")
		})

		Convey("首次获取,函数返回error的情况", func() {
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
				return nil, errors.New("")
			})
			So(ret, ShouldEqual, "")
			So(err, ShouldNotBeNil)
		})

		Convey("首次获取, 函数正常返回struct，不需要序列化到data的情况", func() {
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
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
			fc.ApplyOption(WithNeedUnMarshal())
			fc.ApplyOption(WithLock(lock))
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			data := &testS{}
			funcRet := &testS{1, "test"}
			ret, err := fcSvc.Get(ctx, fc, data, func() (interface{}, error) {
				return funcRet, nil
			})
			So(ret, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			So(data.A, ShouldEqual, 1)
			So(data.B, ShouldEqual, "test")
			tmp, err := rds.Get(rk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldNotEqual, "")
			// check下过期时间
			expTime, _ := rds.TTL(rk).Result()
			// 无过期时间
			So(expTime, ShouldEqual, -1*time.Second)
		})

		Convey("获取2次:函数正常返回struct:存在lock:需要反序列化", func() {
			fc.ApplyOption(WithNeedUnMarshal())
			tmp, err := rds.Get(rk).Result()
			So(err, ShouldEqual, redis.Nil)
			So(tmp, ShouldEqual, "")

			fc.ApplyOption(WithLock(lock))
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			data := &testS{}
			funcRet := &testS{1, "test"}
			for i := 1; i <= 2; i++ {
				ret, err := fcSvc.Get(ctx, fc, data, func() (interface{}, error) {
					return funcRet, nil
				})
				So(ret, ShouldNotEqual, "")
				So(err, ShouldBeNil)
				So(data.A, ShouldEqual, 1)
				So(data.B, ShouldEqual, "test")
			}
			tmp, err = rds.Get(rk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldNotEqual, "")
		})

		Convey("首次获取:函数正常返回struct:存在lock:设过期时间", func() {
			fc.ApplyOption(WithLock(lock))
			fc.ApplyOption(WithExpTime(time.Second * 5))
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			funcRet := &testS{1, "test"}
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
				return funcRet, nil
			})
			So(ret, ShouldNotEqual, "")
			So(err, ShouldBeNil)
			tmp, err := rds.Get(rk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldNotEqual, "")
			// check下过期时间
			expTime, _ := rds.TTL(rk).Result()
			So(expTime, ShouldBeGreaterThan, time.Second*4)
			So(expTime, ShouldBeLessThanOrEqualTo, time.Second*5)
		})

		Convey("函数正常返回struct:存在lock:lock被锁", func() {
			fc.ApplyOption(WithLock(lock))
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			funcRet := &testS{1, "test"}
			fc.lock.Lock()
			nowTs := time.Now().Unix()
			newCtx, cFunc := context.WithTimeout(context.Background(), time.Second*2)
			defer cFunc()
			go func() {
				_, _ = fcSvc.Get(newCtx, fc, nil, func() (interface{}, error) {
					return funcRet, nil
				})
			}()
		HERE:
			for range newCtx.Done() {
				useSec := time.Now().Unix() - nowTs
				// 因为被锁住了，所以耗时一定大于1.5秒
				So(useSec, ShouldBeGreaterThan, 1.5)
				break HERE
			}
			// 超时取消函数结果并没有放入缓存
			_, err := rds.Get(rk).Result()
			So(err, ShouldEqual, redis.Nil)
			// 释放锁后可正常获取
			fc.lock.Unlock()
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
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
		fc, err := NewFCache(rk, common.KTOfHash, WithSK(sk))
		So(err, ShouldBeNil)
		So(fc, ShouldNotBeNil)

		Convey("初始化不带sk", func() {
			tmp, err := NewFCache(rk, common.KTOfHash)
			So(err, ShouldNotBeNil)
			So(tmp, ShouldBeNil)
		})

		Convey("redis key为空", func() {
			tmp, err := NewFCache("", common.KTOfHash)
			So(err, ShouldNotBeNil)
			So(tmp, ShouldBeNil)
		})

		Convey("fCacheService未传入rds", func() {
			tmp, err := NewFCacheService(nil)
			So(err, ShouldNotBeNil)
			So(tmp, ShouldBeNil)
		})

		Convey("需要反序列化到data，但是data为nil的情况", func() {
			fc.ApplyOption(WithNeedUnMarshal())
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
				return nil, nil
			})
			So(ret, ShouldEqual, "")
			So(err, ShouldNotBeNil)
		})

		Convey("首次获取,函数无error并且返回nil,无需缓存零值的情况", func() {
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
				return nil, nil
			})
			So(ret, ShouldEqual, "")
			So(err, ShouldBeNil)
			// 从缓存中获取下，缓存将不存在
			tmp, err := rds.HGet(rk, sk).Result()
			So(err, ShouldEqual, redis.Nil)
			So(tmp, ShouldEqual, "")
		})

		Convey("首次获取,函数无error并且返回nil,需缓存零值的情况", func() {
			fc.ApplyOption(WithNeedCacheZero())
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
				return nil, nil
			})
			So(ret, ShouldEqual, "null")
			So(err, ShouldBeNil)
			// 从缓存中获取下，缓存将不存在
			tmp, err := rds.HGet(rk, sk).Result()
			So(err, ShouldBeNil)
			So(tmp, ShouldEqual, "null")
		})

		Convey("首次获取,函数返回error的情况", func() {
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
				return nil, errors.New("")
			})
			So(ret, ShouldEqual, "")
			So(err, ShouldNotBeNil)
		})

		Convey("首次获取, 函数正常返回struct，不需要序列化到data的情况", func() {
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
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
			fc.ApplyOption(WithNeedUnMarshal())
			fc.ApplyOption(WithLock(lock))
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			data := &testS{}
			funcRet := &testS{1, "test"}
			ret, err := fcSvc.Get(ctx, fc, data, func() (interface{}, error) {
				return funcRet, nil
			})
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
			fc.ApplyOption(WithNeedUnMarshal())
			tmp, err := rds.HGet(rk, sk).Result()
			So(err, ShouldEqual, redis.Nil)
			So(tmp, ShouldEqual, "")

			fc.ApplyOption(WithLock(lock))
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			data := &testS{}
			funcRet := &testS{1, "test"}
			for i := 1; i <= 2; i++ {
				ret, err := fcSvc.Get(ctx, fc, data, func() (interface{}, error) {
					return funcRet, nil
				})
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
			fc.ApplyOption(WithLock(lock))
			fc.ApplyOption(WithExpTime(time.Second * 5))
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			funcRet := &testS{1, "test"}
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
				return funcRet, nil
			})
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
			fc.ApplyOption(WithLock(lock))
			type testS struct {
				A int    `json:"a"`
				B string `json:"b"`
			}
			funcRet := &testS{1, "test"}
			fc.lock.Lock()
			nowTs := time.Now().Unix()
			newCtx, cFunc := context.WithTimeout(context.Background(), time.Second*2)
			defer cFunc()
			go func() {
				_, _ = fcSvc.Get(newCtx, fc, nil, func() (interface{}, error) {
					return funcRet, nil
				})
			}()
		HERE:
			for range newCtx.Done() {
				useSec := time.Now().Unix() - nowTs
				// 因为被锁住了，所以耗时一定大于1.5秒
				So(useSec, ShouldBeGreaterThan, 1.5)
				break HERE
			}
			// 超时取消函数结果并没有放入缓存
			_, err := rds.HGet(rk, sk).Result()
			So(err, ShouldEqual, redis.Nil)
			// 释放锁后可正常获取
			fc.lock.Unlock()
			ret, err := fcSvc.Get(ctx, fc, nil, func() (interface{}, error) {
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
