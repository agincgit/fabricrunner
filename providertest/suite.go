// Package providertest provides a shared conformance suite for provider
// adapters. It is intended for use from adapter test packages.
package providertest

import (
	"context"
	"reflect"
	"testing"

	"github.com/agincgit/fabricrunner"
)

type Fixture struct {
	Provider         fabricrunner.Provider
	Request          fabricrunner.ModelRequest
	WantName         string
	WantModels       []fabricrunner.ModelDescriptor
	WantEvents       []fabricrunner.ModelEvent
	StreamCloseCount func() int
}

type Factory func(*testing.T) Fixture

// Run executes the common catalog and stream contract against fresh fixtures.
func Run(t *testing.T, factory Factory) {
	t.Helper()
	if factory == nil {
		t.Fatal("provider conformance factory is nil")
	}
	t.Run("catalog", func(t *testing.T) {
		fixture := factory(t)
		requireFixture(t, fixture)
		if name := fixture.Provider.Name(); name != fixture.WantName {
			t.Fatalf("provider name = %q, want %q", name, fixture.WantName)
		}
		models, err := fabricrunner.DiscoverModels(context.Background(), fixture.Provider)
		if err != nil {
			t.Fatalf("discover models: %v", err)
		}
		if !reflect.DeepEqual(models, fixture.WantModels) {
			t.Fatalf("models = %#v, want %#v", models, fixture.WantModels)
		}
	})
	t.Run("stream", func(t *testing.T) {
		fixture := factory(t)
		requireFixture(t, fixture)
		before := fabricrunner.CloneModelRequest(fixture.Request)
		stream, err := fixture.Provider.Stream(context.Background(), fixture.Request)
		if err != nil {
			t.Fatalf("open stream: %v", err)
		}
		events, err := fabricrunner.DrainStream(context.Background(), stream)
		if err != nil {
			t.Fatalf("drain stream: %v", err)
		}
		if !reflect.DeepEqual(fixture.Request, before) {
			t.Fatal("provider mutated caller-owned request")
		}
		if !reflect.DeepEqual(events, fixture.WantEvents) {
			t.Fatalf("events = %#v, want %#v", events, fixture.WantEvents)
		}
		if count := fixture.StreamCloseCount(); count != 1 {
			t.Fatalf("stream close count = %d, want 1", count)
		}
	})
}

func requireFixture(t *testing.T, fixture Fixture) {
	t.Helper()
	if fixture.Provider == nil {
		t.Fatal("provider conformance fixture has nil provider")
	}
	if fixture.WantName == "" {
		t.Fatal("provider conformance fixture has empty expected name")
	}
	if err := fixture.Request.Validate(); err != nil {
		t.Fatalf("provider conformance request: %v", err)
	}
	if fixture.StreamCloseCount == nil {
		t.Fatal("provider conformance fixture has no stream close observer")
	}
}
