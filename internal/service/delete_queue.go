// Package service contains background services used by the URL server.
package service

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

const deleteQueueSize = 1024

const (
	deleteBatchSize     = 100
	deleteFlushInterval = 50 * time.Millisecond
)

var (
	// ErrDeleteQueueClosed is returned when a deletion task is submitted after Close.
	ErrDeleteQueueClosed = errors.New("delete queue closed")
	// ErrDeleteQueueFull is returned when the buffered deletion queue has no capacity.
	ErrDeleteQueueFull = errors.New("delete queue full")
)

// DeleteStore is the repository operation needed by DeleteQueue.
type DeleteStore interface {
	// DeleteUserURLs soft-deletes URLs belonging to userID.
	DeleteUserURLs(ids []string, userID string) error
}

// DeleteTask groups short IDs that should be deleted for one user.
type DeleteTask struct {
	// UserID identifies the owner of the short URLs.
	UserID string
	// ShortIDs are the identifiers to soft-delete.
	ShortIDs []string
}

// generate:reset
type deleteBatch struct {
	groups map[string][]string
	count  int
}

// DeleteQueue batches user-scoped deletion requests for background processing.
type DeleteQueue struct {
	store      DeleteStore
	logger     *zap.Logger
	tasks      chan DeleteTask
	stop       chan struct{}
	workerDone chan struct{}
	mu         sync.RWMutex
	closed     bool
	closeErr   error
}

// NewDeleteQueue starts a worker that combines queued deletion tasks into batches.
func NewDeleteQueue(store DeleteStore, logger *zap.Logger) *DeleteQueue {
	if logger == nil {
		logger = zap.NewNop()
	}
	q := &DeleteQueue{
		store:      store,
		logger:     logger,
		tasks:      make(chan DeleteTask, deleteQueueSize),
		stop:       make(chan struct{}),
		workerDone: make(chan struct{}),
	}
	go q.run()

	return q
}

// EnqueueDelete copies and queues IDs without waiting for repository deletion.
func (q *DeleteQueue) EnqueueDelete(userID string, ids []string) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed {
		return ErrDeleteQueueClosed
	}

	task := DeleteTask{
		UserID:   userID,
		ShortIDs: append([]string(nil), ids...),
	}

	select {
	case q.tasks <- task:
		return nil
	default:
		return ErrDeleteQueueFull
	}
}

// Close stops accepting tasks, flushes queued deletions, and returns the final flush error.
func (q *DeleteQueue) Close() error {
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.stop)
	}
	q.mu.Unlock()

	<-q.workerDone
	return q.closeErr
}

func (q *DeleteQueue) run() {
	defer close(q.workerDone)

	ticker := time.NewTicker(deleteFlushInterval)
	defer ticker.Stop()

	batch := deleteBatch{groups: make(map[string][]string)}
	flush := func() error {
		var errs []error
		for userID, ids := range batch.groups {
			if err := q.store.DeleteUserURLs(ids, userID); err != nil {
				q.logger.Error("delete URL batch failed",
					zap.String("user_id", userID),
					zap.Int("url_count", len(ids)),
					zap.Error(err),
				)
				errs = append(errs, fmt.Errorf("delete URLs for user %q: %w", userID, err))
				continue
			}

			delete(batch.groups, userID)
			batch.count -= len(ids)
		}
		if len(errs) == 0 {
			batch.Reset()
		}
		return errors.Join(errs...)
	}
	add := func(task DeleteTask) {
		batch.groups[task.UserID] = append(batch.groups[task.UserID], task.ShortIDs...)
		batch.count += len(task.ShortIDs)
	}
	drain := func() {
		for {
			select {
			case task := <-q.tasks:
				add(task)
			default:
				return
			}
		}
	}

	for {
		select {
		case <-q.stop:
			drain()
			q.closeErr = flush()
			return
		case task := <-q.tasks:
			add(task)
			if batch.count >= deleteBatchSize {
				_ = flush()
			}
		case <-ticker.C:
			if batch.count > 0 {
				_ = flush()
			}
		}
	}
}
