package openai

import (
	"testing"

	"github.com/tidwall/gjson"

	"realms/internal/scheduler"
)

func TestApplyChannelRequestPolicy_FastMode(t *testing.T) {
	tests := []struct {
		name      string
		sel       scheduler.Selection
		body      string
		wantExist bool
		wantValue string
		wantErr   bool
	}{
		{
			name:    "priority rejected when channel disallows service_tier",
			sel:     scheduler.Selection{AllowServiceTier: false, FastMode: true},
			body:    `{"service_tier":"priority","store":true}`,
			wantErr: true,
		},
		{
			name:    "priority rejected when fast mode disabled",
			sel:     scheduler.Selection{AllowServiceTier: true, FastMode: false},
			body:    `{"service_tier":"priority","store":true}`,
			wantErr: true,
		},
		{
			name:      "flex preserved when fast mode disabled",
			sel:       scheduler.Selection{AllowServiceTier: true, FastMode: false},
			body:      `{"service_tier":"flex","store":true}`,
			wantExist: true,
			wantValue: "flex",
		},
		{
			name:      "fast normalized to priority when fast mode enabled",
			sel:       scheduler.Selection{AllowServiceTier: true, FastMode: true},
			body:      `{"service_tier":"fast","store":true}`,
			wantExist: true,
			wantValue: "priority",
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
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
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
