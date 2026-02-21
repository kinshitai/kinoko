package extraction

import (
	"testing"
)

func TestCryptoRandIntn_InRange(t *testing.T) {
	for _, n := range []int{1, 2, 10, 100, 10000} {
		for i := 0; i < 100; i++ {
			v := cryptoRandIntn(n)
			if v < 0 || v >= n {
				t.Fatalf("cryptoRandIntn(%d) = %d, out of range [0, %d)", n, v, n)
			}
		}
	}
}

func TestTruncateContent_UTF8Boundaries(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		max     int
		wantLen int
		wantStr string
	}{
		{"no truncation", []byte("hello"), 10, 5, "hello"},
		{"exact fit", []byte("hello"), 5, 5, "hello"},
		{"ascii truncation", []byte("hello world"), 5, 5, "hello"},
		// € is 3 bytes (E2 82 AC)
		{"cut before multibyte", []byte("ab€"), 3, 2, "ab"},
		{"cut into 2-byte char", []byte("aä"), 2, 1, "a"}, // ä is C3 A4
		// 4-byte char: 𝄞 (F0 9D 84 9E)
		{"cut into 4-byte char", []byte("a𝄞"), 3, 1, "a"},
		{"empty", []byte{}, 10, 0, ""},
		{"zero max", []byte("hello"), 0, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateContent(tt.input, tt.max)
			if len(result) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(result), tt.wantLen)
			}
			if string(result) != tt.wantStr {
				t.Errorf("result = %q, want %q", string(result), tt.wantStr)
			}
		})
	}
}

func TestSanitizeDelimiters(t *testing.T) {
	tests := []struct {
		name  string
		input string
		begin string
		end   string
		want  string
	}{
		{"no match", "hello world", "---BEGIN---", "---END---", "hello world"},
		{"begin match", "---BEGIN---content", "---BEGIN---", "---END---", "[SANITIZED_DELIMITER]content"},
		{"end match", "content---END---", "---BEGIN---", "---END---", "content[SANITIZED_DELIMITER]"},
		{"both match", "---BEGIN---stuff---END---", "---BEGIN---", "---END---", "[SANITIZED_DELIMITER]stuff[SANITIZED_DELIMITER]"},
		{"multiple occurrences", "---BEGIN------BEGIN---", "---BEGIN---", "---END---", "[SANITIZED_DELIMITER][SANITIZED_DELIMITER]"},
		{"adversarial nested", "---BEGIN------BEGIN------END------END---", "---BEGIN---", "---END---",
			"[SANITIZED_DELIMITER][SANITIZED_DELIMITER][SANITIZED_DELIMITER][SANITIZED_DELIMITER]"},
		{"empty content", "", "---BEGIN---", "---END---", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeDelimiters([]byte(tt.input), tt.begin, tt.end)
			if string(result) != tt.want {
				t.Errorf("got %q, want %q", string(result), tt.want)
			}
		})
	}
}
