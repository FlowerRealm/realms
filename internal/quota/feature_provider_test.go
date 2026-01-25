package quota

import (
	"context"
	"testing"

	"realms/internal/store"
)

type fakeFeatures struct {
	disabled map[string]bool
}

func (f fakeFeatures) FeatureDisabledEffective(_ context.Context, _ bool, key string) bool {
	return f.disabled[key]
}

type countingProvider struct {
	reserveCalls int
}

func (p *countingProvider) Reserve(_ context.Context, _ ReserveInput) (ReserveResult, error) {
	p.reserveCalls++
	return ReserveResult{UsageEventID: 1}, nil
}

func (p *countingProvider) Commit(_ context.Context, _ CommitInput) error { return nil }
func (p *countingProvider) Void(_ context.Context, _ int64) error         { return nil }

func TestFeatureProvider_SelectsFreeWhenBillingDisabled(t *testing.T) {
	normal := &countingProvider{}
	free := &countingProvider{}

	fp := NewFeatureProvider(fakeFeatures{
		disabled: map[string]bool{
			store.SettingFeatureDisableBilling: true,
		},
	}, false, normal, free)

	if _, err := fp.Reserve(context.Background(), ReserveInput{}); err != nil {
		t.Fatalf("Reserve failed: %v", err)
	}
	if free.reserveCalls != 1 || normal.reserveCalls != 0 {
		t.Fatalf("unexpected reserve calls: free=%d normal=%d", free.reserveCalls, normal.reserveCalls)
	}
}

func TestFeatureProvider_SelectsNormalWhenBillingEnabled(t *testing.T) {
	normal := &countingProvider{}
	free := &countingProvider{}

	fp := NewFeatureProvider(fakeFeatures{disabled: map[string]bool{}}, false, normal, free)

	if _, err := fp.Reserve(context.Background(), ReserveInput{}); err != nil {
		t.Fatalf("Reserve failed: %v", err)
	}
	if free.reserveCalls != 0 || normal.reserveCalls != 1 {
		t.Fatalf("unexpected reserve calls: free=%d normal=%d", free.reserveCalls, normal.reserveCalls)
	}
}
