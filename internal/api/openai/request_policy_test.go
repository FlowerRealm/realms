package openai

import (
	"testing"

	"realms/internal/scheduler"

	"github.com/tidwall/gjson"
)

func TestApplyChannelRequestPolicy_FastMode(t *testing.T) {
	tests := []struct {
		name      string
		sel       scheduler.Selection
		body      string
		wantExist bool
		wantValue string
	}{
		{
			name:      "service_tier removed when channel disallows service_tier",
			sel:       scheduler.Selection{AllowServiceTier: false, FastMode: true},
			body:      `{"service_tier":"priority","store":true}`,
			wantExist: false,
		},
		{
			name:      "priority removed when fast mode disabled",
			sel:       scheduler.Selection{AllowServiceTier: true, FastMode: false},
			body:      `{"service_tier":"priority","store":true}`,
			wantExist: false,
		},
		{
			name:      "flex preserved when fast mode disabled",
			sel:       scheduler.Selection{AllowServiceTier: true, FastMode: false},
			body:      `{"service_tier":"flex","store":true}`,
			wantExist: true,
			wantValue: "flex",
		},
		{
			name:      "priority preserved when fast mode enabled",
			sel:       scheduler.Selection{AllowServiceTier: true, FastMode: true},
			body:      `{"service_tier":"priority","store":true}`,
			wantExist: true,
			wantValue: "priority",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := applyChannelRequestPolicy([]byte(tt.body), tt.sel)
			if err != nil {
				t.Fatalf("applyChannelRequestPolicy: %v", err)
			}
			got := gjson.GetBytes(out, "service_tier")
			if got.Exists() != tt.wantExist {
				t.Fatalf("service_tier exists=%v, want=%v body=%s", got.Exists(), tt.wantExist, string(out))
			}
			if tt.wantExist && got.String() != tt.wantValue {
				t.Fatalf("service_tier=%q, want=%q", got.String(), tt.wantValue)
			}
		})
	}
}
