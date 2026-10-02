// PicoClaw - Ultra-lightweight personal AI agent
// Memory manager inspired by NousResearch/hermes-agent.
package agent

import (
	"strings"
	"sync"
	"sync/atomic"
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

	// Prefetch state: stores the prefetched memory context for the next turn.
	// Uses atomic.Value for lock-free reads.
	prefetch atomic.Value // stores string

	// Queue control: only one prefetch goroutine runs at a time.
	prefetchMu sync.Mutex
	prefetchIn bool
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

	// Check for prefetched result first (lock-free).
	if val := m.prefetch.Load(); val != nil {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}

	// Fallback to direct memory store access.
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

// QueuePrefetch starts an async prefetch of the memory context for the next turn.
// The prefetched result will be returned by BuildMemoryContextBlock.
// This is non-blocking and safe to call multiple times (only one runs at a time).
func (m *MemoryManager) QueuePrefetch() {
	if m == nil || m.store == nil {
		return
	}

	m.prefetchMu.Lock()
	if m.prefetchIn {
		m.prefetchMu.Unlock()
		return
	}
	m.prefetchIn = true
	m.prefetchMu.Unlock()

	go func() {
		defer func() {
			m.prefetchMu.Lock()
			m.prefetchIn = false
			m.prefetchMu.Unlock()
		}()

		m.prefetch.Store(m.store.GetMemoryContext())
	}()
}

// QueuePrefetchText stores a pre-computed recall result for the next turn.
// A later call to BuildMemoryContextBlock returns this value instead of the
// generic memory context.
func (m *MemoryManager) QueuePrefetchText(text string) {
	if m == nil {
		return
	}
	m.prefetch.Store(text)
}

// Shutdown flushes pending memory work.
func (m *MemoryManager) Shutdown() {
	if m == nil {
		return
	}
	m.prefetch.Store("")
}
