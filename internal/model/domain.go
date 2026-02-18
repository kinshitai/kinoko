// Package model defines the core domain types and interfaces for the Kinoko
// extraction and injection pipelines: skills, sessions, embeddings, queries,
// and storage contracts.
package model

// ValidDomains is the set of recognised domains per spec §2.1.
var ValidDomains = map[string]bool{
	"Frontend":    true,
	"Backend":     true,
	"DevOps":      true,
	"Data":        true,
	"Security":    true,
	"Performance": true,
}

// ValidateDomain returns the domain if known, or "Backend" as default.
func ValidateDomain(d string) string {
	if ValidDomains[d] {
		return d
	}
	return "Backend"
}
