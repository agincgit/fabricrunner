package tool_test

import (
	"context"
	"encoding/json"
	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/tool"
	"os"
	"path/filepath"
	"testing"
)

func execute(t *testing.T, name, root, input string) (fr.ToolOutput, error) {
	t.Helper()
	bindings, err := tool.New(tool.Config{Root: root, MaxOutput: 8})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bindings {
		if b.Definition.Name == name {
			defer b.Handler.Close(context.Background())
			return b.Handler.Execute(context.Background(), fr.ToolInvocation{Call: fr.ToolCall{Name: name, Input: json.RawMessage(input)}})
		}
	}
	t.Fatal("missing tool")
	return fr.ToolOutput{}, nil
}
func TestPathEscapeRefused(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	paths := []string{"../private", outside}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err == nil {
		paths = append(paths, "link")
	}
	for _, path := range paths {
		data, _ := json.Marshal(map[string]string{"path": path})
		if _, err := execute(t, "read", root, string(data)); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
}
func TestSideEffectingToolsDeclared(t *testing.T) {
	bindings, err := tool.New(tool.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bindings {
		if b.Definition.SideEffecting != (b.Definition.Name != "read") {
			t.Errorf("bad declaration: %s", b.Definition.Name)
		}
	}
}
func TestSideEffectingToolFailsClosedWithoutSandbox(t *testing.T) {
	for name, input := range map[string]string{"write": `{"path":"a","text":"b"}`, "edit": `{"path":"a","old":"a","new":"b"}`, "command": `{"args":["/usr/bin/true"]}`} {
		if _, err := execute(t, name, t.TempDir(), input); err == nil {
			t.Errorf("%s ran unconfined", name)
		}
	}
}
func TestUnknownProvenanceClassifiedConfidential(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a"), []byte("value"), 0600)
	out, err := execute(t, "read", root, `{"path":"a"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out.Classification != fr.ClassConfidential {
		t.Fatal(out.Classification)
	}
}
func TestOversizedOutputTruncatedAndRecorded(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a"), []byte("0123456789012345"), 0600)
	out, err := execute(t, "read", root, `{"path":"a"}`)
	if err != nil {
		t.Fatal(err)
	}
	var result tool.Result
	if err := json.Unmarshal(out.JSON, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || len(result.Text) > 8 {
		t.Fatalf("%+v", result)
	}
}
