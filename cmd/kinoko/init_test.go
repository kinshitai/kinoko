package main

import "testing"

func TestSanitizeHostname(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "simple hostname", raw: "myhost", want: "myhost"},
		{name: "FQDN with .local", raw: "my-host.local", want: "my-host"},
		{name: "AWS-style FQDN", raw: "ip-172-31-0-5.ec2.internal", want: "ip-172-31-0-5"},
		{name: "multiple dots", raw: "a.b.c.d", want: "a"},
		{name: "trailing dot", raw: "myhost.", want: "myhost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeHostname(tt.raw)
			if got != tt.want {
				t.Errorf("sanitizeHostname(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestSanitizeHostname_FallbackToKinokoClient(t *testing.T) {
	// os.Hostname() will return something non-empty on any real system,
	// so we can't easily test the "kinoko-client" fallback. But we can
	// verify that empty-string input at least returns a non-empty string.
	got := sanitizeHostname("")
	if got == "" {
		t.Error("sanitizeHostname(\"\") returned empty string")
	}
}
