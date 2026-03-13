package modelcheck

import "testing"

func TestExtractResponseModelBytes(t *testing.T) {
	t.Run("top level model", func(t *testing.T) {
		got := ExtractResponseModelBytes([]byte(`{"model":"gpt-5.2-mini"}`))
		if got == nil || *got != "gpt-5.2-mini" {
			t.Fatalf("unexpected model: %v", got)
		}
	})

	t.Run("nested response model", func(t *testing.T) {
		got := ExtractResponseModelBytes([]byte(`{"response":{"model":"gpt-5.2-mini"}}`))
		if got == nil || *got != "gpt-5.2-mini" {
			t.Fatalf("unexpected model: %v", got)
		}
	})

	t.Run("blank model returns nil", func(t *testing.T) {
		if got := ExtractResponseModelBytes([]byte(`{"model":"   "}`)); got != nil {
			t.Fatalf("expected nil, got %v", *got)
		}
	})

	t.Run("nested message model", func(t *testing.T) {
		got := ExtractResponseModelBytes([]byte(`{"message":{"model":"claude-sonnet-4"}}`))
		if got == nil || *got != "claude-sonnet-4" {
			t.Fatalf("unexpected model: %v", got)
		}
	})

	t.Run("sse data lines", func(t *testing.T) {
		body := []byte("event: response.created\ndata: {\"response\":{\"model\":\"gpt-5\"}}\n\nevent: done\ndata: [DONE]\n\n")
		got := ExtractResponseModelBytes(body)
		if got == nil || *got != "gpt-5" {
			t.Fatalf("unexpected model: %v", got)
		}
	})
}

func TestStatusFrom(t *testing.T) {
	forwarded := "gpt-5.2"
	upstream := "gpt-5.2-mini"
	same := "gpt-5.2"

	if got := StatusFrom(&forwarded, &same); got != StatusOK {
		t.Fatalf("StatusFrom same = %q", got)
	}
	if got := StatusFrom(&forwarded, &upstream); got != StatusMismatch {
		t.Fatalf("StatusFrom mismatch = %q", got)
	}
	if got := StatusFrom(&forwarded, nil); got != StatusUnknown {
		t.Fatalf("StatusFrom unknown = %q", got)
	}
}
