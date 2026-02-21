package llmutil

import (
	"testing"
)

type simple struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type nested struct {
	Outer struct {
		Inner string `json:"inner"`
	} `json:"outer"`
	Tags []string `json:"tags"`
}

func TestExtractJSON_RawJSON(t *testing.T) {
	input := `{"name":"test","value":42}`
	got, err := ExtractJSON[simple](input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "test" || got.Value != 42 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestExtractJSON_JSONFence(t *testing.T) {
	input := "Here is the result:\n```json\n{\"name\":\"fenced\",\"value\":1}\n```\nDone."
	got, err := ExtractJSON[simple](input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "fenced" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestExtractJSON_GenericFence(t *testing.T) {
	input := "Result:\n```\n{\"name\":\"generic\",\"value\":2}\n```"
	got, err := ExtractJSON[simple](input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "generic" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestExtractJSON_FirstBraceToLastBrace(t *testing.T) {
	input := "Some text before {\"name\":\"braces\",\"value\":3} and after."
	got, err := ExtractJSON[simple](input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "braces" || got.Value != 3 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestExtractJSON_NestedBraces(t *testing.T) {
	input := `Preamble {"outer":{"inner":"deep"},"tags":["a","b"]} epilogue`
	got, err := ExtractJSON[nested](input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Outer.Inner != "deep" || len(got.Tags) != 2 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestExtractJSON_Empty(t *testing.T) {
	_, err := ExtractJSON[simple]("")
	if err == nil {
		t.Fatal("expected error on empty input")
	}
}

func TestExtractJSON_Whitespace(t *testing.T) {
	_, err := ExtractJSON[simple]("   \n  ")
	if err == nil {
		t.Fatal("expected error on whitespace-only input")
	}
}

func TestExtractJSON_Malformed(t *testing.T) {
	_, err := ExtractJSON[simple]("{not json at all")
	if err == nil {
		t.Fatal("expected error on malformed input")
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	_, err := ExtractJSON[simple]("This response contains no JSON whatsoever.")
	if err == nil {
		t.Fatal("expected error when no JSON present")
	}
}

func TestExtractJSON_DifferentTypes(t *testing.T) {
	type scores struct {
		A int     `json:"a"`
		B float64 `json:"b"`
	}
	input := `{"a":5,"b":3.14}`
	got, err := ExtractJSON[scores](input)
	if err != nil {
		t.Fatal(err)
	}
	if got.A != 5 || got.B != 3.14 {
		t.Fatalf("unexpected: %+v", got)
	}
}
