package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// ErrBackpressure is returned when the queue depth exceeds the critical threshold.
var ErrBackpressure = fmt.Errorf("queue backpressure: depth exceeds critical threshold")

// SessionQueue manages the extraction work queue.
type SessionQueue interface {
	Enqueue(ctx context.Context, session model.SessionRecord, logContent []byte) error
	Claim(ctx context.Context, workerID string) (*QueueEntry, error)
	Complete(ctx context.Context, sessionID string, result *model.ExtractionResult) error
	Fail(ctx context.Context, sessionID string, err error) error
	FailPermanent(ctx context.Context, sessionID string, err error) error
	Depth(ctx context.Context) (int, error)
	RequeueStale(ctx context.Context, staleDuration time.Duration) (int, error)
}

// QueueEntry is returned by Claim.
type QueueEntry struct {
	SessionID      string
	LogContentPath string
	RetryCount     int
	LibraryID      string
}
