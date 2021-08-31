package rdscache

import "errors"

var (
	ErrNoData           = errors.New("no data error")
	ErrModuleMustNotNil = errors.New("model must not nil")
)
