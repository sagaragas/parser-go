package job

import (
	"sync"
	"time"
)

// State represents the job lifecycle state.
type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateExpired   State = "expired"
)

// Job represents an analysis job.
type Job struct {
	ID        string    `json:"id"`
	State     State     `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Error     *Error    `json:"error,omitempty"`
}

// Error represents a safe terminal error.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Store holds jobs in memory.
// This is a placeholder that will be expanded with persistence.
type Store struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewStore creates a new job store.
func NewStore() *Store {
	return &Store{
		jobs: make(map[string]*Job),
	}
}

// Get retrieves a job by ID.
func (s *Store) Get(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	return j, ok
}

// Create adds a new job.
func (s *Store) Create(j *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.ID] = j
}

// Update modifies an existing job.
func (s *Store) Update(j *Job) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.jobs[j.ID]
	if !ok {
		return false
	}

	updated := *existing
	if j.State != "" {
		updated.State = j.State
	}
	if !j.CreatedAt.IsZero() {
		updated.CreatedAt = j.CreatedAt
	}
	if !j.UpdatedAt.IsZero() {
		updated.UpdatedAt = j.UpdatedAt
	}
	updated.Error = j.Error

	s.jobs[j.ID] = &updated
	return true
}

// List returns all jobs.
func (s *Store) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, j)
	}
	return result
}
