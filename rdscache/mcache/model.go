package mcache

import (
	"github.com/693490554/sponge/rdscache/common"
)

// ICacheModel 通过组件可以获取到的单个model的抽象
type ICacheModel interface {
	// 缓存信息
	CacheInfo() common.ICacheInfo
	// model序列化方法，通过该方法可以获取到缓存的内容
	Marshal() (string, error)
	// model反序列化反方, 将缓存的内容反序列化到model中
	UnMarshal(value string) error
	// 获取原始非缓存数据, 当数据不存在时需返回ErrNoData
	// ErrNoData搭配WithNeedCacheNoData可预防缓存穿透
	GetOri() (ICacheModel, error)
}

// ICanMGetModel 通过组件可以批量获取的单个model的抽象
type ICanMGetModel interface {
	// CacheInfo 缓存信息
	// TODO: 如果使用hash作为缓存，批量获取时，需保证hash的key一致，暂不支持从多个hash的key中获取数据
	CacheInfo() common.ICacheInfo
	// model序列化方法，通过该方法可以获取到缓存的内容
	Marshal() (string, error)
	// model反序列化反方, 将缓存的内容反序列化到model中
	// TODO value=""代表缓存了空数据，此时反序列化后续特殊标示下
	UnMarshal(value string) error
	// UpdateSelf 通过接口更新自身, model == nil代表数据不存在，以方法形式提供对本身的更新操作，代替使用反射操作，提高性能
	UpdateSelf(model ICanMGetModel)
	// Clone 深拷贝方法
	Clone() ICanMGetModel
}
