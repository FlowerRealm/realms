package store

import (
	"context"
	"testing"
)

func TestFeatureStateEffective_SelfModeForcesBillingAndTickets(t *testing.T) {
	t.Parallel()

	st := New(nil)

	got := st.FeatureStateEffective(context.Background(), true)
	if !got.BillingDisabled {
		t.Fatalf("FeatureStateEffective(selfMode=true).BillingDisabled = false, want true")
	}
	if !got.TicketsDisabled {
		t.Fatalf("FeatureStateEffective(selfMode=true).TicketsDisabled = false, want true")
	}

	got = st.FeatureStateEffective(context.Background(), false)
	if got.BillingDisabled {
		t.Fatalf("FeatureStateEffective(selfMode=false).BillingDisabled = true, want false")
	}
	if got.TicketsDisabled {
		t.Fatalf("FeatureStateEffective(selfMode=false).TicketsDisabled = true, want false")
	}
}
