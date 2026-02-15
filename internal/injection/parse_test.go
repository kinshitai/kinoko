package injection

import (
	"testing"
)

func TestParseClassificationResponse_AllStrategies(t *testing.T) {
	validJSON := `{"intent":"BUILD","domain":"Backend","patterns":["BUILD/Backend/APIDesign"]}`

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"raw JSON", validJSON, false},
		// parseClassificationResponse only uses strategy 1 (raw) and strategy 4 (first-{-to-last-}).
		// It does NOT support ```json or ``` fences — this is the simplified version (tech debt C.2).
		{"first-brace-to-last", "Here:\n" + validJSON + "\nDone.", false},
		{"empty", "", true},
		{"no JSON", "I think this is a build task.", true},
		{"malformed", `{"intent": "BUILD"`, true},
		{"json fence NOT supported", "```json\n" + validJSON + "\n```", false}, // works via first-{-to-last
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out classificationResponse
			err := parseClassificationResponse(tt.input, &out)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && out.Intent != "BUILD" {
				t.Errorf("intent = %q, want BUILD", out.Intent)
			}
		})
	}
}
