##Usage
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