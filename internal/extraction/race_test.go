package extraction

import (
	"github.com/kinoko-dev/kinoko/internal/model"
	"context"
	"fmt"
	"sync"
	"testing"
)

// TestBug_PipelineSamplingRace proves the P0 race condition in Pipeline.maybeSample.
// The extractedSamples and rejectedSamples counters are plain ints with no
// synchronization. Concurrent calls to Extract will race on these counters.
// Run with: go test -race -run TestBug_PipelineSamplingRace
func TestBug_PipelineSamplingRace(t *testing.T) {
	rev := &countingReviewer{}
	p, err := NewPipeline(PipelineConfig{
		Stage1:     &mockStage1{result: failStage1("rejected for sampling test")},
		Stage2:     &mockStage2{},
		Stage3:     &mockStage3{},
		Reviewer:   rev,
		Log:        testLog(),
		SampleRate: 1.0, // always sample
		RandIntn:   fixedRand(0),
	})
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 20
	const callsPerGoroutine = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < callsPerGoroutine; i++ {
				sess := model.SessionRecord{
					ID:        fmt.Sprintf("race-sess-%d-%d", id, i),
					LibraryID: "lib-race",
				}
				_, _ = p.Extract(context.Background(), sess, []byte("content"))
			}
		}(g)
	}
	wg.Wait()

	// The test itself may not fail deterministically without -race,
	// but the race detector will flag the unsynchronized counter access.
	ext := p.extractedSamples.Load()
	rej := p.rejectedSamples.Load()
	total := ext + rej
	t.Logf("sampling counters: extracted=%d rejected=%d total=%d (expected %d)",
		ext, rej, total, goroutines*callsPerGoroutine)
}
