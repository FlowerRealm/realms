package openai

import (
	"net/http"
	"strings"
	"time"

	"realms/internal/scheduler"
	"realms/internal/store"
)

type resolvedChannelModelBindings struct {
	upstreamByChannel  map[int64]string
	bindingIDByChannel map[int64]int64
}

func resolveChannelModelBindings(bindings []store.ChannelModelBinding, requireChannelType string) resolvedChannelModelBindings {
	requireChannelType = strings.TrimSpace(requireChannelType)
	out := resolvedChannelModelBindings{
		upstreamByChannel:  make(map[int64]string, len(bindings)),
		bindingIDByChannel: make(map[int64]int64, len(bindings)),
	}
	for _, binding := range bindings {
		if requireChannelType != "" && strings.TrimSpace(binding.ChannelType) != requireChannelType {
			continue
		}
		upstreamModel := strings.TrimSpace(binding.UpstreamModel)
		if upstreamModel == "" {
			continue
		}
		if binding.ChannelID <= 0 {
			continue
		}
		out.upstreamByChannel[binding.ChannelID] = upstreamModel
		if binding.ID > 0 {
			out.bindingIDByChannel[binding.ChannelID] = binding.ID
		}
	}
	return out
}

func (r resolvedChannelModelBindings) Empty() bool {
	return len(r.upstreamByChannel) == 0
}

func (r resolvedChannelModelBindings) UpstreamModel(channelID int64, fallback string) string {
	if upstreamModel, ok := r.upstreamByChannel[channelID]; ok && strings.TrimSpace(upstreamModel) != "" {
		return upstreamModel
	}
	return fallback
}

func (r resolvedChannelModelBindings) BindingID(channelID int64) int64 {
	return r.bindingIDByChannel[channelID]
}

func (r resolvedChannelModelBindings) ApplyToConstraints(cons *scheduler.Constraints) {
	if cons == nil {
		return
	}
	if len(r.upstreamByChannel) == 0 {
		cons.AllowChannelIDs = nil
		cons.ChannelModelBindingIDs = nil
		return
	}
	allowChannelIDs := make(map[int64]struct{}, len(r.upstreamByChannel))
	for channelID := range r.upstreamByChannel {
		allowChannelIDs[channelID] = struct{}{}
	}
	bindingIDs := make(map[int64]int64, len(r.bindingIDByChannel))
	for channelID, bindingID := range r.bindingIDByChannel {
		bindingIDs[channelID] = bindingID
	}
	cons.AllowChannelIDs = allowChannelIDs
	cons.ChannelModelBindingIDs = bindingIDs
}

type upstreamHTTPFailureClassification struct {
	Retriable     bool
	Scope         scheduler.FailureScope
	ErrorClass    string
	CooldownUntil *time.Time
}

func classifyUpstreamHTTPFailure(statusCode int, body []byte, bindingID int64, codexErr codexOAuthUpstreamErr) upstreamHTTPFailureClassification {
	if codexErr.Kind != codexOAuthErrNone {
		return upstreamHTTPFailureClassification{
			Retriable:     true,
			Scope:         classifyRetriableFailureScope(statusCode, codexErr),
			ErrorClass:    codexErr.errorClass(),
			CooldownUntil: codexErr.cooldownUntil(time.Now(), body),
		}
	}
	if bindingID > 0 && isExplicitChannelModelFailure(statusCode, body) {
		return upstreamHTTPFailureClassification{
			Retriable:  true,
			Scope:      scheduler.FailureScopeChannelModel,
			ErrorClass: "upstream_model_unavailable",
		}
	}
	if isRetriableStatus(statusCode) {
		return upstreamHTTPFailureClassification{
			Retriable:  true,
			Scope:      classifyRetriableFailureScope(statusCode, codexErr),
			ErrorClass: "upstream_status",
		}
	}
	return upstreamHTTPFailureClassification{
		Retriable:  false,
		Scope:      classifyNonRetriableFailureScope(statusCode),
		ErrorClass: "upstream_status",
	}
}

func isExplicitChannelModelFailure(statusCode int, body []byte) bool {
	switch statusCode {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnprocessableEntity:
	default:
		return false
	}

	code, typ := extractUpstreamErrorCodeAndType(body)
	code = strings.ToLower(strings.TrimSpace(code))
	typ = strings.ToLower(strings.TrimSpace(typ))
	msg := strings.ToLower(strings.TrimSpace(summarizeUpstreamErrorBody(body)))

	if strings.Contains(msg, "route missing") {
		return false
	}
	if looksLikeModelFailureCode(code) || looksLikeModelFailureCode(typ) {
		return true
	}
	if msg == "" {
		return false
	}
	modelMentioned := strings.Contains(msg, "model") || strings.Contains(msg, "capability")
	if !modelMentioned {
		return false
	}
	if containsAny(msg,
		"model not found",
		"not found",
		"no such model",
		"unknown model",
		"invalid model",
		"unsupported model",
		"unsupported capability",
		"this model",
		"selected model",
	) {
		return true
	}
	if containsAny(msg,
		"does not support",
		"not support",
		"not supported",
		"doesn't support",
		"not available",
		"unavailable",
		"does not exist",
		"not allowed",
		"disabled",
		"decommissioned",
	) {
		return true
	}
	return false
}

func looksLikeModelFailureCode(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if containsAny(v,
		"model_not_found",
		"unknown_model",
		"invalid_model",
		"unsupported_model",
		"model_not_supported",
		"unsupported_model_capability",
		"model_unavailable",
	) {
		return true
	}
	return strings.Contains(v, "model") && containsAny(v, "not_found", "unknown", "invalid", "unsupported", "unavailable")
}

func containsAny(s string, parts ...string) bool {
	for _, part := range parts {
		if part != "" && strings.Contains(s, part) {
			return true
		}
	}
	return false
}
