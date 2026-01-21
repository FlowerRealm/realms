package icons

import (
	"strings"
)

const lobeHubStaticSVGVersion = "1.77.0"

// ModelIconURL 返回模型的品牌/供应商图标 URL（来自 @lobehub/icons-static-svg）。
//
// 设计目标：
// - SSR 模板直接可用（返回空字符串表示“无匹配图标”）
// - 优先使用 ownedBy；缺失时回退到 modelID 规则匹配
func ModelIconURL(modelID, ownedBy string) string {
	key := modelIconKey(modelID, ownedBy)
	if key == "" {
		return ""
	}
	return "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@" + lobeHubStaticSVGVersion + "/icons/" + key + ".svg"
}

func modelIconKey(modelID, ownedBy string) string {
	if k := iconKeyFromText(ownedBy); k != "" {
		return k
	}
	return iconKeyFromText(modelID)
}

func iconKeyFromText(raw string) string {
	s := normalize(raw)
	if s == "" {
		return ""
	}

	switch {
	case containsAny(s, "openai", "gpt", "dalle", "whisper", "textembedding", "textmoderation", "tts", "o1", "o3", "o4"):
		return "openai"
	case containsAny(s, "anthropic", "claude"):
		return "claude-color"
	case containsAny(s, "google", "gemini", "gemma", "vertexai", "palm", "learnlm", "imagen", "veo"):
		return "gemini-color"
	case containsAny(s, "moonshot", "kimi"):
		return "kimi-color"
	case containsAny(s, "zhipu", "chatglm", "glm", "cogview", "cogvideo"):
		return "zhipu-color"
	case containsAny(s, "qwen", "tongyi", "dashscope", "aliyun", "alibaba", "bailian"):
		return "qwen-color"
	case containsAny(s, "deepseek"):
		return "deepseek-color"
	case containsAny(s, "minimax", "abab"):
		return "minimax-color"
	case containsAny(s, "wenxin", "ernie"):
		return "wenxin-color"
	case containsAny(s, "baidu"):
		return "baidu-color"
	case containsAny(s, "spark", "xunfei", "iflytek"):
		return "spark-color"
	case containsAny(s, "hunyuan"):
		return "hunyuan-color"
	case containsAny(s, "tencent"):
		return "tencent-color"
	case containsAny(s, "doubao"):
		return "doubao-color"
	case containsAny(s, "bytedance", "volcengine"):
		return "bytedance-color"
	case containsAny(s, "mistral"):
		return "mistral-color"
	case containsAny(s, "cohere"):
		return "cohere-color"
	case containsAny(s, "bedrock"):
		return "bedrock-color"
	case containsAny(s, "aws"):
		return "aws-color"
	case containsAny(s, "azureai"):
		return "azureai-color"
	case containsAny(s, "azure"):
		return "azure-color"
	case containsAny(s, "cloudflare"):
		return "cloudflare-color"
	case containsAny(s, "openrouter"):
		return "openrouter"
	case containsAny(s, "ollama"):
		return "ollama"
	case containsAny(s, "xai"):
		return "xai"
	case containsAny(s, "perplexity"):
		return "perplexity-color"
	case containsAny(s, "replicate"):
		return "replicate"
	default:
		return ""
	}
}

func normalize(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
