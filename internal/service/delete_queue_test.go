package service

import (
	"reflect"
	"testing"
	"time"
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

func (s *fakeDeleteStore) DeleteUserURLs(ids []string, userID string) error {
	s.calls <- DeleteTask{
		UserID:   userID,
		ShortIDs: append([]string(nil), ids...),
	}

	return nil
}
