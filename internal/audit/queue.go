package audit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	queueCapacity   = 256
	deliveryTimeout = 5 * time.Second
	shutdownTimeout = 10 * time.Second
)

var (
	// ErrQueueFull indicates that an audit event was dropped because the queue is full.
	ErrQueueFull = errors.New("audit queue full")
	// ErrQueueClosed indicates that the audit queue no longer accepts events.
	ErrQueueClosed = errors.New("audit queue closed")
)

// Queue delivers accepted events in the background using a bounded buffer.
// Delivery is best effort: events can be dropped if the buffer is full or shutdown times out.
type Queue struct {
	observer Observer
	logger   *zap.Logger
	events   chan Event
	stop     chan struct{}
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.RWMutex
	closed   bool
	closeErr error
}

// NewQueue starts one worker for the supplied observer. It returns nil when observer is nil.
func NewQueue(observer Observer, logger *zap.Logger) *Queue {
	if observer == nil {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		observer: observer,
		logger:   logger,
		events:   make(chan Event, queueCapacity),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}
	go q.run()

	return q
}

// Notify queues event without waiting for observer delivery or using the request context.
func (q *Queue) Notify(_ context.Context, event Event) error {
	if q == nil {
		return nil
	}

	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrQueueClosed
	}

	select {
	case q.events <- event:
		return nil
	default:
		return ErrQueueFull
	}
}

// Close stops accepting events, drains queued work within a deadline, and waits for the worker.
func (q *Queue) Close() error {
	if q == nil {
		return nil
	}

	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.stop)
	}
	q.mu.Unlock()

	<-q.done
	return q.closeErr
}

func (q *Queue) run() {
	defer close(q.done)
	defer q.cancel()

	for {
		select {
		case <-q.stop:
			q.drain()
			return
		default:
		}

		select {
		case <-q.stop:
			q.drain()
			return
		case event := <-q.events:
			q.deliver(q.ctx, event)
		}
	}
}

func (q *Queue) drain() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	for {
		if ctx.Err() != nil {
			q.closeErr = fmt.Errorf("audit shutdown timed out with %d events pending", len(q.events))
			q.logger.Error("audit queue shutdown timed out", zap.Int("pending", len(q.events)))
			return
		}

		select {
		case event := <-q.events:
			q.deliver(ctx, event)
		default:
			return
		}
	}
}

func (q *Queue) deliver(parent context.Context, event Event) {
	ctx, cancel := context.WithTimeout(parent, deliveryTimeout)
	defer cancel()

	if err := q.observer.Notify(ctx, event); err != nil {
		q.logger.Error("audit event delivery failed", zap.Error(err))
	}
}
