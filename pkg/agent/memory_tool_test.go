package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func newTestMemoryTool(t *testing.T) *MemoryTool {
	t.Helper()
	return NewMemoryTool(NewMemoryStore(t.TempDir()))
}

func TestMemoryToolAddListRoundtrip(t *testing.T) {
	tool := newTestMemoryTool(t)
	ctx := context.Background()

	r := tool.Execute(ctx, map[string]any{"action": "add", "text": "user prefers concise answers"})
	if r.IsError {
		t.Fatalf("add failed: %s", r.ForLLM)
	}
	r = tool.Execute(ctx, map[string]any{"action": "add", "text": "OECT gateway restarts drop weixin session"})
	if r.IsError {
		t.Fatalf("add failed: %s", r.ForLLM)
	}

	r = tool.Execute(ctx, map[string]any{"action": "list"})
	if r.IsError || !strings.Contains(r.ForLLM, "2 entries") {
		t.Fatalf("list should report 2 entries, got: %s", r.ForLLM)
	}
	if !strings.Contains(r.ForLLM, "prefers concise") || !strings.Contains(r.ForLLM, "weixin session") {
		t.Fatalf("list lost entries: %s", r.ForLLM)
	}
}

func TestMemoryToolReplaceRemoveWithExactAnchors(t *testing.T) {
	tool := newTestMemoryTool(t)
	ctx := context.Background()

	tool.Execute(ctx, map[string]any{"action": "add", "text": "backup target is /dev/sda"})

	r := tool.Execute(ctx, map[string]any{"action": "replace", "old_text": "/dev/sda", "new_text": "/dev/sdb"})
	if r.IsError {
		t.Fatalf("replace failed: %s", r.ForLLM)
	}
	r = tool.Execute(ctx, map[string]any{"action": "list"})
	if !strings.Contains(r.ForLLM, "/dev/sdb") || strings.Contains(r.ForLLM, "/dev/sda") {
		t.Fatalf("replace did not land: %s", r.ForLLM)
	}

	r = tool.Execute(ctx, map[string]any{"action": "remove", "old_text": "backup target is /dev/sdb"})
	if r.IsError {
		t.Fatalf("remove failed: %s", r.ForLLM)
	}
	r = tool.Execute(ctx, map[string]any{"action": "list"})
	if strings.Contains(r.ForLLM, "backup target") {
		t.Fatalf("remove did not land: %s", r.ForLLM)
	}
}

func TestMemoryToolRejectsAmbiguousAnchor(t *testing.T) {
	tool := newTestMemoryTool(t)
	ctx := context.Background()

	tool.Execute(ctx, map[string]any{"action": "add", "text": "alpha one"})
	tool.Execute(ctx, map[string]any{"action": "add", "text": "alpha two"})

	r := tool.Execute(ctx, map[string]any{"action": "remove", "old_text": "alpha"})
	if !r.IsError || !strings.Contains(r.ForLLM, "matches 2 places") {
		t.Fatalf("ambiguous anchor must be refused, got: %s", r.ForLLM)
	}
}

func TestMemoryToolRejectsMissingAnchor(t *testing.T) {
	tool := newTestMemoryTool(t)
	ctx := context.Background()

	r := tool.Execute(ctx, map[string]any{"action": "remove", "old_text": "does not exist anywhere"})
	if !r.IsError || !strings.Contains(r.ForLLM, "not found") {
		t.Fatalf("missing anchor must be refused, got: %s", r.ForLLM)
	}
}

func TestMemoryToolCapacityForcesPruning(t *testing.T) {
	tool := newTestMemoryTool(t)
	ctx := context.Background()

	// Fill until capacity refuses, then verify the refusal carries guidance.
	filler := strings.Repeat("x", 1000)
	refused := false
	for i := 0; i < 20; i++ {
		r := tool.Execute(ctx, map[string]any{"action": "add", "text": fmt.Sprintf("%s #%d", filler, i)})
		if r.IsError {
			if !strings.Contains(r.ForLLM, "at capacity") || !strings.Contains(r.ForLLM, "Oldest entries") {
				t.Fatalf("refusal must carry pruning guidance, got: %s", r.ForLLM)
			}
			refused = true
			break
		}
	}
	if !refused {
		t.Fatal("capacity overflow was never refused")
	}

	// Pruning one entry makes room again.
	r := tool.Execute(ctx, map[string]any{"action": "remove", "old_text": filler + " #0"})
	if r.IsError {
		t.Fatalf("prune failed: %s", r.ForLLM)
	}
	r = tool.Execute(ctx, map[string]any{"action": "add", "text": "fits now"})
	if r.IsError {
		t.Fatalf("add after prune failed: %s", r.ForLLM)
	}
}

func TestMemoryToolLegacyContentUntouchedByEntryOps(t *testing.T) {
	tool := newTestMemoryTool(t)
	ctx := context.Background()

	// Simulate the pre-existing free-form MEMORY.md (production had 15KB of it).
	legacy := "# Long-term Memory\n\n## PicoClaw 部署要点（2026-09-27 探查）\n\n- 版本 0.3.1，主机 OECT。\n"
	if err := tool.store.WriteLongTerm(legacy); err != nil {
		t.Fatal(err)
	}

	tool.Execute(ctx, map[string]any{"action": "add", "text": "new curated fact"})

	data := tool.store.ReadLongTerm()
	if !strings.Contains(data, "PicoClaw 部署要点") {
		t.Fatal("legacy content must be preserved verbatim")
	}
	if !strings.Contains(data, "new curated fact") {
		t.Fatal("new entry missing")
	}

	// And the legacy section remains editable through exact anchors.
	r := tool.Execute(ctx, map[string]any{"action": "replace", "old_text": "主机 OECT", "new_text": "主机 OECT (ARM64)"})
	if r.IsError {
		t.Fatalf("legacy edit failed: %s", r.ForLLM)
	}
}
