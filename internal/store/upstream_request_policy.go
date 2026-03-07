package store

import "errors"

var ErrUpstreamChannelFastModeRequiresServiceTier = errors.New("启用 Fast mode 时必须同时允许透传 service_tier")

func validateUpstreamChannelRequestPolicy(allowServiceTier bool, fastMode bool) error {
	if fastMode && !allowServiceTier {
		return ErrUpstreamChannelFastModeRequiresServiceTier
	}
	return nil
}

func NormalizeUpstreamChannelRequestPolicy(allowServiceTier bool, fastMode bool) (bool, bool) {
	if fastMode {
		allowServiceTier = true
	}
	return allowServiceTier, fastMode
}

func ValidateRebuildUpstreamChannelRequestPolicy(allowServiceTier bool, fastMode bool) error {
	allowServiceTier, fastMode = NormalizeUpstreamChannelRequestPolicy(allowServiceTier, fastMode)
	return validateUpstreamChannelRequestPolicy(allowServiceTier, fastMode)
}
