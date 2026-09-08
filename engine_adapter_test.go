package fabricrunner_test

import (
	"context"
	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/provider/openaicompat"
	"github.com/agincgit/fabricrunner/store/sqlite"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEngineWithOpenAICompatibleAdapter(t *testing.T) {
	e, r, _ := engineFixture(t)
	r.Candidates[0].Zone = fr.ZoneManagedCloud
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	e.Store = store
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", request.URL.Path)
		}
		events, loadErr := e.Store.Load(request.Context(), fr.AggregateRef{Type: fr.WorkloadAggregateType, ID: r.WorkloadID}, 0)
		if loadErr != nil {
			t.Error(loadErr)
			return
		}
		var pending fr.ExecutionRecord
		if len(events) == 0 {
			t.Error("provider contacted before durable history")
		}
		// Read the independently loaded SQLite projection while the call is active.
		p, err := e.Replay(request.Context(), r.WorkloadID)
		if err != nil {
			t.Error(err)
		} else {
			pending = p.Execution[len(p.Execution)-1]
		}
		if pending.Loop == nil || pending.Loop.Type != fr.LoopEventTurnStarted {
			t.Error("model called before durable turn start")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	provider, err := openaicompat.New(openaicompat.Config{Name: "test", BaseURL: server.URL + "/v1", AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	e.Providers["test"] = provider
	got := runEngine(t, e, r)
	server.Close()
	replayed, err := e.Replay(context.Background(), r.WorkloadID)
	if err != nil || !reflect.DeepEqual(got, replayed) {
		t.Fatalf("offline replay failed: %v", err)
	}
	result := got.Execution[len(got.Execution)-1].Result
	if result == nil || result.Usage.InputTokens != 2 || result.Usage.OutputTokens != 1 || result.Messages[len(result.Messages)-1].Content[0].Text != "done" {
		t.Fatalf("unexpected normalized result: %v", result)
	}
}
