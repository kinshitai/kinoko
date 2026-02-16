package llm

import "testing"

func TestNewClient_OpenAI(t *testing.T) {
	c, err := NewClient("openai", "key", "gpt-4o-mini", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*OpenAIClient); !ok {
		t.Fatalf("expected *OpenAIClient, got %T", c)
	}
}

func TestNewClient_OpenAI_Default(t *testing.T) {
	c, err := NewClient("", "key", "gpt-4o-mini", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*OpenAIClient); !ok {
		t.Fatalf("expected *OpenAIClient, got %T", c)
	}
}

func TestNewClient_Anthropic(t *testing.T) {
	c, err := NewClient("anthropic", "key", "claude-sonnet-4-20250514", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*AnthropicClient); !ok {
		t.Fatalf("expected *AnthropicClient, got %T", c)
	}
}

func TestNewClient_Unknown(t *testing.T) {
	_, err := NewClient("gemini", "key", "model", "")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
