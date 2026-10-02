// PicoClaw - Ultra-lightweight personal AI agent
// Memory manager tests.
package agent

import (
	"testing"
	"time"
)

func TestMemoryManager_BuildMemoryContextBlock(t *testing.T) {
	// Create a temporary memory store
	tmpDir := t.TempDir()
	store := NewMemoryStore(tmpDir)

	// Add some test memory
	store.RememberFact("test fact 1")
	store.RememberFact("test fact 2")

	mm := NewMemoryManager(store)

	// Test fallback to store
	ctx := mm.BuildMemoryContextBlock()
	if ctx == "" {
		t.Error("BuildMemoryContextBlock returned empty string")
	}

	// Test prefetched value
	mm.QueuePrefetchText("prefetched content")
	ctx = mm.BuildMemoryContextBlock()
	if ctx != "prefetched content" {
		t.Errorf("BuildMemoryContextBlock = %q, want %q", ctx, "prefetched content")
	}
}

func TestMemoryManager_SyncTurn(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMemoryStore(tmpDir)
	mm := NewMemoryManager(store)

	// Test sync with both user and assistant content
	mm.SyncTurn("user question", "assistant answer")

	// Verify daily note was created
	notes := store.LoadTodayNotes()
	if notes == "" {
		t.Error("SyncTurn did not create daily note")
	}
	if !contains(notes, "user question") {
		t.Errorf("Daily note missing user content: %q", notes)
	}
	if !contains(notes, "assistant answer") {
		t.Errorf("Daily note missing assistant content: %q", notes)
	}
}

func TestMemoryManager_SyncTurnEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMemoryStore(tmpDir)
	mm := NewMemoryManager(store)

	// Test sync with empty content should not create note
	mm.SyncTurn("", "")
	notes := store.LoadTodayNotes()
	if notes != "" {
		t.Errorf("SyncTurn created note for empty content: %q", notes)
	}
}

func TestMemoryManager_QueuePrefetch(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMemoryStore(tmpDir)
	store.RememberFact("test fact")
	mm := NewMemoryManager(store)

	// Start async prefetch
	mm.QueuePrefetch()

	// Wait a bit for goroutine to complete
	time.Sleep(100 * time.Millisecond)

	// Should return prefetched content
	ctx := mm.BuildMemoryContextBlock()
	if ctx == "" {
		t.Error("QueuePrefetch did not populate prefetch value")
	}
}

func TestMemoryManager_QueuePrefetchConcurrent(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMemoryStore(tmpDir)
	mm := NewMemoryManager(store)

	// Multiple concurrent calls should be safe
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			mm.QueuePrefetch()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or deadlock
}

func TestMemoryManager_Shutdown(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMemoryStore(tmpDir)
	mm := NewMemoryManager(store)

	mm.QueuePrefetchText("test")
	mm.Shutdown()

	ctx := mm.BuildMemoryContextBlock()
	if ctx != "" && ctx != store.GetMemoryContext() {
		t.Errorf("After Shutdown, BuildMemoryContextBlock = %q, want empty or store fallback", ctx)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
