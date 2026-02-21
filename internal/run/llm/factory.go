package llm

import "fmt"

// NewClient creates an LLMClientV2 for the given provider.
// Supported providers: "openai" (default), "anthropic".
func NewClient(provider, apiKey, model, baseURL string) (LLMClientV2, error) {
	switch provider {
	case "openai", "":
		return NewOpenAIClient(apiKey, model, baseURL), nil
	case "anthropic":
		return NewAnthropicClient(apiKey, model, baseURL), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", provider)
	}
}
