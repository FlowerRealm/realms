package quota

import (
	"context"
	"errors"

	"realms/internal/store"
)

type FeatureGetter interface {
	FeatureDisabledEffective(ctx context.Context, selfMode bool, key string) bool
}

// FeatureProvider 根据功能开关在不同 QuotaProvider 之间切换。
//
// 目前仅实现：
// - feature_disable_billing=true 或 self_mode.enable=true：使用 free provider（无限用量）
// - 否则：使用 normal provider（订阅优先 + 余额兜底）
type FeatureProvider struct {
	features FeatureGetter
	selfMode bool

	normal Provider
	free   Provider
}

func NewFeatureProvider(features FeatureGetter, selfMode bool, normal Provider, free Provider) *FeatureProvider {
	return &FeatureProvider{
		features: features,
		selfMode: selfMode,
		normal:   normal,
		free:     free,
	}
}

func (p *FeatureProvider) selectProvider(ctx context.Context) (Provider, error) {
	if p == nil {
		return nil, errors.New("feature provider 为空")
	}
	if p.features == nil {
		if p.selfMode {
			return p.free, nil
		}
		return p.normal, nil
	}
	if p.features.FeatureDisabledEffective(ctx, p.selfMode, store.SettingFeatureDisableBilling) {
		return p.free, nil
	}
	return p.normal, nil
}

func (p *FeatureProvider) Reserve(ctx context.Context, in ReserveInput) (ReserveResult, error) {
	impl, err := p.selectProvider(ctx)
	if err != nil {
		return ReserveResult{}, err
	}
	if impl == nil {
		return ReserveResult{}, errors.New("quota provider 为空")
	}
	return impl.Reserve(ctx, in)
}

func (p *FeatureProvider) Commit(ctx context.Context, in CommitInput) error {
	impl, err := p.selectProvider(ctx)
	if err != nil {
		return err
	}
	if impl == nil {
		return errors.New("quota provider 为空")
	}
	return impl.Commit(ctx, in)
}

func (p *FeatureProvider) Void(ctx context.Context, usageEventID int64) error {
	impl, err := p.selectProvider(ctx)
	if err != nil {
		return err
	}
	if impl == nil {
		return errors.New("quota provider 为空")
	}
	return impl.Void(ctx, usageEventID)
}
