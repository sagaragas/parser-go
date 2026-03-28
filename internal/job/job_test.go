package job

import (
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	s := NewStore()
	if s == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestStore_CreateAndGet(t *testing.T) {
	s := NewStore()
	j := &Job{
		ID:        "test-job-1",
		State:     StateQueued,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	s.Create(j)

	retrieved, ok := s.Get("test-job-1")
	if !ok {
		t.Fatal("expected job to be found")
	}
	if retrieved.ID != "test-job-1" {
		t.Errorf("expected ID %q, got %q", "test-job-1", retrieved.ID)
	}
	if retrieved.State != StateQueued {
		t.Errorf("expected state %q, got %q", StateQueued, retrieved.State)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	s := NewStore()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("expected job not to be found")
	}
}

func TestStore_Update(t *testing.T) {
	s := NewStore()
	j := &Job{
		ID:        "test-job-1",
		State:     StateQueued,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.Create(j)

	j.State = StateRunning
	updated := s.Update(j)
	if !updated {
		t.Error("expected update to succeed")
	}

	retrieved, _ := s.Get("test-job-1")
	if retrieved.State != StateRunning {
		t.Errorf("expected state %q after update, got %q", StateRunning, retrieved.State)
	}
}

func TestStore_UpdatePreservesExistingFieldsOnPartialUpdate(t *testing.T) {
	s := NewStore()
	createdAt := time.Unix(1_700_000_000, 0).UTC()
	initial := &Job{
		ID:        "test-job-2",
		State:     StateQueued,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	s.Create(initial)

	updatedAt := createdAt.Add(2 * time.Minute)
	updated := s.Update(&Job{
		ID:        initial.ID,
		State:     StateRunning,
		UpdatedAt: updatedAt,
	})
	if !updated {
		t.Fatal("expected update to succeed")
	}

	retrieved, ok := s.Get(initial.ID)
	if !ok {
		t.Fatal("expected job to be found")
	}
	if !retrieved.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected CreatedAt %s to be preserved, got %s", createdAt, retrieved.CreatedAt)
	}
	if !retrieved.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected UpdatedAt %s, got %s", updatedAt, retrieved.UpdatedAt)
	}
}

func TestStore_Update_NotFound(t *testing.T) {
	s := NewStore()
	j := &Job{
		ID:    "nonexistent",
		State: StateRunning,
	}
	updated := s.Update(j)
	if updated {
		t.Error("expected update to fail for nonexistent job")
	}
}

func TestStore_List(t *testing.T) {
	s := NewStore()

	j1 := &Job{ID: "job-1", State: StateQueued, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	j2 := &Job{ID: "job-2", State: StateRunning, CreatedAt: time.Now(), UpdatedAt: time.Now()}

	s.Create(j1)
	s.Create(j2)

	list := s.List()
	if len(list) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(list))
	}
}

func TestJobState_Values(t *testing.T) {
	states := []State{StateQueued, StateRunning, StateSucceeded, StateFailed, StateExpired}
	expected := []string{"queued", "running", "succeeded", "failed", "expired"}

	for i, s := range states {
		if string(s) != expected[i] {
			t.Errorf("expected state %q, got %q", expected[i], s)
		}
	}
}
