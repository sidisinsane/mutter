package session_test

import (
	"testing"

	"github.com/sidisinsane/mutter/internal/session"
)

func TestNew_DefaultCapacity(t *testing.T) {
	s := session.New(0) // 0 should default to 2
	if s.Size() != 0 {
		t.Errorf("expected empty buffer, got size %d", s.Size())
	}
}

func TestAdd_StoresExecution(t *testing.T) {
	s := session.New(2)
	exec := s.Add("exec-1")
	if exec.ID != "exec-1" {
		t.Errorf("expected ID exec-1, got %q", exec.ID)
	}
	if s.Size() != 1 {
		t.Errorf("expected size 1, got %d", s.Size())
	}
}

func TestAdd_FIFOEvictionWhenFull(t *testing.T) {
	s := session.New(2)
	s.Add("exec-1")
	s.Add("exec-2")
	s.Add("exec-3") // should evict exec-1

	if s.Size() != 2 {
		t.Errorf("expected size 2 after eviction, got %d", s.Size())
	}

	// exec-1 should be gone
	if _, err := s.Get("exec-1"); err == nil {
		t.Error("expected exec-1 to be evicted, but Get succeeded")
	}

	// exec-2 and exec-3 should be present
	if _, err := s.Get("exec-2"); err != nil {
		t.Errorf("expected exec-2 to be present: %v", err)
	}
	if _, err := s.Get("exec-3"); err != nil {
		t.Errorf("expected exec-3 to be present: %v", err)
	}
}

func TestGet_ReturnsErrorForMissingID(t *testing.T) {
	s := session.New(2)
	_, err := s.Get("nonexistent")
	if err == nil {
		t.Error("expected error for missing ID, got nil")
	}
}

func TestMarkExecuted_RemovesFromBuffer(t *testing.T) {
	s := session.New(2)
	s.Add("exec-1")

	if err := s.MarkExecuted("exec-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Size() != 0 {
		t.Errorf("expected empty buffer after MarkExecuted, got size %d", s.Size())
	}
}

func TestMarkExecuted_ErrorForMissingID(t *testing.T) {
	s := session.New(2)
	if err := s.MarkExecuted("nonexistent"); err == nil {
		t.Error("expected error for missing ID, got nil")
	}
}

func TestList_ReturnsOnlyUnexecuted(t *testing.T) {
	s := session.New(2)
	s.Add("exec-1")
	s.Add("exec-2")

	pending := s.List()
	if len(pending) != 2 {
		t.Errorf("expected 2 pending, got %d", len(pending))
	}
}

func TestClear_EmptiesBuffer(t *testing.T) {
	s := session.New(2)
	s.Add("exec-1")
	s.Add("exec-2")
	s.Clear()

	if s.Size() != 0 {
		t.Errorf("expected empty buffer after Clear, got size %d", s.Size())
	}
}

func TestAdd_TwoSlotRecoveryScenario(t *testing.T) {
	// Simulates the accidental prompt scenario:
	// 1. exec-1 is pending (the intended command)
	// 2. user accidentally submits exec-2 (garbage)
	// 3. buffer holds both — exec-1 is still recoverable
	s := session.New(2)
	s.Add("exec-1")
	s.Add("exec-2") // accidental

	if _, err := s.Get("exec-1"); err != nil {
		t.Errorf("exec-1 should still be recoverable: %v", err)
	}

	// Now a third entry arrives — exec-1 is evicted, exec-2 remains
	s.Add("exec-3")
	if _, err := s.Get("exec-1"); err == nil {
		t.Error("exec-1 should have been evicted after third entry")
	}
	if _, err := s.Get("exec-2"); err != nil {
		t.Errorf("exec-2 should still be present: %v", err)
	}
}
