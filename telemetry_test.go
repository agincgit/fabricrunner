package fabricrunner_test

import (
	"context"
	"encoding/json"
	fr "github.com/agincgit/fabricrunner"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestConfidentialContentNeverEnteresRecord(t *testing.T) {
	for _, class := range []fr.Classification{fr.ClassConfidential, fr.ClassSecret, ""} {
		r, err := fr.NewRecord(fr.RecordInput{Kind: fr.RecordProvider, Classification: class, Content: []byte("sensitive-value"), Attributes: map[fr.AttributeKey]string{fr.AttrOutcome: "sensitive-value"}})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(r)
		if strings.Contains(string(b), "sensitive-value") || r.ContentSHA256 == "" || r.ContentSize != 15 {
			t.Fatalf("unsafe record: %s", b)
		}
	}
}
func TestAttributeKeysAreClosedSet(t *testing.T) {
	_, err := fr.NewRecord(fr.RecordInput{Kind: fr.RecordProvider, Classification: fr.ClassPublic, Attributes: map[fr.AttributeKey]string{"arbitrary": "value"}})
	if err == nil {
		t.Fatal("unbounded key accepted")
	}
}
func TestSlowObserverDoesNotBlockExecution(t *testing.T) {
	e, r, _ := engineFixture(t)
	release := make(chan struct{})
	defer close(release)
	e.Observer = engineObserver(func(context.Context, fr.Record) error { <-release; return nil })
	e.ObserverTimeout = time.Millisecond
	start := time.Now()
	runEngine(t, e, r)
	if time.Since(start) > time.Second {
		t.Fatal("observer blocked execution")
	}
}
func TestObservationCancelsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	stopped := make(chan struct{})
	o := fr.NewObservation(engineObserver(func(ctx context.Context, _ fr.Record) error { close(entered); <-ctx.Done(); close(stopped); return nil }), time.Second)
	r, _ := fr.NewRecord(fr.RecordInput{Kind: fr.RecordProvider, Classification: fr.ClassPublic})
	go o.Emit(ctx, r)
	<-entered
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("observation context not cancelled")
	}
}

func TestStuckObserversHaveBoundedGoroutines(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	o := fr.NewObservation(engineObserver(func(context.Context, fr.Record) error { <-release; return nil }), time.Second)
	r, _ := fr.NewRecord(fr.RecordInput{Kind: fr.RecordProvider, Classification: fr.ClassPublic})
	before := runtime.NumGoroutine()
	for range 1000 {
		o.Emit(context.Background(), r)
	}
	if added := runtime.NumGoroutine() - before; added > 64 {
		t.Fatalf("unbounded observation goroutines: %d", added)
	}
}
