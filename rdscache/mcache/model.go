package mcache

import (
	"github.com/693490554/sponge/rdscache/common"
)

// ICacheModel 可缓存的model接口
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

// IMultiCacheModel 一对多缓存对象接口
type IMultiCacheModel interface {
	// CacheInfo 缓存信息
	// TODO: 如果使用hash作为缓存，批量获取时，需保证hash的key一致，暂不支持从多个hash的key中获取数据
	CacheInfo() common.ICacheInfo
	// model序列化方法，通过该方法可以获取到缓存的内容
	Marshal() (string, error)
	// model反序列化反方, 将缓存的内容反序列化到model中
	UnMarshal(value string) error
	// UpdateSelf 通过接口更新自身, model == nil代表数据不存在，以方法形式代替反射操作，提高性能
	UpdateSelf(model IMultiCacheModel)
}
