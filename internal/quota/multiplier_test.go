package quota

import (
	"testing"

	"github.com/shopspring/decimal"

	"realms/internal/store"
)

func TestApplyPriceMultiplierUSD(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		base      string
		mult      string
		want      string
		wantError bool
	}{
		{name: "base_zero", base: "0", mult: store.DefaultGroupPriceMultiplier.String(), want: "0"},
		{name: "mult_one", base: "1.234567", mult: store.DefaultGroupPriceMultiplier.String(), want: "1.234567"},
		{name: "mult_zero", base: "1.234567", mult: "0", want: "0"},
		{name: "mult_double", base: "1.234567", mult: "2", want: "2.469134"},
		{name: "mult_half_floor", base: "1.234567", mult: "0.5", want: "0.617283"},
		{name: "negative_mult", base: "0.000001", mult: "-1", wantError: true},
		{name: "negative_base", base: "-1", mult: store.DefaultGroupPriceMultiplier.String(), wantError: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := decimal.RequireFromString(tc.base)
			mult := decimal.RequireFromString(tc.mult)
			got, err := applyPriceMultiplierUSD(base, mult)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%s)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			want := decimal.RequireFromString(tc.want)
			if !got.Equal(want) {
				t.Fatalf("unexpected result: got=%s want=%s", got, want)
			}
		})
	}
}
