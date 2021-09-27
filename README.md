# sponge [![Build Status][travis image]][travis] [![Coverage Status][coveralls image]][coveralls]
sponge直译为海绵，让人联想到缓存的特性。该项目是在golang语言下实现了一些缓存组件，例如redis缓存组件，其中会包含函数缓存，对象缓存等等，项目意在帮助开发更方便的使用缓存，减少重复造轮子。

# 目录介绍
 - rdscache: redis缓存组件，封装了函数缓存功能和model缓存功能，后续可能会有更多的缓存组件。
 - test_local.sh: 本地执行后可查看html观察test覆盖率及覆盖路径
 
 ## func缓存使用
 ```go
 package main
 
 import (
 	"context"
 	"fmt"
 	"sync"
 	"time"
         
 	"github.com/693490554/sponge/rdscache"
 	"github.com/693490554/sponge/rdscache/common"
 	"github.com/693490554/sponge/rdscache/fcache"
 	"github.com/go-redis/redis"
 
 )
 
 var rds = redis.NewClient(&redis.Options{
 	Addr: "localhost:6379",
 })
 
 var lock sync.Locker = &sync.Mutex{}
 
 type User struct {
 	UserId uint64
 	Name   string
 	Age    uint8
 }
 
 // GetUser 从数据源中获取user数据, 可以是mysql等等
 func GetUser(ctx context.Context, userId uint64) (*User, error) {
 	return nil, nil
 }
 
 // GetUserWithCache 从缓存中获取user数据
 func GetUserWithCache(ctx context.Context, userId uint64) (*User, error) {
 	// rds为nil时，缓存组件无法使用返回error，如果确定rds非空，err可不判断
 	svc, err := fcache.NewFCacheService(rds)
 	if err != nil {
 		return nil, err
 	}
 
 	var retUser User
 	// 业务方根据需求决定缓存存入string or hash
 	// cacheInfo :=common.NewHashCache("stringCacheKey", "subKey", time.Second * 10)
 	cacheInfo := common.NewStringCache("stringCacheKey", time.Second*10)
 
 	// GetOrCreate函数返回的第一个值为缓存中记录的字符串值，通常情况下使用不到
 	// 当数据不存在时，err = ErrNoData
 	_, err = svc.GetOrCreate(ctx, cacheInfo, func() (interface{}, error) {
 
 		// 只有缓存不存在时，才会走实际获取数据的函数拿取数据
 		user, err := GetUser(ctx, userId)
 		// 报错直接返回异常
 		if err != nil {
 			return nil, err
 		}
 
 		// todo 如果数据不存在需返回错误ErrNoData供组件捕获到该情况，当需要预防缓存穿透时，该种情况将缓存空字符串
 		if user == nil {
 			return nil, rdscache.ErrNoData
 		}
 		return user, nil
 	}, fcache.WithNeedCacheNoData(), // 可选项，当数据不存在时也需要缓存下来，防止缓存穿透，此时缓存的中记录的是空字符串
 		// 可选项，预防缓存击穿，需注意lock和需要预防缓存击穿的函数为一一对应的关系，lock为单例，同一个lock不可用于多个需要预防缓存穿透的地方
 		fcache.WithLock(lock),
 		fcache.WithUnMarshalData(&retUser)) // 可选项，从缓存中获取到结果后需要序列化到retUser中，需注意不可传入nil指针
 
 	if err != nil {
 		// 数据不存在可以按业务需求决定是否返回error
 		if err == rdscache.ErrNoData {
 			return nil, nil
 		}
 		return nil, err
 	}
 	return &retUser, nil
 }
 
 func main() {
 	user, err := GetUserWithCache(context.Background(), 123)
 	fmt.Println(user, err)
 }
 ```

## model缓存使用
```go

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/693490554/sponge/rdscache"
	"github.com/693490554/sponge/rdscache/mcache"
	"github.com/693490554/sponge/rdscache/common"
	"github.com/go-redis/redis"
	jsoniter "github.com/json-iterator/go"
)

var rds = redis.NewClient(&redis.Options{
	Addr: "localhost:6379",
})
var ctx = context.Background()
var lock sync.Locker = &sync.Mutex{}

type User struct {
	UserId uint64
	Name   string
	Age    uint8
}

// CacheInfo 获取缓存信息, 根据业务方的需要可缓存至string or hash中
func (u *User) CacheInfo() common.ICacheInfo {
	//return common.NewHashCache("userCache", strconv.FormatUint(u.UserId, 10), time.Second*10)
	return common.NewStringCache(fmt.Sprintf("userCache:uid:%d", u.UserId), time.Second*10)
}

// Marshal model提供序列化方法，缓存中缓存的是序列化后的结果
func (u *User) Marshal() (string, error) {
	return jsoniter.MarshalToString(u)
}

// UnMarshal model提供反序列化方法，从缓存中拿到value后反序列化到自身
func (u *User) UnMarshal(value string) error {
	return jsoniter.UnmarshalFromString(value, u)
}

// GetOri 获取原始数据方法，可以是从mysql等数据库中获取数据
// 如果数据不存在需要返回ErrNoData错误，供组件捕获到用于预防缓存穿透
func (u *User) GetOri() (mcache.ICacheModel, error) {
	// 可以根据UserId从db中查询出User
	return nil, rdscache.ErrNoData
}

func GetUserWithCache(ctx context.Context, userId uint64) (*User, error) {
	// rds为nil时，缓存组件无法使用，业务方需保证rds可用
	svc := mcache.NewModelCacheSvc(rds)

	// todo 用GetOrCreate获取缓存时，需要保证记录唯一, 即GetOri方法根据条件仅可获取到一条记录
	user := &User{UserId: userId}
	// 如果存在缓存，获取到的结果将通过组件直接反序列化到user中
	err := svc.GetOrCreate(
		ctx, user,
		// 可选项，预防缓存击穿，需注意lock和需要预防缓存击穿的函数为一一对应的关系，lock为单例，同一个lock不可用于多个需要预防缓存穿透的地方
		mcache.WithLock(lock),
		mcache.WithNeedCacheNoData()) // 可选项，当数据不存在时也需要缓存下来，防止缓存穿透，此时缓存的中记录的是空字符串

	if err != nil {
		// 数据不存在可以按业务需求决定是否返回error
		if err == rdscache.ErrNoData {
			return nil, nil
		}
	}
	return user, nil
}

func main() {
	user, err := GetUserWithCache(ctx, 123)
	fmt.Println(user, err)
}

```
 
[travis]: https://travis-ci.com/github/693490554/sponge
[travis image]: https://travis-ci.org/693490554/sponge.png?branch=master
[coveralls]: https://coveralls.io/github/693490554/sponge?branch=master
[coveralls image]: https://coveralls.io/repos/github/693490554/sponge/badge.svg?branch=master
