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
