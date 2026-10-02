// PicoClaw - Ultra-lightweight personal AI agent
// Turn context inspired by NousResearch/hermes-agent.
package agent

import (
	"context"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/routing"
	runtimeevents "github.com/sipeed/picoclaw/pkg/events"
	"github.com/sipeed/picoclaw/pkg/session"
)

// TurnContext holds per-turn metadata inspired by Hermes's TurnContext.
type TurnContext struct {
	InboundContext *bus.InboundContext
	RouteResult    *routing.ResolvedRoute
	SessionScope   *session.SessionScope

	// Turn identification
	TurnID   string
	AgentID  string
	TraceID  string
	ParentID string

	// Timing
	StartedAt time.Time

	// Runtime event system
	RuntimeEvents runtimeevents.Bus

	// Cancellation
	cancel func()
}

// NewTurnContext creates a new TurnContext.
func NewTurnContext(
	inbound *bus.InboundContext,
	route *routing.ResolvedRoute,
	scope *session.SessionScope,
) *TurnContext {
	return &TurnContext{
		InboundContext: inbound,
		RouteResult:    route,
		SessionScope:   scope,
		StartedAt:      time.Now(),
	}
}

// WithRuntimeEvents sets the runtime event bus.
func (tc *TurnContext) WithRuntimeEvents(bus runtimeevents.Bus) *TurnContext {
	tc.RuntimeEvents = bus
	return tc
}

// WithTurnID sets the turn ID.
func (tc *TurnContext) WithTurnID(id string) *TurnContext {
	tc.TurnID = id
	return tc
}

// WithAgentID sets the agent ID.
func (tc *TurnContext) WithAgentID(id string) *TurnContext {
	tc.AgentID = id
	return tc
}

// WithTraceID sets the trace ID.
func (tc *TurnContext) WithTraceID(id string) *TurnContext {
	tc.TraceID = id
	return tc
}

// WithParentID sets the parent turn ID.
func (tc *TurnContext) WithParentID(id string) *TurnContext {
	tc.ParentID = id
	return tc
}

// WithCancel sets the cancellation function.
func (tc *TurnContext) WithCancel(cancel func()) *TurnContext {
	tc.cancel = cancel
	return tc
}

// Cancel requests cancellation of this turn.
func (tc *TurnContext) Cancel() {
	if tc.cancel != nil {
		tc.cancel()
	}
}

// ToContext converts this TurnContext to a Go context.Context.
// Note: This creates a minimal turnState with only the turn ID set.
// For full turn state, use newTurnState and withTurnState directly.
func (tc *TurnContext) ToContext(ctx context.Context) context.Context {
	if tc == nil {
		return ctx
	}
	ts := &turnState{
		turnID: tc.TurnID,
	}
	return withTurnState(ctx, ts)
}
