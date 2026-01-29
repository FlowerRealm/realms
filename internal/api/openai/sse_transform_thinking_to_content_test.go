package openai

import (
	"encoding/json"
	"testing"
)

func TestThinkingToContentTransformer_PreservesWhitespaceAndClosesThink(t *testing.T) {
	tr := newThinkingToContentTransformer()

	outs, err := tr(`{"choices":[{"index":0,"delta":{"reasoning_content":" hello"}}]}`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(outs) != 1 {
		t.Fatalf("expected 1 out, got=%d", len(outs))
	}
	var evt1 map[string]any
	if err := json.Unmarshal([]byte(outs[0]), &evt1); err != nil {
		t.Fatalf("unmarshal out[0] failed: %v out=%q", err, outs[0])
	}
	if got := stringFromAny(((evt1["choices"].([]any))[0].(map[string]any))["delta"].(map[string]any)["content"]); got != "<think>\n hello" {
		t.Fatalf("unexpected first thinking content: %q", got)
	}
	if _, ok := ((evt1["choices"].([]any))[0].(map[string]any))["delta"].(map[string]any)["reasoning_content"]; ok {
		t.Fatalf("expected reasoning_content to be removed")
	}

	outs, err = tr(`{"choices":[{"index":0,"delta":{"reasoning_content":" world"}}]}`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(outs) != 1 {
		t.Fatalf("expected 1 out, got=%d", len(outs))
	}
	var evt2 map[string]any
	if err := json.Unmarshal([]byte(outs[0]), &evt2); err != nil {
		t.Fatalf("unmarshal out[0] failed: %v out=%q", err, outs[0])
	}
	if got := stringFromAny(((evt2["choices"].([]any))[0].(map[string]any))["delta"].(map[string]any)["content"]); got != " world" {
		t.Fatalf("unexpected follow-up thinking content: %q", got)
	}

	outs, err = tr(`{"choices":[{"index":0,"delta":{"content":"Hi"}}]}`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(outs) != 2 {
		t.Fatalf("expected 2 outs (close + content), got=%d outs=%q", len(outs), outs)
	}
	var closeEvt map[string]any
	if err := json.Unmarshal([]byte(outs[0]), &closeEvt); err != nil {
		t.Fatalf("unmarshal close event failed: %v out=%q", err, outs[0])
	}
	if got := stringFromAny(((closeEvt["choices"].([]any))[0].(map[string]any))["delta"].(map[string]any)["content"]); got != "\n</think>\n" {
		t.Fatalf("unexpected close event content: %q", got)
	}
}
