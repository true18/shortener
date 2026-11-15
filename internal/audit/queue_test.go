package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestQueueUsesWorkerContextAndDrainsOnClose(t *testing.T) {
	deliveries := make(chan Event, 1)
	observer := observerFunc(func(ctx context.Context, event Event) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		deliveries <- event
		return nil
	})
	queue := NewQueue(observer, zap.NewNop())
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	event := Event{Action: ActionShorten, URL: "https://example.com"}

	if err := queue.Notify(requestCtx, event); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if got := <-deliveries; got != event {
		t.Fatalf("delivered event = %+v, want %+v", got, event)
	}
	if err := queue.Notify(context.Background(), event); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("Notify after Close error = %v, want %v", err, ErrQueueClosed)
	}
}

func TestQueueDoesNotWaitForSlowObserver(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var startOnce sync.Once
	observer := observerFunc(func(context.Context, Event) error {
		startOnce.Do(func() { started <- struct{}{} })
		<-release
		return nil
	})
	queue := NewQueue(observer, zap.NewNop())
	if err := queue.Notify(context.Background(), Event{Action: ActionFollow}); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start observer")
	}

	for i := 0; i < queueCapacity; i++ {
		if err := queue.Notify(context.Background(), Event{Action: ActionShorten}); err != nil {
			t.Fatalf("Notify %d returned error: %v", i, err)
		}
	}
	close(release)
	if err := queue.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestQueueReportsFullBuffer(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var startOnce sync.Once
	observer := observerFunc(func(context.Context, Event) error {
		startOnce.Do(func() { started <- struct{}{} })
		<-release
		return nil
	})
	queue := NewQueue(observer, zap.NewNop())
	if err := queue.Notify(context.Background(), Event{}); err != nil {
		t.Fatalf("first Notify returned error: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start observer")
	}
	for i := 0; i < queueCapacity; i++ {
		if err := queue.Notify(context.Background(), Event{}); err != nil {
			t.Fatalf("Notify %d returned error: %v", i, err)
		}
	}
	if err := queue.Notify(context.Background(), Event{}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Notify on full queue error = %v, want %v", err, ErrQueueFull)
	}

	close(release)
	if err := queue.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestQueueKeepsDeliveringAfterObserverError(t *testing.T) {
	wantErr := errors.New("observer failed")
	got := make(chan Event, 1)
	publisher := NewPublisher(
		observerFunc(func(context.Context, Event) error { return wantErr }),
		observerFunc(func(_ context.Context, event Event) error {
			got <- event
			return nil
		}),
	)
	queue := NewQueue(publisher, zap.NewNop())
	event := Event{Action: ActionFollow, URL: "https://example.com"}
	if err := queue.Notify(context.Background(), event); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if delivered := <-got; delivered != event {
		t.Fatalf("second observer received %+v, want %+v", delivered, event)
	}
}

func TestNewQueueWithoutObserver(t *testing.T) {
	if queue := NewQueue(nil, nil); queue != nil {
		t.Fatalf("NewQueue(nil) = %v, want nil", queue)
	}
}

type observerFunc func(context.Context, Event) error

func (f observerFunc) Notify(ctx context.Context, event Event) error {
	return f(ctx, event)
}
