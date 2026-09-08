package fabricrunner_test

import (
	"context"
	"encoding/json"
	"errors"
	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/sandbox"
	"github.com/agincgit/fabricrunner/store/sqlite"
	"github.com/agincgit/fabricrunner/tool"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type exitingCheckpointStore struct{ fr.EventStore }

func (s exitingCheckpointStore) Append(ctx context.Context, ref fr.AggregateRef, version uint64, drafts ...fr.EventDraft) ([]fr.Event, error) {
	events, err := s.EventStore.Append(ctx, ref, version, drafts...)
	if err == nil {
		for _, draft := range drafts {
			var record fr.ExecutionRecord
			if draft.Type == fr.EventTypeExecutionRecorded && json.Unmarshal(draft.Payload, &record) == nil && record.Loop != nil && record.Loop.Type == fr.LoopEventCheckpoint {
				os.Exit(23)
			}
		}
	}
	return events, err
}

func TestProcessRestartPreservesConfinedEffect(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux bubblewrap subprocess acceptance")
	}
	if root := os.Getenv("FABRICRUNNER_TEST_RESTART_ROOT"); root != "" {
		data, err := os.ReadFile(filepath.Join(root, "request.json"))
		if err != nil {
			t.Fatal(err)
		}
		var request fr.EngineRequest
		if err := json.Unmarshal(data, &request); err != nil {
			t.Fatal(err)
		}
		engine, _, provider := engineFixture(t)
		bindings, err := tool.New(tool.Config{Root: filepath.Join(root, "workspace"), Executor: sandbox.New()})
		if err != nil {
			t.Fatal(err)
		}
		for _, binding := range bindings {
			if binding.Definition.Name == "command" {
				request.Tools = []fr.ToolBinding{binding}
			}
		}
		store, err := sqlite.Open(context.Background(), filepath.Join(root, "events.db"))
		if err != nil {
			t.Fatal(err)
		}
		engine.Store = exitingCheckpointStore{store}
		provider.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) {
			return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventToolCall, Sequence: 2, ToolCall: &fr.ToolCall{ID: "once", Name: "command", Input: json.RawMessage(`{"args":["/usr/bin/python3","-c","open('effect','a').write('once\\n')"]}`)}}, {Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopToolUse}}}, nil
		}
		_, err = engine.Run(context.Background(), request)
		t.Fatalf("child did not exit at checkpoint: %v", err)
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	engine, request, provider := engineFixture(t)
	request.Budget.MaxWallTime = time.Minute
	data, _ := json.Marshal(request)
	if err := os.WriteFile(filepath.Join(root, "request.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestProcessRestartPreservesConfinedEffect$")
	command.Env = append(os.Environ(), "FABRICRUNNER_TEST_RESTART_ROOT="+root)
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 23 {
		t.Fatalf("child: %v %s", err, output)
	}
	bindings, err := tool.New(tool.Config{Root: workspace, Executor: sandbox.New()})
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range bindings {
		if binding.Definition.Name == "command" {
			request.Tools = []fr.ToolBinding{binding}
		}
	}
	store, err := sqlite.Open(context.Background(), filepath.Join(root, "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	engine.Store = store
	projection, err := engine.Resume(context.Background(), request)
	if err != nil || projection.Workload.State != fr.WorkloadSucceeded || provider.calls != 1 {
		t.Fatalf("resume: %v calls=%d", err, provider.calls)
	}
	effect, err := os.ReadFile(filepath.Join(workspace, "effect"))
	if err != nil || strings.Count(string(effect), "once") != 1 {
		t.Fatalf("effect repeated: %q %v", effect, err)
	}
}
