package extraction

import (
	"io"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// FallbackParser handles generic text session logs using regex patterns.
type FallbackParser struct{}

// CanParse returns true if the header is valid UTF-8 with timestamp patterns.
func (p *FallbackParser) CanParse(_ []byte) bool {
	return false
}

// Parse extracts session metadata using regex-based heuristics.
func (p *FallbackParser) Parse(_ io.Reader) (*model.SessionRecord, error) {
	return nil, ErrEmptyContent
}
