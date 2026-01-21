package web

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func formatDecimalPlain(d decimal.Decimal, scale int32) string {
	if scale < 0 {
		scale = 0
	}
	d = d.Truncate(scale)
	s := d.StringFixed(scale)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func formatUSDPlain(usd decimal.Decimal) string {
	return formatDecimalPlain(usd, store.USDScale)
}

func formatUSD(usd decimal.Decimal) string {
	if usd.IsNegative() {
		return "-$" + formatUSDPlain(usd.Abs())
	}
	return "$" + formatUSDPlain(usd)
}

func formatUSDOrUnlimited(usd decimal.Decimal) string {
	if usd.LessThanOrEqual(decimal.Zero) {
		return "不限"
	}
	return formatUSD(usd)
}

func formatCNY(cny decimal.Decimal) string {
	if cny.IsNegative() {
		return "-¥" + formatDecimalPlain(cny.Abs(), store.CNYScale)
	}
	return "¥" + formatDecimalPlain(cny, store.CNYScale)
}

func formatCNYFixed(cny decimal.Decimal) string {
	return cny.Truncate(store.CNYScale).StringFixed(store.CNYScale)
}

func parseDecimalNonNeg(raw string, scale int32) (decimal.Decimal, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return decimal.Zero, errors.New("金额为空")
	}
	if strings.HasPrefix(s, "+") {
		s = strings.TrimSpace(strings.TrimPrefix(s, "+"))
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, errors.New("金额格式不合法")
	}
	if d.IsNegative() {
		return decimal.Zero, errors.New("金额不能为负数")
	}
	if d.Exponent() < -scale {
		return decimal.Zero, fmt.Errorf("最多支持 %d 位小数", scale)
	}
	return d.Truncate(scale), nil
}

func parseCNY(raw string) (decimal.Decimal, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "¥")
	return parseDecimalNonNeg(s, store.CNYScale)
}

func cnyToMinorUnits(cny decimal.Decimal) (int64, error) {
	if cny.IsNegative() {
		return 0, errors.New("金额不能为负数")
	}
	if cny.Exponent() < -store.CNYScale {
		return 0, fmt.Errorf("最多支持 %d 位小数", store.CNYScale)
	}
	scaled := cny.Truncate(store.CNYScale).Shift(store.CNYScale)
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, errors.New("金额不合法")
	}
	n := scaled.IntPart()
	if !decimal.NewFromInt(n).Equal(scaled) {
		return 0, errors.New("金额过大")
	}
	return n, nil
}

