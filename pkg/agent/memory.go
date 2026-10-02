// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/fileutil"
)

// MemoryStore manages persistent memory for the agent.
// - Long-term memory: memory/MEMORY.md
// - Daily notes: memory/YYYYMM/YYYYMMDD.md
type MemoryStore struct {
	workspace  string
	memoryDir  string
	memoryFile string
}

// NewMemoryStore creates a new MemoryStore with the given workspace path.
// It ensures the memory directory exists.
func NewMemoryStore(workspace string) *MemoryStore {
	memoryDir := filepath.Join(workspace, "memory")
	memoryFile := filepath.Join(memoryDir, "MEMORY.md")

	// Ensure memory directory exists
	os.MkdirAll(memoryDir, 0o755)

	return &MemoryStore{
		workspace:  workspace,
		memoryDir:  memoryDir,
		memoryFile: memoryFile,
	}
}

// getTodayFile returns the path to today's daily note file (memory/YYYYMM/YYYYMMDD.md).
func (ms *MemoryStore) getTodayFile() string {
	today := time.Now().Format("20060102") // YYYYMMDD
	monthDir := today[:6]                  // YYYYMM
	filePath := filepath.Join(ms.memoryDir, monthDir, today+".md")
	return filePath
}

// ReadLongTerm reads the long-term memory (MEMORY.md).
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadLongTerm() string {
	if data, err := os.ReadFile(ms.memoryFile); err == nil {
		return string(data)
	}
	return ""
}

// WriteLongTerm writes content to the long-term memory file (MEMORY.md).
func (ms *MemoryStore) WriteLongTerm(content string) error {
	// Use unified atomic write utility with explicit sync for flash storage reliability.
	// Using 0o600 (owner read/write only) for secure default permissions.
	return fileutil.WriteFileAtomic(ms.memoryFile, []byte(content), 0o600)
}

// ReadToday reads today's daily note.
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadToday() string {
	todayFile := ms.getTodayFile()
	if data, err := os.ReadFile(todayFile); err == nil {
		return string(data)
	}
	return ""
}

// AppendToday appends content to today's daily note.
// If the file doesn't exist, it creates a new file with a date header.
func (ms *MemoryStore) AppendToday(content string) error {
	todayFile := ms.getTodayFile()

	// Ensure month directory exists
	monthDir := filepath.Dir(todayFile)
	if err := os.MkdirAll(monthDir, 0o755); err != nil {
		return err
	}

	var existingContent string
	if data, err := os.ReadFile(todayFile); err == nil {
		existingContent = string(data)
	}

	var newContent string
	if existingContent == "" {
		// Add header for new day
		header := fmt.Sprintf("# %s\n\n", time.Now().Format("2006-01-02"))
		newContent = header + content
	} else {
		// Append to existing content
		newContent = existingContent + "\n" + content
	}

	// Use unified atomic write utility with explicit sync for flash storage reliability.
	return fileutil.WriteFileAtomic(todayFile, []byte(newContent), 0o600)
}

// GetRecentDailyNotes returns daily notes from the last N days.
// Contents are joined with "---" separator.
func (ms *MemoryStore) GetRecentDailyNotes(days int) string {
	var sb strings.Builder
	first := true

	for i := range days {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102") // YYYYMMDD
		monthDir := dateStr[:6]            // YYYYMM
		filePath := filepath.Join(ms.memoryDir, monthDir, dateStr+".md")

		if data, err := os.ReadFile(filePath); err == nil {
			if !first {
				sb.WriteString("\n\n---\n\n")
			}
			sb.Write(data)
			first = false
		}
	}

	return sb.String()
}

// GetMemoryContext returns formatted memory context for the agent prompt.
// Includes long-term memory and recent daily notes.
func (ms *MemoryStore) GetMemoryContext() string {
	longTerm := ms.ReadLongTerm()
	recentNotes := ms.GetRecentDailyNotes(3)

	if longTerm == "" && recentNotes == "" {
		return ""
	}

	var sb strings.Builder

	if longTerm != "" {
		sb.WriteString("## Long-term Memory\n\n")
		sb.WriteString(longTerm)
	}

	if recentNotes != "" {
		if longTerm != "" {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString("## Recent Daily Notes\n\n")
		sb.WriteString(recentNotes)
	}

	return sb.String()
}

// --- Curated long-term entries (memory-tool backend) ---
//
// Design reference: NousResearch/hermes-agent tools/memory_tool.py — curated,
// capacity-bounded memory instead of an ever-growing log. Entries appended via
// AddLongTermEntry carry a deterministic HTML-comment marker so they can be
// listed and removed individually; legacy free-form markdown (written before
// this store existed, or by hand) has no marker and is never touched by
// entry-level operations — only whole-file capacity accounting sees it.

// MaxLongTermChars bounds the curated long-term memory file. Roughly 2-3k
// tokens: long enough for dense facts, short enough that a runaway log cannot
// bloat every request's system prompt on a 4GB-class device.
const MaxLongTermChars = 8192

const memEntryMarker = "<!-- mem:"

type MemEntry struct {
	Marker  string // the full HTML-comment marker line
	Stamp   string // timestamp inside the marker
	Content string // entry text (marker line excluded)
}

var longTermMu sync.Mutex

// longTermSize returns the current MEMORY.md size in bytes (0 when absent).
func (ms *MemoryStore) longTermSize() int {
	if data, err := os.ReadFile(ms.memoryFile); err == nil {
		return len(data)
	}
	return 0
}

// ListMemEntries returns the marker-tagged entries in file order.
func (ms *MemoryStore) ListMemEntries() []MemEntry {
	data, err := os.ReadFile(ms.memoryFile)
	if err != nil {
		return nil
	}
	var entries []MemEntry
	for _, block := range strings.Split(string(data), memEntryMarker)[1:] {
		end := strings.Index(block, "-->")
		if end < 0 {
			continue
		}
		entries = append(entries, MemEntry{
			Marker:  memEntryMarker + block[:end] + "-->",
			Stamp:   strings.TrimSpace(block[:end]),
			Content: strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(block[end+3:]), "\n")),
		})
	}
	return entries
}

// AddLongTermEntry appends a marker-tagged entry. It refuses when the file is
// at or over capacity and returns the oldest entries so the caller can prune
// first — curation pressure is the point, an ever-growing log is not memory.
func (ms *MemoryStore) AddLongTermEntry(text string) (string, error) {
	longTermMu.Lock()
	defer longTermMu.Unlock()

	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("empty memory entry")
	}

	current := ms.longTermSize()
	newTotal := current + len(text) + len(memEntryMarker) + 20 + 4 + 2 // marker + stamp + "\n\n- " overhead
	if current > 0 && newTotal > MaxLongTermChars {
		var sb strings.Builder
		fmt.Fprintf(&sb, "memory at capacity (%d/%d chars); prune before adding. Oldest entries:\n", current, MaxLongTermChars)
		entries := ms.ListMemEntries()
		for i, e := range entries {
			if i >= 3 {
				break
			}
			fmt.Fprintf(&sb, "- %s\n", oneLine(e.Content))
		}
		if len(entries) == 0 {
			sb.WriteString("(no marker-tagged entries; the bulk is untagged legacy content — remove it with write_file or edit_file)\n")
		}
		return sb.String(), fmt.Errorf("memory at capacity: %d/%d chars", current, MaxLongTermChars)
	}

	existing := ""
	if data, err := os.ReadFile(ms.memoryFile); err == nil {
		existing = strings.TrimRight(string(data), "\n")
	}
	stamp := time.Now().Format("20060102150405")
	var sb strings.Builder
	sb.WriteString(existing)
	if sb.Len() > 0 {
		sb.WriteString("\n\n")
	}
	fmt.Fprintf(&sb, "%s%s -->\n- %s\n", memEntryMarker, stamp, text)

	if err := ms.WriteLongTerm(sb.String()); err != nil {
		return "", err
	}
	return fmt.Sprintf("saved (%d/%d chars)", ms.longTermSize(), MaxLongTermChars), nil
}

// ReplaceLongTermEntry replaces the unique occurrence of oldText with newText.
// Zero or multiple matches are refused: LLM-supplied anchors must be unambiguous.
func (ms *MemoryStore) ReplaceLongTermEntry(oldText, newText string) (string, error) {
	return ms.editLongTermEntry(oldText, func(content string) string {
		return strings.Replace(content, oldText, newText, 1)
	})
}

// RemoveLongTermEntry deletes the unique occurrence of oldText (and its marker
// line when the match is a marker-tagged entry).
func (ms *MemoryStore) RemoveLongTermEntry(oldText string) (string, error) {
	return ms.editLongTermEntry(oldText, func(content string) string {
		// If the anchor lives inside a marker-tagged entry, drop the marker too.
		for _, e := range ms.ListMemEntries() {
			if strings.Contains(e.Content, oldText) {
				content = strings.Replace(content, e.Marker+"\n", "", 1)
				break
			}
		}
		return strings.Replace(content, oldText, "", 1)
	})
}

func (ms *MemoryStore) editLongTermEntry(oldText string, edit func(string) string) (string, error) {
	longTermMu.Lock()
	defer longTermMu.Unlock()

	oldText = strings.TrimSpace(oldText)
	if oldText == "" {
		return "", fmt.Errorf("empty old_text anchor")
	}
	data, err := os.ReadFile(ms.memoryFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("old_text not found (memory is empty)")
		}
		return "", fmt.Errorf("memory file not readable: %w", err)
	}
	content := string(data)
	switch strings.Count(content, oldText) {
	case 0:
		return "", fmt.Errorf("old_text not found in memory; read memory/MEMORY.md and retry with an exact substring")
	case 1:
		updated := strings.TrimSpace(edit(content))
		if err := ms.WriteLongTerm(updated + "\n"); err != nil {
			return "", err
		}
		return fmt.Sprintf("done (%d/%d chars)", ms.longTermSize(), MaxLongTermChars), nil
	default:
		return "", fmt.Errorf("old_text matches %d places; add more surrounding context to make it unique", strings.Count(content, oldText))
	}
}

// oneLine collapses a string to a single bounded line for capacity notices.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}
