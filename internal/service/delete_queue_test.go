package service

import (
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/true18/shortener/internal/repository"
)

func TestDeleteQueueDeletesInBackground(t *testing.T) {
	store := &fakeDeleteStore{
		calls: make(chan DeleteTask, 1),
	}
	queue := NewDeleteQueue(store)
	defer queue.Close()

	ids := []string{"abc12345"}
	if err := queue.EnqueueDelete("user-1", ids); err != nil {
		t.Fatalf("EnqueueDelete returned error: %v", err)
	}
	ids[0] = "changed"

	select {
	case task := <-store.calls:
		if task.UserID != "user-1" {
			t.Fatalf("user id = %q, want %q", task.UserID, "user-1")
		}
		if !reflect.DeepEqual(task.ShortIDs, []string{"abc12345"}) {
			t.Fatalf("short ids = %+v, want [abc12345]", task.ShortIDs)
		}
	case <-time.After(time.Second):
		t.Fatal("delete task was not processed")
	}
}

func TestDeleteQueueBatchesByUser(t *testing.T) {
	store := &fakeDeleteStore{calls: make(chan DeleteTask, 2)}
	queue := NewDeleteQueue(store)
	defer queue.Close()

	if err := queue.EnqueueDelete("user-a", []string{"a1"}); err != nil {
		t.Fatalf("EnqueueDelete returned error: %v", err)
	}
	if err := queue.EnqueueDelete("user-b", []string{"b1"}); err != nil {
		t.Fatalf("EnqueueDelete returned error: %v", err)
	}
	if err := queue.EnqueueDelete("user-a", []string{"a2"}); err != nil {
		t.Fatalf("EnqueueDelete returned error: %v", err)
	}

	calls := []DeleteTask{receiveDeleteTask(t, store.calls), receiveDeleteTask(t, store.calls)}
	for _, call := range calls {
		switch call.UserID {
		case "user-a":
			if !reflect.DeepEqual(call.ShortIDs, []string{"a1", "a2"}) {
				t.Fatalf("user-a ids = %+v, want [a1 a2]", call.ShortIDs)
			}
		case "user-b":
			if !reflect.DeepEqual(call.ShortIDs, []string{"b1"}) {
				t.Fatalf("user-b ids = %+v, want [b1]", call.ShortIDs)
			}
		default:
			t.Fatalf("unexpected user %q", call.UserID)
		}
	}
}

func TestDeleteQueueCloseFlushesPendingTasks(t *testing.T) {
	store := &fakeDeleteStore{calls: make(chan DeleteTask, 1)}
	queue := NewDeleteQueue(store)

	if err := queue.EnqueueDelete("user-1", []string{"abc"}); err != nil {
		t.Fatalf("EnqueueDelete returned error: %v", err)
	}
	queue.Close()

	call := receiveDeleteTask(t, store.calls)
	if !reflect.DeepEqual(call.ShortIDs, []string{"abc"}) {
		t.Fatalf("short ids = %+v, want [abc]", call.ShortIDs)
	}
}

func TestDeleteQueueCloseWaitsForWorker(t *testing.T) {
	store := &blockingDeleteStore{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	queue := NewDeleteQueue(store)
	if err := queue.EnqueueDelete("user-1", []string{"abc"}); err != nil {
		t.Fatalf("EnqueueDelete returned error: %v", err)
	}

	closed := make(chan struct{})
	go func() {
		queue.Close()
		close(closed)
	}()

	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("delete worker did not start")
	}
	select {
	case <-closed:
		t.Fatal("Close returned before repository deletion completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(store.release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not return after repository deletion completed")
	}

	queue.Close()
	if err := queue.EnqueueDelete("user-1", []string{"next"}); !errors.Is(err, ErrDeleteQueueClosed) {
		t.Fatalf("EnqueueDelete after Close error = %v", err)
	}
}

func TestDeleteQueueClosePersistsLastFileBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.json")
	store, err := repository.NewFile(path)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	id, err := store.Save("https://example.com", "user-1")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	queue := NewDeleteQueue(store)
	if err := queue.EnqueueDelete("user-1", []string{id}); err != nil {
		t.Fatalf("EnqueueDelete: %v", err)
	}
	queue.Close()

	reopened, err := repository.NewFile(path)
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	if _, err := reopened.Find(id); !errors.Is(err, repository.ErrDeleted) {
		t.Fatalf("Find error = %v, want %v", err, repository.ErrDeleted)
	}
}

func TestDeleteBatchReset(t *testing.T) {
	batch := deleteBatch{
		groups: map[string][]string{"user": {"one", "two"}},
		count:  2,
	}
	batch.Reset()
	if batch.count != 0 {
		t.Fatalf("count = %d, want 0", batch.count)
	}
	if batch.groups == nil || len(batch.groups) != 0 {
		t.Fatalf("groups = %#v, want an empty non-nil map", batch.groups)
	}
	batch.Reset()
}

func receiveDeleteTask(t *testing.T, calls <-chan DeleteTask) DeleteTask {
	t.Helper()

	select {
	case task := <-calls:
		return task
	case <-time.After(time.Second):
		t.Fatal("delete task was not processed")
		return DeleteTask{}
	}
}

type fakeDeleteStore struct {
	calls chan DeleteTask
}

type blockingDeleteStore struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingDeleteStore) DeleteUserURLs([]string, string) error {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return nil
}

func (s *fakeDeleteStore) DeleteUserURLs(ids []string, userID string) error {
	s.calls <- DeleteTask{
		UserID:   userID,
		ShortIDs: append([]string(nil), ids...),
	}

	return nil
}
