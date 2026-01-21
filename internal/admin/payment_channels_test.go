package admin

import (
	"testing"

	"realms/internal/store"
)

func TestPaymentChannelUsable(t *testing.T) {
	t.Parallel()

	ptr := func(s string) *string { return &s }

	if paymentChannelUsable(store.PaymentChannel{Type: store.PaymentChannelTypeStripe, Status: 1, StripeSecretKey: ptr("sk"), StripeWebhookSecret: ptr("whsec")}) != true {
		t.Fatalf("stripe channel expected usable")
	}
	if paymentChannelUsable(store.PaymentChannel{Type: store.PaymentChannelTypeStripe, Status: 1, StripeWebhookSecret: ptr("whsec")}) != false {
		t.Fatalf("stripe channel missing secret_key expected not usable")
	}
	if paymentChannelUsable(store.PaymentChannel{Type: store.PaymentChannelTypeStripe, Status: 0, StripeSecretKey: ptr("sk"), StripeWebhookSecret: ptr("whsec")}) != false {
		t.Fatalf("disabled stripe channel expected not usable")
	}

	if paymentChannelUsable(store.PaymentChannel{Type: store.PaymentChannelTypeEPay, Status: 1, EPayGateway: ptr("https://epay.example.com"), EPayPartnerID: ptr("10001"), EPayKey: ptr("k")}) != true {
		t.Fatalf("epay channel expected usable")
	}
	if paymentChannelUsable(store.PaymentChannel{Type: store.PaymentChannelTypeEPay, Status: 1, EPayGateway: ptr("https://epay.example.com"), EPayPartnerID: ptr("10001")}) != false {
		t.Fatalf("epay channel missing key expected not usable")
	}
}

func TestToPaymentChannelViewWebhookURL(t *testing.T) {
	t.Parallel()

	chStripe := store.PaymentChannel{ID: 12, Type: store.PaymentChannelTypeStripe, Name: "s", Status: 1}
	vStripe := toPaymentChannelView(chStripe, "http://localhost:8080/", nil)
	if vStripe.WebhookURL != "http://localhost:8080/api/pay/stripe/webhook/12" {
		t.Fatalf("stripe webhook url = %q", vStripe.WebhookURL)
	}

	chEPay := store.PaymentChannel{ID: 34, Type: store.PaymentChannelTypeEPay, Name: "e", Status: 1}
	vEPay := toPaymentChannelView(chEPay, "http://localhost:8080", nil)
	if vEPay.WebhookURL != "http://localhost:8080/api/pay/epay/notify/34" {
		t.Fatalf("epay webhook url = %q", vEPay.WebhookURL)
	}
}
