package rdscache

import "errors"

var (
	ErrNoData                    = errors.New("no data error")
	ErrModuleMustNotNil          = errors.New("model must not nil")
	ErrLocalCacheNoData          = errors.New("local cache no data")
	ErrHotKeyOptionInitFail      = errors.New("init error, please check your parameter") // 预防热key的方式均为nil
	ErrMGetHaveSomeUnMarshalFail = errors.New(                                           // 批量获取缓存数据时，如果有其中一些数据反序列化失败，则报该错误
		"some value is not correct, so unmarshal fail, " + "you can check log get some info")
	ErrMGetFromOriReturnCntNotEqualQueryCnt = errors.New("mget from origin return cnt not equal query cnt") // 回源返回数据数量必须等于请求数据数量
)
