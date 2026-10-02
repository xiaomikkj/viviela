package agent

// History-repair regression tests: these scenarios reproduce the exact
// malformations observed in production (OECT relay_logs, 2026-10-01) that made
// anthropic-protocol upstreams answer HTTP 400 and trip circuit breakers:
//   - empty assistant turns -> "messages.N.content below allowed minimum"
//   - tool results whose assistant(tool_calls) turn vanished -> "unexpected
//     tool_use_id found in tool_result blocks"
//
// Every repair must be DETERMINISTIC: identical input history must always
// render identical wire bytes (prompt-cache prefix stability).

import (
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestSanitizeEmptyAssistantGetsPlaceholder(t *testing.T) {
	history := []providers.Message{
		{Role: "user", Content: "今日头条"},
		{Role: "assistant", Content: ""}, // blank row, no tool calls
		{Role: "user", Content: "继续"},
	}
	got := sanitizeHistoryForProvider(history)
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}
	if got[1].Role != "assistant" {
		t.Fatalf("expected assistant role preserved, got %q", got[1].Role)
	}
	if strings.TrimSpace(got[1].Content) == "" {
		t.Fatal("empty assistant turn was not replaced with placeholder")
	}
	// Determinism: same input -> same bytes.
again := sanitizeHistoryForProvider(history)
	if got[1].Content != again[1].Content {
		t.Fatalf("repair is not deterministic: %q vs %q", got[1].Content, again[1].Content)
	}
}

func TestSanitizeAssistantWithMediaNotPlaceholdered(t *testing.T) {
	history := []providers.Message{
		{Role: "user", Content: "看图"},
		{Role: "assistant", Content: "", Media: []string{"file:///tmp/img.png"}},
	}
	got := sanitizeHistoryForProvider(history)
	if got[1].Content != "" {
		t.Fatalf("media-bearing assistant turn must not be placeholdered, got %q", got[1].Content)
	}
}

func TestSanitizeOrphanToolResultSynthesizesAssistantTurn(t *testing.T) {
	history := []providers.Message{
		{Role: "user", Content: "搜新闻"},
		// The assistant(tool_calls) turn was lost; only the result survived.
		{Role: "tool", Content: "Results for: 今日头条 (via Sogou)", ToolCallID: "call_00_abc"},
	}
	got := sanitizeHistoryForProvider(history)
	if len(got) != 3 {
		t.Fatalf("expected synthesized assistant + tool result, got %d messages", len(got))
	}
	assist := got[1]
	if assist.Role != "assistant" || len(assist.ToolCalls) != 1 {
		t.Fatalf("expected synthesized assistant with one tool call, got %+v", assist)
	}
	if assist.ToolCalls[0].ID != "call_00_abc" {
		t.Fatalf("tool call ID not preserved: %q", assist.ToolCalls[0].ID)
	}
	if assist.ToolCalls[0].Function == nil || assist.ToolCalls[0].Function.Name == "" {
		t.Fatalf("synthesized tool call must carry a Function name for wire round-trips: %+v", assist.ToolCalls[0])
	}
	if got[2].Role != "tool" || got[2].ToolCallID != "call_00_abc" {
		t.Fatalf("tool result must survive after synthesis, got %+v", got[2])
	}
}

func TestSanitizeIncompleteToolBlockSynthesizesMissingResults(t *testing.T) {
	history := []providers.Message{
		{Role: "user", Content: "查两件事"},
		{Role: "assistant", Content: "让我查一下", ToolCalls: []providers.ToolCall{
			{ID: "call_a", Type: "function", Function: &providers.FunctionCall{Name: "web_search", Arguments: "{}"}},
			{ID: "call_b", Type: "function", Function: &providers.FunctionCall{Name: "web_search", Arguments: "{}"}},
		}},
		{Role: "tool", Content: "result for a", ToolCallID: "call_a"},
		// call_b's result was lost (interrupted before recording).
	}
	got := sanitizeHistoryForProvider(history)
	if len(got) != 4 {
		t.Fatalf("expected assistant + 2 tool results after repair, got %d: %+v", len(got), got)
	}
	if got[1].Role != "assistant" || got[1].Content != "让我查一下" {
		t.Fatalf("assistant text must survive the repair, got %+v", got[1])
	}
	found := false
	for _, m := range got[2:] {
		if m.ToolCallID == "call_b" {
			found = true
			if !strings.Contains(m.Content, "Tool result missing") {
				t.Fatalf("synthesized result must explain the interruption, got %q", m.Content)
			}
		}
	}
	if !found {
		t.Fatal("missing tool result was not synthesized")
	}
	// Determinism: repeated repair yields identical ordering.
	again := sanitizeHistoryForProvider(history)
	for i := range got {
		if got[i].ToolCallID != again[i].ToolCallID || got[i].Content != again[i].Content {
			t.Fatalf("repair is not deterministic at %d", i)
		}
	}
}

func TestSanitizeKeepsFullyAnsweredToolBlockIntact(t *testing.T) {
	history := []providers.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "ok", ToolCalls: []providers.ToolCall{
			{ID: "call_x", Type: "function", Function: &providers.FunctionCall{Name: "exec", Arguments: "{}"}},
		}},
		{Role: "tool", Content: "done", ToolCallID: "call_x"},
	}
	got := sanitizeHistoryForProvider(history)
	if len(got) != 3 || got[2].Content != "done" {
		t.Fatalf("healthy block must pass through untouched, got %+v", got)
	}
}

func TestSanitizeDropsToolResultWithoutCallID(t *testing.T) {
	history := []providers.Message{
		{Role: "user", Content: "hi"},
		{Role: "tool", Content: "orphan without id", ToolCallID: ""},
	}
	got := sanitizeHistoryForProvider(history)
	for _, m := range got {
		if m.Role == "tool" {
			t.Fatal("tool result with empty tool_call_id cannot be paired and must be dropped")
		}
	}
}
