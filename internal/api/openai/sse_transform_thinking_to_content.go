package openai

import (
	"encoding/json"
	"strings"
)

type thinkingToContentState struct {
	firstThinking bool
	closedThink   bool
	hasThinking   bool
}

func newThinkingToContentTransformer() func(data string) ([]string, error) {
	st := &thinkingToContentState{
		firstThinking: true,
		closedThink:   false,
		hasThinking:   false,
	}
	return func(data string) ([]string, error) {
		data = strings.TrimSpace(data)
		if data == "" {
			return nil, nil
		}

		var evt map[string]any
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return nil, nil
		}

		choices, ok := evt["choices"].([]any)
		if !ok || len(choices) == 0 {
			return nil, nil
		}

		hasThinking := false
		hasContent := false
		var thinkingBuf strings.Builder

		for _, c := range choices {
			ch, ok := c.(map[string]any)
			if !ok {
				continue
			}
			delta, ok := ch["delta"].(map[string]any)
			if !ok {
				continue
			}
			rc := stringFromAny(delta["reasoning_content"])
			if rc != "" {
				hasThinking = true
				thinkingBuf.WriteString(rc)
			}
			ct := stringFromAny(delta["content"])
			if ct != "" {
				hasContent = true
			}
		}

		if st.firstThinking && hasThinking {
			think := thinkingBuf.String()
			for _, c := range choices {
				ch, ok := c.(map[string]any)
				if !ok {
					continue
				}
				delta, ok := ch["delta"].(map[string]any)
				if !ok {
					continue
				}
				delta["content"] = "<think>\n" + think
				delete(delta, "reasoning_content")
				delete(delta, "reasoning")
				ch["delta"] = delta
			}
			evt["choices"] = choices
			st.firstThinking = false
			st.hasThinking = true
			out, err := json.Marshal(evt)
			if err != nil {
				return nil, nil
			}
			return []string{string(out)}, nil
		}

		var closeEvtJSON string
		if hasContent && st.hasThinking && !st.closedThink {
			closeEvt := make(map[string]any, len(evt))
			for k, v := range evt {
				if k == "choices" {
					continue
				}
				closeEvt[k] = v
			}

			closeChoices := make([]any, 0, len(choices))
			for _, c := range choices {
				ch, ok := c.(map[string]any)
				if !ok {
					continue
				}
				outChoice := make(map[string]any, len(ch))
				if idx, ok := ch["index"]; ok {
					outChoice["index"] = idx
				}
				outChoice["delta"] = map[string]any{
					"content": "\n</think>\n",
				}
				if fr, ok := ch["finish_reason"]; ok {
					outChoice["finish_reason"] = fr
				}
				closeChoices = append(closeChoices, outChoice)
			}
			if len(closeChoices) > 0 {
				closeEvt["choices"] = closeChoices
				b, err := json.Marshal(closeEvt)
				if err == nil {
					closeEvtJSON = string(b)
					st.closedThink = true
				}
			}
		}

		changed := false
		for _, c := range choices {
			ch, ok := c.(map[string]any)
			if !ok {
				continue
			}
			delta, ok := ch["delta"].(map[string]any)
			if !ok {
				continue
			}
			rc := stringFromAny(delta["reasoning_content"])
			if rc != "" {
				delta["content"] = rc
				delete(delta, "reasoning_content")
				delete(delta, "reasoning")
				ch["delta"] = delta
				changed = true
				continue
			}
			if !hasThinking && !hasContent {
				if _, ok := delta["reasoning_content"]; ok {
					delete(delta, "reasoning_content")
					changed = true
				}
				if _, ok := delta["reasoning"]; ok {
					delete(delta, "reasoning")
					changed = true
				}
				ch["delta"] = delta
			}
		}
		if !changed && closeEvtJSON == "" {
			return nil, nil
		}

		evt["choices"] = choices
		out, err := json.Marshal(evt)
		if err != nil {
			return nil, nil
		}
		if closeEvtJSON != "" {
			return []string{closeEvtJSON, string(out)}, nil
		}
		return []string{string(out)}, nil
	}
}
