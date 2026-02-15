package extraction

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/mycelium-dev/mycelium/internal/model"
)

// RandIntn returns a random int in [0, n). Injectable for testing.
type RandIntn func(n int) int

func cryptoRandIntn(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		// Entropy exhaustion is catastrophic; fail loudly rather than silently biasing.
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return int(v.Int64())
}

// maybeSample writes to human_review_samples with stratified sampling per §3.4.
// Maintains ~50/50 split between extracted and rejected pools by always sampling
// from whichever pool is underrepresented, and probabilistically from the other.
func (p *Pipeline) maybeSample(ctx context.Context, sessionID string, result *model.ExtractionResult) {
	if p.reviewer == nil || p.sampleRate <= 0 {
		return
	}

	isExtracted := result.Status == model.StatusExtracted
	pool := "rejected"
	if isExtracted {
		pool = "extracted"
	}

	// Stratified sampling: maintain ~50/50 between extracted and rejected pools.
	underrepresented := false
	overrepresented := false
	if isExtracted {
		underrepresented = p.extractedSamples < p.rejectedSamples
		overrepresented = p.extractedSamples > p.rejectedSamples
	} else {
		underrepresented = p.rejectedSamples < p.extractedSamples
		overrepresented = p.rejectedSamples > p.extractedSamples
	}

	if overrepresented {
		return
	}

	if !underrepresented {
		// Equal counts — use probabilistic sampling at base rate.
		threshold := int(p.sampleRate * 10000)
		if threshold <= 0 {
			return
		}
		roll := p.randIntn(10000)
		if roll >= threshold {
			return
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		p.log.Warn("sample marshal error", "session_id", sessionID, "error", err)
		return
	}

	if err := p.reviewer.InsertReviewSample(ctx, sessionID, data); err != nil {
		p.log.Warn("sample insert error", "session_id", sessionID, "error", err)
		return
	}

	if isExtracted {
		p.extractedSamples++
	} else {
		p.rejectedSamples++
	}

	p.log.Info("human review sampled", "session_id", sessionID, "status", result.Status, "pool", pool,
		"extracted_samples", p.extractedSamples, "rejected_samples", p.rejectedSamples)
}
