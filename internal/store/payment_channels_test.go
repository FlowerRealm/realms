package store

import "testing"

func TestNormalizePaymentChannelType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "stripe", want: PaymentChannelTypeStripe},
		{in: " Stripe ", want: PaymentChannelTypeStripe},
		{in: "STRIPE", want: PaymentChannelTypeStripe},
		{in: "epay", want: PaymentChannelTypeEPay},
		{in: " EPay ", want: PaymentChannelTypeEPay},
		{in: "scripe", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, tc := range cases {
		got, err := normalizePaymentChannelType(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("normalizePaymentChannelType(%q) expected error, got nil", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizePaymentChannelType(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("normalizePaymentChannelType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

