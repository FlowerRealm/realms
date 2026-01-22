package quota

import (
	"testing"

	"github.com/shopspring/decimal"
)

func ptrInt64(v int64) *int64 { return &v }

func TestEstimateCostUSDWithPricing_SplitCache(t *testing.T) {
	t.Parallel()

	inUSDPer1M := decimal.RequireFromString("2")
	outUSDPer1M := decimal.RequireFromString("4")
	cacheInUSDPer1M := decimal.RequireFromString("0.5")
	cacheOutUSDPer1M := decimal.RequireFromString("1")

	got, err := estimateCostUSDWithPricing(
		inUSDPer1M,
		outUSDPer1M,
		cacheInUSDPer1M,
		cacheOutUSDPer1M,
		ptrInt64(100), // input
		ptrInt64(40),  // cached input
		ptrInt64(50),  // output
		ptrInt64(10),  // cached output
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 60*2 + 40*4 + 40*0.5 + 10*1 = 310 (USD/1M) => 0.000310 USD
	want := decimal.RequireFromString("0.00031")
	if !got.Equal(want) {
		t.Fatalf("unexpected result: got=%s want=%s", got, want)
	}
}

func TestEstimateCostUSDWithPricing_SubsetClipCachedTokens(t *testing.T) {
	t.Parallel()

	inUSDPer1M := decimal.RequireFromString("2")
	outUSDPer1M := decimal.RequireFromString("4")
	cacheInUSDPer1M := decimal.RequireFromString("0.5")
	cacheOutUSDPer1M := decimal.RequireFromString("1")

	got, err := estimateCostUSDWithPricing(
		inUSDPer1M,
		outUSDPer1M,
		cacheInUSDPer1M,
		cacheOutUSDPer1M,
		ptrInt64(100), // input
		ptrInt64(120), // cached input (clip -> 100)
		ptrInt64(50),  // output
		ptrInt64(70),  // cached output (clip -> 50)
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// non-cached=0; cached: 100*0.5 + 50*1 = 100 (USD/1M) => 0.000100 USD
	want := decimal.RequireFromString("0.0001")
	if !got.Equal(want) {
		t.Fatalf("unexpected result: got=%s want=%s", got, want)
	}
}

func TestEstimateCostUSDWithPricing_Truncate6Decimals(t *testing.T) {
	t.Parallel()

	got, err := estimateCostUSDWithPricing(
		decimal.RequireFromString("1.999999"),
		decimal.Zero,
		decimal.Zero,
		decimal.Zero,
		ptrInt64(1),
		ptrInt64(0),
		ptrInt64(0),
		ptrInt64(0),
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 * 1.999999 / 1_000_000 = 0.000001999999 -> truncate(6) => 0.000001
	want := decimal.RequireFromString("0.000001")
	if !got.Equal(want) {
		t.Fatalf("unexpected result: got=%s want=%s", got, want)
	}
}

func TestEstimateCostUSDWithPricing_NegativeTokens(t *testing.T) {
	t.Parallel()

	_, err := estimateCostUSDWithPricing(
		decimal.RequireFromString("1"),
		decimal.Zero,
		decimal.Zero,
		decimal.Zero,
		ptrInt64(-1),
		ptrInt64(0),
		ptrInt64(0),
		ptrInt64(0),
		nil,
	)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
