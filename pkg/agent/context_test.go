package agent

import (
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func msg(role, content string) providers.Message {
	return providers.Message{Role: role, Content: content}
}

func assistantWithTools(toolIDs ...string) providers.Message {
	calls := make([]providers.ToolCall, len(toolIDs))
	for i, id := range toolIDs {
		calls[i] = providers.ToolCall{ID: id, Type: "function"}
	}
	return providers.Message{Role: "assistant", ToolCalls: calls}
}

func toolResult(id string) providers.Message {
	return providers.Message{Role: "tool", Content: "result", ToolCallID: id}
}

func TestSanitizeHistoryForProvider_EmptyHistory(t *testing.T) {
	result := sanitizeHistoryForProvider(nil)
	if len(result) != 0 {
		t.Fatalf("expected empty, got %d messages", len(result))
	}

	result = sanitizeHistoryForProvider([]providers.Message{})
	if len(result) != 0 {
		t.Fatalf("expected empty, got %d messages", len(result))
	}
}

func TestSanitizeHistoryForProvider_SingleToolCall(t *testing.T) {
	history := []providers.Message{
		msg("user", "hello"),
		assistantWithTools("A"),
		toolResult("A"),
		msg("assistant", "done"),
	}

	result := sanitizeHistoryForProvider(history)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}
	assertRoles(t, result, "user", "assistant", "tool", "assistant")
}

func TestSanitizeHistoryForProvider_MultiToolCalls(t *testing.T) {
	history := []providers.Message{
		msg("user", "do two things"),
		assistantWithTools("A", "B"),
		toolResult("A"),
		toolResult("B"),
		msg("assistant", "both done"),
	}

	result := sanitizeHistoryForProvider(history)
	if len(result) != 5 {
		t.Fatalf("expected 5 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant", "tool", "tool", "assistant")
}

func TestSanitizeHistoryForProvider_AssistantToolCallAfterPlainAssistant(t *testing.T) {
	history := []providers.Message{
		msg("user", "hi"),
		msg("assistant", "thinking"),
		assistantWithTools("A"),
		toolResult("A"),
	}

	result := sanitizeHistoryForProvider(history)
	// The tool-call turn after a plain assistant is dropped (invalid
	// predecessor), but its result now survives via orphan synthesis so the
	// executed work still reaches the model. The adjacent-same-role merge at
	// the provider boundary keeps the wire alternating.
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant", "assistant", "tool")
	if result[2].ToolCalls[0].ID != "A" {
		t.Fatalf("synthesized call must pair with the surviving result, got %+v", result[2].ToolCalls[0])
	}
}

func TestSanitizeHistoryForProvider_OrphanedLeadingTool(t *testing.T) {
	history := []providers.Message{
		toolResult("A"),
		msg("user", "hello"),
	}

	result := sanitizeHistoryForProvider(history)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user")
}

func TestSanitizeHistoryForProvider_ToolAfterUserDropped(t *testing.T) {
	history := []providers.Message{
		msg("user", "hello"),
		toolResult("A"),
	}

	result := sanitizeHistoryForProvider(history)
	// The orphaned result is repaired with a synthesized assistant turn
	// instead of being dropped, so the real tool output survives replay.
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant", "tool")
}

func TestSanitizeHistoryForProvider_ToolAfterAssistantNoToolCalls(t *testing.T) {
	history := []providers.Message{
		msg("user", "hello"),
		msg("assistant", "hi"),
		toolResult("A"),
	}

	result := sanitizeHistoryForProvider(history)
	// Orphan synthesis keeps the result; the provider boundary merges the
	// adjacent assistant turns for strict-alternation upstreams.
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant", "assistant", "tool")
}

func TestSanitizeHistoryForProvider_AssistantToolCallAtStart(t *testing.T) {
	history := []providers.Message{
		assistantWithTools("A"),
		toolResult("A"),
		msg("user", "hello"),
	}

	result := sanitizeHistoryForProvider(history)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user")
}

func TestSanitizeHistoryForProvider_MultiToolCallsThenNewRound(t *testing.T) {
	history := []providers.Message{
		msg("user", "do two things"),
		assistantWithTools("A", "B"),
		toolResult("A"),
		toolResult("B"),
		msg("assistant", "done"),
		msg("user", "hi"),
		assistantWithTools("C"),
		toolResult("C"),
		msg("assistant", "done again"),
	}

	result := sanitizeHistoryForProvider(history)
	if len(result) != 9 {
		t.Fatalf("expected 9 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant", "tool", "tool", "assistant", "user", "assistant", "tool", "assistant")
}

func TestSanitizeHistoryForProvider_ConsecutiveMultiToolRounds(t *testing.T) {
	history := []providers.Message{
		msg("user", "start"),
		assistantWithTools("A", "B"),
		toolResult("A"),
		toolResult("B"),
		assistantWithTools("C", "D"),
		toolResult("C"),
		toolResult("D"),
		msg("assistant", "all done"),
	}

	result := sanitizeHistoryForProvider(history)
	if len(result) != 8 {
		t.Fatalf("expected 8 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant", "tool", "tool", "assistant", "tool", "tool", "assistant")
}

func TestSanitizeHistoryForProvider_PlainConversation(t *testing.T) {
	history := []providers.Message{
		msg("user", "hello"),
		msg("assistant", "hi"),
		msg("user", "how are you"),
		msg("assistant", "fine"),
	}

	result := sanitizeHistoryForProvider(history)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}
	assertRoles(t, result, "user", "assistant", "user", "assistant")
}

func TestSanitizeHistoryForProvider_DuplicateToolResults(t *testing.T) {
	history := []providers.Message{
		msg("user", "do something"),
		assistantWithTools("A", "B"),
		toolResult("A"),
		toolResult("B"),
		toolResult("A"), // duplicate
		toolResult("B"), // duplicate
		msg("assistant", "done"),
	}

	result := sanitizeHistoryForProvider(history)
	if len(result) != 5 {
		t.Fatalf("expected 5 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant", "tool", "tool", "assistant")
	// Verify the kept tool results have the correct IDs
	if result[2].ToolCallID != "A" {
		t.Errorf("expected tool result A, got %q", result[2].ToolCallID)
	}
	if result[3].ToolCallID != "B" {
		t.Errorf("expected tool result B, got %q", result[3].ToolCallID)
	}
}

func TestSanitizeHistoryForProvider_ReusedToolCallIDAcrossRounds(t *testing.T) {
	history := []providers.Message{
		msg("user", "first"),
		assistantWithTools("call_0"),
		toolResult("call_0"),
		msg("assistant", "first done"),
		msg("user", "second"),
		assistantWithTools("call_0"),
		toolResult("call_0"),
		msg("assistant", "second done"),
	}

	result := sanitizeHistoryForProvider(history)
	if len(result) != 8 {
		t.Fatalf("expected 8 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant", "tool", "assistant", "user", "assistant", "tool", "assistant")
	if result[2].ToolCallID != "call_0" || result[6].ToolCallID != "call_0" {
		t.Fatalf(
			"expected both tool results to be preserved, got IDs %q and %q",
			result[2].ToolCallID,
			result[6].ToolCallID,
		)
	}
}

func TestSanitizeHistoryForProvider_DropsAssistantWithEmptyToolCallID(t *testing.T) {
	history := []providers.Message{
		msg("user", "do something"),
		assistantWithTools(""),
		toolResult(""),
		msg("assistant", "done"),
	}

	result := sanitizeHistoryForProvider(history)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant")
}

func roles(msgs []providers.Message) []string {
	r := make([]string, len(msgs))
	for i, m := range msgs {
		r[i] = m.Role
	}
	return r
}

func assertRoles(t *testing.T, msgs []providers.Message, expected ...string) {
	t.Helper()
	if len(msgs) != len(expected) {
		t.Fatalf("role count mismatch: got %v, want %v", roles(msgs), expected)
	}
	for i, exp := range expected {
		if msgs[i].Role != exp {
			t.Errorf("message[%d]: got role %q, want %q", i, msgs[i].Role, exp)
		}
	}
}

// TestSanitizeHistoryForProvider_IncompleteToolResults tests the forward validation
// that ensures assistant messages with tool_calls have ALL matching tool results.
// This fixes the DeepSeek error: "An assistant message with 'tool_calls' must be
// followed by tool messages responding to each 'tool_call_id'."
//
// Since the history-repair rework, incomplete blocks are REPAIRED (a synthetic
// tool result is synthesized for each missing call) instead of dropped, so the
// assistant's own text and the answered results survive.
func TestSanitizeHistoryForProvider_IncompleteToolResults(t *testing.T) {
	// Assistant expects tool results for both A and B, but only A is present
	history := []providers.Message{
		msg("user", "do two things"),
		assistantWithTools("A", "B"),
		toolResult("A"),
		// toolResult("B") is missing - this would cause DeepSeek to fail
		msg("user", "next question"),
		msg("assistant", "answer"),
	}

	result := sanitizeHistoryForProvider(history)
	// The missing result for B is synthesized; the assistant turn and the
	// answered result for A survive. Remaining messages:
	// user, assistant(tools A,B), tool(A), tool(B, synthesized),
	// user ("next question"), assistant ("answer")
	if len(result) != 6 {
		t.Fatalf("expected 6 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant", "tool", "tool", "user", "assistant")
	assertSynthesizedResult(t, result[3], "B")
}

// TestSanitizeHistoryForProvider_MissingAllToolResults tests the case where
// an assistant message has tool_calls but no tool results follow at all.
func TestSanitizeHistoryForProvider_MissingAllToolResults(t *testing.T) {
	history := []providers.Message{
		msg("user", "do something"),
		assistantWithTools("A"),
		// No tool results at all
		msg("user", "hello"),
		msg("assistant", "hi"),
	}

	result := sanitizeHistoryForProvider(history)
	// The missing result for A is synthesized. Remaining:
	// user, assistant(tools A), tool(A, synthesized), user, assistant
	if len(result) != 5 {
		t.Fatalf("expected 5 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result, "user", "assistant", "tool", "user", "assistant")
	assertSynthesizedResult(t, result[2], "A")
}

// TestSanitizeHistoryForProvider_PartialToolResultsInMiddle tests that
// incomplete tool results in the middle of a conversation are properly handled.
func TestSanitizeHistoryForProvider_PartialToolResultsInMiddle(t *testing.T) {
	history := []providers.Message{
		msg("user", "first"),
		assistantWithTools("A"),
		toolResult("A"),
		msg("assistant", "done"),
		msg("user", "second"),
		assistantWithTools("B", "C"),
		toolResult("B"),
		// toolResult("C") is missing
		msg("user", "third"),
		assistantWithTools("D"),
		toolResult("D"),
		msg("assistant", "all done"),
	}

	result := sanitizeHistoryForProvider(history)
	// First round is complete (user, assistant+tools, tool, assistant).
	// Second round is incomplete and REPAIRED (assistant+tools, tool(B),
	// tool(C, synthesized)) instead of dropped.
	// Third round is complete (user, assistant+tools, tool, assistant).
	// Remaining: user, assistant, tool, assistant, user,
	// assistant, tool, tool, user, assistant, tool, assistant
	if len(result) != 12 {
		t.Fatalf("expected 12 messages, got %d: %+v", len(result), roles(result))
	}
	assertRoles(t, result,
		"user", "assistant", "tool", "assistant",
		"user", "assistant", "tool", "tool",
		"user", "assistant", "tool", "assistant")
	assertSynthesizedResult(t, result[7], "C")
}

// assertSynthesizedResult verifies a tool message is the deterministic
// interrupted-result placeholder synthesized for the given call ID.
func assertSynthesizedResult(t *testing.T, m providers.Message, wantID string) {
	t.Helper()
	if m.Role != "tool" {
		t.Fatalf("expected tool message, got role %q", m.Role)
	}
	if m.ToolCallID != wantID {
		t.Fatalf("expected synthesized result for call %q, got %q", wantID, m.ToolCallID)
	}
	if !strings.Contains(m.Content, "Tool result missing") {
		t.Fatalf("expected interrupted-result placeholder text, got %q", m.Content)
	}
}
