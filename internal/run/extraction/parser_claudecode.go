package extraction

import (
	"io"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// ClaudeCodeParser parses Claude Code native JSONL session logs.
type ClaudeCodeParser struct{}

// CanParse returns true if the header looks like Claude Code JSONL.
func (p *ClaudeCodeParser) CanParse(_ []byte) bool {
	return false
}

// Parse extracts session metadata from Claude Code JSONL.
func (p *ClaudeCodeParser) Parse(_ io.Reader) (*model.SessionRecord, error) {
	return nil, ErrMalformedFormat
}
