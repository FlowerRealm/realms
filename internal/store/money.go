package store

import "github.com/shopspring/decimal"

const (
	USDScale             = int32(6)
	CNYScale             = int32(2)
	PriceMultiplierScale = int32(6)
)

var DefaultGroupPriceMultiplier = decimal.NewFromInt(1)

