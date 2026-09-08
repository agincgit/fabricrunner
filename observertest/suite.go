// Package observertest checks third-party observers against the public record
// envelope and cancellation contract. Exporter-specific assertions belong in
// the caller's factory or its own tests. Returned errors are permitted.
package observertest

import (
	"context"
	fr "github.com/agincgit/fabricrunner"
	"testing"
	"time"
)

type Factory func(testing.TB) fr.Observer

// Run checks that an observer accepts each v1 kind and returns on cancellation.
// The engine separately isolates even nonconforming observers from execution.
func Run(t *testing.T, factory Factory) {
	t.Helper()
	if factory == nil {
		t.Fatal("observer conformance factory is nil")
	}
	for _, kind := range []fr.RecordKind{fr.RecordWorkload, fr.RecordStep, fr.RecordAttempt, fr.RecordRouting, fr.RecordPolicy, fr.RecordProvider, fr.RecordTool} {
		t.Run(string(kind), func(t *testing.T) {
			observer := factory(t)
			if observer == nil {
				t.Fatal("factory returned nil observer")
			}
			workload, _ := fr.NewID()
			step, _ := fr.NewID()
			attempt, _ := fr.NewID()
			record, err := fr.NewRecord(fr.RecordInput{Kind: kind, Classification: fr.ClassConfidential, WorkloadID: workload, StepID: step, AttemptID: attempt, Sequence: 1, Content: []byte("conformance input")})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan any, 1)
			go func() { defer func() { done <- recover() }(); _ = observer.Observe(ctx, record) }()
			cancel()
			select {
			case failure := <-done:
				if failure != nil {
					t.Fatalf("observer panicked: %v", failure)
				}
			case <-time.After(time.Second):
				t.Fatal("observer ignored cancellation")
			}
		})
	}
}
