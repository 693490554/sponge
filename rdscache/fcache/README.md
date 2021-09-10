## Usage

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