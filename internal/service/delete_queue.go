// Package service contains background services used by the URL server.
package service

import (
	"errors"
	"sync"
	"time"
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

// DeleteQueue batches user-scoped deletion requests for background processing.
type DeleteQueue struct {
	store  DeleteStore
	tasks  chan DeleteTask
	done   chan struct{}
	mu     sync.RWMutex
	closed bool
}

// NewDeleteQueue starts a worker that combines queued deletion tasks into batches.
func NewDeleteQueue(store DeleteStore) *DeleteQueue {
	q := &DeleteQueue{
		store: store,
		tasks: make(chan DeleteTask, deleteQueueSize),
		done:  make(chan struct{}),
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

// Close stops the worker after it flushes tasks already in the queue.
func (q *DeleteQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.closed {
		q.closed = true
		close(q.done)
	}
}

func (q *DeleteQueue) run() {
	ticker := time.NewTicker(deleteFlushInterval)
	defer ticker.Stop()

	groups := make(map[string][]string)
	count := 0
	flush := func() {
		for userID, ids := range groups {
			_ = q.store.DeleteUserURLs(ids, userID)
		}
		groups = make(map[string][]string)
		count = 0
	}
	add := func(task DeleteTask) {
		groups[task.UserID] = append(groups[task.UserID], task.ShortIDs...)
		count += len(task.ShortIDs)
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
		case <-q.done:
			drain()
			flush()
			return
		case task := <-q.tasks:
			add(task)
			if count >= deleteBatchSize {
				flush()
			}
		case <-ticker.C:
			if count > 0 {
				flush()
			}
		}
	}
}
