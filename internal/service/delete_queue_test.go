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
