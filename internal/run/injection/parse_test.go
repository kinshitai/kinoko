package injection

import (
	"testing"

	"github.com/kinoko-dev/kinoko/internal/run/llmutil"
)

func TestParseClassificationResponse_AllStrategies(t *testing.T) {
	validJSON := `{"intent":"BUILD","domain":"Backend","patterns":["BUILD/Backend/APIDesign"]}`

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"raw JSON", validJSON, false},
		{"first-brace-to-last", "Here:\n" + validJSON + "\nDone.", false},
		{"empty", "", true},
		{"no JSON", "I think this is a build task.", true},
		{"malformed", `{"intent": "BUILD"`, true},
		{"json fence", "```json\n" + validJSON + "\n```", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := llmutil.ExtractJSON[classificationResponse](tt.input)
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
