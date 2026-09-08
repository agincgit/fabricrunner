package observertest_test

import (
	"context"
	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/observertest"
	"testing"
)

type observer struct{}

func (observer) Observe(ctx context.Context, r fr.Record) error { return ctx.Err() }
func TestObserverConformance(t *testing.T) {
	observertest.Run(t, func(testing.TB) fr.Observer { return observer{} })
}
