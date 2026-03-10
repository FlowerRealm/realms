package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/shopspring/decimal"
)

const (
	ManagedModelHighContextAppliesToFullRequest          = "full_request"
	ManagedModelHighContextServiceTierPolicyInherit      = "inherit"
	ManagedModelHighContextServiceTierPolicyForceStandard = "force_standard"
)

type ManagedModelHighContextPricing struct {
	ThresholdInputTokens int64            `json:"threshold_input_tokens"`
	AppliesTo            string           `json:"applies_to"`
	ServiceTierPolicy    string           `json:"service_tier_policy"`
	InputUSDPer1M        decimal.Decimal  `json:"input_usd_per_1m"`
	OutputUSDPer1M       decimal.Decimal  `json:"output_usd_per_1m"`
	CacheInputUSDPer1M   *decimal.Decimal `json:"cache_input_usd_per_1m,omitempty"`
	CacheOutputUSDPer1M  *decimal.Decimal `json:"cache_output_usd_per_1m,omitempty"`
	Source               string           `json:"source,omitempty"`
	SourceDetail         string           `json:"source_detail,omitempty"`
}

func normalizeManagedModelHighContextPricing(in *ManagedModelHighContextPricing) (*ManagedModelHighContextPricing, error) {
	if in == nil {
		return nil, nil
	}
	out := *in
	if out.ThresholdInputTokens <= 0 {
		return nil, errors.New("高上下文定价不合法")
	}
	out.AppliesTo = strings.TrimSpace(out.AppliesTo)
	if out.AppliesTo == "" {
		out.AppliesTo = ManagedModelHighContextAppliesToFullRequest
	}
	if out.AppliesTo != ManagedModelHighContextAppliesToFullRequest {
		return nil, errors.New("高上下文定价不合法")
	}
	out.ServiceTierPolicy = strings.TrimSpace(out.ServiceTierPolicy)
	if out.ServiceTierPolicy == "" {
		out.ServiceTierPolicy = ManagedModelHighContextServiceTierPolicyInherit
	}
	switch out.ServiceTierPolicy {
	case ManagedModelHighContextServiceTierPolicyInherit, ManagedModelHighContextServiceTierPolicyForceStandard:
	default:
		return nil, errors.New("高上下文定价不合法")
	}
	out.InputUSDPer1M = out.InputUSDPer1M.Truncate(USDScale)
	out.OutputUSDPer1M = out.OutputUSDPer1M.Truncate(USDScale)
	if out.InputUSDPer1M.IsNegative() || out.OutputUSDPer1M.IsNegative() {
		return nil, errors.New("高上下文定价不合法")
	}
	var err error
	out.CacheInputUSDPer1M, err = normalizeOptionalManagedModelPrice(out.CacheInputUSDPer1M)
	if err != nil {
		return nil, errors.New("高上下文定价不合法")
	}
	out.CacheOutputUSDPer1M, err = normalizeOptionalManagedModelPrice(out.CacheOutputUSDPer1M)
	if err != nil {
		return nil, errors.New("高上下文定价不合法")
	}
	out.Source = strings.TrimSpace(out.Source)
	out.SourceDetail = strings.TrimSpace(out.SourceDetail)
	return &out, nil
}

func parseManagedModelHighContextPricing(v sql.NullString) (*ManagedModelHighContextPricing, error) {
	if !v.Valid {
		return nil, nil
	}
	s := strings.TrimSpace(v.String)
	if s == "" {
		return nil, nil
	}
	var out ManagedModelHighContextPricing
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, errors.New("高上下文定价不合法")
	}
	return normalizeManagedModelHighContextPricing(&out)
}

func marshalManagedModelHighContextPricing(in *ManagedModelHighContextPricing) (*string, error) {
	norm, err := normalizeManagedModelHighContextPricing(in)
	if err != nil {
		return nil, err
	}
	if norm == nil {
		return nil, nil
	}
	b, err := json.Marshal(norm)
	if err != nil {
		return nil, errors.New("高上下文定价不合法")
	}
	s := string(b)
	return &s, nil
}

func equalManagedModelHighContextPricing(a, b *ManagedModelHighContextPricing) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	na, err := normalizeManagedModelHighContextPricing(a)
	if err != nil {
		return false
	}
	nb, err := normalizeManagedModelHighContextPricing(b)
	if err != nil {
		return false
	}
	if na.ThresholdInputTokens != nb.ThresholdInputTokens ||
		na.AppliesTo != nb.AppliesTo ||
		na.ServiceTierPolicy != nb.ServiceTierPolicy ||
		!na.InputUSDPer1M.Equal(nb.InputUSDPer1M) ||
		!na.OutputUSDPer1M.Equal(nb.OutputUSDPer1M) ||
		na.Source != nb.Source ||
		na.SourceDetail != nb.SourceDetail {
		return false
	}
	if !equalOptionalDecimal(na.CacheInputUSDPer1M, nb.CacheInputUSDPer1M) {
		return false
	}
	if !equalOptionalDecimal(na.CacheOutputUSDPer1M, nb.CacheOutputUSDPer1M) {
		return false
	}
	return true
}

func equalOptionalDecimal(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Truncate(USDScale).Equal(b.Truncate(USDScale))
}
