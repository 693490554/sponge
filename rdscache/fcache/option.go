package fcache

import (
	"sync"

	"github.com/693490554/sponge/rdscache/common"
)

// fCacheOption 函数缓存可选项
type fCacheOption struct {
	lock            sync.Locker // 预防缓存击穿时，需要传入lock
	needCacheNoData bool        // 是否需要缓存函数不存在数据的情况，默认不需要
	// todo: data需传入非nil指针, 如果为nil反序列化将失败
	data interface{} // data != nil 代表需要将结果UnMarshal到data中
	// getFromRdsCallBack，从redis获取数据时的回调函数，可用于做监控，及热key实时统计等功能
	getFromRdsCallBack func()
	// hotKeyOption 预防热key选项
	// 支持分片处理热key问题或本地缓存处理热key问题
	// 业务方通过getFromRdsCallBack回调可自行实现热key实时统计, 通过注册isHotKey函数,可以实现动态热key处理
	hotKeyOption *common.HotKeyOption
}

func NewFCacheOption(opts ...FCOptionWrap) *fCacheOption {
	option := &fCacheOption{}
	for _, o := range opts {
		o(option)
	}
	return option
}

type FCOptionWrap func(o *fCacheOption)

// WithLock 使用锁，防止缓存击穿
func WithLock(lock sync.Locker) FCOptionWrap {
	return func(option *fCacheOption) {
		option.lock = lock
	}
}

// WithNeedCacheNoData 需要缓存数据不存在，预防缓存穿透
func WithNeedCacheNoData() FCOptionWrap {
	return func(option *fCacheOption) {
		option.needCacheNoData = true
	}
}

// WithUnMarshalData 需要将缓存结果反序列化
func WithUnMarshalData(data interface{}) FCOptionWrap {
	return func(option *fCacheOption) {
		option.data = data
	}
}

// WithGetFromRdsCallBack 注册从redis获取数据的回调函数
func WithGetFromRdsCallBack(cb func()) FCOptionWrap {
	return func(option *fCacheOption) {
		option.getFromRdsCallBack = cb
	}
}

// WithRdsVisitCallBack 注册redis访问回调
func WithHotKeyOption(hotKeyOption *common.HotKeyOption) FCOptionWrap {
	return func(option *fCacheOption) {
		option.hotKeyOption = hotKeyOption
	}
}
