package store

import (
	"context"
	"testing"
)

func TestFeatureStateEffective_DoesNotForceLegacyPersonalDefaults(t *testing.T) {
	t.Parallel()

	st := New(nil)

	got := st.FeatureStateEffective(context.Background())
	if got.BillingDisabled {
		t.Fatalf("FeatureStateEffective().BillingDisabled = true, want false")
	}
	if got.TicketsDisabled {
		t.Fatalf("FeatureStateEffective().TicketsDisabled = true, want false")
	}
	if got.AdminUsersDisabled {
		t.Fatalf("FeatureStateEffective().AdminUsersDisabled = true, want false")
	}

	got = st.FeatureStateEffective(context.Background())
	if got.BillingDisabled {
		t.Fatalf("FeatureStateEffective().BillingDisabled = true, want false")
	}
	if got.TicketsDisabled {
		t.Fatalf("FeatureStateEffective().TicketsDisabled = true, want false")
	}
	if got.AdminUsersDisabled {
		t.Fatalf("FeatureStateEffective().AdminUsersDisabled = true, want false")
	}
}
