package openai

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"

	"realms/internal/scheduler"
)

func TestNormalizeRequestServiceTier_StrictValidation(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		payload     map[string]any
		wantTier    string
		wantHasTier bool
		wantPayload bool
		wantErr     bool
	}{
		{
			name:        "fast normalized to priority",
			raw:         `{"service_tier":"fast"}`,
			payload:     map[string]any{"service_tier": "fast"},
			wantTier:    "priority",
			wantHasTier: true,
			wantPayload: true,
		},
		{
			name:        "empty string removed",
			raw:         `{"service_tier":"  "}`,
			payload:     map[string]any{"service_tier": "  "},
			wantHasTier: false,
			wantPayload: false,
		},
		{
			name:    "unknown tier rejected",
			raw:     `{"service_tier":"turbo"}`,
			payload: map[string]any{"service_tier": "turbo"},
			wantErr: true,
		},
		{
			name:    "non string rejected",
			raw:     `{"service_tier":123}`,
			payload: map[string]any{"service_tier": float64(123)},
			wantErr: true,
		},
		{
			name:        "default preserved from body",
			raw:         `{"service_tier":"default"}`,
			wantTier:    "default",
			wantHasTier: true,
			wantPayload: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload map[string]any
			if tt.payload != nil {
				payload = make(map[string]any, len(tt.payload))
				for k, v := range tt.payload {
					payload[k] = v
				}
			}
			out, tier, err := normalizeRequestServiceTier([]byte(tt.raw), payload)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeRequestServiceTier: %v", err)
			}
			if tt.wantHasTier {
				if tier == nil || *tier != tt.wantTier {
					t.Fatalf("tier=%v, want %q", tier, tt.wantTier)
				}
			} else if tier != nil {
				t.Fatalf("tier=%v, want nil", *tier)
			}
			got := gjson.GetBytes(out, "service_tier")
			if got.Exists() != tt.wantHasTier {
				t.Fatalf("body has service_tier=%v, want %v body=%s", got.Exists(), tt.wantHasTier, string(out))
			}
			if tt.wantHasTier && got.String() != tt.wantTier {
				t.Fatalf("body service_tier=%q, want %q", got.String(), tt.wantTier)
			}
			_, hasPayload := payload["service_tier"]
			if hasPayload != tt.wantPayload {
				b, _ := json.Marshal(payload)
				t.Fatalf("payload has service_tier=%v, want %v payload=%s", hasPayload, tt.wantPayload, string(b))
			}
		})
	}
}

func TestApplyChannelRequestPolicy_RejectsInvalidServiceTier(t *testing.T) {
	_, err := applyChannelRequestPolicy([]byte(`{"service_tier":"turbo","store":true}`), scheduler.Selection{AllowServiceTier: true, FastMode: true})
	if err == nil {
		t.Fatal("expected error")
	}
}
