package tool_test

import (
	"context"
	"encoding/json"
	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/sandbox"
	"github.com/agincgit/fabricrunner/tool"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func nativeTools(t *testing.T) (string, map[string]fr.ToolBinding) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("native sandbox unsupported on " + runtime.GOOS)
	}
	helper := filepath.Join(t.TempDir(), "helper")
	build := exec.Command("go", "build", "-o", helper, "../cmd/fabricrunner-tool")
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("helper build: %v %s", err, data)
	}
	root := t.TempDir()
	bindings, err := tool.New(tool.Config{Root: root, Helper: helper, Executor: sandbox.New(), MaxOutput: 1024})
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]fr.ToolBinding{}
	for _, b := range bindings {
		result[b.Definition.Name] = b
		t.Cleanup(func() {
			if err := b.Handler.Close(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	return root, result
}
func TestConfinedWriteEditReadRoundTrip(t *testing.T) {
	root, bindings := nativeTools(t)
	for _, entry := range []struct{ name, input string }{{"write", `{"path":"a","text":"hello"}`}, {"edit", `{"path":"a","old":"hello","new":"world"}`}, {"read", `{"path":"a"}`}} {
		out, err := bindings[entry.name].Handler.Execute(context.Background(), fr.ToolInvocation{Call: fr.ToolCall{Name: entry.name, Input: json.RawMessage(entry.input)}})
		if err != nil || out.IsError {
			t.Fatalf("%s: %v %s", entry.name, err, out.JSON)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "a"))
	if err != nil || string(data) != "world" {
		t.Fatalf("%q %v", data, err)
	}
}
func TestCommandToolTakesArgumentVector(t *testing.T) {
	root, bindings := nativeTools(t)
	out, err := bindings["command"].Handler.Execute(context.Background(), fr.ToolInvocation{Call: fr.ToolCall{Name: "command", Input: json.RawMessage(`{"args":["/usr/bin/printf","%s","hello; touch injected"]}`)}})
	if err != nil || out.IsError {
		t.Fatalf("%v %s", err, out.JSON)
	}
	var result tool.Result
	_ = json.Unmarshal(out.JSON, &result)
	if result.Text != "hello; touch injected" {
		t.Fatal(result.Text)
	}
	if _, err := os.Stat(filepath.Join(root, "injected")); !os.IsNotExist(err) {
		t.Fatalf("shell injection: %v", err)
	}
}

type eventSink struct{ events []fr.SandboxEvent }

func (s *eventSink) RecordSandboxEvent(_ context.Context, e fr.SandboxEvent) error {
	s.events = append(s.events, e)
	return nil
}
func TestToolCancellationRecordsPartialEffects(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux cancellation probe")
	}
	_, bindings := nativeTools(t)
	sink := &eventSink{}
	ctx, cancel := context.WithTimeout(fr.WithSandboxEventSink(context.Background(), sink), 200*time.Millisecond)
	defer cancel()
	out, err := bindings["command"].Handler.Execute(ctx, fr.ToolInvocation{Call: fr.ToolCall{Name: "command", Input: json.RawMessage(`{"args":["/usr/bin/python3","-c","open('partial','w').write('x'); import time; time.sleep(10)"]}`)}})
	if err != nil || !out.IsError {
		t.Fatalf("%v %s", err, out.JSON)
	}
	found := false
	for _, e := range sink.events {
		found = found || e.Phase == "execution_failed" && e.PartialEffects
	}
	if !found {
		t.Fatalf("missing effect record: %+v", sink.events)
	}
}
