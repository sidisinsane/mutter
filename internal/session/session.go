// Package session manages the pending execution buffer for mutter.
// The buffer preserves unexecuted execution IDs for recovery purposes.
// Executed commands are never stored. When the buffer is full, the oldest
// entry is dropped (FIFO).
package session

import (
	"fmt"
	"sync"
	"time"
)

// Execution represents a pending execution in the buffer.
type Execution struct {
	// ID is the unique identifier for this execution.
	ID string
	// CreatedAt is the timestamp when this execution was added to the buffer.
	CreatedAt time.Time
	// Executed indicates whether this execution has been run.
	// Only unexecuted executions are stored in the buffer.
	Executed bool
}

// Session manages the pending execution buffer.
type Session struct {
	// buffer is the FIFO queue of pending executions.
	buffer []*Execution
	// capacity is the maximum number of unexecuted executions to preserve.
	capacity int
	// mu protects concurrent access to the buffer.
	mu sync.RWMutex
}

// New creates a new Session with the given buffer capacity.
func New(capacity int) *Session {
	if capacity <= 0 {
		capacity = 2 // Default capacity
	}
	return &Session{
		buffer:  make([]*Execution, 0, capacity),
		capacity: capacity,
	}
}

// Add adds a new execution to the buffer. If the buffer is full,
// the oldest entry is dropped (FIFO).
func (s *Session) Add(id string) *Execution {
	s.mu.Lock()
	defer s.mu.Unlock()

	exec := &Execution{
		ID:        id,
		CreatedAt: time.Now(),
		Executed:  false,
	}

	// If buffer is full, remove the oldest entry
	if len(s.buffer) >= s.capacity {
		s.buffer = s.buffer[1:]
	}

	s.buffer = append(s.buffer, exec)
	return exec
}

// Get retrieves an execution by ID from the buffer.
func (s *Session) Get(id string) (*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, exec := range s.buffer {
		if exec.ID == id {
			return exec, nil
		}
	}

	return nil, fmt.Errorf("execution %s not found in buffer", id)
}

// MarkExecuted marks an execution as executed and removes it from the buffer.
func (s *Session) MarkExecuted(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, exec := range s.buffer {
		if exec.ID == id {
			// Remove from buffer
			s.buffer = append(s.buffer[:i], s.buffer[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("execution %s not found in buffer", id)
}

// List returns all pending (unexecuted) executions in the buffer.
func (s *Session) List() []*Execution {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var pending []*Execution
	for _, exec := range s.buffer {
		if !exec.Executed {
			pending = append(pending, exec)
		}
	}

	return pending
}

// Clear removes all executions from the buffer.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buffer = s.buffer[:0]
}

// Size returns the current number of executions in the buffer.
func (s *Session) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.buffer)
}
