package dataset

import (
	"context"
	"testing"
	"time"
)

// signalingRetryRepo embeds the in-memory fakeRepo and signals every time the
// quality-retry scanner queries it, so a test can observe whether the loop is
// still running.
type signalingRetryRepo struct {
	*fakeRepo
	calls chan struct{}
}

func (r *signalingRetryRepo) ListDueQualityRetries(_ context.Context, _ int) ([]QualityRetryRow, error) {
	select {
	case r.calls <- struct{}{}:
	default:
	}
	return nil, nil // no rows → loop just ticks, never sends on qCh
}

// Close() must stop the quality-retry scanner. Before the fix the loop was
// started with context.Background() and never cancelled, so it kept running
// after Close() (a goroutine + ticker leak) and — because Close() also closes
// qCh — could panic with "send on closed channel" on shutdown.
func TestClose_StopsQualityRetryLoop(t *testing.T) {
	calls := make(chan struct{}, 1024)
	repo := &signalingRetryRepo{fakeRepo: newFakeRepo(), calls: calls}
	svc := NewService(repo, fakeIdentity{}, nil,
		WithAsyncQuality(1, 1), withRetryInterval(time.Millisecond))

	// Confirm the loop is alive (it scans on every tick).
	for i := 0; i < 2; i++ {
		select {
		case <-calls:
		case <-time.After(2 * time.Second):
			t.Fatal("quality retry loop never ran")
		}
	}

	svc.Close() // must stop the loop without panicking

	// Drain scans that were already in flight, then confirm none arrive after.
	time.Sleep(20 * time.Millisecond)
	for {
		select {
		case <-calls:
			continue
		default:
		}
		break
	}
	select {
	case <-calls:
		t.Fatal("quality retry loop still scanning after Close() — goroutine leak / not stopped")
	case <-time.After(60 * time.Millisecond):
		// No scans after Close: the loop stopped. (At 1ms cadence a live loop
		// would have ticked ~60 times in this window.)
	}
}
