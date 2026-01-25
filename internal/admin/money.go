package admin

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

func formatCNYPlain(cny decimal.Decimal) string {
	return formatDecimalPlain(cny, store.CNYScale)
}

func formatMultiplierPlain(mult decimal.Decimal) string {
	return formatDecimalPlain(mult, store.PriceMultiplierScale)
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

func parseOptionalDecimalNonNeg(raw string, scale int32) (decimal.Decimal, error) {
	if strings.TrimSpace(raw) == "" {
		return decimal.Zero, nil
	}
	return parseDecimalNonNeg(raw, scale)
}

func parseUSD(raw string) (decimal.Decimal, error) {
	return parseDecimalNonNeg(raw, store.USDScale)
}

func parseOptionalUSD(raw string) (decimal.Decimal, error) {
	return parseOptionalDecimalNonNeg(raw, store.USDScale)
}

func parseCNY(raw string) (decimal.Decimal, error) {
	return parseDecimalNonNeg(raw, store.CNYScale)
}

func parseMultiplier(raw string) (decimal.Decimal, error) {
	return parseDecimalNonNeg(raw, store.PriceMultiplierScale)
}
