// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

// MemoryTool — curated long-term memory CRUD, model-facing.
//
// Design reference: NousResearch/hermes-agent tools/memory_tool.py. The point
// is CURATION, not accumulation: a single tool with add/replace/remove/list,
// an exact-match anchor discipline (LLMs cannot count lines reliably), and a
// hard capacity limit that refuses writes and names the oldest entries so the
// model must prune before it adds. Entries live in workspace memory/MEMORY.md
// alongside legacy free-form content, which entry operations never touch.

import (
	"context"
	"fmt"
	"strings"

	toolshared "github.com/sipeed/picoclaw/pkg/tools/shared"
)

// MemoryTool implements toolshared.Tool.
type MemoryTool struct {
	store *MemoryStore
}

// NewMemoryTool creates a memory tool bound to the agent workspace store.
func NewMemoryTool(store *MemoryStore) *MemoryTool {
	return &MemoryTool{store: store}
}

// Name implements toolshared.Tool.
func (t *MemoryTool) Name() string { return "memory" }

// Description implements toolshared.Tool.
func (t *MemoryTool) Description() string {
	return "Curated long-term memory (memory/MEMORY.md). " +
		"Store dense, durable FACTS about the user, the environment and hard-won lessons — " +
		"not chat logs or step-by-step narratives. " +
		"Actions: add (new entry), replace (old_text -> new_text), remove (old_text), list. " +
		"old_text must be an EXACT unique substring of the current file. " +
		fmt.Sprintf("Capacity is capped at %d chars: when full, prune the oldest entries first.", MaxLongTermChars)
}

// Parameters implements toolshared.Tool.
func (t *MemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{"add", "replace", "remove", "list"},
			},
			"text":     map[string]any{"type": "string", "description": "Entry text (action=add)"},
			"old_text": map[string]any{"type": "string", "description": "Exact unique substring of the current entry (action=replace/remove)"},
			"new_text": map[string]any{"type": "string", "description": "Replacement text (action=replace)"},
		},
		"required": []string{"action"},
	}
}

// PromptMetadata implements toolshared.PromptMetadataProvider.
func (t *MemoryTool) PromptMetadata() toolshared.PromptMetadata {
	return toolshared.PromptMetadata{
		Layer:  toolshared.ToolPromptLayerCapability,
		Slot:   toolshared.ToolPromptSlotTooling,
		Source: toolshared.ToolPromptSourceRegistry,
	}
}

// Execute implements toolshared.Tool.
func (t *MemoryTool) Execute(ctx context.Context, args map[string]any) *toolshared.ToolResult {
	action, _ := args["action"].(string)
	action = strings.TrimSpace(action)

	var forLLM string
	var err error

	switch action {
	case "add":
		text, _ := args["text"].(string)
		forLLM, err = t.store.AddLongTermEntry(text)
	case "replace":
		oldText, _ := args["old_text"].(string)
		newText, _ := args["new_text"].(string)
		if strings.TrimSpace(newText) == "" {
			forLLM, err = "", fmt.Errorf("new_text is required for replace")
			break
		}
		forLLM, err = t.store.ReplaceLongTermEntry(oldText, newText)
	case "remove":
		oldText, _ := args["old_text"].(string)
		forLLM, err = t.store.RemoveLongTermEntry(oldText)
	case "list":
		entries := t.store.ListMemEntries()
		if len(entries) == 0 {
			forLLM = "no marker-tagged entries (file may contain legacy untagged content)"
		} else {
			var sb strings.Builder
			for i, e := range entries {
				fmt.Fprintf(&sb, "%d. %s\n", i+1, oneLine(e.Content))
			}
			forLLM = fmt.Sprintf("%d entries (%d/%d chars):\n%s", len(entries), t.store.longTermSize(), MaxLongTermChars, sb.String())
		}
	default:
		forLLM, err = "", fmt.Errorf("unknown action %q; use add, replace, remove or list", action)
	}

	if err != nil {
		msg := fmt.Sprintf("[MEMORY_ERROR] %v", err)
		// Capacity refusals carry the oldest-entries pruning guidance in
		// forLLM — the model needs it to recover in the next turn.
		if forLLM != "" {
			msg += "\n" + forLLM
		}
		return &toolshared.ToolResult{
			ForLLM:  msg,
			IsError: true,
		}
	}
	return &toolshared.ToolResult{ForLLM: forLLM}
}
