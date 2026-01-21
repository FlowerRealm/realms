package store

import "errors"

var (
	// ErrOrderCanceled 表示订单已被取消（用于支付回调等幂等场景判定）。
	ErrOrderCanceled = errors.New("订单已取消")
)
