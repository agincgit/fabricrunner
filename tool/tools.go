// Package tool supplies rooted read, write, edit and sandboxed command tools.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/internal/toolop"
	"github.com/agincgit/fabricrunner/sandbox"
	"os"
	"sync"
	"time"
)

type Result = toolop.Result
type Config struct {
	Root           string
	Helper         string
	Executor       sandbox.Executor
	MaxOutput      int
	Timeout        time.Duration
	Classification fr.Classification
}
type handler struct {
	config Config
	name   string
	mu     sync.Mutex
	box    sandbox.Sandbox
	closed bool
}

func New(config Config) ([]fr.ToolBinding, error) {
	if config.Root == "" {
		return nil, errors.New("explicit tool root required")
	}
	info, err := os.Stat(config.Root)
	if err != nil || !info.IsDir() {
		return nil, errors.New("tool root must be a directory")
	}
	if config.MaxOutput == 0 {
		config.MaxOutput = 64 << 10
	}
	if config.MaxOutput < 1 || config.MaxOutput > 1<<20 {
		return nil, errors.New("output bound must be between 1 and 1 MiB")
	}
	if config.Timeout == 0 {
		config.Timeout = time.Minute
	}
	if config.Timeout < 0 {
		return nil, errors.New("negative timeout")
	}
	if config.Classification == "" {
		config.Classification = fr.ClassConfidential
	}
	if err := config.Classification.Validate(); err != nil {
		return nil, err
	}
	if config.Executor == nil {
		config.Executor = sandbox.Deny{}
	}
	specs := []struct {
		name       string
		properties string
		required   string
	}{
		{"read", `"path":{"type":"string","minLength":1}`, `"path"`},
		{"write", `"path":{"type":"string","minLength":1},"text":{"type":"string"}`, `"path","text"`},
		{"edit", `"path":{"type":"string","minLength":1},"old":{"type":"string","minLength":1},"new":{"type":"string"}`, `"path","old","new"`},
		{"command", `"args":{"type":"array","minItems":1,"maxItems":256,"items":{"type":"string"}}`, `"args"`},
	}
	var bindings []fr.ToolBinding
	for _, s := range specs {
		definition := fr.ToolDefinition{Name: s.name, Description: "Bounded workspace " + s.name, SideEffecting: s.name != "read", Strict: true, InputSchema: json.RawMessage(`{"type":"object","properties":{` + s.properties + `},"required":[` + s.required + `],"additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"},"truncated":{"type":"boolean"},"partial_effects":{"type":"boolean"},"exit_code":{"type":"integer"},"error":{"type":"string"}},"required":["text","truncated","partial_effects","exit_code"],"additionalProperties":false}`)}
		bindings = append(bindings, fr.ToolBinding{Definition: definition, Handler: &handler{config: config, name: s.name}, ParallelSafe: s.name == "read"})
	}
	return bindings, nil
}
func (h *handler) PrepareSandbox(ctx context.Context) (fr.SandboxCapability, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return fr.SandboxCapability{}, errors.New("tool is closed")
	}
	if h.name == "read" {
		return fr.SandboxCapability{}, nil
	}
	if h.box == nil {
		box, err := h.config.Executor.Establish(ctx, sandbox.Config{Root: h.config.Root, Helper: h.config.Helper})
		if err != nil {
			return fr.SandboxCapability{}, err
		}
		h.box = box
	}
	return h.box.Capability(), nil
}
func (h *handler) Execute(ctx context.Context, call fr.ToolInvocation) (fr.ToolOutput, error) {
	ctx, cancel := context.WithTimeout(ctx, h.config.Timeout)
	defer cancel()
	input, err := toolop.Decode(call.Call.Input)
	if err != nil {
		return fr.ToolOutput{}, err
	}
	if h.name != "command" {
		if err := toolop.ValidatePath(input.Path); err != nil {
			return fr.ToolOutput{}, err
		}
	}
	if _, err := h.PrepareSandbox(ctx); err != nil {
		return fr.ToolOutput{}, err
	}
	result := Result{}
	if h.name == "read" {
		result, err = toolop.Run(ctx, h.config.Root, h.name, input, h.config.MaxOutput)
	} else {
		args := input.Args
		bound := h.config.MaxOutput
		if h.name != "command" {
			if h.config.Helper == "" {
				return fr.ToolOutput{}, fmt.Errorf("%w: trusted helper is required", fr.ErrSandboxDenied)
			}
			args = []string{h.config.Helper, h.name}
			bound = 64 << 10
		}
		if len(args) == 0 || len(args) > 256 {
			return fr.ToolOutput{}, errors.New("bounded argument vector required")
		}
		h.mu.Lock()
		box := h.box
		h.mu.Unlock()
		execution, runErr := box.Run(ctx, sandbox.Command{Args: args, Input: call.Call.Input, MaxOutput: bound})
		phase := "execution_completed"
		if runErr != nil {
			phase = "execution_failed"
		}
		recordCtx, stopRecord := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		recordErr := fr.RecordSandboxEvent(recordCtx, fr.SandboxEvent{Phase: phase, Backend: box.Capability().Backend, PartialEffects: execution.Started})
		stopRecord()
		if recordErr != nil {
			return fr.ToolOutput{}, recordErr
		}
		result = Result{Text: string(execution.Output), Truncated: execution.Truncated, PartialEffects: execution.Started, ExitCode: execution.ExitCode}
		if h.name != "command" && runErr == nil {
			if decodeErr := json.Unmarshal(execution.Output, &result); decodeErr != nil {
				return fr.ToolOutput{}, decodeErr
			}
		}
		err = runErr
	}
	if err != nil {
		if !result.PartialEffects {
			return fr.ToolOutput{}, err
		}
		result.Error = "tool failed; effects may have occurred"
	}
	if len(result.Text) > h.config.MaxOutput {
		result.Text = result.Text[:h.config.MaxOutput]
		result.Truncated = true
	}
	data, encodeErr := json.Marshal(result)
	if encodeErr != nil {
		return fr.ToolOutput{}, encodeErr
	}
	// Failed processes return structured results so the committed tool-completed
	// event preserves conservative partial-effect information even on cancellation.
	return fr.ToolOutput{JSON: data, Classification: h.config.Classification, IsError: err != nil}, nil
}
func (h *handler) Close(ctx context.Context) error {
	h.mu.Lock()
	h.closed = true
	box := h.box
	h.mu.Unlock()
	if box != nil {
		return box.Close(ctx)
	}
	return nil
}
