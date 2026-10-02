// PicoClaw - Ultra-lightweight personal AI agent
// Memory manager inspired by NousResearch/hermes-agent.
package agent

import (
	"strings"
	"sync"
)

// MemoryManager adapts the built-in MemoryStore to a Hermes-style lifecycle:
//   - BuildMemoryContextBlock: assemble recall text for system prompt / user turn
//   - SyncTurn: mirror a completed turn into durable memory
//   - QueuePrefetch: schedule background recall for next turn
//   - Shutdown: flush pending work at session end
//
// The built-in store is always the default backend; external providers are not
// implemented yet to keep the footprint small.
type MemoryManager struct {
	store *MemoryStore

	// Simple single-flight for prefetch so concurrent turns do not spawn
	// duplicate recall work.
	prefetchMu sync.Mutex
	prefetch   string
}

func NewMemoryManager(store *MemoryStore) *MemoryManager {
	return &MemoryManager{store: store}
}

// BuildMemoryContextBlock returns the recall block for the upcoming turn.
// It favors a freshly prefetched recall when available; otherwise it falls
// back to the curated long-term memory plus recent daily notes.
func (m *MemoryManager) BuildMemoryContextBlock() string {
	if m == nil || m.store == nil {
		return ""
	}

	m.prefetchMu.Lock()
	prefetch := m.prefetch
	m.prefetchMu.Unlock()

	if prefetch != "" {
		return prefetch
	}

	return m.store.GetMemoryContext()
}

// SyncTurn mirrors a completed turn into memory.
// The implementation is intentionally lightweight: durable daily notes are
// appended asynchronously by the caller when needed.
func (m *MemoryManager) SyncTurn(userContent, assistantContent string) {
	if m == nil || m.store == nil {
		return
	}

	userText := strings.TrimSpace(userContent)
	assistantText := strings.TrimSpace(assistantContent)
	if userText == "" && assistantText == "" {
		return
	}

	var sb strings.Builder
	if userText != "" {
		sb.WriteString("## User\n\n")
		sb.WriteString(userText)
	}
	if assistantText != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("## Assistant\n\n")
		sb.WriteString(assistantText)
	}

	_ = m.store.AppendToday(sb.String())
}

// QueuePrefetch stores a recall result for the next turn.
// A later call to BuildMemoryContextBlock returns this value instead of the
// generic memory context.
func (m *MemoryManager) QueuePrefetch(text string) {
	if m == nil {
		return
	}

	m.prefetchMu.Lock()
	m.prefetch = text
	m.prefetchMu.Unlock()
}

// Shutdown flushes pending memory work.
func (m *MemoryManager) Shutdown() {
	if m == nil {
		return
	}

	m.prefetchMu.Lock()
	m.prefetch = ""
	m.prefetchMu.Unlock()
}
