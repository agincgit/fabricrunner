package fabricrunner

import (
	"context"
	"errors"
)

var ErrSandboxDenied = errors.New("sandbox execution denied")

// SandboxCapability is an observed capability, not permission to run unconfined.
type SandboxCapability struct {
	Established bool   `json:"established"`
	Backend     string `json:"backend"`
}

type SandboxedTool interface {
	PrepareSandbox(context.Context) (SandboxCapability, error)
}

type SandboxEvent struct {
	Phase          string `json:"phase"`
	Backend        string `json:"backend"`
	PartialEffects bool   `json:"partial_effects,omitempty"`
}
type SandboxEventSink interface {
	RecordSandboxEvent(context.Context, SandboxEvent) error
}
type sandboxSinkKey struct{}

func WithSandboxEventSink(ctx context.Context, sink SandboxEventSink) context.Context {
	return context.WithValue(ctx, sandboxSinkKey{}, sink)
}
func RecordSandboxEvent(ctx context.Context, event SandboxEvent) error {
	if sink, ok := ctx.Value(sandboxSinkKey{}).(SandboxEventSink); ok {
		return sink.RecordSandboxEvent(ctx, event)
	}
	return nil
}
