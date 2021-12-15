package common

import (
	"testing"
	"time"

	"github.com/693490554/sponge/rdscache"
	"github.com/allegro/bigcache"
	. "github.com/glycerine/goconvey/convey"
	goCache "github.com/patrickmn/go-cache"
)

var (
	ck      = "test"
	expTime = time.Second
)

func TestNewHotKeyOption(t *testing.T) {
	Convey("测试热key选项初始化", t, func() {
		Convey("未传入任何初始化选项", func() {

			_, err := NewHotKeyOption()
			So(err, ShouldEqual, rdscache.ErrHotKeyOptionInitFail)
		})

		Convey("传入分片函数+传入判定热key函数", func() {

			option, err := NewHotKeyOption(WithGetShardingKey(func() string {
				return "111"
			}))
			So(err, ShouldBeNil)
			So(option.IsHotKey(), ShouldEqual, true)

			option, err = NewHotKeyOption(WithGetShardingKey(func() string {
				return "111"
			}), WithIsHotKey(func() bool {
				return false
			}))
			So(err, ShouldBeNil)
			So(option.IsHotKey(), ShouldEqual, false)
		})

		Convey("传入本地缓存+分片函数", func() {

			cacheInfo := NewCacheBase(ck, expTime)
			cache, _ := bigcache.NewBigCache(bigcache.DefaultConfig(time.Second))
			wrapBigCache := NewWrapBigCache(cache)
			option, err := NewHotKeyOption(
				WithLocalCache(wrapBigCache, cacheInfo),
				WithGetShardingKey(func() string {
					return "111"
				}))
			So(err, ShouldBeNil)
			So(option.UseLocalCache(), ShouldEqual, true)

			wrapGoCache := NewWrapGoCache(goCache.New(time.Second, time.Second))
			option, err = NewHotKeyOption(
				WithLocalCache(wrapGoCache, cacheInfo),
				WithGetShardingKey(func() string {
					return "111"
				}))
			So(err, ShouldBeNil)
			So(option.UseLocalCache(), ShouldEqual, true)
		})
	})
}
