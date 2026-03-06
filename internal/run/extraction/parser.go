package extraction

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

var (
	ErrEmptyContent       = errors.New("session parser: empty content")
	ErrMalformedFormat    = errors.New("session parser: malformed format")
	ErrUnrecognizedFormat = errors.New("session parser: unrecognized format")
)

// SessionParser extracts structured metadata from a session log.
type SessionParser interface {
	// CanParse returns true if this parser handles the given format.
	// header is the first ~4KB of content. Implementations MUST decide
	// based on this peek buffer only.
	CanParse(header []byte) bool

	// Parse extracts session metadata from a reader.
	// Returns *SessionRecord on success. Returns partial result + error
	// for truncated/corrupt input. Returns nil + error for unparseable input.
	Parse(r io.Reader) (*model.SessionRecord, error)
}

// headerSize is the number of bytes read for format detection.
const headerSize = 4096

// parsers is the ordered list of format-specific parsers.
// Most specific first. Constructed explicitly — no init() registration.
var parsers = []SessionParser{
	&ClaudeCodeParser{},
	&FallbackParser{},
}

// ParseSession detects the format from the first 4KB and dispatches
// to the appropriate parser. Returns ErrUnrecognizedFormat if no parser matches.
func ParseSession(r io.Reader) (*model.SessionRecord, error) {
	header := make([]byte, headerSize)
	n, err := io.ReadAtLeast(r, header, 1)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrEmptyContent
		}
		return nil, fmt.Errorf("read header: %w", err)
	}
	header = header[:n]

	full := io.MultiReader(bytes.NewReader(header), r)

	for _, p := range parsers {
		if p.CanParse(header) {
			return p.Parse(full)
		}
	}
	return nil, ErrUnrecognizedFormat
}
